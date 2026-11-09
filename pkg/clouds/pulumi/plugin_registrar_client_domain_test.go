// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package pulumi

import (
	"context"
	"encoding/json"
	"testing"

	"google.golang.org/grpc"

	. "github.com/onsi/gomega"

	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/logger"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	scPlugin "github.com/simple-container-com/api/pkg/plugin"
	scpluginv1 "github.com/simple-container-com/api/pkg/plugin/proto/v1"
)

const (
	testPluginName        = "sc-plugin-yandex"
	testPluginRegistrar   = "test-yc-dns"
	testPluginComputeType = "test-yc-serverless-container"
)

// zoneRegistrarConfig is a registrar config whose zone survives the bus's JSON
// round-trip, so the stub can tell which zone it was handed.
type zoneRegistrarConfig struct {
	ZoneName string `json:"zoneName"`
}

func (z *zoneRegistrarConfig) ProviderType() string        { return testPluginRegistrar }
func (z *zoneRegistrarConfig) DnsRecords() []api.DnsRecord { return nil }
func (z *zoneRegistrarConfig) Zones() []string             { return []string{z.ZoneName} }

// recordingPluginClient logs every ProvisionRegistrar / Provision in arrival order.
type recordingPluginClient struct {
	scpluginv1.SCCloudPluginClient
	calls []string
}

func (r *recordingPluginClient) ProvisionRegistrar(
	_ context.Context, req *scpluginv1.ProvisionRegistrarRequest, _ ...grpc.CallOption,
) (*scpluginv1.ProvisionRegistrarResponse, error) {
	zone := req.GetConfig()["zoneName"].GetStringValue()
	r.calls = append(r.calls, "ProvisionRegistrar:"+req.GetRegistrarType()+":"+zone)
	return &scpluginv1.ProvisionRegistrarResponse{MainDomain: zone}, nil
}

func (r *recordingPluginClient) Provision(
	_ context.Context, req *scpluginv1.ProvisionRequest, _ ...grpc.CallOption,
) (*scpluginv1.ProvisionResponse, error) {
	r.calls = append(r.calls, "Provision:"+req.GetResourceType())
	return &scpluginv1.ProvisionResponse{Urn: "urn:test"}, nil
}

// clientStackInput mirrors the shape core converts a client stack into for a
// compute template: the template's config plus the client's whole stackConfig.
type clientStackInput struct {
	StackConfig api.StackConfigSingleImage `json:"stackConfig"`
}

// A client stack's `domain:` must reach a plugin template together with the
// registrar for its zone. The parent declares three registrars; core builds them
// lazily and never writes in simple-forge.ru itself, so before this fix the
// yandex plugin got Provision without ever getting ProvisionRegistrar for that
// zone, and refused (formerly: panicked on) the domain.
func TestResourceSidecar_ProvisionsClientDomainRegistrarToPlugin(t *testing.T) {
	for _, tt := range []struct {
		name   string
		domain string
		want   []string
	}{
		{
			name:   "domain in a zone the plugin's registrar serves",
			domain: "x.simple-forge.ru",
			want: []string{
				"ProvisionRegistrar:" + testPluginRegistrar + ":simple-forge.ru",
				"Provision:" + testPluginComputeType,
			},
		},
		{
			// Out of scope: Cloudflare in front of a YC container. Core must not
			// build an unrelated registrar; the plugin reports the missing one.
			name:   "domain served by another registrar",
			domain: "x.simple-forge.com",
			want:   []string{"Provision:" + testPluginComputeType},
		},
		{
			name:   "no domain",
			domain: "",
			want:   []string{"Provision:" + testPluginComputeType},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			withActiveSession(t)

			_, builds := registerFakeRegistrars(t, "cloudflare")
			stub := &recordingPluginClient{}
			cli := scPlugin.NewClientWithStub(stub)
			bundle := &preEnginePluginBundle{schemas: map[string]*scpluginv1.SchemaResponse{
				testPluginName: {
					RegistrarTypes: []string{testPluginRegistrar},
					ResourceTypes:  []string{testPluginComputeType},
				},
			}}
			pApi.RegisterRegistrar(testPluginRegistrar, bundle.makeRegistrarSidecar(cli, testPluginName, testPluginRegistrar))

			cf := descriptorFor("cloudflare", "simple-forge.com")
			cf.Default = true
			multi := newMultiRegistrar("infra", map[string]api.RegistrarDescriptor{
				"cloudflare": cf,
				"yandex": {
					Type:   testPluginRegistrar,
					Config: api.Config{Config: &zoneRegistrarConfig{ZoneName: "atriumdev.ru"}},
				},
				"yandex-simple-forge-ru": {
					Type:   testPluginRegistrar,
					Config: api.Config{Config: &zoneRegistrarConfig{ZoneName: "simple-forge.ru"}},
				},
			}, nil, logger.New())

			provision := bundle.makeResourceSidecar(cli, testPluginName, testPluginComputeType)
			err := sdk.RunErr(func(ctx *sdk.Context) error {
				_, err := provision(ctx, api.Stack{Name: "twopeaks"}, api.ResourceInput{
					Descriptor: &api.ResourceDescriptor{
						Type:   testPluginComputeType,
						Name:   "twopeaks",
						Config: api.Config{Config: &clientStackInput{StackConfig: api.StackConfigSingleImage{Domain: tt.domain}}},
					},
					StackParams: &api.StackParams{StackName: "twopeaks", Environment: "staging"},
				}, pApi.ProvisionParams{Log: logger.New(), Registrar: multi})
				return err
			}, sdk.WithMocks("project", "stack", &refMocks{}))
			Expect(err).To(BeNil())

			Expect(stub.calls).To(Equal(tt.want))
			Expect(*builds["cloudflare"]).To(Equal(0))
		})
	}
}

// staticStackInput is the static-website counterpart of clientStackInput. The
// difference that matters is not the template — it is that StackConfigStatic
// keeps `domain:` inside a NAMED field tagged `json:",inline"`, which
// encoding/json does not inline, so the domain marshals NESTED under `Site`
// while a single-image config carries it flat.
type staticStackInput struct {
	StackConfig api.StackConfigStatic `json:"stackConfig"`
}

// stackConfigDomain must find the domain in both config shapes.
//
// The static arm is the one that broke forge-landing-ru-apex--staging on
// 2026-11-09 (landing run 42609670242): the probe saw "", so
// provisionDomainRegistrar took its `domain == ""` early return, the plugin was
// never sent ProvisionRegistrar, and sc-plugin-yandex refused the deploy with
// `no yc-dns registrar initialised that serves "simple-forge.ru"` — against a
// parent that declared exactly that registrar and had provisioned green. The
// bug was core not being able to see the domain, not the plugin's lifecycle.
func TestStackConfigDomain_FindsTheDomainInBothShapes(t *testing.T) {
	for _, tt := range []struct {
		name string
		cfg  any
		want string
	}{
		{
			// The shape the existing sidecar test already covered.
			name: "flat domain (single-image, compose)",
			cfg:  &clientStackInput{StackConfig: api.StackConfigSingleImage{Domain: "x.simple-forge.ru"}},
			want: "x.simple-forge.ru",
		},
		{
			name: "domain nested under the inline-tagged Site field (static website)",
			cfg:  &staticStackInput{StackConfig: api.StackConfigStatic{BundleDir: "dist", Site: api.StaticSiteConfig{Domain: "simple-forge.ru"}}},
			want: "simple-forge.ru",
		},
		{
			// No domain at all is the legitimate case the early return exists
			// for: a worker-only stack publishes nothing and needs no registrar.
			name: "no domain anywhere",
			cfg:  &staticStackInput{StackConfig: api.StackConfigStatic{BundleDir: "dist"}},
			want: "",
		},
		{
			name: "nil config",
			cfg:  nil,
			want: "",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := stackConfigDomain(tt.cfg); got != tt.want {
				t.Fatalf("stackConfigDomain = %q, want %q", got, tt.want)
			}
		})
	}
}

// The flat spelling wins when both are present, so a config that grows an
// explicit top-level `domain:` is not silently overridden by an inherited one.
func TestStackConfigDomain_PrefersTheFlatSpelling(t *testing.T) {
	// Built as JSON rather than a struct: no Go type carries both spellings
	// today, and the point is which key the probe trusts, not which struct.
	var cfg any
	if err := json.Unmarshal([]byte(`{"stackConfig":{"domain":"flat.example.com","Site":{"domain":"nested.example.com"}}}`), &cfg); err != nil {
		t.Fatal(err)
	}
	if got := stackConfigDomain(cfg); got != "flat.example.com" {
		t.Fatalf("stackConfigDomain = %q, want the flat spelling", got)
	}
}

// The shape the PLUGIN-OWNED path actually produces — built through
// api.NewPluginPreparedInput exactly as api.convertPluginOwnedTemplate does,
// rather than hand-rolled.
//
// This is the case api#587 missed. That fix taught the probe the nested-`Site`
// spelling, which is right for the in-tree static path, and verified it against
// a hand-built `{"stackConfig":{"Site":{...}}}` wrapper. But a plugin-owned
// template's descriptor does not carry the client config at all — it carries a
// PluginPreparedInput, whose JSON is `{"input":…,"baseDnsZone":…,"uses":…}` with
// no `stackConfig` key anywhere. So the probe was being handed the wrong object
// and still returned "", and forge-landing-ru-apex--staging kept failing with
// `no yc-dns registrar initialised that serves "simple-forge.ru"` on a binary
// that contained #587 (v2026.11.41 / 2d72edfa).
func TestStackConfigDomain_FindsTheDomainOnAPluginPreparedInput(t *testing.T) {
	for _, tt := range []struct {
		name      string
		clientCfg any
		want      string
	}{
		{
			// yc-static-website / aws-static-website / gcp-static-website: the
			// domain is nested inside StaticSiteConfig on the client config, and
			// each plugin mirrors it in its own Input under its own name
			// (`siteDomain` for yandex and aws) — which is exactly why core has
			// to carry its own copy.
			name:      "static website",
			clientCfg: &api.StackConfigStatic{BundleDir: "dist", Site: api.StaticSiteConfig{Domain: "simple-forge.ru"}},
			want:      "simple-forge.ru",
		},
		{
			name:      "cloud-compose",
			clientCfg: &api.StackConfigCompose{Domain: "forge.simple-container.com"},
			want:      "forge.simple-container.com",
		},
		{
			name:      "single-image",
			clientCfg: &api.StackConfigSingleImage{Domain: "x.simple-forge.ru"},
			want:      "x.simple-forge.ru",
		},
		{
			// A worker-only stack declares no domain. provisionDomainRegistrar's
			// `domain == ""` early return is correct here: there is no zone to
			// resolve a registrar for.
			name:      "no domain declared",
			clientCfg: &api.StackConfigCompose{},
			want:      "",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			prepared := api.NewPluginPreparedInput(&api.PluginStackConvertResult{
				// Whatever the plugin returned. Deliberately carries a DIFFERENT
				// domain under a plugin-specific key, so a test that passed by
				// reading the plugin's Input would be reading the wrong value.
				Input: map[string]any{"siteDomain": "not-the-one-core-should-use.example.com"},
			})
			prepared.Domain_ = api.StackConfigDomainOf(tt.clientCfg)

			if got := stackConfigDomain(prepared); got != tt.want {
				t.Fatalf("stackConfigDomain = %q, want %q", got, tt.want)
			}
		})
	}
}
