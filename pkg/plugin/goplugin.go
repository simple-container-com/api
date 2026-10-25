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
// SCCloudPlugin gRPC service. The host-side GRPCClient returns a gRPC stub the
// SC-core code can call; the plugin-side GRPCServer attaches the author's
// implementation to the plugin's gRPC server.
type scCloudPlugin struct {
	hplugin.Plugin
	Impl scpluginv1.SCCloudPluginServer // set by plugin authors; nil on the host side
}

// GRPCServer wires the author-supplied implementation into the plugin binary's
// gRPC server. The MuxBroker argument is unused in v1 — bidirectional sub-channels
// are reserved for later (Fingerprint stream, callback multiplexing).
func (p *scCloudPlugin) GRPCServer(_ *hplugin.GRPCBroker, s *grpc.Server) error {
	scpluginv1.RegisterSCCloudPluginServer(s, p.Impl)
	return nil
}

// GRPCClient returns a client stub the SC-core-side code uses to call Schema,
// Provision, Provider, InitStateStore. The context, broker, and conn arguments
// are the hashicorp/go-plugin standard — v1 only uses conn.
func (p *scCloudPlugin) GRPCClient(_ context.Context, _ *hplugin.GRPCBroker, c *grpc.ClientConn) (interface{}, error) {
	return scpluginv1.NewSCCloudPluginClient(c), nil
}

// PluginSet is the map hashicorp/go-plugin uses to resolve the Plugin by name.
// Pass a non-nil Impl on the plugin side; leave it nil on the host side.
func PluginSet(impl scpluginv1.SCCloudPluginServer) hplugin.PluginSet {
	return hplugin.PluginSet{
		PluginName: &scCloudPlugin{Impl: impl},
	}
}
