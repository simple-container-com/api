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

func TestCheckZoneVerifiedPassesVerifiedZone(t *testing.T) {
	RegisterTestingT(t)

	Expect(checkZoneVerified(&DnsZone{Name: "example.ru", ID: "z1", Valid: true})).To(BeNil())

	// Verified-but-undelegated must still pass: a record written there is created
	// and simply does not resolve yet, which is a warning on the way out and not a
	// reason to refuse the write.
	Expect(checkZoneVerified(&DnsZone{Name: "example.ru", ID: "z1", Valid: true, Delegated: false})).To(BeNil())
}

func TestCheckZoneVerifiedRefusesUnverifiedZoneAndNamesTheRemedy(t *testing.T) {
	RegisterTestingT(t)

	err := checkZoneVerified(&DnsZone{
		Name:           "atriumdev.online",
		ID:             "z9",
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

	// A list answer may omit `validationText`; the refusal must still be actionable.
	err := checkZoneVerified(&DnsZone{Name: "example.ru", ID: "z1"})
	Expect(err).NotTo(BeNil())
	Expect(err.Error()).To(ContainSubstring("the zone page in the console shows the value to publish"))
}

// TestDnsNameserversAreTheSbercloudPair locks the NS set against a plausible
// "fix": the hosts are under sbercloud.ru even for an Evolution zone, and every
// other endpoint in this package is a *.cloud.ru host, so the inconsistency reads
// like a mistake. It is not — verified live 2026-10-24.
func TestDnsNameserversAreTheSbercloudPair(t *testing.T) {
	RegisterTestingT(t)

	Expect(DnsNameservers).To(ConsistOf("cdns-ns01.sbercloud.ru", "cdns-ns02.sbercloud.ru"))
	for _, ns := range DnsNameservers {
		Expect(ns).To(HaveSuffix(".sbercloud.ru"))
	}
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
