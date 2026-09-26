// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package pulumi

import (
	"testing"

	. "github.com/onsi/gomega"

	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

// fakeRegistrarConfig is the first RegistrarConfig stand-in in the repo: DNS has only
// ever been exercised by the e2e tests against live Cloudflare.
type fakeRegistrarConfig struct {
	zones []string
}

func (f *fakeRegistrarConfig) ProviderType() string        { return "fake" }
func (f *fakeRegistrarConfig) DnsRecords() []api.DnsRecord { return nil }
func (f *fakeRegistrarConfig) Zones() []string             { return f.zones }

// fakeRegistrar records which calls reached it, so routing can be asserted on.
type fakeRegistrar struct {
	name       string
	records    []api.DnsRecord
	rules      []pApi.OverrideHeaderRule
	provisions int
}

func (f *fakeRegistrar) MainDomain() string { return f.name }

func (f *fakeRegistrar) ProvisionRecords(ctx *sdk.Context, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	f.provisions++
	return &api.ResourceOutput{Ref: f.name}, nil
}

func (f *fakeRegistrar) NewRecord(ctx *sdk.Context, dnsRecord api.DnsRecord) (*api.ResourceOutput, error) {
	f.records = append(f.records, dnsRecord)
	return &api.ResourceOutput{Ref: f.name}, nil
}

func (f *fakeRegistrar) NewOverrideHeaderRule(ctx *sdk.Context, stack api.Stack, rule pApi.OverrideHeaderRule) (*api.ResourceOutput, error) {
	f.rules = append(f.rules, rule)
	return &api.ResourceOutput{Ref: f.name}, nil
}

// registerFakeRegistrars wires a registrar type per name and returns the instances that
// will be handed out, plus a counter of how many times each was actually constructed.
func registerFakeRegistrars(t *testing.T, names ...string) (map[string]*fakeRegistrar, map[string]*int) {
	t.Helper()
	instances := map[string]*fakeRegistrar{}
	builds := map[string]*int{}
	for _, name := range names {
		name := name
		instances[name] = &fakeRegistrar{name: name}
		count := 0
		builds[name] = &count
		pApi.RegisterRegistrar("fake-"+name, func(ctx *sdk.Context, desc api.RegistrarDescriptor, params pApi.ProvisionParams) (pApi.Registrar, error) {
			count++
			return instances[name], nil
		})
	}
	return instances, builds
}

func descriptorFor(name string, zones ...string) api.RegistrarDescriptor {
	return api.RegistrarDescriptor{
		Type:   "fake-" + name,
		Config: api.Config{Config: &fakeRegistrarConfig{zones: zones}},
	}
}

func TestMultiRegistrarRoutesByZone(t *testing.T) {
	RegisterTestingT(t)

	instances, _ := registerFakeRegistrars(t, "cf", "yc")
	m := newMultiRegistrar("infra", map[string]api.RegistrarDescriptor{
		"cf": descriptorFor("cf", "simple-forge.com"),
		"yc": descriptorFor("yc", "simple-forge.ru"),
	}, nil, nil)

	for _, domain := range []string{"storage.simple-forge.com", "simple-forge.com"} {
		_, err := m.NewRecord(nil, api.DnsRecord{Name: domain, Type: "CNAME"})
		Expect(err).NotTo(HaveOccurred())
	}
	_, err := m.NewRecord(nil, api.DnsRecord{Name: "storage.simple-forge.ru", Type: "CNAME"})
	Expect(err).NotTo(HaveOccurred())

	Expect(instances["cf"].records).To(HaveLen(2))
	Expect(instances["yc"].records).To(HaveLen(1))
	Expect(instances["yc"].records[0].Name).To(Equal("storage.simple-forge.ru"))
}

func TestMultiRegistrarRoutesOverrideHeaderRuleByFromHost(t *testing.T) {
	RegisterTestingT(t)

	instances, _ := registerFakeRegistrars(t, "rulecf", "ruleyc")
	m := newMultiRegistrar("infra", map[string]api.RegistrarDescriptor{
		"cf": descriptorFor("rulecf", "simple-forge.com"),
		"yc": descriptorFor("ruleyc", "simple-forge.ru"),
	}, nil, nil)

	_, err := m.NewOverrideHeaderRule(nil, api.Stack{}, pApi.OverrideHeaderRule{FromHost: "ai.simple-forge.ru"})
	Expect(err).NotTo(HaveOccurred())
	Expect(instances["ruleyc"].rules).To(HaveLen(1))
	Expect(instances["rulecf"].rules).To(BeEmpty())
}

// The longest matching zone wins, so a delegated subzone beats its parent.
func TestMultiRegistrarLongestZoneWins(t *testing.T) {
	RegisterTestingT(t)

	instances, _ := registerFakeRegistrars(t, "parent", "child")
	m := newMultiRegistrar("infra", map[string]api.RegistrarDescriptor{
		"parent": descriptorFor("parent", "simple-forge.com"),
		"child":  descriptorFor("child", "eu.simple-forge.com"),
	}, nil, nil)

	_, err := m.NewRecord(nil, api.DnsRecord{Name: "storage.eu.simple-forge.com"})
	Expect(err).NotTo(HaveOccurred())
	Expect(instances["child"].records).To(HaveLen(1))
	Expect(instances["parent"].records).To(BeEmpty())
}

// A stack may declare a zone its services do not sit under and redirect them there via
// baseDnsZone — the fleet's own parent does exactly that — so the preference is the
// fallback when no zone matches the domain outright.
func TestMultiRegistrarFallsBackToBaseZonePreference(t *testing.T) {
	RegisterTestingT(t)

	instances, _ := registerFakeRegistrars(t, "prefcf", "prefyc")
	m := newMultiRegistrar("infra", map[string]api.RegistrarDescriptor{
		"cf": descriptorFor("prefcf", "simple-container.com"),
		"yc": descriptorFor("prefyc", "simple-forge.ru"),
	}, &pApi.DnsPreference{BaseZone: "simple-container.com"}, nil)

	_, err := m.NewRecord(nil, api.DnsRecord{Name: "storage.simple-forge.com"})
	Expect(err).NotTo(HaveOccurred())
	Expect(instances["prefcf"].records).To(HaveLen(1))
	Expect(m.MainDomain()).To(Equal("simple-container.com"))
}

func TestMultiRegistrarUnroutableDomainNamesTheCandidates(t *testing.T) {
	RegisterTestingT(t)

	registerFakeRegistrars(t, "errcf", "erryc")
	m := newMultiRegistrar("infra", map[string]api.RegistrarDescriptor{
		"cf": descriptorFor("errcf", "simple-forge.com"),
		"yc": descriptorFor("erryc", "simple-forge.ru"),
	}, nil, nil)

	_, err := m.NewRecord(nil, api.DnsRecord{Name: "storage.example.org"})
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("storage.example.org"))
	Expect(err.Error()).To(ContainSubstring("simple-forge.com"))
	Expect(err.Error()).To(ContainSubstring("simple-forge.ru"))
}

// Declaring a registrar must not force a deploy that never touches its zone to hold
// credentials for it.
func TestMultiRegistrarBuildsOnlyTheRegistrarItRoutesTo(t *testing.T) {
	RegisterTestingT(t)

	_, builds := registerFakeRegistrars(t, "lazycf", "lazyyc")
	m := newMultiRegistrar("infra", map[string]api.RegistrarDescriptor{
		"cf": descriptorFor("lazycf", "simple-forge.com"),
		"yc": descriptorFor("lazyyc", "simple-forge.ru"),
	}, nil, nil)

	Expect(*builds["lazycf"]).To(Equal(0))
	Expect(*builds["lazyyc"]).To(Equal(0))

	for i := 0; i < 3; i++ {
		_, err := m.NewRecord(nil, api.DnsRecord{Name: "storage.simple-forge.com"})
		Expect(err).NotTo(HaveOccurred())
	}
	Expect(*builds["lazycf"]).To(Equal(1), "repeated calls must reuse the memoised registrar")
	Expect(*builds["lazyyc"]).To(Equal(0), "the unrelated registrar must never be constructed")
}

// ProvisionRecords runs on the parent stack, which owns every registrar it declares.
func TestMultiRegistrarProvisionRecordsCoversEveryRegistrar(t *testing.T) {
	RegisterTestingT(t)

	instances, _ := registerFakeRegistrars(t, "provcf", "provyc")
	m := newMultiRegistrar("infra", map[string]api.RegistrarDescriptor{
		"cf": descriptorFor("provcf", "simple-forge.com"),
		"yc": descriptorFor("provyc", "simple-forge.ru"),
	}, nil, nil)

	_, err := m.ProvisionRecords(nil, pApi.ProvisionParams{})
	Expect(err).NotTo(HaveOccurred())
	Expect(instances["provcf"].provisions).To(Equal(1))
	Expect(instances["provyc"].provisions).To(Equal(1))
}

func TestDomainInZone(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		domain, zone string
		want         bool
	}{
		{"simple-forge.ru", "simple-forge.ru", true},
		{"a.simple-forge.ru", "simple-forge.ru", true},
		{"a.b.simple-forge.ru", "simple-forge.ru", true},
		{"SIMPLE-FORGE.ru", "simple-forge.RU", true},
		{"simple-forge.ru.", "simple-forge.ru", true},
		// a suffix match that is not a zone match: the label boundary matters
		{"notsimple-forge.ru", "simple-forge.ru", false},
		{"simple-forge.com", "simple-forge.ru", false},
		{"simple-forge.ru", "", false},
		{"", "simple-forge.ru", false},
	}
	for _, tt := range tests {
		Expect(domainInZone(tt.domain, tt.zone)).To(Equal(tt.want),
			"domainInZone(%q, %q)", tt.domain, tt.zone)
	}
}
