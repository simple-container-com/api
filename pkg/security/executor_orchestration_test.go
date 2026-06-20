// SPDX-License-Identifier: MIT
// Copyright (c) 2025 Simple Container

package security

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/security/sbom"
	"github.com/simple-container-com/api/pkg/security/scan"
)

// ---- ExecuteScanning: non-exec branches ----

func TestExecuteScanningDisabledAndInvalidRef(t *testing.T) {
	RegisterTestingT(t)
	ctx := context.Background()

	t.Run("security disabled returns nil,nil", func(t *testing.T) {
		RegisterTestingT(t)
		e := newExecutorT(t, &SecurityConfig{Enabled: false})
		res, err := e.ExecuteScanning(ctx, "img:tag")
		Expect(err).ToNot(HaveOccurred())
		Expect(res).To(BeNil())
	})

	t.Run("scan disabled returns nil,nil", func(t *testing.T) {
		RegisterTestingT(t)
		e := newExecutorT(t, &SecurityConfig{Enabled: true, Scan: &ScanConfig{Enabled: false}})
		res, err := e.ExecuteScanning(ctx, "img:tag")
		Expect(err).ToNot(HaveOccurred())
		Expect(res).To(BeNil())
	})

	t.Run("nil scan returns nil,nil", func(t *testing.T) {
		RegisterTestingT(t)
		e := newExecutorT(t, &SecurityConfig{Enabled: true})
		res, err := e.ExecuteScanning(ctx, "img:tag")
		Expect(err).ToNot(HaveOccurred())
		Expect(res).To(BeNil())
	})

	t.Run("invalid image ref errors", func(t *testing.T) {
		RegisterTestingT(t)
		e := newExecutorT(t, &SecurityConfig{
			Enabled: true,
			Scan:    &ScanConfig{Enabled: true, Tools: []ScanToolConfig{{Name: "grype"}}},
		})
		_, err := e.ExecuteScanning(ctx, "--flaglike")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("invalid image ref"))
	})
}

func TestExecuteScanningValidationFailOpen(t *testing.T) {
	RegisterTestingT(t)
	ctx := context.Background()

	// Scan enabled but no tools -> Validate() fails. Required=false => fail-open nil,nil.
	e := newExecutorT(t, &SecurityConfig{
		Enabled: true,
		Scan:    &ScanConfig{Enabled: true, Required: false, Tools: nil},
	})
	res, err := e.ExecuteScanning(ctx, "registry.example.com/demo:tag")
	Expect(err).ToNot(HaveOccurred())
	Expect(res).To(BeNil())
}

func TestExecuteScanningValidationFailClosed(t *testing.T) {
	RegisterTestingT(t)
	ctx := context.Background()

	// Scan enabled, no tools, Required=true => hard error from Validate.
	e := newExecutorT(t, &SecurityConfig{
		Enabled: true,
		Scan:    &ScanConfig{Enabled: true, Required: true, Tools: nil},
	})
	_, err := e.ExecuteScanning(ctx, "registry.example.com/demo:tag")
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("scan validation failed"))
}

func TestExecuteScanningNoEnabledTools(t *testing.T) {
	RegisterTestingT(t)
	ctx := context.Background()

	// Config validates (a named tool exists) but all tools are disabled, so
	// enabledScanTools() is empty. Validate() passes because it does not call
	// scanToolEnabled. Required=false => warning + nil,nil.
	e := newExecutorT(t, &SecurityConfig{
		Enabled: true,
		Scan: &ScanConfig{
			Enabled:  true,
			Required: false,
			Tools:    []ScanToolConfig{{Name: "grype", Enabled: boolPtr(false)}},
		},
	})
	res, err := e.ExecuteScanning(ctx, "registry.example.com/demo:tag")
	Expect(err).ToNot(HaveOccurred())
	Expect(res).To(BeNil())
}

func TestExecuteScanningNoEnabledToolsRequired(t *testing.T) {
	RegisterTestingT(t)
	ctx := context.Background()

	e := newExecutorT(t, &SecurityConfig{
		Enabled: true,
		Scan: &ScanConfig{
			Enabled:  true,
			Required: true,
			Tools:    []ScanToolConfig{{Name: "grype", Enabled: boolPtr(false)}},
		},
	})
	_, err := e.ExecuteScanning(ctx, "registry.example.com/demo:tag")
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("no enabled scan tools"))
}

// ---- ExecuteSBOM: non-exec branches ----

func TestExecuteSBOMDisabledAndInvalidRef(t *testing.T) {
	RegisterTestingT(t)
	ctx := context.Background()

	t.Run("security disabled", func(t *testing.T) {
		RegisterTestingT(t)
		e := newExecutorT(t, &SecurityConfig{Enabled: false})
		res, err := e.ExecuteSBOM(ctx, "img:tag")
		Expect(err).ToNot(HaveOccurred())
		Expect(res).To(BeNil())
	})

	t.Run("sbom disabled", func(t *testing.T) {
		RegisterTestingT(t)
		e := newExecutorT(t, &SecurityConfig{Enabled: true, SBOM: &SBOMConfig{Enabled: false}})
		res, err := e.ExecuteSBOM(ctx, "img:tag")
		Expect(err).ToNot(HaveOccurred())
		Expect(res).To(BeNil())
	})

	t.Run("invalid ref", func(t *testing.T) {
		RegisterTestingT(t)
		e := newExecutorT(t, &SecurityConfig{Enabled: true, SBOM: &SBOMConfig{Enabled: true}})
		_, err := e.ExecuteSBOM(ctx, "-bad")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("invalid image ref"))
	})
}

func TestExecuteSBOMValidationFailOpen(t *testing.T) {
	RegisterTestingT(t)
	ctx := context.Background()

	// Invalid format => Validate fails; Required=false => fail-open nil,nil.
	e, err := NewSecurityExecutorWithSummary(ctx, &SecurityConfig{
		Enabled: true,
		SBOM:    &SBOMConfig{Enabled: true, Required: false, Format: "bogus-format"},
	}, "img:tag")
	Expect(err).ToNot(HaveOccurred())

	res, err := e.ExecuteSBOM(ctx, "registry.example.com/demo:tag")
	Expect(err).ToNot(HaveOccurred())
	Expect(res).To(BeNil())
	// Fail-open path records an SBOM error in the summary.
	Expect(e.Summary.SBOMResult).ToNot(BeNil())
}

func TestExecuteSBOMValidationFailClosed(t *testing.T) {
	RegisterTestingT(t)
	ctx := context.Background()

	e := newExecutorT(t, &SecurityConfig{
		Enabled: true,
		SBOM:    &SBOMConfig{Enabled: true, Required: true, Format: "bogus-format"},
	})
	_, err := e.ExecuteSBOM(ctx, "registry.example.com/demo:tag")
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("SBOM validation failed"))
}

// seedSBOMCache stores a valid signed SBOM entry under the exact key
// ExecuteSBOM will look up, so the cache-hit branch is exercised without
// shelling out to syft.
func seedSBOMCache(t *testing.T, e *SecurityExecutor, imageRef string, obj *sbom.SBOM) {
	t.Helper()
	cache, err := e.newSBOMCache()
	Expect(err).ToNot(HaveOccurred())
	Expect(cache).ToNot(BeNil())
	key, err := e.sbomCacheKey(imageRef)
	Expect(err).ToNot(HaveOccurred())
	data, err := json.Marshal(obj)
	Expect(err).ToNot(HaveOccurred())
	Expect(cache.Set(key, data)).To(Succeed())
}

func TestExecuteSBOMCacheHitSavesLocal(t *testing.T) {
	RegisterTestingT(t)
	ctx := context.Background()

	dir := t.TempDir()
	cacheDir := filepath.Join(dir, "cache")
	outPath := filepath.Join(dir, "out", "sbom.json")
	imageRef := "registry.example.com/demo@sha256:cafe"

	cfg := &SecurityConfig{
		Enabled: true,
		SBOM: &SBOMConfig{
			Enabled:   true,
			Format:    "cyclonedx-json",
			Generator: "syft",
			Cache:     &CacheConfig{Enabled: true, Dir: cacheDir, TTL: "1h"},
			Output:    &OutputConfig{Local: outPath},
			// No attach + no registry => ShouldAttach()=false => no cosign exec.
		},
	}
	e, err := NewSecurityExecutorWithSummary(ctx, cfg, imageRef)
	Expect(err).ToNot(HaveOccurred())

	cached := &sbom.SBOM{
		Format:      sbom.FormatCycloneDXJSON,
		Content:     []byte(`{"bomFormat":"CycloneDX","components":[]}`),
		ImageDigest: "sha256:cafe",
		GeneratedAt: time.Now(),
		Metadata:    &sbom.Metadata{ToolName: "syft", PackageCount: 0},
	}
	seedSBOMCache(t, e, imageRef, cached)

	res, err := e.ExecuteSBOM(ctx, imageRef)
	Expect(err).ToNot(HaveOccurred())
	Expect(res).ToNot(BeNil())
	Expect(res.ImageDigest).To(Equal("sha256:cafe"))

	// Cache hit wrote the SBOM content to the configured local path.
	written, err := os.ReadFile(outPath)
	Expect(err).ToNot(HaveOccurred())
	Expect(string(written)).To(ContainSubstring("CycloneDX"))

	// Summary recorded the cache-hit SBOM with the output path.
	Expect(e.Summary.SBOMResult).ToNot(BeNil())
}

func TestExecuteSBOMCacheHitNoOutputNoAttach(t *testing.T) {
	RegisterTestingT(t)
	ctx := context.Background()

	cacheDir := t.TempDir()
	imageRef := "registry.example.com/demo@sha256:feed"

	cfg := &SecurityConfig{
		Enabled: true,
		SBOM: &SBOMConfig{
			Enabled:   true,
			Format:    "cyclonedx-json",
			Generator: "syft",
			Cache:     &CacheConfig{Enabled: true, Dir: cacheDir, TTL: "1h"},
			// No Output, no Attach => cache-hit just records summary and returns.
		},
	}
	e, err := NewSecurityExecutorWithSummary(ctx, cfg, imageRef)
	Expect(err).ToNot(HaveOccurred())

	cached := &sbom.SBOM{
		Format:      sbom.FormatCycloneDXJSON,
		Content:     []byte(`{"bomFormat":"CycloneDX"}`),
		ImageDigest: "sha256:feed",
		GeneratedAt: time.Now(),
		Metadata:    &sbom.Metadata{ToolName: "syft", PackageCount: 5},
	}
	seedSBOMCache(t, e, imageRef, cached)

	res, err := e.ExecuteSBOM(ctx, imageRef)
	Expect(err).ToNot(HaveOccurred())
	Expect(res).ToNot(BeNil())
	Expect(e.Summary.SBOMResult).ToNot(BeNil())
	Expect(e.Summary.SBOMResult.PackageCount).To(Equal(5))
}

func TestExecuteSBOMCacheHitSaveLocalError(t *testing.T) {
	RegisterTestingT(t)
	ctx := context.Background()

	cacheDir := t.TempDir()
	// Block the output directory so saveSBOMLocal's MkdirAll fails. The
	// cached-SBOM save-local error is always fatal (no Required gate on that
	// specific branch).
	blockerDir := t.TempDir()
	blocker := filepath.Join(blockerDir, "blocker")
	Expect(os.WriteFile(blocker, []byte("x"), 0o600)).To(Succeed())
	outPath := filepath.Join(blocker, "child", "sbom.json")
	imageRef := "registry.example.com/demo@sha256:f00d"

	cfg := &SecurityConfig{
		Enabled: true,
		SBOM: &SBOMConfig{
			Enabled:   true,
			Format:    "cyclonedx-json",
			Generator: "syft",
			Cache:     &CacheConfig{Enabled: true, Dir: cacheDir, TTL: "1h"},
			Output:    &OutputConfig{Local: outPath},
		},
	}
	e := newExecutorT(t, cfg)

	cached := &sbom.SBOM{
		Format:      sbom.FormatCycloneDXJSON,
		Content:     []byte(`{"bomFormat":"CycloneDX"}`),
		ImageDigest: "sha256:f00d",
		GeneratedAt: time.Now(),
		Metadata:    &sbom.Metadata{ToolName: "syft"},
	}
	seedSBOMCache(t, e, imageRef, cached)

	_, err := e.ExecuteSBOM(ctx, imageRef)
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("saving cached SBOM locally"))
}

func TestExecuteSBOMCacheHitAttachRequiresSigning(t *testing.T) {
	RegisterTestingT(t)
	ctx := context.Background()

	cacheDir := t.TempDir()
	imageRef := "registry.example.com/demo@sha256:beef"

	cfg := &SecurityConfig{
		Enabled: true,
		SBOM: &SBOMConfig{
			Enabled:   true,
			Format:    "cyclonedx-json",
			Generator: "syft",
			Required:  false, // fail-open on the signing-required warning
			Cache:     &CacheConfig{Enabled: true, Dir: cacheDir, TTL: "1h"},
			// Registry output forces ShouldAttach()=true, but Signing is nil
			// => the attach branch warns and continues (no cosign exec).
			Output: &OutputConfig{Registry: true},
			Attach: &AttachConfig{Enabled: true, Sign: true},
		},
	}
	e := newExecutorT(t, cfg)

	cached := &sbom.SBOM{
		Format:      sbom.FormatCycloneDXJSON,
		Content:     []byte(`{"bomFormat":"CycloneDX"}`),
		ImageDigest: "sha256:beef",
		GeneratedAt: time.Now(),
		Metadata:    &sbom.Metadata{ToolName: "syft"},
	}
	seedSBOMCache(t, e, imageRef, cached)

	res, err := e.ExecuteSBOM(ctx, imageRef)
	Expect(err).ToNot(HaveOccurred())
	Expect(res).ToNot(BeNil())
}

func TestExecuteSBOMCacheHitAttachRequiresSigningFailClosed(t *testing.T) {
	RegisterTestingT(t)
	ctx := context.Background()

	cacheDir := t.TempDir()
	imageRef := "registry.example.com/demo@sha256:dead"

	cfg := &SecurityConfig{
		Enabled: true,
		SBOM: &SBOMConfig{
			Enabled:   true,
			Format:    "cyclonedx-json",
			Generator: "syft",
			Required:  true, // signing-required becomes a hard error
			Cache:     &CacheConfig{Enabled: true, Dir: cacheDir, TTL: "1h"},
			Output:    &OutputConfig{Registry: true},
			Attach:    &AttachConfig{Enabled: true, Sign: true},
		},
	}
	e := newExecutorT(t, cfg)

	cached := &sbom.SBOM{
		Format:      sbom.FormatCycloneDXJSON,
		Content:     []byte(`{"bomFormat":"CycloneDX"}`),
		ImageDigest: "sha256:dead",
		GeneratedAt: time.Now(),
		Metadata:    &sbom.Metadata{ToolName: "syft"},
	}
	seedSBOMCache(t, e, imageRef, cached)

	_, err := e.ExecuteSBOM(ctx, imageRef)
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("signing.enabled"))
}

func TestSaveSBOMLocal(t *testing.T) {
	RegisterTestingT(t)

	dir := t.TempDir()
	outPath := filepath.Join(dir, "deep", "sbom.json")
	e := newExecutorT(t, &SecurityConfig{
		Enabled: true,
		SBOM:    &SBOMConfig{Enabled: true, Output: &OutputConfig{Local: outPath}},
	})

	obj := &sbom.SBOM{Content: []byte("sbom-bytes")}
	Expect(e.saveSBOMLocal(obj)).To(Succeed())

	info, err := os.Stat(outPath)
	Expect(err).ToNot(HaveOccurred())
	Expect(info.Mode().Perm()).To(Equal(os.FileMode(0o600)))
	data, err := os.ReadFile(outPath)
	Expect(err).ToNot(HaveOccurred())
	Expect(string(data)).To(Equal("sbom-bytes"))
}

func TestSBOMCacheRoundTrip(t *testing.T) {
	RegisterTestingT(t)

	dir := t.TempDir()
	e := newExecutorT(t, &SecurityConfig{
		Enabled: true,
		SBOM:    &SBOMConfig{Enabled: true, Format: "cyclonedx-json", Generator: "syft", Cache: &CacheConfig{Enabled: true, Dir: dir, TTL: "1h"}},
	})
	cache, err := e.newSBOMCache()
	Expect(err).ToNot(HaveOccurred())

	imageRef := "registry.example.com/demo@sha256:1111"
	obj := &sbom.SBOM{Format: sbom.FormatCycloneDXJSON, Content: []byte("x"), ImageDigest: "sha256:1111"}

	_, found, err := e.loadSBOMFromCache(cache, imageRef)
	Expect(err).ToNot(HaveOccurred())
	Expect(found).To(BeFalse())

	Expect(e.saveSBOMToCache(cache, imageRef, obj)).To(Succeed())

	loaded, found, err := e.loadSBOMFromCache(cache, imageRef)
	Expect(err).ToNot(HaveOccurred())
	Expect(found).To(BeTrue())
	Expect(loaded.ImageDigest).To(Equal("sha256:1111"))
}

func TestLoadSBOMFromCacheCorrupt(t *testing.T) {
	RegisterTestingT(t)

	dir := t.TempDir()
	e := newExecutorT(t, &SecurityConfig{
		Enabled: true,
		SBOM:    &SBOMConfig{Enabled: true, Format: "cyclonedx-json", Generator: "syft", Cache: &CacheConfig{Enabled: true, Dir: dir, TTL: "1h"}},
	})
	cache, err := e.newSBOMCache()
	Expect(err).ToNot(HaveOccurred())

	imageRef := "registry.example.com/demo@sha256:2222"
	key, err := e.sbomCacheKey(imageRef)
	Expect(err).ToNot(HaveOccurred())
	// Valid HMAC envelope but JSON that does not unmarshal into sbom.SBOM.
	Expect(cache.Set(key, []byte(`12345`))).To(Succeed())

	_, _, err = e.loadSBOMFromCache(cache, imageRef)
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("unmarshaling cached SBOM"))
}

// ---- ExecuteProvenance: non-exec branches ----

func TestExecuteProvenanceDisabledAndInvalidRef(t *testing.T) {
	RegisterTestingT(t)
	ctx := context.Background()

	t.Run("security disabled", func(t *testing.T) {
		RegisterTestingT(t)
		e := newExecutorT(t, &SecurityConfig{Enabled: false})
		res, err := e.ExecuteProvenance(ctx, "img:tag")
		Expect(err).ToNot(HaveOccurred())
		Expect(res).To(BeNil())
	})

	t.Run("provenance disabled", func(t *testing.T) {
		RegisterTestingT(t)
		e := newExecutorT(t, &SecurityConfig{Enabled: true, Provenance: &ProvenanceConfig{Enabled: false}})
		res, err := e.ExecuteProvenance(ctx, "img:tag")
		Expect(err).ToNot(HaveOccurred())
		Expect(res).To(BeNil())
	})

	t.Run("invalid ref", func(t *testing.T) {
		RegisterTestingT(t)
		e := newExecutorT(t, &SecurityConfig{Enabled: true, Provenance: &ProvenanceConfig{Enabled: true}})
		_, err := e.ExecuteProvenance(ctx, "-bad")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("invalid image ref"))
	})
}

func TestExecuteProvenanceValidationFailClosed(t *testing.T) {
	RegisterTestingT(t)
	ctx := context.Background()

	e := newExecutorT(t, &SecurityConfig{
		Enabled:    true,
		Provenance: &ProvenanceConfig{Enabled: true, Required: true, Format: "slsa-v0.2"},
	})
	_, err := e.ExecuteProvenance(ctx, "registry.example.com/demo:tag")
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("provenance validation failed"))
}

func TestExecuteProvenanceValidationFailOpen(t *testing.T) {
	RegisterTestingT(t)
	ctx := context.Background()

	e, err := NewSecurityExecutorWithSummary(ctx, &SecurityConfig{
		Enabled:    true,
		Provenance: &ProvenanceConfig{Enabled: true, Required: false, Format: "slsa-v0.2"},
	}, "img:tag")
	Expect(err).ToNot(HaveOccurred())

	res, err := e.ExecuteProvenance(ctx, "registry.example.com/demo:tag")
	Expect(err).ToNot(HaveOccurred())
	Expect(res).To(BeNil())
	Expect(e.Summary.ProvenanceResult).ToNot(BeNil())
}

// ---- UploadReports ----

func TestUploadReportsNoReporting(t *testing.T) {
	RegisterTestingT(t)
	e := newExecutorT(t, &SecurityConfig{Enabled: true})
	Expect(e.UploadReports(context.Background(), &scan.ScanResult{}, "img:tag")).To(Succeed())
}

func TestUploadReportsNilResultSkipsPRComment(t *testing.T) {
	RegisterTestingT(t)
	dir := t.TempDir()
	outPath := filepath.Join(dir, "comment.md")

	e := newExecutorT(t, &SecurityConfig{
		Enabled: true,
		Reporting: &ReportingConfig{
			PRComment: &PRCommentConfig{Enabled: true, Output: outPath},
		},
	})
	// nil result => PR comment branch is skipped, no file written.
	Expect(e.UploadReports(context.Background(), nil, "img:tag")).To(Succeed())
	_, err := os.Stat(outPath)
	Expect(os.IsNotExist(err)).To(BeTrue())
}

func TestWritePRCommentEmptyOutputPathIsNoOp(t *testing.T) {
	RegisterTestingT(t)
	e := newExecutorT(t, &SecurityConfig{
		Enabled: true,
		Reporting: &ReportingConfig{
			PRComment: &PRCommentConfig{Enabled: true, Output: ""},
		},
	})
	result := &scan.ScanResult{Tool: scan.ScanToolGrype}
	Expect(e.writePRComment(result, "img:tag")).To(Succeed())
}

// makeFileWhereDirExpected creates a regular file at parent and returns a path
// "<parent>/child" — os.MkdirAll on dir(<parent>/child) then fails with ENOTDIR
// because <parent> is not a directory.
func makeFileWhereDirExpected(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	blocker := dir + "/notadir"
	Expect(os.WriteFile(blocker, []byte("x"), 0o600)).To(Succeed())
	return blocker + "/child.out"
}

func TestSaveScanLocalMkdirError(t *testing.T) {
	RegisterTestingT(t)
	outPath := makeFileWhereDirExpected(t)
	e := newExecutorT(t, &SecurityConfig{
		Enabled: true,
		Scan:    &ScanConfig{Enabled: true, Output: &OutputConfig{Local: outPath}, Tools: []ScanToolConfig{{Name: "grype"}}},
	})
	err := e.saveScanLocal(&scan.ScanResult{Tool: scan.ScanToolGrype})
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("creating output directory"))
}

func TestSaveSBOMLocalMkdirError(t *testing.T) {
	RegisterTestingT(t)
	outPath := makeFileWhereDirExpected(t)
	e := newExecutorT(t, &SecurityConfig{
		Enabled: true,
		SBOM:    &SBOMConfig{Enabled: true, Output: &OutputConfig{Local: outPath}},
	})
	err := e.saveSBOMLocal(&sbom.SBOM{Content: []byte("x")})
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("creating output directory"))
}

func TestWritePRCommentMkdirError(t *testing.T) {
	RegisterTestingT(t)
	outPath := makeFileWhereDirExpected(t)
	e := newExecutorT(t, &SecurityConfig{
		Enabled: true,
		Reporting: &ReportingConfig{
			PRComment: &PRCommentConfig{Enabled: true, Output: outPath},
		},
	})
	err := e.writePRComment(&scan.ScanResult{Tool: scan.ScanToolGrype}, "img:tag")
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("creating PR comment output directory"))
}

func TestWritePRCommentWritesFile(t *testing.T) {
	RegisterTestingT(t)
	dir := t.TempDir()
	outPath := filepath.Join(dir, "nested", "comment.md")
	e := newExecutorT(t, &SecurityConfig{
		Enabled: true,
		Reporting: &ReportingConfig{
			PRComment: &PRCommentConfig{Enabled: true, Output: outPath},
		},
	})
	result := &scan.ScanResult{
		Tool:        scan.ScanToolGrype,
		ImageDigest: "sha256:abc",
		Summary:     scan.VulnerabilitySummary{High: 1, Total: 1},
		Vulnerabilities: []scan.Vulnerability{
			{ID: "CVE-1", Severity: scan.SeverityHigh, Package: "p", Version: "1"},
		},
	}
	Expect(e.writePRComment(result, "registry/img@sha256:abc")).To(Succeed())

	data, err := os.ReadFile(outPath)
	Expect(err).ToNot(HaveOccurred())
	info, err := os.Stat(outPath)
	Expect(err).ToNot(HaveOccurred())
	Expect(info.Mode().Perm()).To(Equal(os.FileMode(0o600)))
	Expect(string(data)).To(ContainSubstring("Image Scan Results"))
}
