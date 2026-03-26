package scan

import (
	"context"
	"encoding/json"
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

func TestParseTrivyVersion(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name: "current multiline format",
			input: `Version: 0.69.0
Vulnerability DB:
  Version: 2`,
			want: "0.69.0",
		},
		{
			name:    "invalid output",
			input:   "no version here",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseTrivyVersion(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("parseTrivyVersion() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("parseTrivyVersion() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestTrivyCVSS_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    float64
		wantErr bool
	}{
		{
			name: "object format",
			input: `{
				"nvd": {"V3Score": 7.5},
				"redhat": {"V2Score": 5.0}
			}`,
			want: 7.5,
		},
		{
			name:  "array format",
			input: `[{"V3Score": 4.2}, {"V2Score": 5.1}]`,
			want:  5.1,
		},
		{
			name:  "null",
			input: `null`,
			want:  0,
		},
		{
			name:    "invalid",
			input:   `"bad"`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cvss trivyCVSS
			err := json.Unmarshal([]byte(tt.input), &cvss)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("json.Unmarshal() error = %v", err)
			}
			if got := extractTrivyCVSS(cvss); got != tt.want {
				t.Fatalf("extractTrivyCVSS() = %v, want %v", got, tt.want)
			}
		})
	}
}
