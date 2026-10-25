// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

// Package main is a minimal fixture plugin used by pkg/plugin's roundtrip test.
// Not a product deliverable — the real sc-plugin-echo reference plugin lives
// in its own repo (Phase 0 Step 7 of sc-plugin-architecture).
package main

import (
	"context"

	hclog "github.com/hashicorp/go-hclog"

	plugin "github.com/simple-container-com/api/pkg/plugin"
	scpluginv1 "github.com/simple-container-com/api/pkg/plugin/proto/v1"
)

type echoImpl struct {
	scpluginv1.UnimplementedSCCloudPluginServer
}

func (e *echoImpl) Schema(_ context.Context, _ *scpluginv1.SchemaRequest) (*scpluginv1.SchemaResponse, error) {
	return &scpluginv1.SchemaResponse{
		PluginVersion:  "test-echo-0.0.1",
		ProviderTypes:  []string{"echo"},
		ResourceTypes:  []string{"echo:say:Say"},
		RegistrarTypes: nil,
	}, nil
}

func main() {
	plugin.Serve(plugin.ServerConfig{
		Impl:   &echoImpl{},
		Logger: hclog.New(&hclog.LoggerOptions{Name: "echo-plugin", Level: hclog.Error}),
	})
}
