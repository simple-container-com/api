package scan

import (
	"context"
	"testing"
)

func TestTrivyScanner_Tool(t *testing.T) {
	scanner := NewTrivyScanner()
	if scanner.Tool() != ScanToolTrivy {
		t.Errorf("expected tool %s, got %s", ScanToolTrivy, scanner.Tool())
	}
}

func TestNormalizeTrivySeverity(t *testing.T) {
	tests := []struct {
		input    string
		expected Severity
	}{
		{"CRITICAL", SeverityCritical},
		{"critical", SeverityCritical},
		{"Critical", SeverityCritical},
		{"HIGH", SeverityHigh},
		{"high", SeverityHigh},
		{"High", SeverityHigh},
		{"MEDIUM", SeverityMedium},
		{"medium", SeverityMedium},
		{"LOW", SeverityLow},
		{"low", SeverityLow},
		{"UNKNOWN", SeverityUnknown},
		{"unknown", SeverityUnknown},
		{"invalid", SeverityUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizeTrivySeverity(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeTrivySeverity(%s) = %s, want %s", tt.input, result, tt.expected)
			}
		})
	}
}

func TestExtractImageDigestFromTrivy(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "with sha256 prefix",
			input:    "sha256:1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
			expected: "sha256:1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
		},
		{
			name:     "without sha256 prefix",
			input:    "1234567890abcdef",
			expected: "",
		},
		{
			name:     "empty",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractImageDigestFromTrivy(tt.input)
			if result != tt.expected {
				t.Errorf("extractImageDigestFromTrivy(%s) = %s, want %s", tt.input, result, tt.expected)
			}
		})
	}
}

func TestTrivyScanner_CheckInstalled(t *testing.T) {
	scanner := NewTrivyScanner()
	ctx := context.Background()

	// This test will skip if trivy is not installed
	err := scanner.CheckInstalled(ctx)
	if err != nil {
		t.Skipf("trivy not installed: %v", err)
	}
}

func TestTrivyScanner_Version(t *testing.T) {
	scanner := NewTrivyScanner()
	ctx := context.Background()

	// Check if trivy is installed
	if err := scanner.CheckInstalled(ctx); err != nil {
		t.Skipf("trivy not installed: %v", err)
	}

	version, err := scanner.Version(ctx)
	if err != nil {
		t.Errorf("Version() error = %v", err)
	}

	if version == "" {
		t.Error("Version() returned empty version")
	}

	t.Logf("Trivy version: %s", version)
}

func TestTrivyScanner_CheckVersion(t *testing.T) {
	scanner := NewTrivyScanner()
	ctx := context.Background()

	// Check if trivy is installed
	if err := scanner.CheckInstalled(ctx); err != nil {
		t.Skipf("trivy not installed: %v", err)
	}

	err := scanner.CheckVersion(ctx)
	if err != nil {
		t.Logf("CheckVersion() error = %v (this is expected if trivy version is below minimum)", err)
	}
}
