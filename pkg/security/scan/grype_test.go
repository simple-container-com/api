package scan

import (
	"context"
	"testing"
)

func TestGrypeScanner_Tool(t *testing.T) {
	scanner := NewGrypeScanner()
	if scanner.Tool() != ScanToolGrype {
		t.Errorf("expected tool %s, got %s", ScanToolGrype, scanner.Tool())
	}
}

func TestNormalizeSeverity(t *testing.T) {
	tests := []struct {
		input    string
		expected Severity
	}{
		{"critical", SeverityCritical},
		{"Critical", SeverityCritical},
		{"CRITICAL", SeverityCritical},
		{"high", SeverityHigh},
		{"High", SeverityHigh},
		{"medium", SeverityMedium},
		{"low", SeverityLow},
		{"negligible", SeverityLow},
		{"unknown", SeverityUnknown},
		{"invalid", SeverityUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizeSeverity(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeSeverity(%s) = %s, want %s", tt.input, result, tt.expected)
			}
		})
	}
}

func TestIsVersionGreaterOrEqual(t *testing.T) {
	tests := []struct {
		version    string
		minVersion string
		expected   bool
	}{
		{"0.106.0", "0.106.0", true},
		{"0.106.1", "0.106.0", true},
		{"0.107.0", "0.106.0", true},
		{"1.0.0", "0.106.0", true},
		{"0.105.0", "0.106.0", false},
		{"0.106.0", "0.106.1", false},
		{"0.100.0", "0.106.0", false},
	}

	for _, tt := range tests {
		t.Run(tt.version+"_vs_"+tt.minVersion, func(t *testing.T) {
			result := isVersionGreaterOrEqual(tt.version, tt.minVersion)
			if result != tt.expected {
				t.Errorf("isVersionGreaterOrEqual(%s, %s) = %v, want %v",
					tt.version, tt.minVersion, result, tt.expected)
			}
		})
	}
}

func TestExtractImageDigestFromGrype(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "with digest",
			input:    "registry:alpine@sha256:1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
			expected: "sha256:1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
		},
		{
			name:     "without digest",
			input:    "alpine:latest",
			expected: "alpine:latest",
		},
		{
			name:     "empty",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractImageDigestFromGrype(tt.input)
			if result != tt.expected {
				t.Errorf("extractImageDigestFromGrype(%s) = %s, want %s", tt.input, result, tt.expected)
			}
		})
	}
}

func TestGrypeScanner_CheckInstalled(t *testing.T) {
	scanner := NewGrypeScanner()
	ctx := context.Background()

	// This test will skip if grype is not installed
	err := scanner.CheckInstalled(ctx)
	if err != nil {
		t.Skipf("grype not installed: %v", err)
	}
}

func TestGrypeScanner_Version(t *testing.T) {
	scanner := NewGrypeScanner()
	ctx := context.Background()

	// Check if grype is installed
	if err := scanner.CheckInstalled(ctx); err != nil {
		t.Skipf("grype not installed: %v", err)
	}

	version, err := scanner.Version(ctx)
	if err != nil {
		t.Errorf("Version() error = %v", err)
	}

	if version == "" {
		t.Error("Version() returned empty version")
	}

	t.Logf("Grype version: %s", version)
}

func TestGrypeScanner_CheckVersion(t *testing.T) {
	scanner := NewGrypeScanner()
	ctx := context.Background()

	// Check if grype is installed
	if err := scanner.CheckInstalled(ctx); err != nil {
		t.Skipf("grype not installed: %v", err)
	}

	err := scanner.CheckVersion(ctx)
	if err != nil {
		t.Logf("CheckVersion() error = %v (this is expected if grype version is below minimum)", err)
	}
}
