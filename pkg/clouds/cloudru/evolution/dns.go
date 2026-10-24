// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package evolution

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"github.com/pkg/errors"
	"github.com/samber/lo"
)

// ──────────────────────────────────────────────────────────────────────────────
// Cloud.ru **Cloud DNS**.
//
// READ THIS FIRST: unlike everything else in this package, the routes and field
// names below are taken from Cloud.ru's published API reference and have NOT been
// measured against a live project. The service-account key the rest of this
// package was built with lacks the Cloud DNS role, and the API answers that with
//
//	403 {"code":7,"message":"permission denied: invalid scopes"}
//
// on the one path that reaches the backend at all. Which is the second thing to
// know: **every unknown path under /api/clouddns answers a bodiless 415**, not a
// 404 — including deliberately bogus ones. So route probing cannot discover or
// confirm anything here, and a 415 from any call below means "this path is not a
// route", not "wrong content type".
//
// What that means for the code: the shapes are a faithful transcription, the
// error messages say what to check, and the first live smoke should expect to
// correct something. The candidates, in order of likelihood:
//
//  1. Whether a record `name` is relative to the zone, absolute, or
//     absolute-with-trailing-dot (see dnsRecordName — SC sends the FQDN).
//  2. The `enabled` field's wire name: the reference spells it `enables`, which
//     reads like a typo in the reference itself, so BOTH are accepted on the way
//     in and the field is omitted on the way out unless explicitly set.
//  3. Whether PATCH's `updateMask` wants `values` or `record.values`.
//
// A fourth unknown, and the one with a way around it: **what `parentId` actually
// is has never been established.** An earlier note here asserted it was a "Cloud
// DNS service instance id, the trailing segment of
// `https://console.cloud.ru/spa/cloud-dns/<parentId>`" — confidently, and wrongly:
// that URL shape does not exist. A real zone's console URL is
// `/spa/dns/zones/public/<zoneId>/edit?customerId=…&projectId=…`, which offers a
// zone id, a customer id and a project id, and no fourth id. Whether `parentId`
// wants one of those three is unknown, because every call here from an account
// without the Cloud DNS role answers 403 regardless of the value — so no value has
// been seen working *or* being rejected on its merits.
//
// The cost of a wrong value is why this is worth the paragraph: a list scoped to an
// unrelated parent authenticates fine and returns an **empty set of zones**, which
// reads exactly like "we have no zones". Hence the way around it — prefer
// `zoneId`, which names the zone outright, is readable straight off the console
// URL, and needs no parent scope at all (resolveZone's ZoneID branch never touches
// parentId).
// ──────────────────────────────────────────────────────────────────────────────

// DefaultDnsEndpoint serves the Cloud DNS API.
//
// Note the host: `console.cloud.ru`, not the `*.api.cloud.ru` pattern every other
// endpoint in this package follows, and the `/api/clouddns` prefix is part of the
// base rather than of the paths. Both are as published.
const DefaultDnsEndpoint = "https://console.cloud.ru/api/clouddns"

// DefaultDnsRecordTtl is the TTL applied when the config names none. It is also
// the API's own documented default, so sending it changes nothing — SC sends it
// anyway, because a TTL that lives only in a remote default is one SC cannot
// report in a diff.
const DefaultDnsRecordTtl = 3600

// DnsNameservers is the FALLBACK NS set a domain's registrar must point at for
// Evolution Cloud DNS to become authoritative for it. Prefer ZoneNameservers,
// which reads the pair the zone itself publishes.
//
// **The two estates have different nameservers, and an earlier version of this
// comment got it backwards.** It asserted — from Cloud.ru's own quickstart and
// delegation guide, which agree with each other — that `cdns-ns01/cdns-ns02.
// sbercloud.ru` serves zones on BOTH estates and "there is no Evolution-specific
// nameserver". A real Evolution zone page says otherwise: it publishes
//
//	evo-cns01.cloud.ru.   evo-cns02.cloud.ru.
//
// and those are not aliases of the other pair — they resolve into a different
// network entirely (94.158.23.11 / 94.158.23.22, against 37.18.8.44 / 37.18.8.88
// for the sbercloud pair, measured 2026-10-24 over TCP via a public resolver).
// The `evo-` prefix is doing exactly what it looks like it is doing. So those two
// docs pages describe the Advanced/ex-SberCloud estate, and this package is
// Evolution.
//
// The lesson generalises past DNS: on this cloud, documentation that does not say
// which estate it is about should be assumed to be about the other one.
//
// Whether these hosts answer *authoritatively* is still not established from here
// — the box this was measured on runs a fake-IP DNS proxy that intercepts
// `@server` queries and answers them itself.
var DnsNameservers = []string{"evo-cns01.cloud.ru", "evo-cns02.cloud.ru"}

// ZoneNameservers is what to put at the registrar for this zone.
//
// Prefer the zone's own list over the package default. The console shows an
// "NS-серверы" field per zone, so the API plausibly carries one too — and a value
// the zone states about itself cannot go stale the way a constant in SC can, which
// it just did (see DnsNameservers). The wire name `nameservers` is a guess until
// the first live call; when it is absent or empty we fall back, so being wrong
// about it costs nothing worse than today's behaviour.
func ZoneNameservers(zone *DnsZone) []string {
	if zone != nil && len(zone.Nameservers) > 0 {
		return zone.Nameservers
	}
	return DnsNameservers
}

// DnsZone is a zone as the API returns it.
//
// `Valid`, `ValidationText` and `Delegated` describe two different gates, and
// **how many of them actually apply on Evolution is not settled**:
//
//  1. `Valid` — the zone is *verified*: the operator published the TXT record in
//     `ValidationText` (shape `sbc-verification: <uuid>`) at the apex, proving
//     they control the domain, and Cloud DNS refuses record creation until they
//     have. These three fields are a faithful transcription of the API reference,
//     but an Evolution zone created in the console on 2026-10-24 went straight to
//     **Активен** with its NS pair published and **no verification value anywhere
//     on the page** — which is the DNS-hosting model (Route 53, Yandex), not the
//     domain-verification model. So verification may well be an Advanced-estate
//     concept that the shared reference documents without saying so, exactly like
//     the nameservers were (see DnsNameservers).
//  2. `Delegated` — the registrar's NS records point at the zone's nameservers.
//     Reported rather than enforced, so a record written into an undelegated zone
//     is created successfully and resolves nowhere, which is the single most
//     confusing way for this path to fail.
//
// IF verification applies, the order matters operationally and not just logically:
// it is done while the OLD nameservers are still authoritative, because that is
// where the operator can publish a TXT record. Switching NS first would produce a
// window in which Cloud DNS is authoritative for the domain and still refuses to
// hold records for it — the domain goes dark and SC's own deploy is what fails.
//
// **`Valid` and `Delegated` are pointers, and that is load-bearing.** As plain
// bools they were indistinguishable from absent — Go decodes a missing JSON field
// to `false` — so a response that simply does not carry `valid` read as
// "affirmatively unverified" and `checkZoneVerified` refused every write, naming a
// TXT record that does not exist. A gate meant to turn one confusing failure into
// a clear message would instead have manufactured a hard block out of a field the
// API may never send. Both are now only acted on when the API says so.
type DnsZone struct {
	ID       string `json:"id,omitempty"`
	Name     string `json:"name,omitempty"`
	ParentID string `json:"parentId,omitempty"`

	// Nameservers is the NS pair this zone publishes for itself, shown in the
	// console as "NS-серверы". The wire name is a guess; ZoneNameservers falls
	// back to the package default when it is absent.
	Nameservers []string `json:"nameservers,omitempty"`

	Delegated      *bool  `json:"delegated,omitempty"`
	Valid          *bool  `json:"valid,omitempty"`
	ValidationText string `json:"validationText,omitempty"`
	LastCheck      string `json:"lastCheck,omitempty"`
	CreatedAt      string `json:"createdAt,omitempty"`
	UpdatedAt      string `json:"updatedAt,omitempty"`
}

// DnsRecord is a record set: one name + type holding every value, as in Yandex
// Cloud DNS and unlike Cloudflare, where each value is its own resource.
type DnsRecord struct {
	ZoneID string   `json:"zoneId,omitempty"`
	Name   string   `json:"name,omitempty"`
	Type   string   `json:"type,omitempty"`
	TTL    int      `json:"ttl,omitempty"`
	Values []string `json:"values,omitempty"`

	// Enabled is `enables` in the published reference. Treated as a quirk of the
	// reference rather than of the API: it is decoded from either spelling and
	// encoded only when non-nil, so SC never sends a field it is guessing about.
	Enabled *bool `json:"enabled,omitempty"`
}

// UnmarshalJSON accepts both spellings of the enabled flag. See DnsRecord.Enabled.
func (r *DnsRecord) UnmarshalJSON(data []byte) error {
	type plain DnsRecord
	var aux struct {
		plain
		Enables *bool `json:"enables,omitempty"`
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	*r = DnsRecord(aux.plain)
	if r.Enabled == nil {
		r.Enabled = aux.Enables
	}
	return nil
}

// dnsRecordKey is the `{name}/{type}` path suffix the record routes take. Both
// segments are escaped: a record name legitimately contains characters (`*`, `_`)
// that a path segment must encode, and an unescaped `*` silently matches a
// different route on some gateways.
func dnsRecordKey(name, recordType string) string {
	return url.PathEscape(name) + "/" + url.PathEscape(strings.ToUpper(recordType))
}

// dnsRecordName is the name SC sends.
//
// The FQDN with no trailing dot, which is the form the reference's examples use.
// This is the most likely thing to be wrong on the first live smoke (see the
// package note above): a relative name written as an FQDN would create
// `app.example.ru.example.ru`, which the API accepts and which resolves nowhere.
// Check a created record's own `name` in the console before writing a second one.
func dnsRecordName(name, zoneName string) string {
	name = strings.TrimSuffix(strings.TrimSpace(name), ".")
	zone := strings.TrimSuffix(strings.TrimSpace(zoneName), ".")
	if name == "" || zone == "" {
		return name
	}
	if strings.EqualFold(name, zone) || strings.HasSuffix(strings.ToLower(name), "."+strings.ToLower(zone)) {
		return name
	}
	return name + "." + zone
}

// dnsQuery is the zone-listing scope: the Cloud DNS **service instance** id, as
// `parentId`. Not `projectId` and not `project_id` — this API is a third scoping
// convention in one cloud, which is why it gets its own helper next to the two
// others rather than reusing either.
func dnsQuery(parentID string) url.Values {
	return url.Values{"parentId": []string{parentID}}
}

// listDnsCollection reads a Cloud DNS list answer.
//
// It goes through decodeCollection for the reason that helper documents: this cloud
// answers collections in two shapes with no rule saying which, the compute API
// already proved it the hard way, and NOTHING about the DNS collection shapes has
// been measured (see the package note). Sniffing costs a byte comparison and makes
// both shapes work; a hard-coded guess would make exactly one of them work and would
// be found out on the first live record, not here.
//
// No pagination: the reference documents neither `limit` nor `offset` on these
// routes, so following an offset would be inventing a protocol. A zone with more
// records than one page would silently read short — if a live smoke ever sees a
// truncated list, that is the thing to fix, and `total` in the envelope is where the
// evidence will be.
func listDnsCollection[T any](ctx context.Context, c *Client, path string, query url.Values) ([]T, error) {
	var raw json.RawMessage
	if err := c.request(ctx, http.MethodGet, c.dnsURL, path, query, nil, &raw); err != nil {
		return nil, err
	}
	envelope, err := decodeCollection[T](raw)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to decode cloud.ru DNS %s", path)
	}
	return envelope.Items, nil
}

// ListZones returns every zone under one Cloud DNS parent.
func (c *Client) ListZones(ctx context.Context, parentID string) ([]DnsZone, error) {
	if strings.TrimSpace(parentID) == "" {
		return nil, errors.Errorf("`parentId` is required to list Cloud.ru DNS zones — but if you are trying to " +
			"reach ONE known zone, set `zoneId` instead and skip the listing entirely: it is the path segment " +
			"of the zone's own console URL (.../dns/zones/public/<zoneId>/edit). Which id `parentId` wants is " +
			"not something we can tell you (see the package comment); a wrong one does not error, it lists an " +
			"empty set")
	}
	out, err := listDnsCollection[DnsZone](ctx, c, "/v1/zones", dnsQuery(parentID))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to list cloud.ru DNS zones under parent %q", parentID)
	}
	return out, nil
}

// FindZone resolves a zone by DNS name under one Cloud DNS parent.
//
// By name rather than by id because that is what an operator writes in
// server.yaml, and the comparison is case-insensitive and trailing-dot-insensitive
// because the API's spelling of a zone name is one of the unmeasured things here.
func (c *Client) FindZone(ctx context.Context, parentID, zoneName string) (*DnsZone, bool, error) {
	zones, err := c.ListZones(ctx, parentID)
	if err != nil {
		return nil, false, err
	}
	want := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(zoneName), "."))
	for i := range zones {
		if strings.ToLower(strings.TrimSuffix(zones[i].Name, ".")) == want {
			return &zones[i], true, nil
		}
	}
	return nil, false, nil
}

// GetZone reads one zone by id.
func (c *Client) GetZone(ctx context.Context, zoneID string) (*DnsZone, error) {
	var out DnsZone
	if err := c.request(ctx, http.MethodGet, c.dnsURL, "/v1/zones/"+url.PathEscape(zoneID), nil, nil, &out); err != nil {
		return nil, errors.Wrapf(err, "failed to read cloud.ru DNS zone %q", zoneID)
	}
	return &out, nil
}

// CreateZone creates a zone under a service instance.
//
// Exposed, but NOT reached by the registrar: like the Yandex one, the registrar
// looks a zone up and never creates it. A public zone is authoritative for a real
// domain, so creating one from a deploy either collides with the delegation that
// exists or produces a second zone nobody's nameservers point at. The method is
// here for `sc` to offer as an explicit operator action.
func (c *Client) CreateZone(ctx context.Context, parentID, zoneName string) (*DnsZone, error) {
	body := map[string]string{"name": strings.TrimSuffix(zoneName, "."), "parentId": parentID}
	var out DnsZone
	if err := c.request(ctx, http.MethodPost, c.dnsURL, "/v1/zones", nil, body, &out); err != nil {
		return nil, errors.Wrapf(err, "failed to create cloud.ru DNS zone %q", zoneName)
	}
	return &out, nil
}

// DeleteZone removes a zone. Absence is success.
func (c *Client) DeleteZone(ctx context.Context, zoneID string) error {
	err := c.request(ctx, http.MethodDelete, c.dnsURL, "/v1/zones/"+url.PathEscape(zoneID), nil, nil, nil)
	if err != nil && !NotFound(err) {
		return errors.Wrapf(err, "failed to delete cloud.ru DNS zone %q", zoneID)
	}
	return nil
}

// ListRecords returns every record set in a zone.
func (c *Client) ListRecords(ctx context.Context, zoneID string) ([]DnsRecord, error) {
	path := "/v1/zones/" + url.PathEscape(zoneID) + "/records"
	out, err := listDnsCollection[DnsRecord](ctx, c, path, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to list records of cloud.ru DNS zone %q", zoneID)
	}
	return out, nil
}

// FindRecord reads one record set, reporting absence rather than erroring on it.
func (c *Client) FindRecord(ctx context.Context, zoneID, name, recordType string) (*DnsRecord, bool, error) {
	var out DnsRecord
	path := "/v1/zones/" + url.PathEscape(zoneID) + "/records/" + dnsRecordKey(name, recordType)
	err := c.request(ctx, http.MethodGet, c.dnsURL, path, nil, nil, &out)
	if NotFound(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, errors.Wrapf(err, "failed to read %s record %q of cloud.ru DNS zone %q", recordType, name, zoneID)
	}
	return &out, true, nil
}

// CreateRecord creates one record set.
func (c *Client) CreateRecord(ctx context.Context, record DnsRecord) (*DnsRecord, error) {
	body := map[string]any{
		"name":   record.Name,
		"type":   strings.ToUpper(record.Type),
		"ttl":    lo.If(record.TTL > 0, record.TTL).Else(DefaultDnsRecordTtl),
		"values": record.Values,
	}
	if record.Enabled != nil {
		body["enabled"] = *record.Enabled
	}
	var out DnsRecord
	path := "/v1/zones/" + url.PathEscape(record.ZoneID) + "/records"
	if err := c.request(ctx, http.MethodPost, c.dnsURL, path, nil, body, &out); err != nil {
		return nil, errors.Wrapf(err, "failed to create %s record %q in cloud.ru DNS zone %q",
			record.Type, record.Name, record.ZoneID)
	}
	return &out, nil
}

// PatchRecord updates the values and TTL of an existing record set.
//
// A patch rather than delete-then-create, and that is not a style choice: a record
// deleted and recreated is a window in which the name does not resolve, and the
// window is as long as the old TTL. The `updateMask` is what keeps the patch from
// clearing the fields it does not name.
func (c *Client) PatchRecord(ctx context.Context, record DnsRecord) (*DnsRecord, error) {
	inner := map[string]any{
		"zoneId": record.ZoneID,
		"name":   record.Name,
		"type":   strings.ToUpper(record.Type),
		"ttl":    lo.If(record.TTL > 0, record.TTL).Else(DefaultDnsRecordTtl),
		"values": record.Values,
	}
	mask := []string{"values", "ttl"}
	if record.Enabled != nil {
		inner["enabled"] = *record.Enabled
		mask = append(mask, "enabled")
	}
	body := map[string]any{"record": inner, "updateMask": strings.Join(mask, ",")}

	var out DnsRecord
	path := "/v1/zones/" + url.PathEscape(record.ZoneID) + "/records/" + dnsRecordKey(record.Name, record.Type)
	if err := c.request(ctx, http.MethodPatch, c.dnsURL, path, nil, body, &out); err != nil {
		return nil, errors.Wrapf(err, "failed to update %s record %q in cloud.ru DNS zone %q",
			record.Type, record.Name, record.ZoneID)
	}
	return &out, nil
}

// DeleteRecord removes one record set. Absence is success, so a destroy converges
// when it runs twice or after a create that failed halfway.
func (c *Client) DeleteRecord(ctx context.Context, zoneID, name, recordType string) error {
	path := "/v1/zones/" + url.PathEscape(zoneID) + "/records/" + dnsRecordKey(name, recordType)
	err := c.request(ctx, http.MethodDelete, c.dnsURL, path, nil, nil, nil)
	if err != nil && !NotFound(err) {
		return errors.Wrapf(err, "failed to delete %s record %q of cloud.ru DNS zone %q", recordType, name, zoneID)
	}
	return nil
}

// DnsRecordSpec is the desired state of one record set, as the `sc cloudru
// dns-record` subcommands receive it.
type DnsRecordSpec struct {
	// ParentID is the Cloud DNS service instance the zone lives under. Required
	// unless ZoneID is given, since it is what the zone lookup is scoped to.
	ParentID string `json:"parentId,omitempty"`
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
	// ZoneDelegated is reported rather than enforced: a record in an undelegated
	// zone is created successfully and resolves nowhere, and the operator has no
	// other place to learn that from a deploy log.
	//
	// A pointer so that "the API did not tell us" is omitted from the JSON rather
	// than printed as `false` — carrying the tri-state only as far as the Client
	// and then flattening it here would put the same lie in the CLI's output.
	ZoneDelegated *bool `json:"zoneDelegated,omitempty"`
}

// resolveZone turns a spec's zone reference into a zone id, checking that what
// came back serves the zone the config names.
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
		if spec.ZoneName != "" && !strings.EqualFold(
			strings.TrimSuffix(zone.Name, "."), strings.TrimSuffix(spec.ZoneName, ".")) {
			return nil, errors.Errorf("cloud.ru DNS zone %q serves %q, not the configured zoneName %q",
				spec.ZoneID, zone.Name, spec.ZoneName)
		}
		return zone, nil
	}
	zone, found, err := c.FindZone(ctx, spec.ParentID, spec.ZoneName)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, errors.Errorf("cloud.ru DNS zone %q was not found under parentId %q. "+
			"The zone is looked up and never created by a deploy — create it in the console (Cloud DNS → Зоны), "+
			"verify it with the TXT record the zone page shows, then point the domain's registrar at %s. "+
			"If the zone DOES exist, prefer `zoneId` over `parentId`: the zone's own id is the path segment in "+
			"its console URL (.../dns/zones/public/<zoneId>/edit) and skips this lookup entirely, whereas a "+
			"wrong `parentId` does not error — it lists an empty set of zones, which looks identical to having "+
			"none",
			spec.ZoneName, spec.ParentID, strings.Join(DnsNameservers, " and "))
	}
	return zone, nil
}

// checkZoneVerified refuses a write into a zone the API says is unverified.
//
// Worth a pre-check rather than letting the write fail, for two reasons: the API
// rejects the create itself, so without this the operator gets Cloud DNS's own
// error for a cause that is nowhere in it, and the remedy is a value only the zone
// carries.
//
// **Silence is not a refusal.** It acts only on `valid: false` arriving on the
// wire, never on the field being absent — an Evolution zone appears to have no
// verification step at all, so a gate that treated "no field" as "not verified"
// would hard-block every deploy over a TXT record that does not exist. The same
// reasoning is why a zone that IS affirmatively unverified still fails loudly:
// there the API has told us something true and specific.
func checkZoneVerified(zone *DnsZone) error {
	if zone.Valid == nil || *zone.Valid {
		return nil
	}
	hint := "the zone page in the console shows the value to publish"
	if zone.ValidationText != "" {
		hint = errors.Errorf("publish this TXT record at the apex of %s: %q", zone.Name, zone.ValidationText).Error()
	}
	return errors.Errorf("cloud.ru DNS zone %q (id %s) is not verified yet (valid=false), and Cloud DNS refuses "+
		"to create records in an unverified zone — so this is not something a retry fixes. To verify it, %s. "+
		"Do this BEFORE moving the domain's NS records to %s: verification needs a TXT record published where "+
		"the domain resolves TODAY, which is the registrar or the current DNS provider, not Cloud.ru",
		zone.Name, zone.ID, hint, strings.Join(ZoneNameservers(zone), " and "))
}

// ApplyDnsRecord converges one record set: create when absent, patch when present.
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
	if spec.Name == "" || spec.Type == "" {
		return nil, errors.Errorf("`dnsSpec.name` and `dnsSpec.type` are required")
	}
	if len(spec.Values) == 0 {
		return nil, errors.Errorf("`dnsSpec.values` is required: a record set with no values is a deletion, "+
			"and a deploy must not delete %q implicitly", spec.Name)
	}

	zone, err := c.resolveZone(ctx, spec)
	if err != nil {
		return nil, err
	}
	if err := checkZoneVerified(zone); err != nil {
		return nil, err
	}
	name := dnsRecordName(spec.Name, zone.Name)

	record := DnsRecord{ZoneID: zone.ID, Name: name, Type: strings.ToUpper(spec.Type), TTL: spec.TTL, Values: spec.Values}

	existing, found, err := c.FindRecord(ctx, zone.ID, name, record.Type)
	if err != nil {
		return nil, err
	}
	switch {
	case !found:
		progress("creating %s record %q in zone %q", record.Type, name, zone.Name)
		if _, err := c.CreateRecord(ctx, record); err != nil {
			return nil, err
		}
	case recordMatches(existing, record):
		progress("%s record %q in zone %q is already up to date", record.Type, name, zone.Name)
	default:
		progress("updating %s record %q in zone %q", record.Type, name, zone.Name)
		if _, err := c.PatchRecord(ctx, record); err != nil {
			return nil, err
		}
	}

	// Only when the API affirmatively says so — an absent `delegated` would
	// otherwise warn on every deploy of a perfectly delegated zone, and a warning
	// that cries wolf is one nobody reads on the day it is true.
	if zone.Delegated != nil && !*zone.Delegated {
		progress("WARNING: cloud.ru DNS zone %q reports delegated=false — the record exists but will not "+
			"resolve until the domain's registrar points its NS records at %s",
			zone.Name, strings.Join(ZoneNameservers(zone), " and "))
	}

	return &Result{
		Name: name,
		DnsResult: DnsResult{
			ZoneID:        zone.ID,
			ZoneName:      zone.Name,
			RecordName:    name,
			RecordType:    record.Type,
			ZoneDelegated: zone.Delegated,
		},
	}, nil
}

// DestroyDnsRecord removes one record set. Absence is success.
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
	if spec.Name == "" || spec.Type == "" {
		return nil, errors.Errorf("`dnsSpec.name` and `dnsSpec.type` are required")
	}

	zone, err := c.resolveZone(ctx, spec)
	if err != nil {
		// A destroy must converge against a zone that is already gone: deleting the
		// zone deletes its records, so there is nothing left to do and nothing wrong.
		if NotFound(err) || strings.Contains(err.Error(), "was not found under service instance") {
			return &Result{Name: spec.Name, DnsResult: DnsResult{RecordName: spec.Name, RecordType: spec.Type}}, nil
		}
		return nil, err
	}
	name := dnsRecordName(spec.Name, zone.Name)
	recordType := strings.ToUpper(spec.Type)
	progress("deleting %s record %q from zone %q", recordType, name, zone.Name)
	if err := c.DeleteRecord(ctx, zone.ID, name, recordType); err != nil {
		return nil, err
	}
	return &Result{
		Name: name,
		DnsResult: DnsResult{
			ZoneID: zone.ID, ZoneName: zone.Name, RecordName: name, RecordType: recordType,
			ZoneDelegated: zone.Delegated,
		},
	}, nil
}

// recordMatches reports whether the live record already is the desired one, so an
// unchanged deploy performs no write. Values are compared as a set: the API is
// free to return them in its own order, and a reorder is not a change.
func recordMatches(existing *DnsRecord, desired DnsRecord) bool {
	if existing == nil {
		return false
	}
	if existing.TTL != lo.If(desired.TTL > 0, desired.TTL).Else(DefaultDnsRecordTtl) {
		return false
	}
	if len(existing.Values) != len(desired.Values) {
		return false
	}
	return len(lo.Without(desired.Values, existing.Values...)) == 0
}
