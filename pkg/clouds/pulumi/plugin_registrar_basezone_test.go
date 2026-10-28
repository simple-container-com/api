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

// stubProvisionRegistrarClient captures the ProvisionRegistrar request so the
// test can assert what actually crossed the plugin bus. The embedded interface
// is nil, so any other RPC panics loudly rather than returning a zero value.
type stubProvisionRegistrarClient struct {
	scpluginv1.SCCloudPluginClient
	got *scpluginv1.ProvisionRegistrarRequest
}

func (s *stubProvisionRegistrarClient) ProvisionRegistrar(
	_ context.Context, req *scpluginv1.ProvisionRegistrarRequest, _ ...grpc.CallOption,
) (*scpluginv1.ProvisionRegistrarResponse, error) {
	s.got = req
	return &scpluginv1.ProvisionRegistrarResponse{MainDomain: "simple-container.com"}, nil
}

// withActiveSession fakes the session requireMonitorAddr insists on. The
// address is never dialled — the stub answers in-process — but it must be
// non-empty or the sidecar refuses to dispatch at all.
func withActiveSession(t *testing.T) {
	t.Helper()
	activeSessionMu.Lock()
	prev := activeSession
	activeSession = &pluginSession{proxyAddr: "127.0.0.1:1", providerURNs: map[string]string{}}
	activeSessionMu.Unlock()
	t.Cleanup(func() {
		activeSessionMu.Lock()
		activeSession = prev
		activeSessionMu.Unlock()
	})
}

// A plugin registrar must learn the DEPLOYING STACK's preferred base zone.
//
// SC core computes it (deploy.go: dnsPreference.BaseZone = a.OverriddenBaseZone())
// and hands it to every in-tree registrar via ProvisionParams.DnsPreference
// (registrars.go multiRegistrar.build). The plugin sidecar received the same
// params but ProvisionRegistrarRequest had no field for it, so the preference
// died at the bus boundary and sc-plugin-cloudflare fell back to its own
// configured zoneName.
//
// For the fleet that fallback is catastrophic rather than cosmetic. The parent
// server.yaml declares ONE catch-all Cloudflare registrar (`default: true`,
// zoneName simple-container.com) that is expected to look up whichever zone the
// deploying stack names, and every service publishes under simple-forge.com. So
// a plugin-era deploy wrote <svc>.simple-forge.com into the simple-container.com
// zone; Pulumi saw a changed zone_id — an INPUT diff, which a stable URN does
// nothing to prevent — replaced the record create-before-delete, and the delete
// half removed the LIVE record while the run reported success.
//
// Hence this test asserts on the wire field, not on any registrar behaviour:
// the regression was an omission at the serialization boundary.
func TestRegistrarSidecar_ForwardsStackBaseDnsZone(t *testing.T) {
	for _, tt := range []struct {
		name string
		pref *pApi.DnsPreference
		want string
	}{
		{name: "stack overrides the base zone", pref: &pApi.DnsPreference{BaseZone: "simple-forge.com"}, want: "simple-forge.com"},
		{name: "stack states no preference", pref: &pApi.DnsPreference{}, want: ""},
		// A nil preference is the common case for stacks that never set
		// baseDnsZone; it must not panic on the way to the bus.
		{name: "nil preference", pref: nil, want: ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			withActiveSession(t)

			stub := &stubProvisionRegistrarClient{}
			sidecar := (&preEnginePluginBundle{}).makeRegistrarSidecar(
				scPlugin.NewClientWithStub(stub), "sc-plugin-cloudflare", "cloudflare",
			)

			err := sdk.RunErr(func(ctx *sdk.Context) error {
				registrar, err := sidecar(ctx, api.RegistrarDescriptor{Type: "cloudflare"}, pApi.ProvisionParams{
					Log:           logger.New(),
					DnsPreference: tt.pref,
				})
				Expect(err).To(BeNil())
				Expect(registrar).NotTo(BeNil())
				return nil
			}, sdk.WithMocks("project", "stack", &refMocks{}))
			Expect(err).To(BeNil())

			Expect(stub.got).NotTo(BeNil())
			Expect(stub.got.GetBaseDnsZone()).To(Equal(tt.want))
			// The registrar type still has to arrive, or a green assertion
			// above would be meaningless.
			Expect(stub.got.GetRegistrarType()).To(Equal("cloudflare"))
		})
	}
}
