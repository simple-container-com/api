// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package api

// Bootstrap for the five canonical SC plugins.
//
// Why this file exists. Pre-A3, each plugin's /api module was blank-
// imported from pkg/clouds/pulumi/init.go, and its init() populated
// pluginByType via RegisterPluginRequirement. ValidatePluginRequirements
// relied on that pre-populated map to detect "operator forgot to declare
// plugins:" and auto-inject (or at worst, emit a clear paste-this-block
// error). A3 (api #539) stripped those blank imports. Nothing else
// populates pluginByType at init(), so without this file the fallback
// becomes functionally dead: pluginByType is empty, ValidatePluginRequirements
// never detects a missing plugin, parse succeeds, and the deploy fails
// much later with an unhelpful "unknown template type".
//
// The seed below declares each canonical plugin and the type tokens its
// /api module registers. Keeping the list in SC core is a small debt
// (SC core "knows about" the plugin set it decoupled from), but the
// alternative is losing the backwards-compatibility error UX for every
// customer whose server.yaml predates the plugin split. The list is
// used ONLY for the "did you forget plugins:?" fallback — once an
// operator declares plugins: in server.yaml, the Schema RPC from the
// declared plugin is authoritative (pkg/stackread.DiscoverPluginTypes)
// and this bootstrap is bypassed.
//
// When adding a new plugin:
//   - If it is one of the five canonical plugins, add its tokens here.
//   - If it is a third-party plugin or a new canonical plugin, prefer
//     teaching operators to declare plugins: and let dynamic discovery
//     via Schema RPC own it. This file is NOT a dependency registry.
func init() {
	RegisterPluginRequirement(PluginRequirement{
		PluginName: "aws",
		OwnedTypes: []string{
			"aws",
			"aws-token",
			"aws-secrets-manager",
			"s3-bucket",
			"ecs-fargate",
			"aws-lambda",
			"aws-static-website",
			"aws-rds-postgres",
			"aws-rds-mysql",
			"aws-cloudtrail-security-alerts",
			"ecr-repository",
			"aws-kms",
		},
		DefaultSource: PluginSource{
			Type:       "github-release",
			Repository: "simple-container-com/sc-plugin-aws",
		},
	})
	RegisterPluginRequirement(PluginRequirement{
		PluginName: "yandex",
		OwnedTypes: []string{
			"yandex",
			"yc-dns",
			"yc-bucket",
			"yc-wildcard-cert",
			"yc-serverless-container",
			"yc-static-website",
			"yc-object-storage",
			"yc-kms",
			"yc-lockbox",
			"yc-service-account",
		},
		DefaultSource: PluginSource{
			Type:       "github-release",
			Repository: "simple-container-com/sc-plugin-yandex",
		},
	})
	RegisterPluginRequirement(PluginRequirement{
		PluginName: "cloudflare",
		OwnedTypes: []string{
			"cloudflare",
			"cloudflare-static-website",
		},
		DefaultSource: PluginSource{
			Type:       "github-release",
			Repository: "simple-container-com/sc-plugin-cloudflare",
		},
	})
	RegisterPluginRequirement(PluginRequirement{
		PluginName: "cloudru",
		OwnedTypes: []string{
			"cloudru",
			"cloudru-dns",
			"cloudru-container-app",
			"cloudru-access-key",
			"cloudru-evolution-key",
			"cloudru-vm",
			"cloudru-evolution-vm",
		},
		DefaultSource: PluginSource{
			Type:       "github-release",
			Repository: "simple-container-com/sc-plugin-cloudru",
		},
	})
	RegisterPluginRequirement(PluginRequirement{
		PluginName: "mongodb",
		OwnedTypes: []string{
			"mongodb-atlas",
		},
		DefaultSource: PluginSource{
			Type:       "github-release",
			Repository: "simple-container-com/sc-plugin-mongodb",
		},
	})
}
