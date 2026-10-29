// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package plugin

import (
	"context"
	"testing"

	"google.golang.org/protobuf/proto"

	. "github.com/onsi/gomega"

	scpluginv1 "github.com/simple-container-com/api/pkg/plugin/proto/v1"
)

// stubPeerDispatcher captures the parameters of a DispatchPeer call so
// the test can assert that CallbackServer.PeerInvoke forwarded the
// right payload to the right (name, rpc). Returns a canned success
// payload the test unmarshals back out.
type stubPeerDispatcher struct {
	gotPlugin  string
	gotRpc     string
	gotPayload []byte
	resp       []byte
	err        error
}

func (s *stubPeerDispatcher) DispatchPeer(_ context.Context, pluginName, rpcName string, payload []byte) ([]byte, error) {
	s.gotPlugin = pluginName
	s.gotRpc = rpcName
	s.gotPayload = payload
	return s.resp, s.err
}

// TestCallbackServer_PeerInvoke_RoutesToDispatcher — the SCCore server
// unwraps the PeerInvoke proto and hands it to the dispatcher verbatim.
// The dispatcher's response payload rides back on PeerInvokeResponse.
func TestCallbackServer_PeerInvoke_RoutesToDispatcher(t *testing.T) {
	RegisterTestingT(t)

	// Build a canned response the dispatcher returns — a real peer
	// plugin's ProvisionResponse serialized to proto bytes.
	cannedResp := &scpluginv1.ProvisionResponse{Urn: "urn:pulumi:test::proj::aws::b"}
	cannedBytes, err := proto.Marshal(cannedResp)
	Expect(err).ToNot(HaveOccurred())

	disp := &stubPeerDispatcher{resp: cannedBytes}
	srv := NewCallbackServer(disp)

	// Compose the sub-request the composite plugin would have sent.
	subReq := &scpluginv1.ProvisionRequest{
		Header:       &scpluginv1.RequestHeader{StackName: "backend"},
		ResourceType: "aws-security-group",
		ResourceName: "sg-primary",
	}
	subPayload, err := proto.Marshal(subReq)
	Expect(err).ToNot(HaveOccurred())

	resp, err := srv.PeerInvoke(context.Background(), &scpluginv1.PeerInvokeRequest{
		RunToken:   "test-run-token",
		PluginName: "sc-plugin-aws",
		RpcName:    "Provision",
		Payload:    subPayload,
	})
	Expect(err).ToNot(HaveOccurred())
	Expect(resp.GetError()).To(Equal(""))

	// The dispatcher saw the exact fields.
	Expect(disp.gotPlugin).To(Equal("sc-plugin-aws"))
	Expect(disp.gotRpc).To(Equal("Provision"))
	Expect(disp.gotPayload).To(Equal(subPayload))

	// The response payload round-trips back to the typed message.
	var out scpluginv1.ProvisionResponse
	Expect(proto.Unmarshal(resp.GetPayload(), &out)).To(Succeed())
	Expect(out.GetUrn()).To(Equal("urn:pulumi:test::proj::aws::b"))
}

// TestCallbackServer_PeerInvoke_UnwiredReturnsClearError — a plugin
// that calls PeerInvoke on an SC core that didn't wire a dispatcher
// gets a readable error, not a nil deref. Covers the test-harness and
// older-core scenarios.
func TestCallbackServer_PeerInvoke_UnwiredReturnsClearError(t *testing.T) {
	RegisterTestingT(t)

	srv := NewCallbackServer(nil)
	resp, err := srv.PeerInvoke(context.Background(), &scpluginv1.PeerInvokeRequest{
		PluginName: "sc-plugin-aws",
		RpcName:    "Provision",
	})
	Expect(err).ToNot(HaveOccurred(), "SCCore RPCs must not return gRPC errors for in-plugin logic errors — the error rides on PeerInvokeResponse.error")
	Expect(resp.GetError()).To(ContainSubstring("PeerInvoke is not wired"))
}

// TestCallbackServer_PeerInvoke_ValidatesRequiredFields — missing
// plugin_name / rpc_name is an author error that lands as a readable
// response rather than a cryptic transport-level NotFound.
func TestCallbackServer_PeerInvoke_ValidatesRequiredFields(t *testing.T) {
	RegisterTestingT(t)

	srv := NewCallbackServer(&stubPeerDispatcher{})

	for _, tc := range []struct {
		name string
		req  *scpluginv1.PeerInvokeRequest
		msg  string
	}{
		{"missing plugin_name", &scpluginv1.PeerInvokeRequest{RpcName: "Provision"}, "plugin_name"},
		{"missing rpc_name", &scpluginv1.PeerInvokeRequest{PluginName: "sc-plugin-aws"}, "rpc_name"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := srv.PeerInvoke(context.Background(), tc.req)
			Expect(err).ToNot(HaveOccurred())
			Expect(resp.GetError()).To(ContainSubstring(tc.msg))
		})
	}
}

// TestCallbackServer_ResolveSecret_UnimplementedUntilVault — the two
// Vault-pattern RPCs stay returning gRPC errors (Unimplemented-shaped)
// until the Addendum R follow-up wires them. A plugin that calls them
// gets a clear "not wired" message instead of a hang.
func TestCallbackServer_ResolveSecret_UnimplementedUntilVault(t *testing.T) {
	RegisterTestingT(t)

	srv := NewCallbackServer(&stubPeerDispatcher{})

	_, err := srv.ResolveSecret(context.Background(), &scpluginv1.ResolveSecretRequest{})
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("ResolveSecret is not wired"))

	_, err = srv.ResolveCredential(context.Background(), &scpluginv1.ResolveCredentialRequest{})
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("ResolveCredential is not wired"))
}
