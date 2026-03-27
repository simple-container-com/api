package security

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/simple-container-com/api/pkg/security/scan"
	"github.com/simple-container-com/api/pkg/security/signing"
)

func TestNewSecurityExecutor(t *testing.T) {
	tests := []struct {
		name    string
		config  *SecurityConfig
		wantErr bool
	}{
		{
			name:    "nil config",
			config:  nil,
			wantErr: false,
		},
		{
			name: "disabled config",
			config: &SecurityConfig{
				Enabled: false,
			},
			wantErr: false,
		},
		{
			name: "enabled config",
			config: &SecurityConfig{
				Enabled: true,
				Signing: &signing.Config{
					Enabled: true,
					Keyless: true,
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			executor, err := NewSecurityExecutor(ctx, tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewSecurityExecutor() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && executor == nil {
				t.Error("NewSecurityExecutor() returned nil without error")
			}
		})
	}
}

func TestSecurityExecutor_ValidateConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  *SecurityConfig
		wantErr bool
	}{
		{
			name: "disabled config",
			config: &SecurityConfig{
				Enabled: false,
			},
			wantErr: false,
		},
		{
			name: "valid keyless signing config",
			config: &SecurityConfig{
				Enabled: true,
				Signing: &signing.Config{
					Enabled:        true,
					Keyless:        true,
					OIDCIssuer:     "https://token.actions.githubusercontent.com",
					IdentityRegexp: "^https://github.com/org/.*$",
				},
			},
			wantErr: false,
		},
		{
			name: "invalid signing config",
			config: &SecurityConfig{
				Enabled: true,
				Signing: &signing.Config{
					Enabled: true,
					Keyless: true,
					// Missing required OIDCIssuer
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			executor, err := NewSecurityExecutor(ctx, tt.config)
			if err != nil {
				t.Fatalf("NewSecurityExecutor() failed: %v", err)
			}

			err = executor.ValidateConfig()
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSecurityExecutor_ExecuteSigning_Disabled(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name   string
		config *SecurityConfig
	}{
		{
			name: "security disabled",
			config: &SecurityConfig{
				Enabled: false,
			},
		},
		{
			name: "signing disabled",
			config: &SecurityConfig{
				Enabled: true,
				Signing: &signing.Config{
					Enabled: false,
				},
			},
		},
		{
			name: "nil signing config",
			config: &SecurityConfig{
				Enabled: true,
				Signing: nil,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executor, err := NewSecurityExecutor(ctx, tt.config)
			if err != nil {
				t.Fatalf("NewSecurityExecutor() failed: %v", err)
			}

			result, err := executor.ExecuteSigning(ctx, "test-image:latest")
			if err != nil {
				t.Errorf("ExecuteSigning() returned error for disabled config: %v", err)
			}
			if result != nil {
				t.Error("ExecuteSigning() should return nil for disabled config")
			}
		})
	}
}

func TestSecurityExecutor_ExecuteSigning_FailOpen(t *testing.T) {
	ctx := context.Background()

	config := &SecurityConfig{
		Enabled: true,
		Signing: &signing.Config{
			Enabled:  true,
			Required: false, // Fail-open
			Keyless:  true,
			// Invalid config: missing OIDCIssuer
		},
	}

	executor, err := NewSecurityExecutor(ctx, config)
	if err != nil {
		t.Fatalf("NewSecurityExecutor() failed: %v", err)
	}

	// Should not error because fail-open is enabled
	result, err := executor.ExecuteSigning(ctx, "test-image:latest")
	if err != nil {
		t.Errorf("ExecuteSigning() with fail-open should not error: %v", err)
	}
	if result != nil {
		t.Error("ExecuteSigning() should return nil when validation fails with fail-open")
	}
}

func TestSecurityExecutor_ExecuteSigning_FailClosed(t *testing.T) {
	ctx := context.Background()

	config := &SecurityConfig{
		Enabled: true,
		Signing: &signing.Config{
			Enabled:  true,
			Required: true, // Fail-closed
			Keyless:  true,
			// Invalid config: missing OIDCIssuer
		},
	}

	executor, err := NewSecurityExecutor(ctx, config)
	if err != nil {
		t.Fatalf("NewSecurityExecutor() failed: %v", err)
	}

	// Should error because fail-closed is enabled
	_, err = executor.ExecuteSigning(ctx, "test-image:latest")
	if err == nil {
		t.Error("ExecuteSigning() with fail-closed should error on invalid config")
	}
}

func TestSecurityExecutor_SBOMCacheKeyIgnoresOutputPath(t *testing.T) {
	ctx := context.Background()
	imageRef := "registry.example.com/demo@sha256:1234"

	newExecutor := func(outputPath string) *SecurityExecutor {
		executor, err := NewSecurityExecutor(ctx, &SecurityConfig{
			Enabled: true,
			SBOM: &SBOMConfig{
				Enabled:   true,
				Format:    "cyclonedx-json",
				Generator: "syft",
				Output: &OutputConfig{
					Local: outputPath,
				},
			},
		})
		if err != nil {
			t.Fatalf("NewSecurityExecutor() failed: %v", err)
		}
		return executor
	}

	keyA, err := newExecutor("/tmp/a/sbom.json").sbomCacheKey(imageRef)
	if err != nil {
		t.Fatalf("sbomCacheKey() error = %v", err)
	}

	keyB, err := newExecutor("/tmp/b/sbom.json").sbomCacheKey(imageRef)
	if err != nil {
		t.Fatalf("sbomCacheKey() error = %v", err)
	}

	if keyA != keyB {
		t.Fatalf("sbomCacheKey() should ignore output path, got %v and %v", keyA, keyB)
	}
}

func TestSecurityExecutor_ScanCacheKeyIgnoresPolicyAndOutput(t *testing.T) {
	ctx := context.Background()
	imageRef := "registry.example.com/demo@sha256:5678"
	tool := ScanToolConfig{Name: "grype", WarnOn: SeverityHigh}

	newExecutor := func(outputPath string, failOn Severity) *SecurityExecutor {
		executor, err := NewSecurityExecutor(ctx, &SecurityConfig{
			Enabled: true,
			Scan: &ScanConfig{
				Enabled: true,
				FailOn:  failOn,
				WarnOn:  SeverityHigh,
				Output: &OutputConfig{
					Local: outputPath,
				},
				Tools: []ScanToolConfig{tool},
			},
		})
		if err != nil {
			t.Fatalf("NewSecurityExecutor() failed: %v", err)
		}
		return executor
	}

	keyA, err := newExecutor("/tmp/a/scan.json", SeverityCritical).scanCacheKey(tool, imageRef)
	if err != nil {
		t.Fatalf("scanCacheKey() error = %v", err)
	}

	keyB, err := newExecutor("/tmp/b/scan.json", SeverityLow).scanCacheKey(tool, imageRef)
	if err != nil {
		t.Fatalf("scanCacheKey() error = %v", err)
	}

	if keyA != keyB {
		t.Fatalf("scanCacheKey() should ignore policy and output path, got %v and %v", keyA, keyB)
	}
}

func TestSecurityExecutor_UploadReportsWritesPRComment(t *testing.T) {
	tempDir := t.TempDir()
	outputPath := filepath.Join(tempDir, "scan-comment.md")

	executor, err := NewSecurityExecutorWithSummary(context.Background(), &SecurityConfig{
		Enabled: true,
		Reporting: &ReportingConfig{
			PRComment: &PRCommentConfig{
				Enabled: true,
				Output:  outputPath,
			},
		},
	}, "registry.example.com/demo@sha256:1234")
	if err != nil {
		t.Fatalf("NewSecurityExecutorWithSummary() failed: %v", err)
	}

	executor.Summary.RecordUpload("defectdojo", nil, "https://dojo.example.com/engagement/42", time.Second)

	result := &scan.ScanResult{
		Tool:        scan.ScanToolAll,
		ImageDigest: "sha256:1234",
		Summary: scan.VulnerabilitySummary{
			Critical: 1,
			High:     2,
			Total:    3,
		},
		Vulnerabilities: []scan.Vulnerability{
			{ID: "CVE-1", Severity: scan.SeverityCritical, Package: "openssl", Version: "1.0.0"},
		},
	}

	if err := executor.UploadReports(context.Background(), result, "registry.example.com/demo@sha256:1234"); err != nil {
		t.Fatalf("UploadReports() error = %v", err)
	}

	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", outputPath, err)
	}

	text := string(content)
	for _, expected := range []string{
		"## Image Scan Results",
		"registry.example.com/demo@sha256:1234",
		"defectdojo",
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("comment output missing %q: %s", expected, text)
		}
	}
}
