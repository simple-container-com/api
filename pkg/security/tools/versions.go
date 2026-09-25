// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package tools

// Canonical pinned versions for the external security tools SC installs and
// version-gates.
//
// registry.go uses each as both the install target and the minimum acceptable
// version, and pkg/security/scan derives its scanner pins from the same
// constants. Before this was centralized the two disagreed — the registry
// floor sat at grype 0.106.0 / trivy 0.68.2 while the scanners pinned 0.111.0 /
// 0.70.0 — so a bump in one place silently left the other behind.
//
// Bump here to upgrade everywhere. Per-scan overrides remain available via
// ScanToolConfig.Version and the SC_GRYPE_VERSION / SC_TRIVY_VERSION env vars.
const (
	// DefaultCosignVersion is at least 3.0.4, which fixes GHSA-whqx-f9j3-ch6m
	// ("verification accepts any valid Rekor entry under certain conditions").
	DefaultCosignVersion = "3.1.3"
	DefaultSyftVersion   = "1.51.0"
	DefaultGrypeVersion  = "0.117.0"
	DefaultTrivyVersion  = "0.74.0"
)
