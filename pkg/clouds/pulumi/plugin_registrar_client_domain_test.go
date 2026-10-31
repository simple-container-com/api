// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package pulumi

import (
	"context"
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
