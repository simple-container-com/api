// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package vm

import (
	"fmt"
	"sync"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/pkg/errors"

	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/logger"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

// Pointing one name at N machines is one A record per machine, and the thing that can
// go wrong is invisible: the registrar plugin — not SC — decides what resource a record
// becomes, and three of the four derive that from the record's name and type WITHOUT
// its value. On those, record 2 replaces record 1, the deploy is green, the name
// resolves, and two thirds of the fleet never receive a request. These tests are about
// catching that from SC's side, which is possible because NewRecord hands back the
// resource's reference.

// errRegistrarRefused is a registrar failure that is NOT "not configured" — the kind
// that must fail the deploy.
var errRegistrarRefused = errors.New("zone not found")

// fakeRegistrar records what it was asked to create and hands back whatever reference
// the test tells it to — the two things that matter here.
type fakeRegistrar struct {
	mu      sync.Mutex
	records []api.DnsRecord
	// ref decides what the i-th call returns. A registrar that collapses same-name
	// records is modelled by returning one reference for every call.
	ref func(i int) sdk.Output
	// err, when set, is what every call fails with.
	err error
}

func (f *fakeRegistrar) MainDomain() string { return "example.com" }

func (f *fakeRegistrar) NewRecord(_ *sdk.Context, rec api.DnsRecord) (*api.ResourceOutput, error) {
	f.mu.Lock()
	i := len(f.records)
	f.records = append(f.records, rec)
	f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	return &api.ResourceOutput{Ref: f.ref(i)}, nil
}

func (f *fakeRegistrar) ProvisionRecords(*sdk.Context, pApi.ProvisionParams) (*api.ResourceOutput, error) {
	return nil, nil
}

func (f *fakeRegistrar) NewOverrideHeaderRule(*sdk.Context, api.Stack, pApi.OverrideHeaderRule) (*api.ResourceOutput, error) {
	return nil, nil
}

func (f *fakeRegistrar) ProvisionDomainForEndpoint(*sdk.Context, api.Stack, pApi.DomainEndpoint) (*api.ResourceOutput, error) {
	return nil, nil
}

// distinctRefs is a correct registrar: every record is its own resource.
func distinctRefs(i int) sdk.Output {
	return sdk.String(fmt.Sprintf("urn:pulumi:rec-%d", i)).ToStringOutput()
}

// collapsedRefs is the bug, modelled: every record is the SAME resource, which is what
// a plugin deriving its resource name from name+type alone produces.
func collapsedRefs(int) sdk.Output {
	return sdk.String("urn:pulumi:example-com-A").ToStringOutput()
}

// runFleetDns calls fleetDnsRecords inside a Pulumi run, which it needs because the
// guard awaits the references it is handed.
func runFleetDns(reg pApi.Registrar, addresses []string) error {
	return sdk.RunErr(func(ctx *sdk.Context) error {
		params := pApi.ProvisionParams{Log: logger.New()}
		if reg != nil {
			params.Registrar = reg
		}
		return fleetDnsRecords(ctx, params, testComposeStack, testDomain, addresses, false)
	}, sdk.WithMocks("project", testComposeStack, newComposeMocks()))
}

func TestFleetDns_OneRecordPerMachine(t *testing.T) {
	RegisterTestingT(t)

	reg := &fakeRegistrar{ref: distinctRefs}
	Expect(runFleetDns(reg, []string{"10.0.0.1", "10.0.0.2", "10.0.0.3"})).To(Succeed())

	Expect(reg.records).To(HaveLen(3), "one A record per machine of the fleet")
	for i, rec := range reg.records {
		Expect(rec.Name).To(Equal(testDomain), "record %d", i)
		Expect(rec.Type).To(Equal("A"), "record %d", i)
	}
	// Every machine, and each exactly once: a loop that read one index N times would
	// create three records that all passed the collision guard and all pointed at the
	// same host.
	Expect([]string{reg.records[0].Value, reg.records[1].Value, reg.records[2].Value}).
		To(ConsistOf("10.0.0.1", "10.0.0.2", "10.0.0.3"))
}

// THE assertion of this file. SC does not know which registrar plugins hash the value
// into their resource name, and does not need to: if two calls hand back the same
// resource, the registrar collapsed them. Refused rather than warned about, because the
// deploy succeeds either way and a warning is the one thing nobody would notice.
func TestFleetDns_CollapsedRegistrarIsRefused(t *testing.T) {
	RegisterTestingT(t)

	reg := &fakeRegistrar{ref: collapsedRefs}
	err := runFleetDns(reg, []string{"10.0.0.1", "10.0.0.2", "10.0.0.3"})

	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("the same resource"))
	// Both hosts, the name, and the fleet's real size — enough to tell the two
	// failures apart without reading the code: a collapsed registrar versus a fleet
	// that genuinely has one machine.
	Expect(err.Error()).To(ContainSubstring("hosts 0 and 1"))
	Expect(err.Error()).To(ContainSubstring(testDomain))
	Expect(err.Error()).To(ContainSubstring("one machine instead of 3"))
	// Named as the plugin's bug, because the operator's first instinct on a DNS
	// failure is to go looking at their own `domain:` setting.
	Expect(err.Error()).To(ContainSubstring("bug in that plugin"))
}

// A fleet of one is the overwhelmingly common case and must behave exactly as it did
// before any of this existed — including on a registrar that collapses, where there is
// nothing to collapse.
func TestFleetDns_SingleHostIsUnaffected(t *testing.T) {
	RegisterTestingT(t)

	reg := &fakeRegistrar{ref: collapsedRefs}
	Expect(runFleetDns(reg, []string{"10.0.0.1"})).To(Succeed())
	Expect(reg.records).To(HaveLen(1))
	Expect(reg.records[0].Value).To(Equal("10.0.0.1"))
}

// A parent stack with no registrar is a supported arrangement — the operator points the
// name at the fleet by hand, which is what the docs for every cloud without a registrar
// already say. So this warns and continues; a refusal here would break every vm stack
// that has ever set `domain:`.
func TestFleetDns_NoRegistrarIsNotAFailure(t *testing.T) {
	RegisterTestingT(t)

	// Nil, which is what a test fixture and some callers have.
	Expect(runFleetDns(nil, []string{"10.0.0.1", "10.0.0.2"})).To(Succeed())

	// And the way it actually arrives in production: the empty registrar type is
	// registered to NotConfiguredRegistrar, so params.Registrar is NON-nil and every
	// method fails with the sentinel. A `== nil` check alone would turn this into a
	// hard deploy failure for every stack whose parent declares no registrar.
	reg := &fakeRegistrar{ref: distinctRefs, err: pApi.NotConfiguredRegistrarError}
	Expect(runFleetDns(reg, []string{"10.0.0.1", "10.0.0.2"})).To(Succeed())
}

// A registrar whose call genuinely fails must fail the deploy — the point of the
// not-configured branch above is that it is the ONE error that does not.
func TestFleetDns_RealRegistrarErrorFails(t *testing.T) {
	RegisterTestingT(t)

	reg := &fakeRegistrar{ref: distinctRefs, err: errRegistrarRefused}
	err := runFleetDns(reg, []string{"10.0.0.1", "10.0.0.2"})

	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("host 0 of the fleet"))
	Expect(err.Error()).To(ContainSubstring("zone not found"))
}

// During a preview the records do not exist yet, so their references are unknown. The
// guard has nothing to compare and must not invent a verdict either way — comparing
// unknowns would make every multi-host preview fail with a collision that is not there.
func TestFleetDns_UnknownRefsAreSkipped(t *testing.T) {
	RegisterTestingT(t)

	unknown := func(int) sdk.Output {
		return sdk.UnsafeUnknownOutput(nil).ApplyT(func(any) string { return "" })
	}
	reg := &fakeRegistrar{ref: unknown}
	Expect(runFleetDns(reg, []string{"10.0.0.1", "10.0.0.2", "10.0.0.3"})).To(Succeed())
	Expect(reg.records).To(HaveLen(3), "the records are still created; only the guard stands down")
}

// A registrar whose Ref is not a string Output at all. Nothing to compare, so nothing
// is claimed: the records were created and the deploy proceeds.
func TestFleetDns_NonStringRefsAreSkipped(t *testing.T) {
	RegisterTestingT(t)

	reg := &fakeRegistrar{ref: func(int) sdk.Output { return sdk.Int(7).ToIntOutput() }}
	Expect(runFleetDns(reg, []string{"10.0.0.1", "10.0.0.2"})).To(Succeed())
	Expect(reg.records).To(HaveLen(2))
}

// The wiring, asserted end to end through VmComposeStack rather than on
// fleetDnsRecords directly — because the bug this catches is not in the function, it is
// in what the function is handed. The addresses have to come from the parent stack's
// per-host exports, and `withHosts` gives each machine a distinct address precisely so
// that a caller passing host 0's address N times cannot pass.
func TestVmCompose_FleetGetsOneARecordPerMachine(t *testing.T) {
	RegisterTestingT(t)

	reg := &fakeRegistrar{ref: distinctRefs}
	f := newComposeFixture().withIngress().withHosts(3)
	f.registrar = reg

	_, err := f.run()
	Expect(err).ToNot(HaveOccurred())

	Expect(reg.records).To(HaveLen(3))
	values := make([]string, 0, len(reg.records))
	for _, rec := range reg.records {
		Expect(rec.Name).To(Equal(testDomain))
		values = append(values, rec.Value)
	}
	// Machine 0 keeps the single-host fixture's address; 1 and 2 are the fleet's.
	Expect(values).To(ConsistOf(testHostAddress, fleetHostAddress(1), fleetHostAddress(2)))
}

// And the other half of the wiring: no record when Caddy is not in front. An A record
// at `domain` would then resolve to a machine with nothing listening on 443 — the stack
// answers on a host port instead, which the existing warnings already say.
func TestVmCompose_NoIngressMeansNoDnsRecord(t *testing.T) {
	RegisterTestingT(t)

	reg := &fakeRegistrar{ref: distinctRefs}
	// withHosts but NOT withIngress: the stack sets no domain and the machine runs no
	// Caddy.
	f := newComposeFixture().withHosts(2)
	f.registrar = reg

	_, err := f.run()
	Expect(err).ToNot(HaveOccurred())
	Expect(reg.records).To(BeEmpty())
}

// Nothing to point anywhere. Both guards exist because the caller reads the addresses
// out of the parent stack, which answers with nothing at all when the parent predates
// the export.
func TestFleetDns_NothingToDo(t *testing.T) {
	RegisterTestingT(t)

	reg := &fakeRegistrar{ref: distinctRefs}
	Expect(sdk.RunErr(func(ctx *sdk.Context) error {
		params := pApi.ProvisionParams{Log: logger.New(), Registrar: reg}
		if err := fleetDnsRecords(ctx, params, testComposeStack, "", []string{"10.0.0.1"}, false); err != nil {
			return err
		}
		return fleetDnsRecords(ctx, params, testComposeStack, testDomain, nil, false)
	}, sdk.WithMocks("project", testComposeStack, newComposeMocks()))).To(Succeed())
	Expect(reg.records).To(BeEmpty())
}
