// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package plugin

import (
	"context"

	"google.golang.org/protobuf/proto"

	"github.com/pkg/errors"

	scpluginv1 "github.com/simple-container-com/api/pkg/plugin/proto/v1"
)

// PeerDispatcher is the host-side route from one plugin's PeerInvoke call
// to another plugin's regular SCCloudPlugin RPC. SC core implements this
// against its loaded-plugins registry (`preEnginePluginBundle.clients`
// in pkg/clouds/pulumi/plugin_preengine.go).
//
// `pluginName` is the plugin's advertised name — the key SC core stores
// in its registry on load. `rpcName` is the SCCloudPlugin method name,
// case-sensitive (e.g. "Provision", "NewDnsRecord"). The payload is
// proto wire bytes; the dispatcher unmarshals into the right request
// type, calls the stub, and marshals the response.
//
// D8 of 06-decisions.md: a direct plugin-to-plugin connection was
// rejected in favour of this broker pattern. SC core is the only entity
// that knows every plugin's identity and credentials.
type PeerDispatcher interface {
	DispatchPeer(ctx context.Context, pluginName, rpcName string, payload []byte) ([]byte, error)
}

// SecretResolver and CredentialResolver are placeholders for the two
// other SCCore RPCs. Neither has a core-side implementation today — the
// bus declares them but no plugin can usefully call them yet (noted in
// Addendum R and this PR's design). The server below returns
// Unimplemented until they land, which is observable but harmless.
type SecretResolver interface {
	ResolveSecret(ctx context.Context, req *scpluginv1.ResolveSecretRequest) (*scpluginv1.ResolveSecretResponse, error)
}

type CredentialResolver interface {
	ResolveCredential(ctx context.Context, req *scpluginv1.ResolveCredentialRequest) (*scpluginv1.ResolveCredentialResponse, error)
}

// CallbackServer implements SCCoreServer. SC core registers one of
// these per spawned plugin, exposed over the hashicorp/go-plugin broker
// at the stream id passed in RequestHeader.sccore_broker_stream.
//
// Fields are set per-plugin by NewCallbackServer so lookups into the
// plugin registry route to the right loader context. Nil fields answer
// Unimplemented — which is correct for ResolveSecret/ResolveCredential
// in Phase 1e and lets us ship PeerInvoke without the Vault pattern's
// callback plumbing.
type CallbackServer struct {
	scpluginv1.UnimplementedSCCoreServer

	// Peer dispatches a PeerInvoke to another plugin via SC core's
	// loaded-plugins registry. Nil → PeerInvoke returns Unimplemented.
	Peer PeerDispatcher

	// Secret and Credential are the Vault-pattern callbacks. Nil in
	// Phase 1e; the proto fields and this server slot are reserved so a
	// future Addendum can wire them without reshaping the server.
	Secret     SecretResolver
	Credential CredentialResolver
}

// NewCallbackServer builds the SCCore impl SC core serves per-plugin.
// Pass only Peer today; Secret/Credential are reserved and ignored.
func NewCallbackServer(peer PeerDispatcher) *CallbackServer {
	return &CallbackServer{Peer: peer}
}

// PeerInvoke is the composite-plugin broker. Looks the target plugin
// up by name and dispatches rpc_name against its client stub.
func (s *CallbackServer) PeerInvoke(ctx context.Context, req *scpluginv1.PeerInvokeRequest) (*scpluginv1.PeerInvokeResponse, error) {
	if s.Peer == nil {
		return &scpluginv1.PeerInvokeResponse{
			Error: "PeerInvoke is not wired on this SC core — Phase 1e PR 3 is the first release that supports composite plugins",
		}, nil
	}
	if req.GetPluginName() == "" {
		return &scpluginv1.PeerInvokeResponse{Error: "PeerInvoke requires plugin_name"}, nil
	}
	if req.GetRpcName() == "" {
		return &scpluginv1.PeerInvokeResponse{Error: "PeerInvoke requires rpc_name"}, nil
	}
	// TODO Phase 1e+ : validate req.GetRunToken() against the token the
	// calling plugin was issued, so a rogue plugin cannot impersonate a
	// peer. Today SC core trusts the calling plugin because it spawned
	// it, which is a weaker guarantee than the token check promises.
	out, err := s.Peer.DispatchPeer(ctx, req.GetPluginName(), req.GetRpcName(), req.GetPayload())
	if err != nil {
		return &scpluginv1.PeerInvokeResponse{Error: err.Error()}, nil
	}
	return &scpluginv1.PeerInvokeResponse{Payload: out}, nil
}

// ResolveSecret / ResolveCredential — kept at Unimplemented until the
// Vault pattern is wired (Addendum R follow-up). The proto field + this
// server slot are reserved so adding them later is a behaviour change,
// not a wire change.
func (s *CallbackServer) ResolveSecret(ctx context.Context, req *scpluginv1.ResolveSecretRequest) (*scpluginv1.ResolveSecretResponse, error) {
	if s.Secret == nil {
		return nil, errors.New("ResolveSecret is not wired on this SC core — Vault pattern is still Addendum R work")
	}
	return s.Secret.ResolveSecret(ctx, req)
}

func (s *CallbackServer) ResolveCredential(ctx context.Context, req *scpluginv1.ResolveCredentialRequest) (*scpluginv1.ResolveCredentialResponse, error) {
	if s.Credential == nil {
		return nil, errors.New("ResolveCredential is not wired on this SC core — Vault pattern is still Addendum R work")
	}
	return s.Credential.ResolveCredential(ctx, req)
}

// MarshalPeerPayload is a convenience for a PeerDispatcher implementation:
// marshal a response message into the bytes PeerInvokeResponse expects.
func MarshalPeerPayload(resp proto.Message) ([]byte, error) {
	if resp == nil {
		return nil, nil
	}
	return proto.Marshal(resp)
}

// UnmarshalPeerPayload is the matching convenience for a PeerDispatcher:
// unmarshal the request payload into a typed message chosen by rpc_name.
func UnmarshalPeerPayload(payload []byte, into proto.Message) error {
	return proto.Unmarshal(payload, into)
}
