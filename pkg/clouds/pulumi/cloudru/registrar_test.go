// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package cloudru

import (
	"testing"

	. "github.com/onsi/gomega"

	"github.com/pulumi/pulumi-command/sdk/go/command/local"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/cloudru"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

// TestRegistrarIsRegistered guards the wiring. A registrar absent from
// RegistrarFuncByType is not a compile error — a server.yaml naming it would be
// provisioned with the not-configured registrar, which silently writes no DNS at all.
func TestRegistrarIsRegistered(t *testing.T) {
	RegisterTestingT(t)

	fn, found := pApi.RegistrarFuncByType[cloudru.RegistrarTypeCloudruDns]
	Expect(found).To(BeTrue(), "%q must be registered in pkg/clouds/pulumi/cloudru/init.go", cloudru.RegistrarTypeCloudruDns)
	Expect(fn).ToNot(BeNil())
}

// TestRegistrarImplementsInterface pins the full pApi.Registrar surface. Two of the
// four methods are deliberate refusals, and a refusal is only reachable if the type
// still satisfies the interface.
func TestRegistrarImplementsInterface(t *testing.T) {
	RegisterTestingT(t)

	var r any = &registrar{}
	_, ok := r.(pApi.Registrar)
	Expect(ok).To(BeTrue(), "*registrar must implement pApi.Registrar")
}

// TestDnsRecordResourceName covers the collision the type suffix exists to prevent: a
// name legitimately carries several record types at once (a domain and its SPF), and
// two Pulumi resources sharing a name would present as the second record replacing
// the first.
func TestDnsRecordResourceName(t *testing.T) {
	RegisterTestingT(t)

	Expect(dnsRecordResourceName("api.simple-forge.ru", "A")).To(Equal("api.simple-forge.ru-a-dns-record"))
	Expect(dnsRecordResourceName("api.simple-forge.ru.", "A")).To(Equal("api.simple-forge.ru-a-dns-record"),
		"a trailing dot must not make a second resource for the same record")
	Expect(dnsRecordResourceName("simple-forge.ru", "TXT")).ToNot(Equal(dnsRecordResourceName("simple-forge.ru", "A")))
}

// TestRecordOptionsOmitsANilProvider is the regression test for the panic that a real
// `sc provision` found on 2026-10-24: initRegistrar builds its ProvisionParams with Log
// and DnsPreference only, so params.Provider is nil for every registrar, and
// sdk.Provider(nil) does not ignore it — it panics the inline program with
// `invalid memory address or nil pointer dereference` during preview, before any record
// is written. Pulumi reports it as an unattributed "go inline source runtime error"
// naming neither this package nor the registrar.
//
// The baseline is one option, not none: delete-before-replace is unconditional, because
// these records are keyed name+type on the server and Pulumi's default
// create-then-delete replacement has the delete remove what the create just wrote.
func TestRecordOptionsOmitsANilProvider(t *testing.T) {
	RegisterTestingT(t)

	r := &registrar{}
	Expect(r.recordOptions()).To(HaveLen(1), "a nil provider must produce no provider option at all")

	// And the serialisation chain still has to be built, since losing it loses records
	// rather than crashing — the quieter of the two failures.
	r.lastRecord = &local.Command{}
	Expect(r.recordOptions()).To(HaveLen(2), "the previous record must still be depended on")
}

func TestGuardZoneInfrastructureRecord(t *testing.T) {
	RegisterTestingT(t)

	t.Run("refuses the zone's own delegation records", func(t *testing.T) {
		for _, recordType := range []string{"NS", "ns", "SOA"} {
			err := guardZoneInfrastructureRecord(api.DnsRecord{Name: "simple-forge.ru", Type: recordType})
			Expect(err).To(MatchError(ContainSubstring("belong to the zone itself")), "type %q", recordType)
		}
	})

	t.Run("refuses the ACME challenge record", func(t *testing.T) {
		err := guardZoneInfrastructureRecord(api.DnsRecord{Name: "_acme-challenge.simple-forge.ru", Type: "CNAME"})
		Expect(err).To(MatchError(ContainSubstring("breaks renewal")))
	})

	t.Run("allows an ordinary record", func(t *testing.T) {
		Expect(guardZoneInfrastructureRecord(api.DnsRecord{Name: "api.simple-forge.ru", Type: "A"})).To(Succeed())
		Expect(guardZoneInfrastructureRecord(api.DnsRecord{Name: "simple-forge.ru", Type: "TXT"})).To(Succeed())
	})
}
