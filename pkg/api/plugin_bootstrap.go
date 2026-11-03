// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package api

import "strings"

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

	// ALSO populate providerConfigMapping (via RegisterPluginOwnedType)
	// for every bootstrap token, so DetectResourcesType / DetectAuthType /
	// DetectTemplateType / DetectRegistrarType / ReadProvisionerFieldConfig
	// resolve plugin-owned types without needing a successful spawn of
	// stackread.DiscoverPluginTypes first. RegisterPluginRequirement
	// above populates pluginByType (for the error-hint and for
	// ValidatePluginRequirements's "did you declare plugins?" check);
	// this line populates the type-resolution maps the full parse path
	// actually reads from.
	//
	// Live-caught on forge-runtime isolated verify run 41711977418
	// (2026-11-03): walker-diag showed `${auth:aws}` entering and leaving
	// the walker unchanged. Root cause: DetectAuthProvider looked up
	// `aws-token` in providerConfigMapping, found nothing (bootstrap only
	// seeded pluginByType), failed with "unknown auth type", and
	// ReadSecretsDescriptor silently dropped the whole secrets.yaml
	// (IgnoreSecretsMissing=true in the deploy ReadOpts). That left
	// stack.Secrets.Auth["aws"] empty, so tplAuth returned the input
	// placeholder unchanged, and the plugin received `${auth:aws}` as a
	// literal.
	//
	// Kinds are approximate — every type is registered as Resource because
	// RegisterPluginOwnedType's current behaviour is to seed
	// providerConfigMapping for all kinds alike, and the kind field is
	// only read by the auth / state-storage / secrets-provider
	// post-processing in RegisterPluginOwnedType itself. If a future kind
	// gains distinct behaviour, this list needs to be re-categorised. The
	// state-storage and secrets-provider tokens below are tagged with
	// their real kind so `provisionerFieldConfigMapping` gets populated
	// via the kind-gated branch.
	for pluginName, kinds := range map[string]map[PluginOwnedKind][]string{
		"aws": {
			PluginOwnedKindResource:        {"aws", "ecs-fargate", "aws-lambda", "aws-static-website", "aws-rds-postgres", "aws-rds-mysql", "aws-cloudtrail-security-alerts", "ecr-repository"},
			PluginOwnedKindStateStore:      {"s3-bucket"},
			PluginOwnedKindSecretsProvider: {"aws-kms", "aws-secrets-manager"},
			// Auth tokens MUST be registered too — see the comment
			// above for the forge-runtime regression.
			PluginOwnedKindResource + "/auth": {"aws-token"},
		},
		"yandex": {
			PluginOwnedKindResource:           {"yandex", "yc-bucket", "yc-wildcard-cert", "yc-serverless-container", "yc-static-website", "yc-lockbox"},
			PluginOwnedKindRegistrar:          {"yc-dns"},
			PluginOwnedKindStateStore:         {"yc-object-storage"},
			PluginOwnedKindSecretsProvider:    {"yc-kms"},
			PluginOwnedKindResource + "/auth": {"yc-service-account"},
		},
		"cloudflare": {
			PluginOwnedKindResource:  {"cloudflare-static-website"},
			PluginOwnedKindRegistrar: {"cloudflare"},
		},
		"cloudru": {
			PluginOwnedKindResource:           {"cloudru", "cloudru-container-app", "cloudru-vm", "cloudru-evolution-vm"},
			PluginOwnedKindRegistrar:          {"cloudru-dns"},
			PluginOwnedKindResource + "/auth": {"cloudru-access-key", "cloudru-evolution-key"},
		},
		"mongodb": {
			PluginOwnedKindResource: {"mongodb-atlas"},
		},
	} {
		for kind, tokens := range kinds {
			// Strip the "/auth" sentinel — the kind for auth is
			// Resource as far as RegisterPluginOwnedType cares
			// (providerConfigMapping gets seeded regardless). The
			// sentinel is only here so the grouping stays readable.
			realKind := kind
			if idx := strings.Index(string(kind), "/"); idx >= 0 {
				realKind = PluginOwnedKind(string(kind)[:idx])
			}
			for _, t := range tokens {
				RegisterPluginOwnedType(t, realKind, pluginName)
			}
		}
	}
}
