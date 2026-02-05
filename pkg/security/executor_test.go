package security

import (
	"context"
	"testing"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/logger"
)

func TestNewExecutor(t *testing.T) {
	log := logger.New()
	config := &api.SecurityDescriptor{
		Signing: &api.SigningConfig{
			Enabled: true,
		},
	}

	executor, err := NewExecutor(config, nil, log)
	if err != nil {
		t.Fatalf("NewExecutor failed: %v", err)
	}

	if executor == nil {
		t.Fatal("Expected non-nil executor")
	}
}

func TestNewExecutor_NilConfig(t *testing.T) {
	log := logger.New()

	_, err := NewExecutor(nil, nil, log)
	if err == nil {
		t.Error("Expected error for nil config")
	}
}

func TestNewExecutor_NilLogger(t *testing.T) {
	config := &api.SecurityDescriptor{}

	_, err := NewExecutor(config, nil, nil)
	if err == nil {
		t.Error("Expected error for nil logger")
	}
}

func TestValidateConfig_ValidSigning(t *testing.T) {
	log := logger.New()
	config := &api.SecurityDescriptor{
		Signing: &api.SigningConfig{
			Enabled: true,
			Keyless: true,
		},
	}

	executor, _ := NewExecutor(config, nil, log)
	err := executor.ValidateConfig(config)

	if err != nil {
		t.Errorf("Expected valid config, got error: %v", err)
	}
}

func TestValidateConfig_InvalidSigning_NoPrivateKey(t *testing.T) {
	log := logger.New()
	config := &api.SecurityDescriptor{
		Signing: &api.SigningConfig{
			Enabled:    true,
			Keyless:    false, // Key-based signing
			PrivateKey: "",    // Missing private key
		},
	}

	executor, _ := NewExecutor(config, nil, log)
	err := executor.ValidateConfig(config)

	if err == nil {
		t.Error("Expected error for missing private key")
	}
}

func TestValidateConfig_ValidSBOM(t *testing.T) {
	log := logger.New()
	config := &api.SecurityDescriptor{
		SBOM: &api.SBOMConfig{
			Enabled:   true,
			Format:    "cyclonedx-json",
			Generator: "syft",
		},
	}

	executor, _ := NewExecutor(config, nil, log)
	err := executor.ValidateConfig(config)

	if err != nil {
		t.Errorf("Expected valid config, got error: %v", err)
	}
}

func TestValidateConfig_InvalidSBOM_Format(t *testing.T) {
	log := logger.New()
	config := &api.SecurityDescriptor{
		SBOM: &api.SBOMConfig{
			Enabled: true,
			Format:  "invalid-format",
		},
	}

	executor, _ := NewExecutor(config, nil, log)
	err := executor.ValidateConfig(config)

	if err == nil {
		t.Error("Expected error for invalid SBOM format")
	}
}

func TestValidateConfig_ValidScan(t *testing.T) {
	log := logger.New()
	config := &api.SecurityDescriptor{
		Scan: &api.ScanConfig{
			Enabled: true,
			Tools: []api.ScanToolConfig{
				{
					Name:     "grype",
					Required: true,
					FailOn:   api.SeverityCritical,
				},
			},
		},
	}

	executor, _ := NewExecutor(config, nil, log)
	err := executor.ValidateConfig(config)

	if err != nil {
		t.Errorf("Expected valid config, got error: %v", err)
	}
}

func TestValidateConfig_InvalidScan_NoTools(t *testing.T) {
	log := logger.New()
	config := &api.SecurityDescriptor{
		Scan: &api.ScanConfig{
			Enabled: true,
			Tools:   []api.ScanToolConfig{}, // Empty tools list
		},
	}

	executor, _ := NewExecutor(config, nil, log)
	err := executor.ValidateConfig(config)

	if err == nil {
		t.Error("Expected error for empty tools list")
	}
}

func TestValidateConfig_InvalidScan_UnknownTool(t *testing.T) {
	log := logger.New()
	config := &api.SecurityDescriptor{
		Scan: &api.ScanConfig{
			Enabled: true,
			Tools: []api.ScanToolConfig{
				{
					Name:     "unknown-scanner",
					Required: true,
				},
			},
		},
	}

	executor, _ := NewExecutor(config, nil, log)
	err := executor.ValidateConfig(config)

	if err == nil {
		t.Error("Expected error for unknown scanner")
	}
}

func TestValidateConfig_InvalidScan_Severity(t *testing.T) {
	log := logger.New()
	config := &api.SecurityDescriptor{
		Scan: &api.ScanConfig{
			Enabled: true,
			Tools: []api.ScanToolConfig{
				{
					Name:   "grype",
					FailOn: "invalid-severity",
				},
			},
		},
	}

	executor, _ := NewExecutor(config, nil, log)
	err := executor.ValidateConfig(config)

	if err == nil {
		t.Error("Expected error for invalid severity")
	}
}

func TestExecute_MinimalConfig(t *testing.T) {
	log := logger.New()
	config := &api.SecurityDescriptor{
		// No features enabled
	}

	executor, _ := NewExecutor(config, nil, log)

	image := ImageReference{
		Registry:   "docker.io",
		Repository: "test/image",
		Tag:        "latest",
	}

	result, err := executor.Execute(context.Background(), image)

	if err != nil {
		t.Errorf("Execute failed: %v", err)
	}

	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	if result.Image.Repository != "test/image" {
		t.Errorf("Expected repository 'test/image', got '%s'", result.Image.Repository)
	}

	if result.Duration == 0 {
		t.Error("Expected non-zero duration")
	}
}

func TestImageReference_String(t *testing.T) {
	tests := []struct {
		name     string
		ref      ImageReference
		expected string
	}{
		{
			name: "With registry and tag",
			ref: ImageReference{
				Registry:   "docker.io",
				Repository: "test/image",
				Tag:        "latest",
			},
			expected: "docker.io/test/image:latest",
		},
		{
			name: "With registry and digest",
			ref: ImageReference{
				Registry:   "docker.io",
				Repository: "test/image",
				Digest:     "sha256:abc123",
			},
			expected: "docker.io/test/image@sha256:abc123",
		},
		{
			name: "Without registry",
			ref: ImageReference{
				Repository: "test/image",
				Tag:        "v1.0",
			},
			expected: "test/image:v1.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.ref.String()
			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

func TestImageReference_WithDigest(t *testing.T) {
	ref := ImageReference{
		Registry:   "docker.io",
		Repository: "test/image",
		Tag:        "latest",
	}

	newRef := ref.WithDigest("sha256:new123")

	if newRef.Digest != "sha256:new123" {
		t.Errorf("Expected digest 'sha256:new123', got '%s'", newRef.Digest)
	}

	if newRef.Registry != ref.Registry {
		t.Error("Registry should be preserved")
	}

	if newRef.Repository != ref.Repository {
		t.Error("Repository should be preserved")
	}
}

func TestSecurityResult_HasCriticalIssues(t *testing.T) {
	tests := []struct {
		name     string
		result   *SecurityResult
		expected bool
	}{
		{
			name: "No critical issues",
			result: &SecurityResult{
				ScanResults: []*ScanResult{
					{
						Summary: VulnerabilitySummary{
							Critical: 0,
							High:     5,
						},
					},
				},
			},
			expected: false,
		},
		{
			name: "Has critical issues",
			result: &SecurityResult{
				ScanResults: []*ScanResult{
					{
						Summary: VulnerabilitySummary{
							Critical: 2,
							High:     5,
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "No scan results",
			result: &SecurityResult{
				ScanResults: []*ScanResult{},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.result.HasCriticalIssues()
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}
