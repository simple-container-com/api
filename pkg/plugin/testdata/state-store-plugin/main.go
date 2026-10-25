// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

// Package main is a fixture plugin for Step 4b: pre-engine state-storage
// plugin dispatch. Advertises one state-store type "plugin-smoke-fs" and
// on InitStateStore returns a filesystem backend URL from env.
//
// Env var the plugin reads: SC_PLUGIN_STATE_PATH — the directory where
// the Pulumi state should land. The plugin returns backend_url =
// "file:///<SC_PLUGIN_STATE_PATH>". If unset, defaults to /tmp/sc-plugin-state.
package main

import (
	"context"
	"os"

	hclog "github.com/hashicorp/go-hclog"

	plugin "github.com/simple-container-com/api/pkg/plugin"
	scpluginv1 "github.com/simple-container-com/api/pkg/plugin/proto/v1"
)

type stateImpl struct {
	scpluginv1.UnimplementedSCCloudPluginServer
}

func (s *stateImpl) Schema(_ context.Context, _ *scpluginv1.SchemaRequest) (*scpluginv1.SchemaResponse, error) {
	return &scpluginv1.SchemaResponse{
		PluginVersion: "state-store-smoke-0.0.1",
		// Plugin-advertised types MUST carry a colon prefix — that's what
		// SC's config-tier fall-through uses to route the YAML `type:`
		// string to the pre-engine plugin dispatch instead of in-tree.
		StateStoreTypes: []string{"smoke:plugin-smoke-fs"},
	}, nil
}

func (s *stateImpl) InitStateStore(_ context.Context, _ *scpluginv1.InitStateStoreRequest) (*scpluginv1.InitStateStoreResponse, error) {
	path := os.Getenv("SC_PLUGIN_STATE_PATH")
	if path == "" {
		path = "/tmp/sc-plugin-state"
	}
	if err := os.MkdirAll(path, 0o700); err != nil {
		return nil, err
	}
	return &scpluginv1.InitStateStoreResponse{
		BackendUrl: "file://" + path,
	}, nil
}

func main() {
	plugin.Serve(plugin.ServerConfig{
		Impl: &stateImpl{},
		Logger: hclog.New(&hclog.LoggerOptions{
			Name:   "state-store-plugin",
			Level:  hclog.Error,
			Output: os.Stderr,
		}),
	})
}
