// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package evolution

import (
	"strings"
	"testing"

	. "github.com/onsi/gomega"
)

// The Cloud DNS routes in dns.go are a transcription of a published reference and
// have never been measured (see the package note there), so an HTTP-level test
// here would only assert that our own fake agrees with our own guess. These tests
// cover the two things that are true regardless of what the wire turns out to look
// like: the order of the two zone gates, and the record-name construction that is
// the first named candidate to be wrong on the live smoke.

func boolPtr(b bool) *bool { return &b }

func TestCheckZoneVerifiedPassesVerifiedZone(t *testing.T) {
	RegisterTestingT(t)

	Expect(checkZoneVerified(&DnsZone{Name: "example.ru", ID: "z1", Valid: boolPtr(true)})).To(BeNil())

	// Verified-but-undelegated must still pass: a record written there is created
	// and simply does not resolve yet, which is a warning on the way out and not a
	// reason to refuse the write.
	Expect(checkZoneVerified(&DnsZone{
		Name: "example.ru", ID: "z1", Valid: boolPtr(true), Delegated: boolPtr(false),
	})).To(BeNil())
}

// TestCheckZoneVerifiedPassesAZoneTheApiSaysNothingAboutGuards the bug this gate
// shipped with: `Valid` was a plain bool, so a response that simply does not carry
// `valid` decoded to false and the gate refused EVERY write, naming a TXT record
// that does not exist. A real Evolution zone went straight to Активен with no
// verification value anywhere on its page, so "absent" is the expected case, not
// the exotic one — a gate added to turn a confusing failure into a clear message
// would have manufactured a hard block instead.
func TestCheckZoneVerifiedPassesAZoneTheApiSaysNothingAbout(t *testing.T) {
	RegisterTestingT(t)

	Expect(checkZoneVerified(&DnsZone{Name: "atriumdev.online", ID: "z9"})).To(BeNil())
	Expect(checkZoneVerified(&DnsZone{Name: "atriumdev.online", ID: "z9", Valid: nil})).To(BeNil())
}

func TestCheckZoneVerifiedRefusesUnverifiedZoneAndNamesTheRemedy(t *testing.T) {
	RegisterTestingT(t)

	err := checkZoneVerified(&DnsZone{
		Name:           "atriumdev.online",
		ID:             "z9",
		Valid:          boolPtr(false),
		ValidationText: "sbc-verification: 4a420000-4700-000d-a0f0-8b71933f0000",
	})
	Expect(err).NotTo(BeNil())

	msg := err.Error()
	// The value to publish, verbatim — it exists nowhere else SC can reach.
	Expect(msg).To(ContainSubstring("sbc-verification: 4a420000-4700-000d-a0f0-8b71933f0000"))
	// That a retry will not help, so the operator stops waiting.
	Expect(msg).To(ContainSubstring("not something a retry fixes"))
	// And the ordering, which is the part that causes an outage when inverted.
	Expect(msg).To(ContainSubstring("BEFORE"))
	for _, ns := range DnsNameservers {
		Expect(msg).To(ContainSubstring(ns))
	}
}

func TestCheckZoneVerifiedWithoutValidationTextPointsAtTheConsole(t *testing.T) {
	RegisterTestingT(t)

	// A list answer may omit `validationText` while still saying valid=false; the
	// refusal must stay actionable.
	err := checkZoneVerified(&DnsZone{Name: "example.ru", ID: "z1", Valid: boolPtr(false)})
	Expect(err).NotTo(BeNil())
	Expect(err.Error()).To(ContainSubstring("the zone page in the console shows the value to publish"))
}

// TestDnsNameserversAreTheEvolutionPair pins the correction of 2026-10-24, and the
// test it replaces is the reason it is worth pinning. That one asserted the hosts
// were `cdns-ns01/cdns-ns02.sbercloud.ru` and explained, confidently, that the
// sbercloud.ru domain "is not a mistake" because both of Cloud.ru's own Cloud DNS
// pages name that pair. The pages do — they are just describing the Advanced
// estate without saying so. A real Evolution zone publishes `evo-cns01.cloud.ru`
// and `evo-cns02.cloud.ru`, which resolve into a different network entirely
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

// TestZoneNameserversPrefersTheZonesOwnList is the structural half of the same
// lesson: a constant in SC went stale, a value the zone states about itself
// cannot.
func TestZoneNameserversPrefersTheZonesOwnList(t *testing.T) {
	RegisterTestingT(t)

	Expect(ZoneNameservers(nil)).To(Equal(DnsNameservers))
	Expect(ZoneNameservers(&DnsZone{})).To(Equal(DnsNameservers))
	Expect(ZoneNameservers(&DnsZone{Nameservers: []string{}})).To(Equal(DnsNameservers))

	Expect(ZoneNameservers(&DnsZone{
		Nameservers: []string{"evo-cns01.cloud.ru.", "evo-cns02.cloud.ru."},
	})).To(Equal([]string{"evo-cns01.cloud.ru.", "evo-cns02.cloud.ru."}))
}

func TestDnsRecordNameAppendsTheZoneOnlyWhenAbsent(t *testing.T) {
	RegisterTestingT(t)

	for _, tc := range []struct {
		name, zone, want string
	}{
		{"app", "example.ru", "app.example.ru"},
		{"app.example.ru", "example.ru", "app.example.ru"},          // already absolute
		{"app.example.ru.", "example.ru", "app.example.ru"},         // trailing dot dropped
		{"example.ru", "example.ru", "example.ru"},                  // apex
		{"APP.Example.RU", "example.ru", "APP.Example.RU"},          // suffix match is case-insensitive
		{"app.example.ru", "example.ru.", "app.example.ru"},         // zone given with a dot
		{"notexample.ru", "example.ru", "notexample.ru.example.ru"}, // suffix, not substring
		{"", "example.ru", ""},
		{"app", "", "app"},
	} {
		Expect(dnsRecordName(tc.name, tc.zone)).
			To(Equal(tc.want), "name=%q zone=%q", tc.name, tc.zone)
	}
}

func TestDnsRecordKeyEscapesBothSegments(t *testing.T) {
	RegisterTestingT(t)

	Expect(dnsRecordKey("app.example.ru", "a")).To(Equal("app.example.ru/A"))
	Expect(dnsRecordKey("app.example.ru", "txt")).To(Equal("app.example.ru/TXT"))
	// A wildcard is legal in a record name and must not break the path.
	Expect(strings.Contains(dnsRecordKey("*.example.ru", "A"), "/")).To(BeTrue())
}
