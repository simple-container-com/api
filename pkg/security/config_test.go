package security

import (
	"testing"

	"github.com/simple-container-com/api/pkg/security/signing"
)

func TestSecurityConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		config  *SecurityConfig
		wantErr bool
	}{
		{
			name: "valid config with all features disabled",
			config: &SecurityConfig{
				Enabled: false,
			},
			wantErr: false,
		},
		{
			name: "valid config with signing enabled",
			config: &SecurityConfig{
				Enabled: true,
				Signing: &signing.Config{
					Enabled:        true,
					Keyless:        true,
					OIDCIssuer:     "https://token.actions.githubusercontent.com",
					IdentityRegexp: ".*",
				},
			},
			wantErr: false,
		},
		{
			name: "valid config with SBOM enabled",
			config: &SecurityConfig{
				Enabled: true,
				SBOM: &SBOMConfig{
					Enabled:   true,
					Format:    "cyclonedx-json",
					Generator: "syft",
				},
			},
			wantErr: false,
		},
		{
			name: "valid config with scan enabled",
			config: &SecurityConfig{
				Enabled: true,
				Scan: &ScanConfig{
					Enabled: true,
					Tools: []ScanToolConfig{
						{Name: "grype", Enabled: boolPtr(true)},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid SBOM format",
			config: &SecurityConfig{
				Enabled: true,
				SBOM: &SBOMConfig{
					Enabled: true,
					Format:  "invalid-format",
				},
			},
			wantErr: true,
		},
		{
			name: "invalid scan tool",
			config: &SecurityConfig{
				Enabled: true,
				Scan: &ScanConfig{
					Enabled: true,
					Tools: []ScanToolConfig{
						{Name: "invalid-tool"},
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSBOMConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		config  *SBOMConfig
		wantErr bool
	}{
		{
			name:    "disabled config is valid",
			config:  &SBOMConfig{Enabled: false},
			wantErr: false,
		},
		{
			name:    "valid cyclonedx-json format",
			config:  &SBOMConfig{Enabled: true, Format: "cyclonedx-json"},
			wantErr: false,
		},
		{
			name:    "valid spdx-json format",
			config:  &SBOMConfig{Enabled: true, Format: "spdx-json"},
			wantErr: false,
		},
		{
			name:    "invalid format",
			config:  &SBOMConfig{Enabled: true, Format: "invalid"},
			wantErr: true,
		},
		{
			name:    "empty format is valid (will use default)",
			config:  &SBOMConfig{Enabled: true, Format: ""},
			wantErr: false,
		},
		{
			name: "attach requires signed attestation",
			config: &SBOMConfig{
				Enabled: true,
				Attach: &AttachConfig{
					Enabled: true,
					Sign:    false,
				},
			},
			wantErr: true,
		},
		{
			name: "registry output cannot disable attachment",
			config: &SBOMConfig{
				Enabled: true,
				Output: &OutputConfig{
					Registry: true,
				},
				Attach: &AttachConfig{
					Enabled: false,
					Sign:    true,
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestScanConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		config  *ScanConfig
		wantErr bool
	}{
		{
			name:    "disabled config is valid",
			config:  &ScanConfig{Enabled: false},
			wantErr: false,
		},
		{
			name: "valid grype config",
			config: &ScanConfig{
				Enabled: true,
				Tools: []ScanToolConfig{
					{Name: "grype"},
				},
			},
			wantErr: false,
		},
		{
			name: "valid trivy config",
			config: &ScanConfig{
				Enabled: true,
				Tools: []ScanToolConfig{
					{Name: "trivy"},
				},
			},
			wantErr: false,
		},
		{
			name: "no tools specified",
			config: &ScanConfig{
				Enabled: true,
				Tools:   []ScanToolConfig{},
			},
			wantErr: true,
		},
		{
			name: "invalid severity",
			config: &ScanConfig{
				Enabled: true,
				FailOn:  "invalid",
				Tools: []ScanToolConfig{
					{Name: "grype"},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSeverityValidation(t *testing.T) {
	tests := []struct {
		severity Severity
		wantErr  bool
	}{
		{SeverityCritical, false},
		{SeverityHigh, false},
		{SeverityMedium, false},
		{SeverityLow, false},
		{SeverityNone, false},
		{"invalid", true},
	}

	for _, tt := range tests {
		t.Run(string(tt.severity), func(t *testing.T) {
			err := tt.severity.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDefectDojoConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		config  *DefectDojoConfig
		wantErr bool
	}{
		{
			name: "disabled config",
			config: &DefectDojoConfig{
				Enabled: false,
			},
			wantErr: false,
		},
		{
			name: "existing engagement",
			config: &DefectDojoConfig{
				Enabled:      true,
				URL:          "https://dojo.example.com",
				APIKey:       "secret",
				EngagementID: 123,
			},
			wantErr: false,
		},
		{
			name: "auto-create with names",
			config: &DefectDojoConfig{
				Enabled:        true,
				URL:            "https://dojo.example.com",
				APIKey:         "secret",
				AutoCreate:     true,
				ProductName:    "demo",
				EngagementName: "staging",
			},
			wantErr: false,
		},
		{
			name: "auto-create missing engagement name",
			config: &DefectDojoConfig{
				Enabled:     true,
				URL:         "https://dojo.example.com",
				APIKey:      "secret",
				AutoCreate:  true,
				ProductName: "demo",
			},
			wantErr: true,
		},
		{
			name: "auto-create missing product reference",
			config: &DefectDojoConfig{
				Enabled:        true,
				URL:            "https://dojo.example.com",
				APIKey:         "secret",
				AutoCreate:     true,
				EngagementName: "staging",
			},
			wantErr: true,
		},
		{
			name: "non auto-create missing engagement id",
			config: &DefectDojoConfig{
				Enabled: true,
				URL:     "https://dojo.example.com",
				APIKey:  "secret",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSeverityIsAtLeast(t *testing.T) {
	tests := []struct {
		name     string
		severity Severity
		other    Severity
		want     bool
	}{
		{"critical >= critical", SeverityCritical, SeverityCritical, true},
		{"critical >= high", SeverityCritical, SeverityHigh, true},
		{"high >= critical", SeverityHigh, SeverityCritical, false},
		{"high >= high", SeverityHigh, SeverityHigh, true},
		{"high >= medium", SeverityHigh, SeverityMedium, true},
		{"medium >= high", SeverityMedium, SeverityHigh, false},
		{"low >= medium", SeverityLow, SeverityMedium, false},
		{"low >= low", SeverityLow, SeverityLow, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.severity.IsAtLeast(tt.other)
			if got != tt.want {
				t.Errorf("IsAtLeast() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDefaultSecurityConfig(t *testing.T) {
	config := DefaultSecurityConfig()

	if config == nil {
		t.Fatal("DefaultSecurityConfig() returned nil")
	}

	if config.Enabled {
		t.Error("Expected default config to be disabled")
	}

	if config.Signing == nil {
		t.Error("Expected signing config to be present")
	}

	if config.SBOM == nil {
		t.Error("Expected SBOM config to be present")
	}

	if config.Provenance == nil {
		t.Error("Expected provenance config to be present")
	}

	if config.Scan == nil {
		t.Error("Expected scan config to be present")
	}

	// Validate default config
	if err := config.Validate(); err != nil {
		t.Errorf("Default config should be valid: %v", err)
	}
}

func TestProvenanceConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		config  *ProvenanceConfig
		wantErr bool
	}{
		{
			name:    "disabled config is valid",
			config:  &ProvenanceConfig{Enabled: false},
			wantErr: false,
		},
		{
			name:    "valid slsa-v1.0 format",
			config:  &ProvenanceConfig{Enabled: true, Format: "slsa-v1.0"},
			wantErr: false,
		},
		{
			name:    "legacy slsa-v0.2 format is rejected",
			config:  &ProvenanceConfig{Enabled: true, Format: "slsa-v0.2"},
			wantErr: true,
		},
		{
			name:    "invalid format",
			config:  &ProvenanceConfig{Enabled: true, Format: "invalid"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
