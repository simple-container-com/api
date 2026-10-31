// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package pulumi

import (
	"context"

	"google.golang.org/protobuf/proto"

	"github.com/pkg/errors"

	scpluginv1 "github.com/simple-container-com/api/pkg/plugin/proto/v1"
)

// bundlePeerDispatcher implements scPlugin.PeerDispatcher by looking the
// target plugin up in the preEnginePluginBundle's clients registry and
// dispatching the named RPC against its stub.
//
// The bundle lookup is lazy — done at call time, not capture time — so a
// plugin that spawns later in load order can still answer PeerInvoke
// calls from a plugin spawned earlier (as long as the earlier plugin
// does not invoke the peer BEFORE the peer finishes loading, which
// requires Phase 1e+ coordination to express).
//
// D8 of 06-decisions.md: SC core is the broker; no plugin-to-plugin
// connections exist.
type bundlePeerDispatcher struct {
	bundle *preEnginePluginBundle
}

// DispatchPeer looks the target plugin up in the bundle, unmarshals the
// payload into the request type keyed by rpc_name, calls the stub, and
// marshals the response back.
func (d *bundlePeerDispatcher) DispatchPeer(ctx context.Context, pluginName, rpcName string, payload []byte) ([]byte, error) {
	d.bundle.mu.Lock()
	cli, ok := d.bundle.clients[pluginName]
	d.bundle.mu.Unlock()
	if !ok {
		return nil, errors.Errorf("peer invoke %q.%s: plugin not loaded in this run", pluginName, rpcName)
	}

	switch rpcName {
	case "Provision":
		req := &scpluginv1.ProvisionRequest{}
		if err := proto.Unmarshal(payload, req); err != nil {
			return nil, errors.Wrapf(err, "peer invoke %q.%s: unmarshal request", pluginName, rpcName)
		}
		resp, err := cli.Provision(ctx, req)
		if err != nil {
			return nil, err
		}
		return proto.Marshal(resp)

	case "Provider":
		req := &scpluginv1.ProviderRequest{}
		if err := proto.Unmarshal(payload, req); err != nil {
			return nil, errors.Wrapf(err, "peer invoke %q.%s: unmarshal request", pluginName, rpcName)
		}
		resp, err := cli.Provider(ctx, req)
		if err != nil {
			return nil, err
		}
		return proto.Marshal(resp)

	case "NewDnsRecord":
		req := &scpluginv1.NewDnsRecordRequest{}
		if err := proto.Unmarshal(payload, req); err != nil {
			return nil, errors.Wrapf(err, "peer invoke %q.%s: unmarshal request", pluginName, rpcName)
		}
		resp, err := cli.NewDnsRecord(ctx, req)
		if err != nil {
			return nil, err
		}
		return proto.Marshal(resp)

	case "ProvisionRegistrar":
		req := &scpluginv1.ProvisionRegistrarRequest{}
		if err := proto.Unmarshal(payload, req); err != nil {
			return nil, errors.Wrapf(err, "peer invoke %q.%s: unmarshal request", pluginName, rpcName)
		}
		resp, err := cli.ProvisionRegistrar(ctx, req)
		if err != nil {
			return nil, err
		}
		return proto.Marshal(resp)

	case "VmHosts":
		req := &scpluginv1.VmHostsRequest{}
		if err := proto.Unmarshal(payload, req); err != nil {
			return nil, errors.Wrapf(err, "peer invoke %q.%s: unmarshal request", pluginName, rpcName)
		}
		resp, err := cli.VmHosts(ctx, req)
		if err != nil {
			return nil, err
		}
		return proto.Marshal(resp)

	case "NewOverrideHeaderRule":
		req := &scpluginv1.NewOverrideHeaderRuleRequest{}
		if err := proto.Unmarshal(payload, req); err != nil {
			return nil, errors.Wrapf(err, "peer invoke %q.%s: unmarshal request", pluginName, rpcName)
		}
		resp, err := cli.NewOverrideHeaderRule(ctx, req)
		if err != nil {
			return nil, err
		}
		return proto.Marshal(resp)

	case "ProvisionDomainForEndpoint":
		req := &scpluginv1.ProvisionDomainForEndpointRequest{}
		if err := proto.Unmarshal(payload, req); err != nil {
			return nil, errors.Wrapf(err, "peer invoke %q.%s: unmarshal request", pluginName, rpcName)
		}
		resp, err := cli.ProvisionDomainForEndpoint(ctx, req)
		if err != nil {
			return nil, err
		}
		return proto.Marshal(resp)

	// Add further RPCs here as composite-plugin needs surface. New
	// cases are purely additive — a plugin asking for an unlisted RPC
	// gets the clear "not supported in this core" error below rather
	// than an opaque transport failure.

	default:
		return nil, errors.Errorf("peer invoke %q.%s: SC core does not support that RPC on PeerInvoke (supported: Provision, Provider, NewDnsRecord, NewOverrideHeaderRule, ProvisionDomainForEndpoint, ProvisionRegistrar, VmHosts)", pluginName, rpcName)
	}
}
