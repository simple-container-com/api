// SPDX-License-Identifier: MIT
// Copyright (c) 2025 Simple Container

package security

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/security/scan"
)

// installFakeScanner writes an executable shell stub named `name` to a fresh
// tempdir, prepends that dir to PATH, and returns. The stub responds to
// `<name> version` with a parseable version string so the scanner's
// CheckInstalled / Version succeed WITHOUT a real scanner. Any other
// invocation exits non-zero — tests must arrange a cache hit so the real
// Scan subcommand is never run.
func installFakeScanner(t *testing.T, name string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake-binary PATH harness is POSIX-shell only")
	}
	dir := t.TempDir()
	// grype uses `grype version`; trivy uses `trivy --version`. Handle both so
	// CheckInstalled / Version succeed for either scanner.
	script := "#!/bin/sh\n" +
		"if [ \"$1\" = \"version\" ] || [ \"$1\" = \"--version\" ]; then echo \"Version: v1.2.3\"; exit 0; fi\n" +
		"echo 'fake scanner: unexpected invocation' >&2\n" +
		"exit 3\n"
	bin := filepath.Join(dir, name)
	Expect(os.WriteFile(bin, []byte(script), 0o755)).To(Succeed())
	// Prepend so our stub wins over any real install on the runner.
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// seedScanCache stores a valid signed scan result under the exact key
// runScannerForTool will look up, so the cache-hit branch returns it without
// invoking the (fake) scanner's Scan subcommand.
func seedScanCache(t *testing.T, e *SecurityExecutor, cacheDir string, tool ScanToolConfig, imageRef string, result *scan.ScanResult) {
	t.Helper()
	cache, err := e.newScanCache()
	Expect(err).ToNot(HaveOccurred())
	Expect(cache).ToNot(BeNil())
	Expect(e.saveScanResultToCache(cache, tool, imageRef, result)).To(Succeed())
}

func TestExecuteScanningCacheHitFullPipeline(t *testing.T) {
	RegisterTestingT(t)
	ctx := context.Background()

	installFakeScanner(t, "grype")

	cacheDir := t.TempDir()
	outDir := t.TempDir()
	outPath := filepath.Join(outDir, "scan.json")
	imageRef := "registry.example.com/demo@sha256:abc"
	toolCfg := ScanToolConfig{Name: "grype", Enabled: boolPtr(true)}

	cfg := &SecurityConfig{
		Enabled: true,
		Scan: &ScanConfig{
			Enabled: true,
			FailOn:  SeverityHigh,
			WarnOn:  SeverityLow,
			Output:  &OutputConfig{Local: outPath},
			Cache:   &CacheConfig{Enabled: true, Dir: cacheDir, TTL: "1h"},
			Tools:   []ScanToolConfig{toolCfg},
		},
	}
	e, err := NewSecurityExecutorWithSummary(ctx, cfg, imageRef)
	Expect(err).ToNot(HaveOccurred())

	// Clean result so no policy violation occurs (FailOn high, zero vulns).
	cached := &scan.ScanResult{
		ImageDigest: "sha256:abc",
		Tool:        scan.ScanToolGrype,
		Summary:     scan.VulnerabilitySummary{Total: 0},
	}
	seedScanCache(t, e, cacheDir, toolCfg, imageRef, cached)

	result, err := e.ExecuteScanning(ctx, imageRef)
	Expect(err).ToNot(HaveOccurred())
	Expect(result).ToNot(BeNil())
	Expect(result.Tool).To(Equal(scan.ScanToolGrype))

	// Saved locally (pure save path exercised end-to-end).
	_, statErr := os.Stat(outPath)
	Expect(statErr).ToNot(HaveOccurred())

	// Summary recorded the per-tool scan.
	Expect(e.Summary.ScanResults).ToNot(BeEmpty())
}

func TestExecuteScanningCacheHitPolicyViolation(t *testing.T) {
	RegisterTestingT(t)
	ctx := context.Background()

	installFakeScanner(t, "grype")

	cacheDir := t.TempDir()
	imageRef := "registry.example.com/demo@sha256:def"
	toolCfg := ScanToolConfig{Name: "grype", Enabled: boolPtr(true)}

	cfg := &SecurityConfig{
		Enabled: true,
		Scan: &ScanConfig{
			Enabled: true,
			FailOn:  SeverityHigh,
			Cache:   &CacheConfig{Enabled: true, Dir: cacheDir, TTL: "1h"},
			Tools:   []ScanToolConfig{toolCfg},
		},
	}
	e := newExecutorT(t, cfg)

	// Critical vuln present => failOn:high policy is violated. ExecuteScanning
	// returns the result AND a non-nil policy error (soft, not a hard failure).
	cached := &scan.ScanResult{
		ImageDigest: "sha256:def",
		Tool:        scan.ScanToolGrype,
		Summary:     scan.VulnerabilitySummary{Critical: 1, Total: 1},
		Vulnerabilities: []scan.Vulnerability{
			{ID: "CVE-X", Severity: scan.SeverityCritical, Package: "p", Version: "1"},
		},
	}
	seedScanCache(t, e, cacheDir, toolCfg, imageRef, cached)

	result, policyErr := e.ExecuteScanning(ctx, imageRef)
	Expect(result).ToNot(BeNil())
	Expect(policyErr).To(HaveOccurred())
	Expect(policyErr.Error()).To(ContainSubstring("policy violation"))
}

func TestExecuteScanningCacheReadErrorThenScanFails(t *testing.T) {
	RegisterTestingT(t)
	ctx := context.Background()

	installFakeScanner(t, "grype")

	cacheDir := t.TempDir()
	imageRef := "registry.example.com/demo@sha256:bad0"
	toolCfg := ScanToolConfig{Name: "grype", Enabled: boolPtr(true)}

	cfg := &SecurityConfig{
		Enabled: true,
		Scan: &ScanConfig{
			Enabled:  true,
			Required: false, // tool not required => scan failure is non-fatal
			Cache:    &CacheConfig{Enabled: true, Dir: cacheDir, TTL: "1h"},
			Tools:    []ScanToolConfig{toolCfg},
		},
	}
	e := newExecutorT(t, cfg)

	// Make the cache lookup return a read error (not a miss): plant a directory
	// exactly where the cache file would be. runScannerForTool warns and falls
	// through to a real scan, which the fake stub rejects (exit 3) => the tool
	// outcome errors and, being non-required, ExecuteScanning returns nil,nil.
	cache, err := e.newScanCache()
	Expect(err).ToNot(HaveOccurred())
	key, err := e.scanCacheKey(toolCfg, imageRef)
	Expect(err).ToNot(HaveOccurred())
	Expect(os.MkdirAll(cache.getPath(key), 0o700)).To(Succeed())

	result, err := e.ExecuteScanning(ctx, imageRef)
	Expect(err).ToNot(HaveOccurred())
	Expect(result).To(BeNil())
}

func TestExecuteScanningTwoToolsMergedFromCache(t *testing.T) {
	RegisterTestingT(t)
	ctx := context.Background()

	installFakeScanner(t, "grype")
	installFakeScanner(t, "trivy")

	cacheDir := t.TempDir()
	imageRef := "registry.example.com/demo@sha256:999"
	grypeCfg := ScanToolConfig{Name: "grype", Enabled: boolPtr(true)}
	trivyCfg := ScanToolConfig{Name: "trivy", Enabled: boolPtr(true)}

	cfg := &SecurityConfig{
		Enabled: true,
		Scan: &ScanConfig{
			Enabled: true,
			// No FailOn so the merged-result global policy block is skipped,
			// but tool-level WarnOn still exercises enforceToolPolicy.
			WarnOn: SeverityLow,
			Cache:  &CacheConfig{Enabled: true, Dir: cacheDir, TTL: "1h"},
			Tools:  []ScanToolConfig{grypeCfg, trivyCfg},
		},
	}
	e, err := NewSecurityExecutorWithSummary(ctx, cfg, imageRef)
	Expect(err).ToNot(HaveOccurred())

	grypeResult := &scan.ScanResult{
		ImageDigest: "sha256:999",
		Tool:        scan.ScanToolGrype,
		Summary:     scan.VulnerabilitySummary{Medium: 1, Total: 1},
		Vulnerabilities: []scan.Vulnerability{
			{ID: "CVE-A", Severity: scan.SeverityMedium, Package: "a", Version: "1"},
		},
	}
	trivyResult := &scan.ScanResult{
		ImageDigest: "sha256:999",
		Tool:        scan.ScanToolTrivy,
		Summary:     scan.VulnerabilitySummary{Low: 1, Total: 1},
		Vulnerabilities: []scan.Vulnerability{
			{ID: "CVE-B", Severity: scan.SeverityLow, Package: "b", Version: "2"},
		},
	}
	seedScanCache(t, e, cacheDir, grypeCfg, imageRef, grypeResult)
	seedScanCache(t, e, cacheDir, trivyCfg, imageRef, trivyResult)

	result, err := e.ExecuteScanning(ctx, imageRef)
	Expect(err).ToNot(HaveOccurred())
	Expect(result).ToNot(BeNil())
	// Merged result is tagged ScanToolAll and carries both findings.
	Expect(result.Tool).To(Equal(scan.ScanToolAll))
	Expect(result.Summary.Total).To(Equal(2))

	// Both per-tool scans plus the merged scan are recorded.
	Expect(e.Summary.ScanResults).To(HaveLen(2))
	Expect(e.Summary.MergedResult).ToNot(BeNil())
}
