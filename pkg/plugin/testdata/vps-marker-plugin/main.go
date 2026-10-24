// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

// Package main is a fixture plugin used by pkg/plugin's VPS integration smoke.
// It implements SCCloudPlugin.Provision by dialing MonitorAddress and
// registering a command:remote:Command that SSHes to a configured VPS and
// writes a marker file. Lifecycle (create + delete) are both wired.
//
// Reads SC_PLUGIN_VPS_{HOST,USER,PASSWORD} from the environment the host
// passed in; the plugin never sees the password over the plugin bus.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/structpb"

	hclog "github.com/hashicorp/go-hclog"
	pulumirpc "github.com/pulumi/pulumi/sdk/v3/proto/go"

	plugin "github.com/simple-container-com/api/pkg/plugin"
	scpluginv1 "github.com/simple-container-com/api/pkg/plugin/proto/v1"
)

type vpsImpl struct {
	scpluginv1.UnimplementedSCCloudPluginServer
}

func (v *vpsImpl) Schema(_ context.Context, _ *scpluginv1.SchemaRequest) (*scpluginv1.SchemaResponse, error) {
	return &scpluginv1.SchemaResponse{
		PluginVersion: "vps-marker-0.0.1",
		ResourceTypes: []string{"command:remote:Command"},
		PulumiPlugins: []*scpluginv1.PulumiPluginRequirement{
			{Package: "command", Version: "1.2.1"},
		},
	}, nil
}

func (v *vpsImpl) Provision(ctx context.Context, req *scpluginv1.ProvisionRequest) (*scpluginv1.ProvisionResponse, error) {
	host := os.Getenv("SC_PLUGIN_VPS_HOST")
	user := os.Getenv("SC_PLUGIN_VPS_USER")
	password := os.Getenv("SC_PLUGIN_VPS_PASSWORD")
	if host == "" || user == "" || password == "" {
		return nil, errors.New("SC_PLUGIN_VPS_{HOST,USER,PASSWORD} env vars not set on the plugin process")
	}

	conn, err := grpc.NewClient(req.GetHeader().GetMonitorAddress(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("dial monitor: %w", err)
	}
	defer conn.Close()
	monitor := pulumirpc.NewResourceMonitorClient(conn)

	// Build the command:remote:Command inputs as a structpb.Struct. The shape
	// matches pulumi-command/sdk's connection + create + delete fields.
	createCmd := "echo sc-plugin-marker-" + req.GetResourceName() + " > /tmp/sc-plugin-marker.txt && cat /tmp/sc-plugin-marker.txt"
	deleteCmd := "rm -f /tmp/sc-plugin-marker.txt"
	obj, err := structpb.NewStruct(map[string]interface{}{
		"connection": map[string]interface{}{
			"host":     host,
			"user":     user,
			"password": password,
		},
		"create": createCmd,
		"delete": deleteCmd,
	})
	if err != nil {
		return nil, fmt.Errorf("build inputs: %w", err)
	}

	resp, err := monitor.RegisterResource(ctx, &pulumirpc.RegisterResourceRequest{
		Type:                  "command:remote:Command",
		Name:                  req.GetResourceName(),
		Custom:                true,
		Object:                obj,
		Version:               "1.2.1", // matches pulumi-command/sdk v1.2.1 pinned by SC
		AcceptSecrets:         true,
		AcceptResources:       true,
		SupportsPartialValues: true,
	})
	if err != nil {
		return nil, fmt.Errorf("register command:remote:Command: %w", err)
	}

	return &scpluginv1.ProvisionResponse{
		Urn: resp.GetUrn(),
	}, nil
}

func main() {
	plugin.Serve(plugin.ServerConfig{
		Impl: &vpsImpl{},
		Logger: hclog.New(&hclog.LoggerOptions{
			Name:   "vps-marker-plugin",
			Level:  hclog.Error,
			Output: os.Stderr,
		}),
	})
}
