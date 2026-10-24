// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

// Package main is a fixture plugin for Phase 1 surface completion. It
// advertises three NEW surfaces so the sc provision/destroy flow exercises
// each sidecar path:
//
//   - secrets_provider_types = ["smoke:plugin-secrets"]
//     InitSecretsProvider returns a passphrase scheme + a fixed passphrase
//     SC core sets p.secretsProviderPassphrase from.
//
//   - registrar_types = ["smoke:plugin-dns"]
//     ProvisionRegistrar returns a main_domain SC's initRegistrar accepts.
//
//   - PreDestroy hook (one per plugin, no type keying)
//     Writes a marker file so the test can confirm the hook fired.
//
// Env vars the plugin reads at test time:
//
//	SC_PLUGIN_MULTI_PASSPHRASE — the passphrase to return from InitSecretsProvider.
//	SC_PLUGIN_MULTI_DOMAIN     — the main_domain to return from ProvisionRegistrar.
//	SC_PLUGIN_MULTI_HOOK_FILE  — path the PreDestroy hook touches.
package main

import (
	"context"
	"os"

	hclog "github.com/hashicorp/go-hclog"

	plugin "github.com/simple-container-com/api/pkg/plugin"
	scpluginv1 "github.com/simple-container-com/api/pkg/plugin/proto/v1"
)

type multiImpl struct {
	scpluginv1.UnimplementedSCCloudPluginServer
}

func (m *multiImpl) Schema(_ context.Context, _ *scpluginv1.SchemaRequest) (*scpluginv1.SchemaResponse, error) {
	return &scpluginv1.SchemaResponse{
		PluginVersion:        "multi-surface-0.0.1",
		SecretsProviderTypes: []string{"smoke:plugin-secrets"},
		RegistrarTypes:       []string{"smoke:plugin-dns"},
	}, nil
}

func (m *multiImpl) InitSecretsProvider(_ context.Context, req *scpluginv1.InitSecretsProviderRequest) (*scpluginv1.InitSecretsProviderResponse, error) {
	passphrase := os.Getenv("SC_PLUGIN_MULTI_PASSPHRASE")
	if passphrase == "" {
		passphrase = "default-plugin-passphrase"
	}
	// Passphrase-only: leave provider_url empty so SC follows the in-tree
	// passphrase path (p.secretsProviderPassphrase set; p.secretsProviderUrl
	// stays "" so prepareStackForOperations's URL-dispatch is skipped and the
	// passphrase-dispatch at line 99+ fires).
	return &scpluginv1.InitSecretsProviderResponse{
		Passphrase: passphrase,
	}, nil
}

func (m *multiImpl) ProvisionRegistrar(_ context.Context, req *scpluginv1.ProvisionRegistrarRequest) (*scpluginv1.ProvisionRegistrarResponse, error) {
	domain := os.Getenv("SC_PLUGIN_MULTI_DOMAIN")
	if domain == "" {
		domain = "smoke.plugin.example.com"
	}
	return &scpluginv1.ProvisionRegistrarResponse{
		MainDomain: domain,
	}, nil
}

func (m *multiImpl) PreDestroy(_ context.Context, _ *scpluginv1.PreDestroyRequest) (*scpluginv1.PreDestroyResponse, error) {
	if path := os.Getenv("SC_PLUGIN_MULTI_HOOK_FILE"); path != "" {
		if err := os.WriteFile(path, []byte("pre-destroy-hook-fired\n"), 0o644); err != nil {
			// Return the error so the test knows the hook couldn't do its job.
			return nil, err
		}
	}
	return &scpluginv1.PreDestroyResponse{}, nil
}

func main() {
	plugin.Serve(plugin.ServerConfig{
		Impl: &multiImpl{},
		Logger: hclog.New(&hclog.LoggerOptions{
			Name:   "multi-surface-plugin",
			Level:  hclog.Error,
			Output: os.Stderr,
		}),
	})
}
