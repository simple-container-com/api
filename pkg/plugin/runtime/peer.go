// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package runtime

import (
	"context"
	"sync"

	"github.com/pkg/errors"

	scpluginv1 "github.com/simple-container-com/api/pkg/plugin/proto/v1"
)

// PeerCapabilityHandler is a plugin-authored function that handles one
// named capability dispatched via SCCloudPlugin.PluginInvoke. Payload
// and reply are opaque JSON bytes; shapes are documented in each
// plugin's README.
//
// Handlers are registered up-front (typically in plugin's `main.go` on
// start-up) via `RegisterPeerCapability`. The capability names must
// match the plugin's SchemaResponse.peer_capabilities; SC core rejects
// calls to unadvertised capabilities before they reach the plugin.
type PeerCapabilityHandler func(ctx context.Context, callingPlugin string, payloadJSON []byte) ([]byte, error)

// peerCapabilities is the plugin-wide registry of capability handlers.
// Guarded by mu — handlers are typically registered at init time, but
// nothing prevents a plugin from registering lazily.
var (
	peerCapsMu sync.RWMutex
	peerCaps   = map[string]PeerCapabilityHandler{}
)

// RegisterPeerCapability stores a handler against a capability name.
// Overwrites a prior registration for the same name; a plugin re-
// registering is a legal "replace" operation (useful under tests).
// Returns an error only for empty inputs.
func RegisterPeerCapability(capability string, handler PeerCapabilityHandler) error {
	if capability == "" {
		return errors.New("runtime.RegisterPeerCapability: capability name is required")
	}
	if handler == nil {
		return errors.New("runtime.RegisterPeerCapability: handler is nil")
	}
	peerCapsMu.Lock()
	peerCaps[capability] = handler
	peerCapsMu.Unlock()
	return nil
}

// HandlePluginInvoke is the plugin-side dispatch for
// SCCloudPlugin.PluginInvoke. The plugin's main.go wires its RPC
// handler as a one-liner:
//
//	func (p *myPlugin) PluginInvoke(ctx context.Context,
//	        req *scpluginv1.PluginInvokeRequest) (*scpluginv1.PluginInvokeResponse, error) {
//	    return runtime.HandlePluginInvoke(ctx, req)
//	}
//
// HandlePluginInvoke looks the capability up in the registry, runs the
// handler, and wraps the reply bytes. Returns a clear "not registered"
// error when the capability has no handler on this binary — distinct
// from the broker's "not advertised" error, which fires before the
// RPC reaches the plugin at all.
func HandlePluginInvoke(ctx context.Context, req *scpluginv1.PluginInvokeRequest) (*scpluginv1.PluginInvokeResponse, error) {
	cap := req.GetCapability()
	if cap == "" {
		return nil, errors.New("runtime.HandlePluginInvoke: PluginInvokeRequest.capability is empty")
	}
	peerCapsMu.RLock()
	handler, ok := peerCaps[cap]
	peerCapsMu.RUnlock()
	if !ok {
		return nil, errors.Errorf("runtime.HandlePluginInvoke: no handler registered for capability %q (plugin advertised it in SchemaResponse.peer_capabilities but forgot runtime.RegisterPeerCapability)", cap)
	}
	reply, err := handler(ctx, req.GetCallingPlugin(), req.GetPayloadJson())
	if err != nil {
		return nil, errors.Wrapf(err, "peer capability %q handler", cap)
	}
	return &scpluginv1.PluginInvokeResponse{ReplyJson: reply}, nil
}

// ResetPeerCapabilitiesForTesting wipes the registry. Only meant for
// tests — a production plugin registers capabilities once and does not
// un-register them.
func ResetPeerCapabilitiesForTesting() {
	peerCapsMu.Lock()
	peerCaps = map[string]PeerCapabilityHandler{}
	peerCapsMu.Unlock()
}
