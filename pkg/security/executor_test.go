package security

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/security/scan"
	"github.com/simple-container-com/api/pkg/security/signing"
)

func TestNewSecurityExecutor(t *testing.T) {
	RegisterTestingT(t)

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
			RegisterTestingT(t)
			ctx := context.Background()
			executor, err := NewSecurityExecutor(ctx, tt.config)
			if tt.wantErr {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(err).ToNot(HaveOccurred())
				Expect(executor).ToNot(BeNil())
			}
		})
	}
}

func TestSecurityExecutor_ValidateConfig(t *testing.T) {
	RegisterTestingT(t)

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
			RegisterTestingT(t)
			ctx := context.Background()
			executor, err := NewSecurityExecutor(ctx, tt.config)
			Expect(err).ToNot(HaveOccurred())

			err = executor.ValidateConfig()
			if tt.wantErr {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(err).ToNot(HaveOccurred())
			}
		})
	}
}

func TestSecurityExecutor_ExecuteSigning_Disabled(t *testing.T) {
	RegisterTestingT(t)
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
			RegisterTestingT(t)
			executor, err := NewSecurityExecutor(ctx, tt.config)
			Expect(err).ToNot(HaveOccurred())

			result, err := executor.ExecuteSigning(ctx, "test-image:latest")
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(BeNil(), "ExecuteSigning() should return nil for disabled config")
		})
	}
}

func TestSecurityExecutor_ExecuteSigning_FailOpen(t *testing.T) {
	RegisterTestingT(t)
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
	Expect(err).ToNot(HaveOccurred())

	// Should not error because fail-open is enabled
	result, err := executor.ExecuteSigning(ctx, "test-image:latest")
	Expect(err).ToNot(HaveOccurred())
	Expect(result).To(BeNil(), "ExecuteSigning() should return nil when validation fails with fail-open")
}

func TestSecurityExecutor_ExecuteSigning_FailClosed(t *testing.T) {
	RegisterTestingT(t)
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
	Expect(err).ToNot(HaveOccurred())

	// Should error because fail-closed is enabled
	_, err = executor.ExecuteSigning(ctx, "test-image:latest")
	Expect(err).To(HaveOccurred())
}

func TestSecurityExecutor_SBOMCacheKeyIgnoresOutputPath(t *testing.T) {
	RegisterTestingT(t)
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
		Expect(err).ToNot(HaveOccurred())
		return executor
	}

	keyA, err := newExecutor("/tmp/a/sbom.json").sbomCacheKey(imageRef)
	Expect(err).ToNot(HaveOccurred())

	keyB, err := newExecutor("/tmp/b/sbom.json").sbomCacheKey(imageRef)
	Expect(err).ToNot(HaveOccurred())

	Expect(keyA).To(Equal(keyB), "sbomCacheKey() should ignore output path")
}

func TestSecurityExecutor_ScanCacheKeyIgnoresPolicyAndOutput(t *testing.T) {
	RegisterTestingT(t)
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
		Expect(err).ToNot(HaveOccurred())
		return executor
	}

	keyA, err := newExecutor("/tmp/a/scan.json", SeverityCritical).scanCacheKey(tool, imageRef)
	Expect(err).ToNot(HaveOccurred())

	keyB, err := newExecutor("/tmp/b/scan.json", SeverityLow).scanCacheKey(tool, imageRef)
	Expect(err).ToNot(HaveOccurred())

	Expect(keyA).To(Equal(keyB), "scanCacheKey() should ignore policy and output path")
}

func TestSecurityExecutor_UploadReportsWritesPRComment(t *testing.T) {
	RegisterTestingT(t)

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
	Expect(err).ToNot(HaveOccurred())

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

	err = executor.UploadReports(context.Background(), result, "registry.example.com/demo@sha256:1234")
	Expect(err).ToNot(HaveOccurred())

	content, err := os.ReadFile(outputPath)
	Expect(err).ToNot(HaveOccurred())

	text := string(content)
	for _, expected := range []string{
		"## Image Scan Results",
		"registry.example.com/demo@sha256:1234",
		"defectdojo",
	} {
		Expect(text).To(ContainSubstring(expected))
	}
}

func TestValidateImageRef(t *testing.T) {
	RegisterTestingT(t)

	valid := []string{
		"registry.example.com/repo:tag",
		"registry.example.com/repo@sha256:abcdef1234567890",
		"my_org/my_image:latest",
		"ghcr.io/org/repo:v1.0.0+build.123",
		"docker.io/library/ubuntu:22.04",
		"000000000000.dkr.ecr.eu-central-1.amazonaws.com/repo:tag",
		"europe-north1-docker.pkg.dev/project/repo:tag",
	}
	invalid := []string{
		"",                             // empty
		"--image-that-looks-like-flag", // starts with -
		"image;rm -rf /",               // shell metacharacter
		"image$(whoami)",               // command substitution
		"image`id`",                    // backtick injection
	}

	for _, ref := range valid {
		Expect(ValidateImageRef(ref)).ToNot(HaveOccurred(), "ValidateImageRef(%q) should be nil", ref)
	}
	for _, ref := range invalid {
		Expect(ValidateImageRef(ref)).To(HaveOccurred(), "ValidateImageRef(%q) should error", ref)
	}
}
