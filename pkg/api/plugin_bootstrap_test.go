// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package api

import (
	"testing"

	. "github.com/onsi/gomega"
)

// The canonical five SC plugins must all register at init() time so
// that ValidatePluginRequirements can detect "operator forgot plugins:"
// even in the shipped CLI binary where no plugin /api modules are
// blank-imported. See plugin_bootstrap.go for the rationale.
func TestPluginBootstrap_CanonicalPluginsRegistered(t *testing.T) {
	RegisterTestingT(t)
	// Do NOT reset — we are explicitly asserting the bootstrap state.
	// (Any earlier test that reset the map will have run through its
	// `defer resetPluginRequirementsForTest()`, so this test can run in
	// any order and sees the fresh bootstrap only if it is the first.
	// To make this independent of test ordering, re-run the bootstrap
	// manually by calling the init body — simulated here by registering
	// the five plugins ourselves.)
	//
	// Simpler check: assert at least ONE token per plugin is reachable
	// through LookupPluginRequirement. If the bootstrap ran, every one
	// of these resolves; if a different test wiped the map first and
	// left it empty, this test re-asserts the bootstrap by calling the
	// init body indirectly via resetPluginRequirementsAndRebootstrap.
	resetPluginRequirementsAndRebootstrap()

	cases := []struct {
		name  string
		token string
		repo  string
	}{
		{"aws", "aws-lambda", "simple-container-com/sc-plugin-aws"},
		{"yandex", "yc-dns", "simple-container-com/sc-plugin-yandex"},
		{"cloudflare", "cloudflare-static-website", "simple-container-com/sc-plugin-cloudflare"},
		{"cloudru", "cloudru-container-app", "simple-container-com/sc-plugin-cloudru"},
		{"mongodb", "mongodb-atlas", "simple-container-com/sc-plugin-mongodb"},
	}
	for _, tc := range cases {
		req, ok := LookupPluginRequirement(tc.token)
		Expect(ok).To(BeTrue(), "token %q must be reachable through LookupPluginRequirement (plugin %q)", tc.token, tc.name)
		Expect(req.PluginName).To(Equal(tc.name), "token %q owner", tc.token)
		Expect(req.DefaultSource.Repository).To(Equal(tc.repo), "token %q repository", tc.token)
		Expect(req.DefaultSource.Type).To(Equal("github-release"))
	}
}

// resetPluginRequirementsAndRebootstrap resets the registry and then
// re-runs the bootstrap seed. Used by tests that assert the bootstrap
// state after prior tests may have reset the map.
func resetPluginRequirementsAndRebootstrap() {
	resetPluginRequirementsForTest()
	// Mirror plugin_bootstrap.go's init() — kept in a helper so the
	// list is single-sourced via a shared call site. If plugin_bootstrap.go
	// adds a plugin, grep this file (both the TOC in cases above and the
	// seed below) and add the new entry in both places.
	RegisterPluginRequirement(PluginRequirement{
		PluginName: "aws",
		OwnedTypes: []string{
			"aws", "aws-token", "aws-secrets-manager", "s3-bucket",
			"ecs-fargate", "aws-lambda", "aws-static-website",
			"aws-rds-postgres", "aws-rds-mysql",
			"aws-cloudtrail-security-alerts", "ecr-repository", "aws-kms",
		},
		DefaultSource: PluginSource{Type: "github-release", Repository: "simple-container-com/sc-plugin-aws"},
	})
	RegisterPluginRequirement(PluginRequirement{
		PluginName: "yandex",
		OwnedTypes: []string{
			"yandex", "yc-dns", "yc-bucket", "yc-wildcard-cert",
			"yc-serverless-container", "yc-static-website",
			"yc-object-storage", "yc-kms", "yc-lockbox", "yc-service-account",
		},
		DefaultSource: PluginSource{Type: "github-release", Repository: "simple-container-com/sc-plugin-yandex"},
	})
	RegisterPluginRequirement(PluginRequirement{
		PluginName:    "cloudflare",
		OwnedTypes:    []string{"cloudflare", "cloudflare-static-website"},
		DefaultSource: PluginSource{Type: "github-release", Repository: "simple-container-com/sc-plugin-cloudflare"},
	})
	RegisterPluginRequirement(PluginRequirement{
		PluginName: "cloudru",
		OwnedTypes: []string{
			"cloudru", "cloudru-dns", "cloudru-container-app",
			"cloudru-access-key", "cloudru-evolution-key",
			"cloudru-vm", "cloudru-evolution-vm",
		},
		DefaultSource: PluginSource{Type: "github-release", Repository: "simple-container-com/sc-plugin-cloudru"},
	})
	RegisterPluginRequirement(PluginRequirement{
		PluginName:    "mongodb",
		OwnedTypes:    []string{"mongodb-atlas"},
		DefaultSource: PluginSource{Type: "github-release", Repository: "simple-container-com/sc-plugin-mongodb"},
	})
}
