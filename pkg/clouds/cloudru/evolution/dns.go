// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package evolution

import (
	"context"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/samber/lo"
)

// ──────────────────────────────────────────────────────────────────────────────
// Cloud.ru **Cloud DNS** — measured live on 2026-10-24 against a real public zone.
//
// Everything in this file used to be a transcription of Cloud.ru's published API
// reference, and almost none of it survived contact with the API. The corrections
// are recorded here because each one presents as success or as a plausible-looking
// failure, and a future reader working from the same reference will re-derive the
// same wrong answers:
//
//  1. **The host was wrong.** The reference's `console.cloud.ru/api/clouddns` is
//     the console's own BFF: it has the right routes but authenticates browser
//     sessions, so a perfectly good service-account token gets
//     `403 permission denied: invalid scopes` there forever. That 403 is NOT a
//     missing role — it is the wrong front door. The real API is
//     `https://dns.api.cloud.ru` (an `*.api.cloud.ru` host like every other
//     endpoint in this package, behind the public API gateway).
//  2. **`parentId` does not exist.** The scope is `projectId`, the same project id
//     every other Evolution API in this package uses. The "Cloud DNS service
//     instance id" this file once described at length is not a thing.
//  3. **The collection is `publicZones`, camelCase, and records are not a
//     sub-resource.** There is no `/zones`, no `/zones/{id}/records`, and no
//     per-record route of any kind. A zone carries its records inline, and the
//     only way to change them is to PATCH the zone — see ApplyDnsRecord for why
//     that makes a read-modify-write mandatory rather than an optimisation.
//  4. **Record `type` is lowercase on the wire.** `"a"`, not `"A"`. Uppercase is
//     refused with `must be a valid value` while *also* passing the API's own
//     record-type lookup, so the error does not look like a casing problem.
//  5. **Writes are asynchronous, and the operation lies about being finished.**
//     PATCH answers 200 with a long-running operation; the zone reads back with
//     `records: []` while it settles, so reading without waiting is how you
//     conclude a write wiped the zone when it did not. Worse — and measured — the
//     operation reaching `done: true` does NOT mean the zone has converged, and a
//     second PATCH issued in that window **silently discards records**, up to and
//     including emptying the zone, while reporting another clean `done` with no
//     error. Two writes lost exactly this way on 2026-10-24. putZoneRecords
//     therefore verifies the zone against what it wrote rather than trusting
//     `done`; see the comment there.
//  6. **`ttl` IS honoured** — 900 stores as 900, and an omitted ttl stores as 0.
//     This corrects a measurement taken earlier the same day, which read `ttl: 0`
//     back from a record and concluded the field was ignored; that read was of a
//     record created without a ttl, taken while an operation was in flight. The
//     wrong conclusion was briefly load-bearing: recordMatches stopped comparing
//     TTLs, which made a TTL-only change a silent no-op.
//
// Two gates this file used to guard — `valid`/`validationText` (zone verification)
// and `delegated` — **do not exist on this API at all.** A zone reports `state` and
// `confirmState` instead, and the live zone was `state: "ok"`, `confirmState: ""`
// with no verification value anywhere. So the elaborate "verify the zone BEFORE
// moving its NS records" sequencing that this file once insisted on is an
// Advanced/ex-SberCloud estate concept, documented in a reference shared between
// the two estates. On Evolution, a zone is authoritative as soon as it exists.
//
// Diagnosing a 404 here: the gateway and the backend answer differently, and the
// difference is the only route-discovery signal there is. `{"error_msg":"404 Not
// Found"}` is the gateway — no such route. `{"code":5,"message":"Not Found"}` is a
// real backend — the route exists, the resource does not. Unauthenticated, the
// gateway answers `403 RBAC: access denied`, which is how you tell a credential
// problem from a path problem without guessing.
// ──────────────────────────────────────────────────────────────────────────────

// DefaultDnsEndpoint serves the Cloud DNS API. See correction (1) above: NOT
// `console.cloud.ru/api/clouddns`, which is the console BFF and 403s every
// service-account token.
const DefaultDnsEndpoint = "https://dns.api.cloud.ru"

// DefaultDnsRecordTtl is the TTL SC writes when a registrar does not set one.
//
// Cloud DNS has no automatic-TTL sentinel (unlike Cloudflare's `ttl: 1`) and stores
// a literal 0 when the field is omitted, so some number has to be chosen. 300 is the
// same default the Yandex registrar uses.
const DefaultDnsRecordTtl = 300

// DnsNameservers is the NS set a domain's registrar must point at for Evolution
// Cloud DNS to become authoritative for it.
//
// **The two estates have different nameservers, and an earlier version of this
// comment got it backwards.** It asserted — from Cloud.ru's own quickstart and
// delegation guide, which agree with each other — that `cdns-ns01/cdns-ns02.
// sbercloud.ru` serves zones on BOTH estates. A real Evolution zone publishes
//
//	evo-cns01.cloud.ru.   evo-cns02.cloud.ru.
//
// and those are not aliases of the other pair — they resolve into a different
// network entirely (94.158.23.11 / 94.158.23.22, against 37.18.8.44 / 37.18.8.88
// for the sbercloud pair). So those two docs pages describe the Advanced estate,
// and this package is Evolution. The lesson generalises past DNS: on this cloud,
// documentation that does not say which estate it is about should be assumed to be
// about the other one.
//
// The API carries no nameserver field, so this constant is the only answer SC can
// give; read the zone's own "NS-серверы" field in the console to confirm it.
var DnsNameservers = []string{"evo-cns01.cloud.ru", "evo-cns02.cloud.ru"}

// dnsRecordNamePattern is the API's own validation regex for a record name,
// recovered from a `google.rpc.BadRequest` field violation.
//
// Relative labels only. It is pinned here so SC can refuse a bad name with an
// explanation instead of forwarding it and relaying `must be in a valid format`.
// Three things it rejects that an operator will reasonably try: a trailing dot, a
// bare `@` for the apex (use the empty string), and `*` for a wildcard (that is
// the separate `wildcard` field on the record).
var dnsRecordNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9-]{0,62}(\.[a-zA-Z0-9][a-zA-Z0-9-]{0,62})*?$`)

// DnsZone is a zone as the API returns it.
//
// Note `Name` vs `Domain`: `Name` is a console *slug* (`atriumdev-online` for
// `atriumdev.online.`), `Domain` is the DNS name and carries a trailing dot.
// Matching a configured zone against `Name` therefore never matches — the earlier
// version of this file did exactly that. Use ZoneDomain.
type DnsZone struct {
	ID        string `json:"id,omitempty"`
	Name      string `json:"name,omitempty"`
	Domain    string `json:"domain,omitempty"`
	ProjectID string `json:"projectId,omitempty"`

	// State is the zone's lifecycle state; the live zone read `ok`.
	State string `json:"state,omitempty"`
	// ConfirmState is the API's only nod to domain confirmation, and it was empty
	// on a perfectly working zone. SC does not gate on it: a gate keyed on a field
	// whose vocabulary we have never seen is a hard block waiting to happen, which
	// is precisely the bug the `valid` gate shipped with before it was removed.
	ConfirmState string `json:"confirmState,omitempty"`

	ReadOnly    bool   `json:"readOnly,omitempty"`
	Description string `json:"description,omitempty"`
	CreateAt    string `json:"createAt,omitempty"` // sic — not `createdAt`
	UpdatedAt   string `json:"updatedAt,omitempty"`

	// Records are inline. There is no record sub-resource; this is the whole of
	// the zone's record state and the whole of what a PATCH replaces.
	Records []DnsRecord `json:"records"`
}

// ZoneDomain is the zone's DNS name, lowercased and without the trailing dot.
func ZoneDomain(zone *DnsZone) string {
	if zone == nil {
		return ""
	}
	return strings.ToLower(strings.TrimSuffix(strings.TrimSpace(zone.Domain), "."))
}

// DnsRecord is a record set: one name + type holding every value.
type DnsRecord struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name"`
	// Type is lowercase on the wire (`a`, `cname`, `txt`). See correction (4).
	Type   string   `json:"type"`
	Values []string `json:"values"`

	// TTL is honoured: a record written with 900 reads back as 900. Omitting the
	// field stores a literal 0 rather than a platform default, which is why SC
	// always sends one (DefaultDnsRecordTtl).
	TTL int `json:"ttl,omitempty"`

	// Wildcard is how this API spells a wildcard record. `*` in Name is refused
	// by dnsRecordNamePattern.
	Wildcard    bool   `json:"wildcard,omitempty"`
	ReadOnly    bool   `json:"readOnly,omitempty"`
	Description string `json:"description,omitempty"`
}

// dnsOperation is the long-running operation a write answers with.
type dnsOperation struct {
	ID           string         `json:"id,omitempty"`
	ResourceName string         `json:"resourceName,omitempty"`
	ResourceID   string         `json:"resourceId,omitempty"`
	Done         bool           `json:"done"`
	Description  string         `json:"description,omitempty"`
	Error        map[string]any `json:"error,omitempty"`
}

// dnsZoneList is the list envelope. One shape, measured — unlike the compute API,
// this one does not need collection sniffing.
type dnsZoneList struct {
	Zones         []DnsZone `json:"zones"`
	NextPageToken string    `json:"nextPageToken,omitempty"`
}

// dnsRecordType normalises a record type to the lowercase the API demands.
func dnsRecordType(recordType string) string {
	return strings.ToLower(strings.TrimSpace(recordType))
}

// dnsRecordName turns the name an operator wrote into the relative label the API
// wants.
//
// Operators write `app.example.ru` or `example.ru` (SC's other registrars take
// FQDNs); this API wants `app` and `""`. Getting it wrong is not an error — the
// earlier code sent the FQDN, which the API refuses outright, and a *relative* name
// sent where an FQDN was meant would have created `app.example.ru.example.ru`,
// which resolves nowhere and reads as "DNS has not propagated".
func dnsRecordName(name, zoneDomain string) (string, error) {
	n := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(name), "."))
	zone := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(zoneDomain), "."))

	switch {
	case n == "" || n == "@":
		return "", nil // the apex; the API spells it as the empty string
	case zone != "" && n == zone:
		return "", nil
	case zone != "" && strings.HasSuffix(n, "."+zone):
		n = strings.TrimSuffix(n, "."+zone)
	}
	if n != "" && !dnsRecordNamePattern.MatchString(n) {
		return "", errors.Errorf("%q is not a valid cloud.ru DNS record name (relative to zone %q it is %q, and the "+
			"API requires %s). Note what this rejects: a trailing dot, `@` for the apex (use an empty name), and "+
			"`*` for a wildcard (set the record's `wildcard` flag instead)",
			name, zone, n, dnsRecordNamePattern.String())
	}
	return n, nil
}

// dnsRecordsEqual compares two record sets by identity (name + type).
func dnsRecordsEqual(a, b DnsRecord) bool {
	return a.Name == b.Name && dnsRecordType(a.Type) == dnsRecordType(b.Type)
}

// ListZones returns every public zone in a project.
func (c *Client) ListZones(ctx context.Context, projectID string) ([]DnsZone, error) {
	if strings.TrimSpace(projectID) == "" {
		return nil, errors.Errorf("`projectId` is required to list Cloud.ru DNS zones")
	}
	var out []DnsZone
	token := ""
	for {
		query := url.Values{"projectId": []string{projectID}}
		if token != "" {
			query.Set("pageToken", token)
		}
		var page dnsZoneList
		if err := c.request(ctx, http.MethodGet, c.dnsURL, "/v1/publicZones", query, nil, &page); err != nil {
			return nil, errors.Wrapf(err, "failed to list cloud.ru DNS zones in project %q", projectID)
		}
		out = append(out, page.Zones...)
		// Bounded by the token changing: a server that echoes the same token would
		// otherwise spin forever on a page that never advances.
		if page.NextPageToken == "" || page.NextPageToken == token {
			return out, nil
		}
		token = page.NextPageToken
	}
}

// FindZone resolves a zone by DNS name within a project.
//
// Matched on `domain`, not `name` — see DnsZone. Case- and trailing-dot-insensitive
// because the API returns `atriumdev.online.` and an operator writes
// `atriumdev.online`.
func (c *Client) FindZone(ctx context.Context, projectID, zoneName string) (*DnsZone, bool, error) {
	zones, err := c.ListZones(ctx, projectID)
	if err != nil {
		return nil, false, err
	}
	want := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(zoneName), "."))
	for i := range zones {
		if ZoneDomain(&zones[i]) == want {
			return &zones[i], true, nil
		}
	}
	return nil, false, nil
}

// GetZone reads one zone by id, records included.
func (c *Client) GetZone(ctx context.Context, zoneID string) (*DnsZone, error) {
	var out DnsZone
	if err := c.request(ctx, http.MethodGet, c.dnsURL, "/v1/publicZones/"+url.PathEscape(zoneID), nil, nil, &out); err != nil {
		return nil, errors.Wrapf(err, "failed to read cloud.ru DNS zone %q", zoneID)
	}
	return &out, nil
}

// DeleteZone removes a zone. Absence is success.
func (c *Client) DeleteZone(ctx context.Context, zoneID string) error {
	err := c.request(ctx, http.MethodDelete, c.dnsURL, "/v1/publicZones/"+url.PathEscape(zoneID), nil, nil, nil)
	if err != nil && !NotFound(err) {
		return errors.Wrapf(err, "failed to delete cloud.ru DNS zone %q", zoneID)
	}
	return nil
}

// putZoneRecords replaces a zone's ENTIRE record set, then CHECKS that it did.
//
// Three API properties collapse into this one function, and each of them can
// destroy an operator's zone quietly:
//
//  1. There is no per-record route, so a PATCH carrying one record deletes every
//     other record in the zone. Callers must pass the full desired set — which is
//     why this is unexported and ApplyDnsRecord reads before it writes.
//  2. The write is asynchronous, and the zone reads back `records: []` while it is
//     in flight. Without the wait, a caller concludes the write destroyed the zone.
//  3. **`done: true` does not mean converged.** A PATCH issued while the previous
//     write is still settling drops records — measured twice on 2026-10-24: a
//     two-record write landed one record, and a following two-record write landed
//     none, emptying the zone. Both operations reported `done: true` with an empty
//     `error`, and the zone was still empty 80 seconds later, so this is lost data
//     and not a stale read. The same three-record write, issued after a quiet
//     period, applied perfectly.
//
// So the operation's own verdict is not trusted. After it reports done, the zone is
// re-read until it actually holds what was written, and the PATCH is re-issued if it
// does not. That turns the failure mode from "green deploy, empty zone" into either
// a correct zone or a loud error.
func (c *Client) putZoneRecords(ctx context.Context, zoneID string, records []DnsRecord) error {
	path := "/v1/publicZones/" + url.PathEscape(zoneID)
	body := map[string]any{"records": records}

	var lastSeen int
	for attempt := 1; attempt <= dnsOperationPoll.writeAttempts; attempt++ {
		var op dnsOperation
		if err := c.request(ctx, http.MethodPatch, c.dnsURL, path, nil, body, &op); err != nil {
			return errors.Wrapf(err, "failed to write records of cloud.ru DNS zone %q", zoneID)
		}
		if op.ID != "" {
			if err := c.waitDnsOperation(ctx, op.ID); err != nil {
				return err
			}
		}

		converged, seen, err := c.zoneHoldsRecords(ctx, zoneID, records)
		if err != nil {
			return err
		}
		lastSeen = seen
		if converged {
			return nil
		}
		// Diverged. Let the zone settle before trying again — retrying immediately
		// is the very thing that loses records.
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(dnsOperationPoll.settle):
		}
	}
	return errors.Errorf("cloud.ru DNS zone %q did not converge after %d write attempt(s): %d of %d record(s) "+
		"present. The API reported every write as successful, which it does even when it discards records, so "+
		"check the zone in the console before re-running — records that were in the zone before may be gone",
		zoneID, dnsOperationPoll.writeAttempts, lastSeen, len(records))
}

// zoneHoldsRecords reports whether the zone's live record set IS what was just
// written, and how much of it is there.
//
// Identity plus values plus TTL: a partial write is the failure being guarded
// against, so "the record is there but with the old value" has to count as
// divergence too. The writable count has to match as well — a PATCH carries the
// whole set, so a record SC meant to drop and that is still there is divergence in
// the other direction, which is how a destroy is confirmed. Read-only records (the
// zone's own) are excluded from that count since SC never sends them.
func (c *Client) zoneHoldsRecords(ctx context.Context, zoneID string, want []DnsRecord) (bool, int, error) {
	deadline := time.Now().Add(dnsOperationPoll.convergeTimeout)
	best := 0
	for {
		zone, err := c.GetZone(ctx, zoneID)
		if err != nil {
			return false, best, err
		}
		found, writable := 0, 0
		for i := range zone.Records {
			if !zone.Records[i].ReadOnly {
				writable++
			}
		}
		for _, w := range want {
			for i := range zone.Records {
				if dnsRecordsEqual(zone.Records[i], w) && recordMatches(&zone.Records[i], w) {
					found++
					break
				}
			}
		}
		if found > best {
			best = found
		}
		if found == len(want) && writable == len(want) {
			return true, found, nil
		}
		if time.Now().After(deadline) {
			return false, best, nil
		}
		select {
		case <-ctx.Done():
			return false, best, ctx.Err()
		case <-time.After(dnsOperationPoll.interval):
		}
	}
}

// dnsOperationPoll is how often a write is polled, and for how long. A var rather
// than a const only so the tests do not have to sleep through it.
//
// convergeTimeout/settle/writeAttempts exist because the operation lies about being
// finished (see putZoneRecords). settle is generous on purpose: the one probe that
// applied cleanly waited ~30s after the previous write, and every probe that wrote
// sooner lost records.
var dnsOperationPoll = struct {
	interval        time.Duration
	timeout         time.Duration
	convergeTimeout time.Duration
	settle          time.Duration
	writeAttempts   int
}{
	interval:        2 * time.Second,
	timeout:         3 * time.Minute,
	convergeTimeout: 45 * time.Second,
	settle:          30 * time.Second,
	writeAttempts:   2,
}

// waitDnsOperation polls one write to completion.
func (c *Client) waitDnsOperation(ctx context.Context, opID string) error {
	interval, timeout := dnsOperationPoll.interval, dnsOperationPoll.timeout
	deadline := time.Now().Add(timeout)
	for {
		var op dnsOperation
		if err := c.request(ctx, http.MethodGet, c.dnsURL, "/v1/operations/"+url.PathEscape(opID), nil, nil, &op); err != nil {
			return errors.Wrapf(err, "failed to read cloud.ru DNS operation %q", opID)
		}
		if len(op.Error) > 0 {
			return errors.Errorf("cloud.ru DNS operation %q failed: %v", opID, op.Error)
		}
		if op.Done {
			return nil
		}
		if time.Now().After(deadline) {
			return errors.Errorf("cloud.ru DNS operation %q did not complete within %s — the record may still be "+
				"applied later; re-run to converge", opID, timeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}
	}
}

// DnsRecordSpec is the desired state of one record set, as the `sc cloudru
// dns-record` subcommands receive it.
type DnsRecordSpec struct {
	// ProjectID scopes the zone lookup. Required unless ZoneID is given. It is the
	// ordinary Evolution project id — the same one the rest of this package uses.
	ProjectID string `json:"projectId,omitempty"`
	// ZoneName is the DNS zone. Looked up, never created.
	ZoneName string `json:"zoneName,omitempty"`
	// ZoneID skips the lookup.
	ZoneID string `json:"zoneId,omitempty"`

	Name   string   `json:"name,omitempty"`
	Type   string   `json:"type,omitempty"`
	TTL    int      `json:"ttl,omitempty"`
	Values []string `json:"values,omitempty"`
}

// DnsResult is the record half of a `sc cloudru` result document.
type DnsResult struct {
	ZoneID     string `json:"zoneId,omitempty"`
	ZoneName   string `json:"zoneName,omitempty"`
	RecordName string `json:"recordName,omitempty"`
	RecordType string `json:"recordType,omitempty"`
}

// resolveZone turns a spec's zone reference into a zone, checking that what came
// back serves the zone the config names.
//
// The check matters for the same reason it does in the Yandex registrar: a record
// written into the wrong zone is inert rather than an error, so it presents as
// "DNS has not propagated yet" indefinitely.
func (c *Client) resolveZone(ctx context.Context, spec DnsRecordSpec) (*DnsZone, error) {
	if spec.ZoneID != "" {
		zone, err := c.GetZone(ctx, spec.ZoneID)
		if err != nil {
			return nil, err
		}
		want := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(spec.ZoneName), "."))
		if want != "" && ZoneDomain(zone) != want {
			return nil, errors.Errorf("cloud.ru DNS zone %q serves %q, not the configured zoneName %q",
				spec.ZoneID, ZoneDomain(zone), spec.ZoneName)
		}
		return zone, nil
	}
	// The spec may carry its own project, but the credentials already name one and
	// that is where the operator put it — a request with `projectId` only at the top
	// level failed the lookup with "`projectId` is required", which reads like the
	// field was missing when it was right there (live-caught 2026-10-24).
	projectID := spec.ProjectID
	if projectID == "" {
		projectID = c.creds.ProjectID
	}
	zone, found, err := c.FindZone(ctx, projectID, spec.ZoneName)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, errors.Errorf("cloud.ru DNS zone %q was not found in project %q. The zone is looked up and "+
			"never created by a deploy — create it in the console (Cloud DNS → Зоны), then point the domain's "+
			"registrar at %s. If the zone DOES exist, check that `projectId` names the project that owns it, or "+
			"set `zoneId` outright: it is the path segment of the zone's own console URL "+
			"(.../dns/zones/public/<zoneId>/edit) and skips this lookup entirely",
			spec.ZoneName, projectID, strings.Join(DnsNameservers, " and "))
	}
	return zone, nil
}

// ApplyDnsRecord converges one record set inside a zone.
//
// Read-modify-write, and not for tidiness: a PATCH carries the zone's whole record
// array, so writing one record without first reading the others deletes them. The
// read is the only thing standing between a `domain:` in a client.yaml and a
// deploy that silently empties a shared zone.
func (c *Client) ApplyDnsRecord(ctx context.Context, req Request, progress func(string, ...any)) (*Result, error) {
	if progress == nil {
		progress = func(string, ...any) {}
	}
	if err := c.creds.Validate(); err != nil {
		return nil, err
	}
	if req.DnsSpec == nil {
		return nil, errors.Errorf("`dnsSpec` is required")
	}
	spec := *req.DnsSpec
	if spec.Type == "" {
		return nil, errors.Errorf("`dnsSpec.type` is required")
	}
	if len(spec.Values) == 0 {
		return nil, errors.Errorf("`dnsSpec.values` is required: a record set with no values is a deletion, "+
			"and a deploy must not delete %q implicitly", spec.Name)
	}

	zone, err := c.resolveZone(ctx, spec)
	if err != nil {
		return nil, err
	}
	name, err := dnsRecordName(spec.Name, ZoneDomain(zone))
	if err != nil {
		return nil, err
	}
	desired := DnsRecord{Name: name, Type: dnsRecordType(spec.Type), Values: spec.Values, TTL: spec.TTL}

	records := append([]DnsRecord(nil), zone.Records...)
	idx := -1
	for i := range records {
		if dnsRecordsEqual(records[i], desired) {
			idx = i
			break
		}
	}
	label := lo.If(name == "", "the apex").Else(name)
	switch {
	case idx < 0:
		progress("creating %s record %q in zone %q (zone holds %d other record(s))",
			desired.Type, label, ZoneDomain(zone), len(records))
		records = append(records, desired)
	case recordMatches(&records[idx], desired):
		progress("%s record %q in zone %q is already up to date", desired.Type, label, ZoneDomain(zone))
		return dnsResultOf(zone, desired), nil
	default:
		progress("updating %s record %q in zone %q", desired.Type, label, ZoneDomain(zone))
		// Preserve the fields the API owns (id, wildcard, description) — a PATCH
		// that dropped them would re-create the record rather than amend it.
		records[idx].Values = desired.Values
		// An unset desired TTL leaves the live one alone. Writing 0 here would be a
		// real change — the API stores a literal 0 — so a caller that simply did not
		// mention a TTL would silently clear one that was set deliberately.
		if desired.TTL != 0 {
			records[idx].TTL = desired.TTL
		}
	}

	if err := c.putZoneRecords(ctx, zone.ID, records); err != nil {
		return nil, err
	}
	return dnsResultOf(zone, desired), nil
}

// DestroyDnsRecord removes one record set, leaving the zone's others alone.
// Absence is success, so a destroy converges when it runs twice.
func (c *Client) DestroyDnsRecord(ctx context.Context, req Request, progress func(string, ...any)) (*Result, error) {
	if progress == nil {
		progress = func(string, ...any) {}
	}
	if err := c.creds.Validate(); err != nil {
		return nil, err
	}
	if req.DnsSpec == nil {
		return nil, errors.Errorf("`dnsSpec` is required")
	}
	spec := *req.DnsSpec
	if spec.Type == "" {
		return nil, errors.Errorf("`dnsSpec.type` is required")
	}

	zone, err := c.resolveZone(ctx, spec)
	if err != nil {
		// A destroy must converge against a zone that is already gone: deleting the
		// zone deletes its records, so there is nothing left to do and nothing wrong.
		if NotFound(err) || strings.Contains(err.Error(), "was not found in project") {
			return &Result{Name: spec.Name, DnsResult: DnsResult{RecordName: spec.Name, RecordType: dnsRecordType(spec.Type)}}, nil
		}
		return nil, err
	}
	name, err := dnsRecordName(spec.Name, ZoneDomain(zone))
	if err != nil {
		return nil, err
	}
	desired := DnsRecord{Name: name, Type: dnsRecordType(spec.Type)}

	kept := make([]DnsRecord, 0, len(zone.Records))
	removed := false
	for _, r := range zone.Records {
		if dnsRecordsEqual(r, desired) {
			removed = true
			continue
		}
		kept = append(kept, r)
	}
	if !removed {
		progress("%s record %q is already absent from zone %q", desired.Type, spec.Name, ZoneDomain(zone))
		return dnsResultOf(zone, desired), nil
	}
	progress("deleting %s record %q from zone %q (%d record(s) kept)",
		desired.Type, lo.If(name == "", "the apex").Else(name), ZoneDomain(zone), len(kept))
	if err := c.putZoneRecords(ctx, zone.ID, kept); err != nil {
		return nil, err
	}
	return dnsResultOf(zone, desired), nil
}

func dnsResultOf(zone *DnsZone, record DnsRecord) *Result {
	return &Result{
		Name: record.Name,
		DnsResult: DnsResult{
			ZoneID:     zone.ID,
			ZoneName:   ZoneDomain(zone),
			RecordName: record.Name,
			RecordType: record.Type,
		},
	}
}

// recordMatches reports whether the live record already is the desired one, so an
// unchanged deploy performs no write.
//
// Values are compared as a set: the API is free to return them in its own order,
// and a reorder is not a change.
//
// TTL is compared only when the desired TTL is set, so an unset TTL means "don't
// care" rather than "set it to zero". This was briefly the other way round — TTL
// skipped entirely, on a measurement that said the API ignored the field — and that
// made a TTL change a silent no-op: SC printed "already up to date" and wrote
// nothing. The API does honour TTL (900 reads back as 900), so it is a real part of
// the record's identity.
func recordMatches(existing *DnsRecord, desired DnsRecord) bool {
	if existing == nil {
		return false
	}
	if desired.TTL != 0 && existing.TTL != desired.TTL {
		return false
	}
	if len(existing.Values) != len(desired.Values) {
		return false
	}
	return len(lo.Without(desired.Values, existing.Values...)) == 0
}
