// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package evolution

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	. "github.com/onsi/gomega"
)

// These tests are written against a contract MEASURED live on 2026-10-24, which is
// a material change from what they used to be: the previous set asserted a
// transcription of Cloud.ru's published reference, said so, and was wrong in nearly
// every particular — wrong host, wrong collection, a `parentId` scope that does not
// exist, per-record routes that do not exist, and two zone gates (`valid`,
// `delegated`) keyed on fields the API never sends.
//
// So the fake below is built from recorded responses, and the assertions that look
// pedantic are the ones that cost a round-trip to learn: lowercase types, relative
// names, the whole-array PATCH, and the asynchronous operation that makes a zone
// read back empty mid-write.

// fakeDnsAPI serves IAM plus Cloud DNS from one httptest server, modelling the two
// behaviours that make this API easy to misuse:
//
//   - a PATCH REPLACES the zone's whole record array (there is no record route), so
//     a client that writes without reading first destroys the rest of the zone;
//   - a write is a long-running operation, and the zone reads back with NO records
//     until that operation is polled to completion.
//
// Both are faithful to what was measured, and both are invisible against a
// permissive fake.
type fakeDnsAPI struct {
	mu sync.Mutex

	projectID string
	zone      *DnsZone

	patches    int
	opPolls    int
	pollsToRun int // how many polls the pending operation takes to finish
	loseWrites int // how many completed writes to silently discard, as the real API does

	pendingOp string
	pendingRc []DnsRecord
}

func (f *fakeDnsAPI) handler(t *testing.T) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/v1/auth/token", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, tokenResponse{AccessToken: "token", ExpiresIn: 3600, TokenType: "Bearer"})
	})

	mux.HandleFunc("/v1/publicZones", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		Expect(r.Header.Get("Authorization")).To(Equal("Bearer token"))
		Expect(r.Method).To(Equal(http.MethodGet))
		// The scope is `projectId`. A request carrying `parentId` instead — what SC
		// sent before this was measured — would list nothing and look like an empty
		// project.
		Expect(r.URL.Query().Get("projectId")).To(Equal(f.projectID))
		Expect(r.URL.Query()).NotTo(HaveKey("parentId"))
		if f.zone == nil {
			writeJSON(w, dnsZoneList{Zones: []DnsZone{}})
			return
		}
		writeJSON(w, dnsZoneList{Zones: []DnsZone{*f.zone}})
	})

	mux.HandleFunc("/v1/publicZones/", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		Expect(r.Header.Get("Authorization")).To(Equal("Bearer token"))
		if f.zone == nil {
			http.Error(w, `{"code":5,"message":"Not Found","details":[]}`, http.StatusNotFound)
			return
		}
		switch r.Method {
		case http.MethodGet:
			zone := *f.zone
			// Mid-write, the real API answers with an EMPTY record array. Reading the
			// zone without waiting for the operation is how a working write gets
			// reported as having wiped the zone.
			if f.pendingOp != "" {
				zone.Records = nil
			}
			writeJSON(w, zone)
		case http.MethodPatch:
			var body struct {
				Records []DnsRecord `json:"records"`
			}
			Expect(json.NewDecoder(r.Body).Decode(&body)).To(Succeed())
			for _, rec := range body.Records {
				// Uppercase is refused by the real API with `must be a valid value`,
				// while passing its own record-type lookup — so the error does not read
				// as a casing problem.
				Expect(rec.Type).To(Equal(strings.ToLower(rec.Type)), "record type must be lowercase on the wire")
				// A trailing dot, an `@` and a `*` are all refused; the apex is "".
				Expect(rec.Name).NotTo(HaveSuffix("."))
				Expect(rec.Name).NotTo(Or(Equal("@"), ContainSubstring("*")))
			}
			f.patches++
			f.pendingOp = "op-dns-1"
			f.pendingRc = body.Records
			writeJSON(w, dnsOperation{
				ID:           f.pendingOp,
				ResourceName: "urn:cloud:dns:multizonal:" + f.projectID + ":update:" + f.zone.ID,
				ResourceID:   f.zone.ID,
				Done:         false,
			})
		default:
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
	})

	mux.HandleFunc("/v1/operations/", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.opPolls++
		done := f.opPolls >= f.pollsToRun
		if done && f.pendingOp != "" {
			// The write lands only now. TTL is kept: the API honours it (900 reads back
			// as 900) and stores a literal 0 only when the field is omitted.
			applied := make([]DnsRecord, 0, len(f.pendingRc))
			for _, rec := range f.pendingRc {
				if rec.ID == "" {
					rec.ID = "rec-" + rec.Name + "-" + rec.Type
				}
				applied = append(applied, rec)
			}
			// loseWrites models the measured hazard: an operation reports `done: true`
			// with no error and the records are simply not there. Two live writes were
			// lost exactly this way, one of them emptying the zone.
			if f.loseWrites > 0 {
				f.loseWrites--
				applied = nil
			}
			f.zone.Records = applied
			f.pendingOp = ""
		}
		writeJSON(w, dnsOperation{ID: strings.TrimPrefix(r.URL.Path, "/v1/operations/"), Done: done})
	})

	return mux
}

func newDnsTestClient(t *testing.T, f *fakeDnsAPI) *Client {
	f.projectID = "project-1"
	if f.pollsToRun == 0 {
		f.pollsToRun = 1
	}
	// Poll fast. The real intervals are seconds (and the post-write settle is 30s,
	// deliberately), which would make this file sleep to assert something that has
	// nothing to do with wall-clock.
	saved := dnsOperationPoll
	dnsOperationPoll.interval = time.Millisecond
	dnsOperationPoll.convergeTimeout = 20 * time.Millisecond
	dnsOperationPoll.settle = time.Millisecond
	t.Cleanup(func() { dnsOperationPoll = saved })
	srv := httptest.NewServer(f.handler(t))
	t.Cleanup(srv.Close)
	return New(Credentials{KeyID: "key-id", Secret: "key-secret", ProjectID: f.projectID}, WithAllEndpoints(srv.URL))
}

// testZone is the shape a real zone came back in, trimmed. Note `name` vs `domain`:
// the console slugs the name, and matching a configured zone against it never
// matches — which is what the code did before this was measured.
func testZone(records ...DnsRecord) *DnsZone {
	return &DnsZone{
		ID:        "88fc588d",
		Name:      "example-ru",
		Domain:    "example.ru.",
		ProjectID: "project-1",
		State:     "ok",
		CreateAt:  "2026-10-24T09:00:00Z",
		Records:   records,
	}
}

func dnsSpec(name, recordType string, values ...string) DnsRecordSpec {
	return DnsRecordSpec{
		ProjectID: "project-1",
		ZoneName:  "example.ru",
		Name:      name,
		Type:      recordType,
		Values:    values,
	}
}

func applyReq(spec DnsRecordSpec) Request {
	return Request{
		Credentials: Credentials{KeyID: "key-id", Secret: "key-secret", ProjectID: "project-1"},
		Name:        spec.Name,
		DnsSpec:     &spec,
	}
}

// TestDefaultDnsEndpointIsTheApiHostNotTheConsole pins the correction that unblocked
// this driver. SC was pointed at `console.cloud.ru/api/clouddns` — the console's own
// BFF, which has the right-looking routes and rejects every service-account token
// with `403 permission denied: invalid scopes`. That 403 survives any amount of role
// granting, because it is the wrong front door and not a permission problem.
func TestDefaultDnsEndpointIsTheApiHostNotTheConsole(t *testing.T) {
	RegisterTestingT(t)

	Expect(DefaultDnsEndpoint).To(Equal("https://dns.api.cloud.ru"))
	Expect(DefaultDnsEndpoint).NotTo(ContainSubstring("console.cloud.ru"))
}

// TestDnsNameserversAreTheEvolutionPair pins the correction of 2026-10-24, and the
// test it replaces is the reason it is worth pinning. That one asserted the hosts
// were `cdns-ns01/cdns-ns02.sbercloud.ru` and explained, confidently, that the
// sbercloud.ru domain "is not a mistake" because both of Cloud.ru's own Cloud DNS
// pages name that pair. The pages do — they are just describing the Advanced estate
// without saying so. A real Evolution zone publishes `evo-cns01.cloud.ru` and
// `evo-cns02.cloud.ru`, which resolve into a different network entirely
// (94.158.23.11 / 94.158.23.22 against 37.18.8.44 / 37.18.8.88).
//
// So the assertion is now the opposite of what it was, which is exactly why the
// explanation lives next to it: a future reader who finds `.cloud.ru` surprising
// should not "fix" it back from the same two docs pages.
func TestDnsNameserversAreTheEvolutionPair(t *testing.T) {
	RegisterTestingT(t)

	Expect(DnsNameservers).To(ConsistOf("evo-cns01.cloud.ru", "evo-cns02.cloud.ru"))
	for _, ns := range DnsNameservers {
		Expect(ns).To(HaveSuffix(".cloud.ru"))
		Expect(ns).NotTo(HaveSuffix(".sbercloud.ru"))
	}
}

// TestDnsRecordNameIsRelativeToTheZone covers the conversion with the widest blast
// radius: the API wants a label, SC's other registrars take FQDNs, and neither
// mistake errors. Sending the FQDN is refused outright (loud, cheap); sending a
// label where the zone was already stripped would create `app.example.ru.example.ru`,
// which resolves nowhere and presents as "DNS has not propagated".
func TestDnsRecordNameIsRelativeToTheZone(t *testing.T) {
	RegisterTestingT(t)

	for _, tc := range []struct{ name, zone, want string }{
		{"app", "example.ru", "app"},
		{"app.example.ru", "example.ru", "app"},
		{"app.example.ru.", "example.ru", "app"},         // trailing dot dropped
		{"APP.Example.RU", "example.ru", "app"},          // and lowercased
		{"app.example.ru", "example.ru.", "app"},         // zone given with a dot
		{"a.b.example.ru", "example.ru", "a.b"},          // multi-label stays multi-label
		{"example.ru", "example.ru", ""},                 // the apex, spelled as ""
		{"example.ru.", "example.ru", ""},                //
		{"", "example.ru", ""},                           //
		{"@", "example.ru", ""},                          // `@` is refused by the API; we translate it
		{"notexample.ru", "example.ru", "notexample.ru"}, // suffix, not substring
	} {
		got, err := dnsRecordName(tc.name, tc.zone)
		Expect(err).To(BeNil(), "name=%q zone=%q", tc.name, tc.zone)
		Expect(got).To(Equal(tc.want), "name=%q zone=%q", tc.name, tc.zone)
	}
}

// TestDnsRecordNameRefusesWhatTheApiRefuses keeps the explanation local. The API's
// own answer is `must be in a valid format` with the regex attached, which does not
// tell an operator that a wildcard is a separate flag or that the apex is the empty
// string.
func TestDnsRecordNameRefusesWhatTheApiRefuses(t *testing.T) {
	RegisterTestingT(t)

	_, err := dnsRecordName("*.example.ru", "example.ru")
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("wildcard"))

	_, err = dnsRecordName("-leading-hyphen", "example.ru")
	Expect(err).To(HaveOccurred())

	_, err = dnsRecordName("under_score", "example.ru")
	Expect(err).To(HaveOccurred())
}

func TestZoneDomainStripsTheTrailingDot(t *testing.T) {
	RegisterTestingT(t)

	Expect(ZoneDomain(nil)).To(Equal(""))
	Expect(ZoneDomain(&DnsZone{Domain: "example.ru."})).To(Equal("example.ru"))
	Expect(ZoneDomain(&DnsZone{Domain: "Example.RU"})).To(Equal("example.ru"))
	// The slug is NOT the domain, and this is the pair that made FindZone never match.
	Expect(ZoneDomain(&DnsZone{Name: "example-ru", Domain: "example.ru."})).To(Equal("example.ru"))
}

// TestFindZoneMatchesTheDomainNotTheSlug is the regression for that bug: the zone
// `atriumdev.online` is stored as `name: atriumdev-online`, so a lookup keyed on
// `name` reports "zone not found" against a zone sitting right there.
func TestFindZoneMatchesTheDomainNotTheSlug(t *testing.T) {
	RegisterTestingT(t)

	f := &fakeDnsAPI{zone: testZone()}
	c := newDnsTestClient(t, f)

	zone, found, err := c.FindZone(context.Background(), "project-1", "example.ru")
	Expect(err).To(BeNil())
	Expect(found).To(BeTrue())
	Expect(zone.ID).To(Equal("88fc588d"))

	// The slug must NOT match: accepting it would make a server.yaml that works by
	// accident, and break the day the console re-slugs.
	_, found, err = c.FindZone(context.Background(), "project-1", "example-ru")
	Expect(err).To(BeNil())
	Expect(found).To(BeFalse())
}

// TestApplyDnsRecordFallsBackToTheCredentialsProject: the project may be named once,
// in the credentials, and that has to be enough. Without the fallback a request that
// carried `projectId` only at the top level — where the operator naturally puts it,
// next to the key — failed with "`projectId` is required", which reads as if the
// field were missing while it is right there in the request (live-caught 2026-10-24).
func TestApplyDnsRecordFallsBackToTheCredentialsProject(t *testing.T) {
	RegisterTestingT(t)

	f := &fakeDnsAPI{zone: testZone()}
	c := newDnsTestClient(t, f)

	spec := dnsSpec("api", "a", "203.0.113.10")
	spec.ProjectID = "" // named only by the credentials

	res, err := c.ApplyDnsRecord(context.Background(), applyReq(spec), nil)
	Expect(err).To(BeNil())
	Expect(res.ZoneID).To(Equal("88fc588d"))
	Expect(f.zone.Records).To(HaveLen(1))
}

// TestApplyDnsRecordPreservesTheRestOfTheZone is the single most important test in
// this file. Cloud DNS has no record endpoint: a PATCH carries the zone's entire
// record array, so a client that writes one record without reading the others
// DELETES them — silently, with a green deploy, in whatever zone the operator pointed
// SC at.
func TestApplyDnsRecordPreservesTheRestOfTheZone(t *testing.T) {
	RegisterTestingT(t)

	f := &fakeDnsAPI{zone: testZone(
		DnsRecord{ID: "r1", Name: "www", Type: "a", Values: []string{"203.0.113.1"}},
		DnsRecord{ID: "r2", Name: "", Type: "txt", Values: []string{"v=spf1 -all"}},
	)}
	c := newDnsTestClient(t, f)

	_, err := c.ApplyDnsRecord(context.Background(), applyReq(dnsSpec("api.example.ru", "A", "203.0.113.10")), nil)
	Expect(err).To(BeNil())

	Expect(f.patches).To(Equal(1))
	Expect(f.zone.Records).To(HaveLen(3))

	byName := map[string]DnsRecord{}
	for _, rec := range f.zone.Records {
		byName[rec.Name+"/"+rec.Type] = rec
	}
	Expect(byName).To(HaveKey("www/a"))
	Expect(byName).To(HaveKey("/txt"))
	// The new one arrived relative and lowercase.
	Expect(byName["api/a"].Values).To(ConsistOf("203.0.113.10"))
}

// TestApplyDnsRecordWaitsForTheOperation covers the async write. Without the wait the
// zone reads back with no records for a few seconds, which produced two separate
// false conclusions that a write had wiped the zone before polling was added.
func TestApplyDnsRecordWaitsForTheOperation(t *testing.T) {
	RegisterTestingT(t)

	f := &fakeDnsAPI{zone: testZone(), pollsToRun: 3}
	c := newDnsTestClient(t, f)

	_, err := c.ApplyDnsRecord(context.Background(), applyReq(dnsSpec("api", "a", "203.0.113.10")), nil)
	Expect(err).To(BeNil())
	Expect(f.opPolls).To(BeNumerically(">=", 3))
	Expect(f.pendingOp).To(BeEmpty())
	Expect(f.zone.Records).To(HaveLen(1))
}

// TestApplyDnsRecordIsIdempotent: an unchanged deploy must not write. The record is
// matched through a fully-qualified name and an uppercase type, both of which the
// API refuses on the wire, so this also pins that the comparison happens in the
// API's own spelling rather than the operator's.
func TestApplyDnsRecordIsIdempotent(t *testing.T) {
	RegisterTestingT(t)

	f := &fakeDnsAPI{zone: testZone(
		DnsRecord{ID: "r1", Name: "api", Type: "a", Values: []string{"203.0.113.10"}, TTL: 300},
	)}
	c := newDnsTestClient(t, f)

	spec := dnsSpec("api.example.ru", "A", "203.0.113.10")
	spec.TTL = 300

	_, err := c.ApplyDnsRecord(context.Background(), applyReq(spec), nil)
	Expect(err).To(BeNil())
	Expect(f.patches).To(Equal(0), "an unchanged record must not be rewritten")
}

// TestApplyDnsRecordWritesATtlChange is a regression test for a bug that reached a
// live zone. recordMatches had stopped comparing TTLs, on a measurement that said
// the API ignored the field — so SC answered a 300→900 change with "already up to
// date" and wrote nothing at all.
func TestApplyDnsRecordWritesATtlChange(t *testing.T) {
	RegisterTestingT(t)

	f := &fakeDnsAPI{zone: testZone(
		DnsRecord{ID: "r1", Name: "api", Type: "a", Values: []string{"203.0.113.10"}, TTL: 300},
	)}
	c := newDnsTestClient(t, f)

	spec := dnsSpec("api", "a", "203.0.113.10")
	spec.TTL = 900

	_, err := c.ApplyDnsRecord(context.Background(), applyReq(spec), nil)
	Expect(err).To(BeNil())
	Expect(f.patches).To(Equal(1), "a TTL-only change is a change")
	Expect(f.zone.Records).To(HaveLen(1))
	Expect(f.zone.Records[0].TTL).To(Equal(900))
	Expect(f.zone.Records[0].ID).To(Equal("r1"), "an update amends the record rather than replacing it")
}

// TestApplyDnsRecordKeepsAnExistingTtlWhenNoneIsAskedFor: an unset TTL means "don't
// care", not "set it to zero". It has to, because the API stores a literal 0 — so a
// caller that simply did not mention a TTL would otherwise clear a deliberate one.
func TestApplyDnsRecordKeepsAnExistingTtlWhenNoneIsAskedFor(t *testing.T) {
	RegisterTestingT(t)

	f := &fakeDnsAPI{zone: testZone(
		DnsRecord{ID: "r1", Name: "api", Type: "a", Values: []string{"203.0.113.10"}, TTL: 900},
	)}
	c := newDnsTestClient(t, f)

	// Same name/type, different value, no TTL asked for.
	_, err := c.ApplyDnsRecord(context.Background(), applyReq(dnsSpec("api", "a", "203.0.113.20")), nil)
	Expect(err).To(BeNil())
	Expect(f.zone.Records).To(HaveLen(1))
	Expect(f.zone.Records[0].Values).To(ConsistOf("203.0.113.20"))
	Expect(f.zone.Records[0].TTL).To(Equal(900), "an unmentioned TTL must survive a value change")
}

// TestApplyDnsRecordFailsLoudlyWhenAWriteIsSilentlyDiscarded is the measured
// hazard: `done: true` does not mean the zone converged. A write issued while the
// previous one is settling drops records — observed twice live on 2026-10-24, once
// emptying the zone — and the API reports success either way.
//
// So the operation's verdict is not trusted: the zone is re-read, the write is
// re-issued once, and if it still has not landed SC errors instead of reporting a
// green deploy over an empty zone.
func TestApplyDnsRecordFailsLoudlyWhenAWriteIsSilentlyDiscarded(t *testing.T) {
	RegisterTestingT(t)

	f := &fakeDnsAPI{zone: testZone(), loseWrites: 99}
	c := newDnsTestClient(t, f)

	_, err := c.ApplyDnsRecord(context.Background(), applyReq(dnsSpec("api", "a", "203.0.113.10")), nil)
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("did not converge"))
	Expect(err.Error()).To(ContainSubstring("check the zone in the console"))
	Expect(f.patches).To(Equal(dnsOperationPoll.writeAttempts), "a discarded write is retried, not reported as done")
}

// TestApplyDnsRecordRecoversFromOneDiscardedWrite: the retry is there to work, not
// just to make the error honest.
func TestApplyDnsRecordRecoversFromOneDiscardedWrite(t *testing.T) {
	RegisterTestingT(t)

	f := &fakeDnsAPI{zone: testZone(), loseWrites: 1}
	c := newDnsTestClient(t, f)

	_, err := c.ApplyDnsRecord(context.Background(), applyReq(dnsSpec("api", "a", "203.0.113.10")), nil)
	Expect(err).To(BeNil())
	Expect(f.patches).To(Equal(2))
	Expect(f.zone.Records).To(HaveLen(1))
}

func TestApplyDnsRecordWritesTheApexAsAnEmptyName(t *testing.T) {
	RegisterTestingT(t)

	f := &fakeDnsAPI{zone: testZone()}
	c := newDnsTestClient(t, f)

	res, err := c.ApplyDnsRecord(context.Background(), applyReq(dnsSpec("example.ru", "a", "203.0.113.10")), nil)
	Expect(err).To(BeNil())
	Expect(res.RecordName).To(Equal(""))
	Expect(f.zone.Records).To(HaveLen(1))
	Expect(f.zone.Records[0].Name).To(Equal(""))
}

// TestApplyDnsRecordRefusesAnEmptyValueSet: a record set with no values is a
// deletion on this API. A deploy must not perform one implicitly.
func TestApplyDnsRecordRefusesAnEmptyValueSet(t *testing.T) {
	RegisterTestingT(t)

	f := &fakeDnsAPI{zone: testZone()}
	c := newDnsTestClient(t, f)

	_, err := c.ApplyDnsRecord(context.Background(), applyReq(dnsSpec("api", "a")), nil)
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("values"))
	Expect(f.patches).To(Equal(0))
}

// TestApplyDnsRecordNamesTheNameserversWhenTheZoneIsMissing — the lookup is the one
// failure an operator hits before anything else works, and the remedy (delegate the
// domain) is not inferable from "not found".
func TestApplyDnsRecordNamesTheNameserversWhenTheZoneIsMissing(t *testing.T) {
	RegisterTestingT(t)

	f := &fakeDnsAPI{} // no zone
	c := newDnsTestClient(t, f)

	_, err := c.ApplyDnsRecord(context.Background(), applyReq(dnsSpec("api", "a", "203.0.113.10")), nil)
	Expect(err).To(HaveOccurred())
	msg := err.Error()
	Expect(msg).To(ContainSubstring("was not found in project"))
	Expect(msg).To(ContainSubstring("zoneId"))
	for _, ns := range DnsNameservers {
		Expect(msg).To(ContainSubstring(ns))
	}
}

func TestDestroyDnsRecordRemovesOnlyItsOwn(t *testing.T) {
	RegisterTestingT(t)

	f := &fakeDnsAPI{zone: testZone(
		DnsRecord{ID: "r1", Name: "api", Type: "a", Values: []string{"203.0.113.10"}},
		DnsRecord{ID: "r2", Name: "api", Type: "txt", Values: []string{"keep me"}},
		DnsRecord{ID: "r3", Name: "www", Type: "a", Values: []string{"203.0.113.1"}},
	)}
	c := newDnsTestClient(t, f)

	_, err := c.DestroyDnsRecord(context.Background(), applyReq(dnsSpec("api.example.ru", "A")), nil)
	Expect(err).To(BeNil())
	Expect(f.zone.Records).To(HaveLen(2))
	for _, rec := range f.zone.Records {
		Expect(rec.Name + "/" + rec.Type).NotTo(Equal("api/a"))
	}
}

// A destroy must converge: run twice, and run against a zone that is already gone.
func TestDestroyDnsRecordIsIdempotent(t *testing.T) {
	RegisterTestingT(t)

	f := &fakeDnsAPI{zone: testZone()}
	c := newDnsTestClient(t, f)

	_, err := c.DestroyDnsRecord(context.Background(), applyReq(dnsSpec("api", "a")), nil)
	Expect(err).To(BeNil())
	Expect(f.patches).To(Equal(0), "nothing to remove must not write")

	f.zone = nil
	_, err = c.DestroyDnsRecord(context.Background(), applyReq(dnsSpec("api", "a")), nil)
	Expect(err).To(BeNil(), "a destroy against a zone that is already gone is success")
}

// TestRecordMatchesIgnoresValueOrderButNotTtl: the API returns values in its own
// order, so a reorder is not a change — but a TTL difference is, unless the desired
// TTL is unset, which means "don't care".
func TestRecordMatchesIgnoresValueOrderButNotTtl(t *testing.T) {
	RegisterTestingT(t)

	existing := &DnsRecord{Name: "api", Type: "a", Values: []string{"203.0.113.10", "203.0.113.11"}, TTL: 300}

	Expect(recordMatches(existing, DnsRecord{Values: []string{"203.0.113.11", "203.0.113.10"}, TTL: 300})).To(BeTrue())
	Expect(recordMatches(existing, DnsRecord{Values: []string{"203.0.113.11", "203.0.113.10"}})).To(BeTrue(),
		"an unset desired TTL must not force a rewrite on every deploy")
	Expect(recordMatches(existing, DnsRecord{Values: []string{"203.0.113.10", "203.0.113.11"}, TTL: 900})).To(BeFalse(),
		"a TTL change is a change — this was live-caught as a silent no-op")
	Expect(recordMatches(existing, DnsRecord{Values: []string{"203.0.113.10"}, TTL: 300})).To(BeFalse())
	Expect(recordMatches(existing, DnsRecord{Values: []string{"203.0.113.10", "203.0.113.99"}, TTL: 300})).To(BeFalse())
	Expect(recordMatches(nil, DnsRecord{Values: []string{"203.0.113.10"}})).To(BeFalse())
}

func TestDnsRecordTypeIsLowercased(t *testing.T) {
	RegisterTestingT(t)

	Expect(dnsRecordType("A")).To(Equal("a"))
	Expect(dnsRecordType(" CNAME ")).To(Equal("cname"))
	Expect(dnsRecordType("txt")).To(Equal("txt"))
}

// TestZoneDecodesTheMeasuredPayload guards the field names against a tidy-up. Two
// are easy to "correct" into silence: `createAt` is genuinely spelled without the
// `d`, and `records` is an embedded array rather than a sub-resource link.
func TestZoneDecodesTheMeasuredPayload(t *testing.T) {
	RegisterTestingT(t)

	const payload = `{
      "id": "88fc588d-4ca5-4504-a2f6-5cbbdf54f331",
      "createAt": "2026-10-24T09:00:00Z",
      "updatedAt": "2026-10-24T10:27:03Z",
      "projectId": "project-1",
      "name": "example-ru",
      "domain": "example.ru.",
      "readOnly": false,
      "state": "ok",
      "confirmState": "",
      "tags": [],
      "records": [
        {"id": "rec-1", "createAt": "2026-10-24T10:26:49Z", "name": "api", "type": "a",
         "values": ["203.0.113.10"], "ttl": 0, "readOnly": false, "wildcard": false}
      ]
    }`

	var zone DnsZone
	Expect(json.Unmarshal([]byte(payload), &zone)).To(Succeed())
	Expect(zone.CreateAt).NotTo(BeEmpty(), "`createAt` is spelled without the `d`")
	Expect(zone.Domain).To(Equal("example.ru."))
	Expect(zone.Name).To(Equal("example-ru"))
	Expect(zone.State).To(Equal("ok"))
	Expect(zone.Records).To(HaveLen(1))
	Expect(zone.Records[0].Type).To(Equal("a"))
	// This record was created without a ttl, and the zone stores a literal 0 rather
	// than substituting a default. Reading that 0 as "the API ignores ttl" is the
	// mistake that was made on 2026-10-24 and then written into the driver; a record
	// created WITH a ttl of 900 reads back as 900.
	Expect(zone.Records[0].TTL).To(Equal(0))
}
