package security

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/security/scan"
)

// newExecutorT builds an executor for a config, failing the test on error.
func newExecutorT(t *testing.T, config *SecurityConfig) *SecurityExecutor {
	t.Helper()
	executor, err := NewSecurityExecutor(context.Background(), config)
	Expect(err).ToNot(HaveOccurred())
	return executor
}

func TestNormalizedScanToolName(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name string
		in   string
		want scan.ScanTool
	}{
		{"grype passes through", "grype", scan.ScanToolGrype},
		{"trivy passes through", "trivy", scan.ScanToolTrivy},
		{"all maps to grype", "all", scan.ScanToolGrype},
		{"unknown passes through verbatim", "snyk", scan.ScanTool("snyk")},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			Expect(normalizedScanToolName(tc.in)).To(Equal(tc.want))
		})
	}
}

func TestScanToolEnabled(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name string
		tool ScanToolConfig
		want bool
	}{
		{"explicit enabled true", ScanToolConfig{Name: "grype", Enabled: boolPtr(true)}, true},
		{"explicit enabled false overrides everything", ScanToolConfig{Name: "grype", Required: true, Enabled: boolPtr(false)}, false},
		{"required implies enabled when nil ptr", ScanToolConfig{Name: "", Required: true}, true},
		{"failOn implies enabled when nil ptr", ScanToolConfig{Name: "", FailOn: SeverityHigh}, true},
		{"warnOn implies enabled when nil ptr", ScanToolConfig{Name: "", WarnOn: SeverityLow}, true},
		{"name implies enabled when nil ptr", ScanToolConfig{Name: "grype"}, true},
		{"empty everything is disabled", ScanToolConfig{Name: ""}, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			Expect(scanToolEnabled(tc.tool)).To(Equal(tc.want))
		})
	}
}

func TestEnabledScanTools(t *testing.T) {
	RegisterTestingT(t)

	t.Run("nil scan config yields nil", func(t *testing.T) {
		RegisterTestingT(t)
		e := newExecutorT(t, &SecurityConfig{Enabled: true})
		Expect(e.enabledScanTools()).To(BeNil())
	})

	t.Run("filters disabled and unnamed tools", func(t *testing.T) {
		RegisterTestingT(t)
		e := newExecutorT(t, &SecurityConfig{
			Enabled: true,
			Scan: &ScanConfig{
				Enabled: true,
				Tools: []ScanToolConfig{
					{Name: "grype", Enabled: boolPtr(true)},
					{Name: "trivy", Enabled: boolPtr(false)}, // disabled
					{Name: ""},                               // unnamed -> skipped
					{Name: "grype", Enabled: boolPtr(true)},  // second enabled
				},
			},
		})
		tools := e.enabledScanTools()
		Expect(tools).To(HaveLen(2))
		Expect(tools[0].Name).To(Equal("grype"))
		Expect(tools[1].Name).To(Equal("grype"))
	})
}

func TestIsScanToolRequired(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name        string
		scanReq     bool
		toolReq     bool
		wantReq     bool
	}{
		{"neither required", false, false, false},
		{"scan-level required", true, false, true},
		{"tool-level required", false, true, true},
		{"both required", true, true, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			e := newExecutorT(t, &SecurityConfig{
				Enabled: true,
				Scan:    &ScanConfig{Enabled: true, Required: tc.scanReq, Tools: []ScanToolConfig{{Name: "grype"}}},
			})
			Expect(e.isScanToolRequired(ScanToolConfig{Name: "grype", Required: tc.toolReq})).To(Equal(tc.wantReq))
		})
	}
}

func TestGetScanOutputPathAndShouldSave(t *testing.T) {
	RegisterTestingT(t)

	t.Run("nil scan config", func(t *testing.T) {
		RegisterTestingT(t)
		e := newExecutorT(t, &SecurityConfig{Enabled: true})
		Expect(e.getScanOutputPath()).To(Equal(""))
		Expect(e.shouldSaveScanLocal()).To(BeFalse())
	})

	t.Run("nil output config", func(t *testing.T) {
		RegisterTestingT(t)
		e := newExecutorT(t, &SecurityConfig{Enabled: true, Scan: &ScanConfig{Enabled: true, Tools: []ScanToolConfig{{Name: "grype"}}}})
		Expect(e.getScanOutputPath()).To(Equal(""))
		Expect(e.shouldSaveScanLocal()).To(BeFalse())
	})

	t.Run("output local set", func(t *testing.T) {
		RegisterTestingT(t)
		e := newExecutorT(t, &SecurityConfig{
			Enabled: true,
			Scan:    &ScanConfig{Enabled: true, Output: &OutputConfig{Local: "/tmp/out.json"}, Tools: []ScanToolConfig{{Name: "grype"}}},
		})
		Expect(e.getScanOutputPath()).To(Equal("/tmp/out.json"))
		Expect(e.shouldSaveScanLocal()).To(BeTrue())
	})
}

func TestConvertToScanConfig(t *testing.T) {
	RegisterTestingT(t)

	t.Run("nil scan returns nil", func(t *testing.T) {
		RegisterTestingT(t)
		e := newExecutorT(t, &SecurityConfig{Enabled: true})
		Expect(e.convertToScanConfig()).To(BeNil())
	})

	t.Run("maps fields and normalizes tool names", func(t *testing.T) {
		RegisterTestingT(t)
		e := newExecutorT(t, &SecurityConfig{
			Enabled: true,
			Scan: &ScanConfig{
				Enabled:  true,
				FailOn:   SeverityCritical,
				WarnOn:   SeverityMedium,
				Required: true,
				Output:   &OutputConfig{Local: "/tmp/x.json"},
				Tools: []ScanToolConfig{
					{Name: "all", Enabled: boolPtr(true)},
					{Name: "trivy", Enabled: boolPtr(true)},
				},
			},
		})
		cfg := e.convertToScanConfig()
		Expect(cfg).ToNot(BeNil())
		Expect(cfg.Enabled).To(BeTrue())
		Expect(cfg.Required).To(BeTrue())
		Expect(cfg.FailOn).To(Equal(scan.Severity(SeverityCritical)))
		Expect(cfg.WarnOn).To(Equal(scan.Severity(SeverityMedium)))
		Expect(cfg.Output.Local).To(Equal("/tmp/x.json"))
		// "all" normalizes to grype, trivy stays trivy.
		Expect(cfg.Tools).To(ConsistOf(scan.ScanToolGrype, scan.ScanToolTrivy))
	})
}

func TestEnforceToolPolicy(t *testing.T) {
	RegisterTestingT(t)

	criticalResult := &scan.ScanResult{
		Tool:    scan.ScanToolGrype,
		Summary: scan.VulnerabilitySummary{Critical: 2, Total: 2},
	}
	cleanResult := &scan.ScanResult{
		Tool:    scan.ScanToolGrype,
		Summary: scan.VulnerabilitySummary{Total: 0},
	}

	t.Run("no thresholds returns nil without enforcing", func(t *testing.T) {
		RegisterTestingT(t)
		e := newExecutorT(t, &SecurityConfig{
			Enabled: true,
			Scan:    &ScanConfig{Enabled: true, Tools: []ScanToolConfig{{Name: "grype"}}},
		})
		// scan-level failOn/warnOn empty, tool-level empty -> short circuit.
		Expect(e.enforceToolPolicy(ScanToolConfig{Name: "grype"}, criticalResult)).ToNot(HaveOccurred())
	})

	t.Run("tool failOn critical with critical vulns violates", func(t *testing.T) {
		RegisterTestingT(t)
		e := newExecutorT(t, &SecurityConfig{
			Enabled: true,
			Scan:    &ScanConfig{Enabled: true, Tools: []ScanToolConfig{{Name: "grype"}}},
		})
		err := e.enforceToolPolicy(ScanToolConfig{Name: "grype", FailOn: SeverityCritical}, criticalResult)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("critical"))
	})

	t.Run("falls back to scan-level thresholds", func(t *testing.T) {
		RegisterTestingT(t)
		e := newExecutorT(t, &SecurityConfig{
			Enabled: true,
			Scan:    &ScanConfig{Enabled: true, FailOn: SeverityHigh, WarnOn: SeverityLow, Tools: []ScanToolConfig{{Name: "grype"}}},
		})
		// Tool-level empty, scan-level FailOn high; critical exceeds high.
		Expect(e.enforceToolPolicy(ScanToolConfig{Name: "grype"}, criticalResult)).To(HaveOccurred())
	})

	t.Run("clean result passes policy", func(t *testing.T) {
		RegisterTestingT(t)
		e := newExecutorT(t, &SecurityConfig{
			Enabled: true,
			Scan:    &ScanConfig{Enabled: true, FailOn: SeverityHigh, Tools: []ScanToolConfig{{Name: "grype"}}},
		})
		Expect(e.enforceToolPolicy(ScanToolConfig{Name: "grype"}, cleanResult)).ToNot(HaveOccurred())
	})
}

func TestNewScanCache(t *testing.T) {
	RegisterTestingT(t)

	t.Run("nil when cache disabled", func(t *testing.T) {
		RegisterTestingT(t)
		e := newExecutorT(t, &SecurityConfig{
			Enabled: true,
			Scan:    &ScanConfig{Enabled: true, Cache: &CacheConfig{Enabled: false}, Tools: []ScanToolConfig{{Name: "grype"}}},
		})
		cache, err := e.newScanCache()
		Expect(err).ToNot(HaveOccurred())
		Expect(cache).To(BeNil())
	})

	t.Run("nil when no cache config", func(t *testing.T) {
		RegisterTestingT(t)
		e := newExecutorT(t, &SecurityConfig{
			Enabled: true,
			Scan:    &ScanConfig{Enabled: true, Tools: []ScanToolConfig{{Name: "grype"}}},
		})
		cache, err := e.newScanCache()
		Expect(err).ToNot(HaveOccurred())
		Expect(cache).To(BeNil())
	})

	t.Run("constructs cache when enabled", func(t *testing.T) {
		RegisterTestingT(t)
		dir := t.TempDir()
		e := newExecutorT(t, &SecurityConfig{
			Enabled: true,
			Scan:    &ScanConfig{Enabled: true, Cache: &CacheConfig{Enabled: true, Dir: dir}, Tools: []ScanToolConfig{{Name: "grype"}}},
		})
		cache, err := e.newScanCache()
		Expect(err).ToNot(HaveOccurred())
		Expect(cache).ToNot(BeNil())
		Expect(cache.baseDir).To(Equal(dir))
	})
}

func TestNewSBOMCache(t *testing.T) {
	RegisterTestingT(t)

	t.Run("nil when cache disabled", func(t *testing.T) {
		RegisterTestingT(t)
		e := newExecutorT(t, &SecurityConfig{
			Enabled: true,
			SBOM:    &SBOMConfig{Enabled: true, Cache: &CacheConfig{Enabled: false}},
		})
		cache, err := e.newSBOMCache()
		Expect(err).ToNot(HaveOccurred())
		Expect(cache).To(BeNil())
	})

	t.Run("constructs cache when enabled", func(t *testing.T) {
		RegisterTestingT(t)
		dir := t.TempDir()
		e := newExecutorT(t, &SecurityConfig{
			Enabled: true,
			SBOM:    &SBOMConfig{Enabled: true, Cache: &CacheConfig{Enabled: true, Dir: dir}},
		})
		cache, err := e.newSBOMCache()
		Expect(err).ToNot(HaveOccurred())
		Expect(cache).ToNot(BeNil())
		Expect(cache.baseDir).To(Equal(dir))
	})
}

func TestScanCacheTTL(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name string
		cfg  *ScanConfig
		want time.Duration
	}{
		{"nil cache uses grype default", &ScanConfig{Enabled: true, Tools: []ScanToolConfig{{Name: "grype"}}}, TTL_SCAN_GRYPE},
		{"empty ttl uses default", &ScanConfig{Enabled: true, Cache: &CacheConfig{Enabled: true}, Tools: []ScanToolConfig{{Name: "grype"}}}, TTL_SCAN_GRYPE},
		{"invalid ttl falls back to default", &ScanConfig{Enabled: true, Cache: &CacheConfig{Enabled: true, TTL: "notaduration"}, Tools: []ScanToolConfig{{Name: "grype"}}}, TTL_SCAN_GRYPE},
		{"negative ttl falls back to default", &ScanConfig{Enabled: true, Cache: &CacheConfig{Enabled: true, TTL: "-3h"}, Tools: []ScanToolConfig{{Name: "grype"}}}, TTL_SCAN_GRYPE},
		{"valid ttl honored", &ScanConfig{Enabled: true, Cache: &CacheConfig{Enabled: true, TTL: "2h"}, Tools: []ScanToolConfig{{Name: "grype"}}}, 2 * time.Hour},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			e := newExecutorT(t, &SecurityConfig{Enabled: true, Scan: tc.cfg})
			Expect(e.scanCacheTTL()).To(Equal(tc.want))
		})
	}
}

func TestSBOMCacheTTL(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name string
		cfg  *SBOMConfig
		want time.Duration
	}{
		{"nil cache uses sbom default", &SBOMConfig{Enabled: true}, TTL_SBOM},
		{"empty ttl uses default", &SBOMConfig{Enabled: true, Cache: &CacheConfig{Enabled: true}}, TTL_SBOM},
		{"invalid ttl falls back to default", &SBOMConfig{Enabled: true, Cache: &CacheConfig{Enabled: true, TTL: "bogus"}}, TTL_SBOM},
		{"valid ttl honored", &SBOMConfig{Enabled: true, Cache: &CacheConfig{Enabled: true, TTL: "12h"}}, 12 * time.Hour},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			e := newExecutorT(t, &SecurityConfig{Enabled: true, SBOM: tc.cfg})
			Expect(e.sbomCacheTTL()).To(Equal(tc.want))
		})
	}
}

func TestBuilderID(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name string
		cfg  *ProvenanceConfig
		want string
	}{
		{"nil config", nil, ""},
		{"nil builder", &ProvenanceConfig{}, ""},
		{"with builder id", &ProvenanceConfig{Builder: &BuilderConfig{ID: "gha://acme"}}, "gha://acme"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			Expect(builderID(tc.cfg)).To(Equal(tc.want))
		})
	}
}

func TestProvenanceShouldAttach(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name string
		cfg  *ProvenanceConfig
		want bool
	}{
		{"nil config", nil, false},
		{"nil output", &ProvenanceConfig{}, false},
		{"registry false", &ProvenanceConfig{Output: &OutputConfig{Registry: false}}, false},
		{"registry true", &ProvenanceConfig{Output: &OutputConfig{Registry: true}}, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			Expect(provenanceShouldAttach(tc.cfg)).To(Equal(tc.want))
		})
	}
}

func TestSBOMConfig_ShouldAttach(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name string
		cfg  *SBOMConfig
		want bool
	}{
		{"nil config", nil, false},
		{"disabled", &SBOMConfig{Enabled: false}, false},
		{"registry output forces attach", &SBOMConfig{Enabled: true, Output: &OutputConfig{Registry: true}}, true},
		{"attach enabled", &SBOMConfig{Enabled: true, Attach: &AttachConfig{Enabled: true}}, true},
		{"attach disabled and no registry", &SBOMConfig{Enabled: true, Attach: &AttachConfig{Enabled: false}}, false},
		{"enabled but no attach/output", &SBOMConfig{Enabled: true}, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			Expect(tc.cfg.ShouldAttach()).To(Equal(tc.want))
		})
	}
}

func TestSaveScanLocal(t *testing.T) {
	RegisterTestingT(t)

	dir := t.TempDir()
	outputPath := filepath.Join(dir, "nested", "scan.json")
	e := newExecutorT(t, &SecurityConfig{
		Enabled: true,
		Scan:    &ScanConfig{Enabled: true, Output: &OutputConfig{Local: outputPath}, Tools: []ScanToolConfig{{Name: "grype"}}},
	})

	result := &scan.ScanResult{
		ImageDigest: "sha256:abc",
		Tool:        scan.ScanToolGrype,
		Summary:     scan.VulnerabilitySummary{Critical: 1, Total: 1},
		Vulnerabilities: []scan.Vulnerability{
			{ID: "CVE-2024-1", Severity: scan.SeverityCritical, Package: "openssl", Version: "1.0.0"},
		},
	}

	Expect(e.saveScanLocal(result)).To(Succeed())

	// File written with 0600, directory auto-created, valid JSON round-trips.
	info, err := os.Stat(outputPath)
	Expect(err).ToNot(HaveOccurred())
	Expect(info.Mode().Perm()).To(Equal(os.FileMode(0o600)))

	data, err := os.ReadFile(outputPath)
	Expect(err).ToNot(HaveOccurred())
	var round scan.ScanResult
	Expect(json.Unmarshal(data, &round)).To(Succeed())
	Expect(round.ImageDigest).To(Equal("sha256:abc"))
	Expect(round.Tool).To(Equal(scan.ScanToolGrype))
}

func TestScanCacheRoundTrip(t *testing.T) {
	RegisterTestingT(t)

	dir := t.TempDir()
	e := newExecutorT(t, &SecurityConfig{
		Enabled: true,
		Scan: &ScanConfig{
			Enabled: true,
			Cache:   &CacheConfig{Enabled: true, Dir: dir, TTL: "1h"},
			Tools:   []ScanToolConfig{{Name: "grype"}},
		},
	})
	cache, err := e.newScanCache()
	Expect(err).ToNot(HaveOccurred())
	Expect(cache).ToNot(BeNil())

	toolCfg := ScanToolConfig{Name: "grype"}
	imageRef := "registry.example.com/demo@sha256:1234"

	result := &scan.ScanResult{
		ImageDigest: "sha256:1234",
		Tool:        scan.ScanToolGrype,
		Summary:     scan.VulnerabilitySummary{High: 3, Total: 3},
	}

	// Miss before save.
	_, found, err := e.loadScanResultFromCache(cache, toolCfg, imageRef)
	Expect(err).ToNot(HaveOccurred())
	Expect(found).To(BeFalse())

	// Save then hit.
	Expect(e.saveScanResultToCache(cache, toolCfg, imageRef, result)).To(Succeed())

	loaded, found, err := e.loadScanResultFromCache(cache, toolCfg, imageRef)
	Expect(err).ToNot(HaveOccurred())
	Expect(found).To(BeTrue())
	Expect(loaded.Tool).To(Equal(scan.ScanToolGrype))
	Expect(loaded.Summary.High).To(Equal(3))
}

func TestLoadScanResultFromCacheCorrupt(t *testing.T) {
	RegisterTestingT(t)

	dir := t.TempDir()
	e := newExecutorT(t, &SecurityConfig{
		Enabled: true,
		Scan: &ScanConfig{
			Enabled: true,
			Cache:   &CacheConfig{Enabled: true, Dir: dir, TTL: "1h"},
			Tools:   []ScanToolConfig{{Name: "grype"}},
		},
	})
	cache, err := e.newScanCache()
	Expect(err).ToNot(HaveOccurred())

	toolCfg := ScanToolConfig{Name: "grype"}
	imageRef := "registry.example.com/demo@sha256:5678"

	// Store non-ScanResult JSON under the proper signed cache key so the HMAC
	// passes but the executor's json.Unmarshal into scan.ScanResult fails.
	key, err := e.scanCacheKey(toolCfg, imageRef)
	Expect(err).ToNot(HaveOccurred())
	Expect(cache.Set(key, []byte(`"not a scan result object"`))).To(Succeed())

	_, _, err = e.loadScanResultFromCache(cache, toolCfg, imageRef)
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("unmarshaling cached scan result"))
}

func TestSummaryUploads(t *testing.T) {
	RegisterTestingT(t)

	t.Run("nil summary returns nil", func(t *testing.T) {
		RegisterTestingT(t)
		e := newExecutorT(t, &SecurityConfig{Enabled: true})
		Expect(e.summaryUploads()).To(BeNil())
	})

	t.Run("returns recorded uploads", func(t *testing.T) {
		RegisterTestingT(t)
		e, err := NewSecurityExecutorWithSummary(context.Background(), &SecurityConfig{Enabled: true}, "img:tag")
		Expect(err).ToNot(HaveOccurred())
		e.Summary.RecordUpload("defectdojo", nil, "https://dojo/engagement/1", time.Second)
		Expect(e.summaryUploads()).To(HaveLen(1))
		Expect(e.summaryUploads()[0].Target).To(Equal("defectdojo"))
	})
}

func TestNewSecurityExecutorWithSummary(t *testing.T) {
	RegisterTestingT(t)

	e, err := NewSecurityExecutorWithSummary(context.Background(), &SecurityConfig{Enabled: true}, "registry/img:tag")
	Expect(err).ToNot(HaveOccurred())
	Expect(e).ToNot(BeNil())
	Expect(e.Summary).ToNot(BeNil())
}

func TestValidateConfigNilConfig(t *testing.T) {
	RegisterTestingT(t)

	// Construct executor then null out Config to exercise the nil branch.
	e := newExecutorT(t, &SecurityConfig{Enabled: false})
	e.Config = nil
	Expect(e.ValidateConfig()).To(Succeed())
}

// Guard: scanCacheKey embeds the tool name into the cache Operation so two
// different tools never collide on the same image, while ignoring policy fields.
func TestScanCacheKeyDistinctPerTool(t *testing.T) {
	RegisterTestingT(t)

	e := newExecutorT(t, &SecurityConfig{
		Enabled: true,
		Scan:    &ScanConfig{Enabled: true, Tools: []ScanToolConfig{{Name: "grype"}, {Name: "trivy"}}},
	})
	imageRef := "registry.example.com/demo@sha256:abcd"

	grypeKey, err := e.scanCacheKey(ScanToolConfig{Name: "grype"}, imageRef)
	Expect(err).ToNot(HaveOccurred())
	trivyKey, err := e.scanCacheKey(ScanToolConfig{Name: "trivy"}, imageRef)
	Expect(err).ToNot(HaveOccurred())

	Expect(grypeKey.Operation).To(Equal("scan-grype"))
	Expect(trivyKey.Operation).To(Equal("scan-trivy"))
	Expect(grypeKey).ToNot(Equal(trivyKey))
}
