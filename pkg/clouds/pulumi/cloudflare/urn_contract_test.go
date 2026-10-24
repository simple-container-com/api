// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package cloudflare

import (
	"sort"
	"sync"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/logger"
	cfApi "github.com/simple-container-com/api/pkg/clouds/cloudflare"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

// This test locks the resource-name contract this driver produces. It exists for the
// sc-plugin-architecture back-compat invariant (see
// forge/docs/design/2026-10-24/sc-plugin-architecture/04-no-destroy-invariant.md):
// any future move of this driver into an out-of-tree plugin MUST reproduce these names
// byte-for-byte, because the Pulumi state's URN embeds them and `pulumi preview` sees
// a URN delta as a replace+create — triggering a destroy of real cloud resources.
//
// The names captured here are: the explicit cloudflare provider, the DNS record, the
// worker script, and the workers route. The capture happens via Pulumi's mock monitor,
// which records every NewResource call the driver makes; the test does not deploy
// anything to Cloudflare.

type captureMocks struct {
	mu       sync.Mutex
	captured []pulumi.MockResourceArgs
}

func (m *captureMocks) NewResource(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.captured = append(m.captured, args)
	return args.Name + "-mock-id", args.Inputs, nil
}

func (m *captureMocks) Call(args pulumi.MockCallArgs) (resource.PropertyMap, error) {
	// The driver calls LookupZone (cloudflare:index/getZone:getZone) which must return
	// a result shaped like cloudflare.LookupZoneResult — the fields the driver reads
	// are ZoneId and Name.
	if args.Token == "cloudflare:index/getZone:getZone" {
		return resource.PropertyMap{
			"zoneId": resource.NewStringProperty("zone-id-mock"),
			"name":   resource.NewStringProperty("example.com"),
		}, nil
	}
	return args.Args, nil
}

func (m *captureMocks) namesByType(typeToken string) []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []string
	for _, a := range m.captured {
		if a.TypeToken == typeToken {
			out = append(out, a.Name)
		}
	}
	sort.Strings(out)
	return out
}

func newRegistrarDescriptor(accountID, zoneName string, records []api.DnsRecord) api.RegistrarDescriptor {
	return api.RegistrarDescriptor{
		Type: cfApi.RegistrarType,
		Config: api.Config{
			Config: &cfApi.RegistrarConfig{
				AuthConfig: cfApi.AuthConfig{
					Credentials: api.Credentials{Credentials: "test-api-token"},
					AccountId:   accountID,
				},
				ZoneName: zoneName,
				Records:  records,
			},
		},
	}
}

// TestURNContract_ProviderName — the explicit Cloudflare provider resource must be
// named with the account id and nothing else. Changing this breaks every pre-existing
// stack's `provider` URN on its DNS record resources.
func TestURNContract_ProviderName(t *testing.T) {
	RegisterTestingT(t)

	m := &captureMocks{}
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		_, err := Registrar(ctx, newRegistrarDescriptor("acct-abc-123", "example.com", nil), pApi.ProvisionParams{Log: logger.New()})
		return err
	}, pulumi.WithMocks("test-project", "test-stack", m))
	Expect(err).ToNot(HaveOccurred())

	providers := m.namesByType("pulumi:providers:cloudflare")
	Expect(providers).To(Equal([]string{"acct-abc-123"}),
		"the Cloudflare provider resource must be named <AccountId> — see registrar.go `cfImpl.NewProvider(ctx, cfg.AccountId, ...)`. "+
			"Changing this invalidates every pre-existing state file.")
}

// TestURNContract_DnsRecordName — a single DnsRecord lands under the name
// "<record.Name>-record". Duplicate record names (two with the same Name but different
// Value) each get an md5(value) suffix to disambiguate.
func TestURNContract_DnsRecordName(t *testing.T) {
	RegisterTestingT(t)

	records := []api.DnsRecord{
		{Name: "@", Type: "A", Value: "192.0.2.1"},
		{Name: "www", Type: "CNAME", Value: "example.com"},
	}
	m := &captureMocks{}
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		r, err := Registrar(ctx, newRegistrarDescriptor("acct-x", "example.com", records), pApi.ProvisionParams{Log: logger.New()})
		if err != nil {
			return err
		}
		_, err = r.ProvisionRecords(ctx, pApi.ProvisionParams{Log: logger.New()})
		return err
	}, pulumi.WithMocks("test-project", "test-stack", m))
	Expect(err).ToNot(HaveOccurred())

	names := m.namesByType("cloudflare:index/record:Record")
	Expect(names).To(Equal([]string{"@-record", "www-record"}),
		"DnsRecord resource names must be `<Name>-record` — see registrar.go `recordName := fmt.Sprintf(\"%s%s-record\", record.Name, suffix)`. "+
			"Changing this invalidates every pre-existing state file.")
}

// TestURNContract_DnsRecordName_DuplicateNames — two records with the same Name but
// different Value must disambiguate via an md5(Value) suffix. Documented in registrar.go
// at the `lo.CountBy ... md5.Sum([]byte(record.Value))` block.
func TestURNContract_DnsRecordName_DuplicateNames(t *testing.T) {
	RegisterTestingT(t)

	// Two A records at the same name — a common pattern for round-robin or multi-origin.
	records := []api.DnsRecord{
		{Name: "api", Type: "A", Value: "192.0.2.1"},
		{Name: "api", Type: "A", Value: "192.0.2.2"},
	}
	m := &captureMocks{}
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		r, err := Registrar(ctx, newRegistrarDescriptor("acct-x", "example.com", records), pApi.ProvisionParams{Log: logger.New()})
		if err != nil {
			return err
		}
		_, err = r.ProvisionRecords(ctx, pApi.ProvisionParams{Log: logger.New()})
		return err
	}, pulumi.WithMocks("test-project", "test-stack", m))
	Expect(err).ToNot(HaveOccurred())

	// md5("192.0.2.1") = d0f88d6c87767262ba8e93d6acccd784
	// md5("192.0.2.2") = 7f83fddecaba901abfd469d899958433
	names := m.namesByType("cloudflare:index/record:Record")
	Expect(names).To(Equal([]string{
		"api7f83fddecaba901abfd469d899958433-record",
		"apid0f88d6c87767262ba8e93d6acccd784-record",
	}),
		"duplicate DnsRecord names must disambiguate via hex(md5(Value)) suffix — see registrar.go. "+
			"Changing this invalidates every pre-existing state file that carries duplicate record names.")
}

// TestURNContract_WorkerScriptAndRouteName — a worker script lands under
// "<name>-worker-script" and its matching route under "<name>-worker-script-route".
func TestURNContract_WorkerScriptAndRouteName(t *testing.T) {
	RegisterTestingT(t)

	m := &captureMocks{}
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		r, err := Registrar(ctx, newRegistrarDescriptor("acct-x", "example.com", nil), pApi.ProvisionParams{Log: logger.New()})
		if err != nil {
			return err
		}
		// NewWorkerScript is on the concrete *provisioner, not on the pApi.Registrar
		// interface — same-package cast is safe here (and documents the contract).
		prov := r.(*provisioner)
		_, err = prov.NewWorkerScript(ctx, "my-worker", "api.example.com", "// script")
		return err
	}, pulumi.WithMocks("test-project", "test-stack", m))
	Expect(err).ToNot(HaveOccurred())

	Expect(m.namesByType("cloudflare:index/workerScript:WorkerScript")).To(Equal([]string{"my-worker-worker-script"}),
		"WorkerScript resource name must be `<workerName>-worker-script`. Changing this invalidates existing state.")
	Expect(m.namesByType("cloudflare:index/workersRoute:WorkersRoute")).To(Equal([]string{"my-worker-worker-script-route"}),
		"WorkersRoute resource name must be `<workerName>-worker-script-route`. Changing this invalidates existing state.")
}

// TestURNContract_OverrideHeaderRuleName — a NewOverrideHeaderRule produces three
// resources: a DNS record `<domain>-record`, a worker script
// `<stack><name>-host-override-script`, and a workers route
// `<stack><name>-host-override-route`.
func TestURNContract_OverrideHeaderRuleName(t *testing.T) {
	RegisterTestingT(t)

	m := &captureMocks{}
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		r, err := Registrar(ctx, newRegistrarDescriptor("acct-x", "example.com", nil), pApi.ProvisionParams{Log: logger.New()})
		if err != nil {
			return err
		}
		_, err = r.NewOverrideHeaderRule(ctx, api.Stack{Name: "mystack"}, pApi.OverrideHeaderRule{
			Name:     "myservice",
			FromHost: "app.example.com",
			ToHost:   pulumi.String("origin.example.net").ToStringOutput(),
		})
		return err
	}, pulumi.WithMocks("test-project", "test-stack", m))
	Expect(err).ToNot(HaveOccurred())

	// Expected names, see registrar.go NewOverrideHeaderRule:
	//   ruleName   = "<stack><name>-host-override"       e.g. "mystackmyservice-host-override"
	//   script     = ruleName + "-script"
	//   route      = ruleName + "-route"
	Expect(m.namesByType("cloudflare:index/workerScript:WorkerScript")).To(Equal([]string{
		"mystackmyservice-host-override-script",
	}))
	Expect(m.namesByType("cloudflare:index/workersRoute:WorkersRoute")).To(Equal([]string{
		"mystackmyservice-host-override-route",
	}))
}
