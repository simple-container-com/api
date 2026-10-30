// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

// Package plugin wires hashicorp/go-plugin's lifecycle to the sc_plugin.proto
// service, providing both the SC-core-side client wrapper and the
// plugin-side server registration helper.
//
// See forge/docs/design/2026-10-24/sc-plugin-architecture/03-architecture.md §6
// for lifecycle details, §6.3 for credential passing, §6.4 for the health channel,
// and 06-decisions.md D2 for why hashicorp/go-plugin is the transport.
package plugin

import (
	"context"
	"time"

	"google.golang.org/grpc"

	hplugin "github.com/hashicorp/go-plugin"

	scpluginv1 "github.com/simple-container-com/api/pkg/plugin/proto/v1"
)

// Handshake is the magic-cookie guard every sc-plugin-<cloud> binary must declare.
// It is NOT a security mechanism — go-plugin's own README is explicit on that
// point — it is a UX guard that makes a plugin binary print a friendly error
// instead of serving when run by hand without SC core setting the env var.
// Mutual TLS (AutoMTLS) provides the real transport integrity.
var Handshake = hplugin.HandshakeConfig{
	ProtocolVersion:  1,
	MagicCookieKey:   "SC_PLUGIN_MAGIC_COOKIE",
	MagicCookieValue: "sc-plugin-v1-c2ltcGxlLWNvbnRhaW5lcgo",
}

// PluginName is the single plugin.Plugin entry every SC cloud plugin advertises.
// Plugins that eventually need to serve multiple distinct interfaces over the
// same binary will register additional entries alongside this one.
const PluginName = "sc_cloud_plugin"

// scCloudPlugin satisfies hashicorp/go-plugin's Plugin interface for the
// SCCloudPlugin gRPC service. The host-side GRPCClient returns a wrapper
// that carries both the plugin stub and the broker stream id SC core
// serves its SCCore callback on; the plugin-side GRPCServer attaches
// the author's implementation to the plugin's gRPC server.
type scCloudPlugin struct {
	hplugin.Plugin

	// Impl is set by plugin authors; nil on the host side.
	Impl scpluginv1.SCCloudPluginServer

	// Peer dispatches a PeerInvoke callback across to another loaded
	// plugin. Non-nil on the host side when SC core wants to expose the
	// SCCore endpoint over the plugin's go-plugin broker (D8 PeerInvoke
	// — see 03 §9). Nil on the plugin side; nil on the host side when
	// composite plugins aren't needed. When nil, GRPCClient still picks
	// a broker stream id and serves an SCCore endpoint that answers
	// Unimplemented, so a plugin that calls PeerInvoke anyway gets a
	// clear error rather than a hang.
	Peer PeerDispatcher
}

// scCloudPluginClient wraps the SCCloudPlugin stub with the broker
// stream id SC core picked for this plugin's SCCore callback endpoint.
// Returned from GRPCClient on the host side so NewClient can carry both
// through to the Client struct's SccoreBrokerStream().
type scCloudPluginClient struct {
	stub     scpluginv1.SCCloudPluginClient
	brokerID uint32
}

// BrokerAware is the opt-in interface a plugin's Impl can implement to
// capture SC's go-plugin broker at GRPCServer time, so handlers can later
// call pkg/plugin/peer.go NewPeerClient to dial SC core's SCCore endpoint.
// Terraform-provider pattern — see the sc-plugin-aws composite demo.
//
// Non-composite plugins (cloudflare, yandex, cloudru) leave this
// unimplemented and are unaffected: GRPCServer below type-asserts and
// skips the SetBroker call when the Impl does not satisfy it.
//
// (Re-added here after PR #510 accidentally removed the BrokerAware
// shipped in #509. Without it NewPeerClient on the plugin side errors
// with "nil broker" — the broker never reached the handler, and the
// registrar-via-PeerInvoke bridge (D8) cannot work.)
type BrokerAware interface {
	SetBroker(*hplugin.GRPCBroker)
}

// GRPCServer wires the author-supplied implementation into the plugin binary's
// gRPC server. If the Impl is BrokerAware, SC's go-plugin broker is passed in
// via SetBroker before the gRPC server is registered.
func (p *scCloudPlugin) GRPCServer(broker *hplugin.GRPCBroker, s *grpc.Server) error {
	if ba, ok := p.Impl.(BrokerAware); ok {
		ba.SetBroker(broker)
	}
	scpluginv1.RegisterSCCloudPluginServer(s, p.Impl)
	return nil
}

// GRPCClient returns a client wrapper the SC-core-side code uses to call
// Schema, Provision, Provider, InitStateStore. Also picks a go-plugin broker
// stream id and starts serving the SCCore callback service on it so a
// composite plugin can dial back for PeerInvoke (D8).
//
// Accept is synchronous (opens the listener + announces its address via
// the hashicorp/go-plugin broker service) and the gRPC Serve runs in a
// goroutine. Doing the whole AcceptAndServe async races the plugin's
// first broker.Dial — "timeout waiting for connection info" — because
// the listener's address hasn't been advertised yet.
func (p *scCloudPlugin) GRPCClient(_ context.Context, broker *hplugin.GRPCBroker, c *grpc.ClientConn) (interface{}, error) {
	brokerID := broker.NextId()
	cb := NewCallbackServer(p.Peer)
	readyCh := make(chan struct{})
	go func() {
		broker.AcceptAndServe(brokerID, func(opts []grpc.ServerOption) *grpc.Server {
			s := grpc.NewServer(opts...)
			scpluginv1.RegisterSCCoreServer(s, cb)
			close(readyCh)
			return s
		})
	}()
	// Block briefly for the listener to be accepted + ConnInfo announced
	// to the plugin side. Without this, the plugin's first broker.Dial
	// races the goroutine and times out "waiting for connection info".
	// 100ms is enough on a healthy host; a slower one takes longer only
	// at the cost of the plugin's own Dial timeout.
	select {
	case <-readyCh:
	case <-time.After(500 * time.Millisecond):
	}
	return &scCloudPluginClient{
		stub:     scpluginv1.NewSCCloudPluginClient(c),
		brokerID: brokerID,
	}, nil
}

// pluginSet is the map hashicorp/go-plugin uses to resolve the Plugin by name.
// Pass a non-nil Impl on the plugin side; pass a non-nil Peer on the host
// side when composite plugins are needed.
func pluginSet(impl scpluginv1.SCCloudPluginServer, peer PeerDispatcher) hplugin.PluginSet {
	return hplugin.PluginSet{
		PluginName: &scCloudPlugin{Impl: impl, Peer: peer},
	}
}

// PluginSet is the plugin-side PluginSet (Impl set, no Peer).
// Kept for Serve() callers in plugin binaries.
func PluginSet(impl scpluginv1.SCCloudPluginServer) hplugin.PluginSet {
	return pluginSet(impl, nil)
}
