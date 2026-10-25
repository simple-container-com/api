// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

// Package main is a fixture plugin used by pkg/plugin's monitor-dial smoke
// test. It implements SCCloudPlugin.Provision by dialing the MonitorAddress
// in the request header and calling pulumirpc.ResourceMonitor.RegisterResource
// — exactly the mechanism 03-architecture.md §3's option (b) relies on.
//
// Not a product deliverable; the real sc-plugin-echo lives in its own repo
// (Phase 0 Step 7).
package main

import (
	"context"
	"os"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	hclog "github.com/hashicorp/go-hclog"
	pulumirpc "github.com/pulumi/pulumi/sdk/v3/proto/go"

	plugin "github.com/simple-container-com/api/pkg/plugin"
	scpluginv1 "github.com/simple-container-com/api/pkg/plugin/proto/v1"
)

type provImpl struct {
	scpluginv1.UnimplementedSCCloudPluginServer
}

func (p *provImpl) Schema(_ context.Context, _ *scpluginv1.SchemaRequest) (*scpluginv1.SchemaResponse, error) {
	return &scpluginv1.SchemaResponse{
		PluginVersion: "provision-smoke-0.0.1",
		ResourceTypes: []string{"random:index/randomPet:RandomPet"},
	}, nil
}

func (p *provImpl) Provision(ctx context.Context, req *scpluginv1.ProvisionRequest) (*scpluginv1.ProvisionResponse, error) {
	// Dial the Pulumi Resource Monitor whose address the host handed us in the
	// request header. Insecure is fine here — monitor_address is a loopback /
	// unix-socket endpoint owned by the SC core process that spawned us; see
	// 03 §3.1.
	conn, err := grpc.NewClient(req.GetHeader().GetMonitorAddress(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	monitor := pulumirpc.NewResourceMonitorClient(conn)

	// Call RegisterResource with the type and name the host asked for. Pass the
	// explicit provider URN from provider_urns if the host set one (that's how
	// backward-compat on upgrade works per 04 §3). Keyed by resource type's
	// package prefix.
	providerURN := req.GetHeader().GetProviderUrns()[req.GetResourceType()]
	resp, err := monitor.RegisterResource(ctx, &pulumirpc.RegisterResourceRequest{
		Type:     req.GetResourceType(),
		Name:     req.GetResourceName(),
		Custom:   true,
		Parent:   req.GetParentUrn(),
		Provider: providerURN,
	})
	if err != nil {
		return nil, err
	}

	return &scpluginv1.ProvisionResponse{
		Urn: resp.GetUrn(),
	}, nil
}

func main() {
	plugin.Serve(plugin.ServerConfig{
		Impl: &provImpl{},
		Logger: hclog.New(&hclog.LoggerOptions{
			Name:   "provision-plugin",
			Level:  hclog.Error,
			Output: os.Stderr,
		}),
	})
}
