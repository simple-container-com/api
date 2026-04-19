package scan

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/gomega"
)

func TestGrypeScanner_Tool(t *testing.T) {
	RegisterTestingT(t)
	Expect(NewGrypeScanner().Tool()).To(Equal(ScanToolGrype))
}

func TestNewScanner_UnsupportedTool(t *testing.T) {
	RegisterTestingT(t)
	_, err := NewScanner("unknown-tool")
	Expect(err).To(HaveOccurred())
}

func TestNewScanner_SupportedTools(t *testing.T) {
	RegisterTestingT(t)
	for _, tool := range []ScanTool{ScanToolGrype, ScanToolTrivy} {
		s, err := NewScanner(tool)
		Expect(err).ToNot(HaveOccurred(), "NewScanner(%q)", tool)
		Expect(s).ToNot(BeNil(), "NewScanner(%q)", tool)
	}
}

func TestNormalizeSeverity(t *testing.T) {
	RegisterTestingT(t)

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
			RegisterTestingT(t)
			Expect(normalizeSeverity(tt.input)).To(Equal(tt.expected))
		})
	}
}

func TestIsVersionGreaterOrEqual(t *testing.T) {
	RegisterTestingT(t)

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
			RegisterTestingT(t)
			Expect(isVersionGreaterOrEqual(tt.version, tt.minVersion)).To(Equal(tt.expected))
		})
	}
}

func TestExtractImageDigestFromGrype(t *testing.T) {
	RegisterTestingT(t)

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
			RegisterTestingT(t)
			Expect(extractImageDigestFromGrype(tt.input)).To(Equal(tt.expected))
		})
	}
}

func TestGrypeScanner_CheckInstalled(t *testing.T) {
	RegisterTestingT(t)

	scanner := NewGrypeScanner()
	ctx := context.Background()

	// This test will skip if grype is not installed
	err := scanner.CheckInstalled(ctx)
	if err != nil {
		t.Skipf("grype not installed: %v", err)
	}
}

func TestGrypeScanner_Version(t *testing.T) {
	RegisterTestingT(t)

	scanner := NewGrypeScanner()
	ctx := context.Background()

	// Check if grype is installed
	if err := scanner.CheckInstalled(ctx); err != nil {
		t.Skipf("grype not installed: %v", err)
	}

	version, err := scanner.Version(ctx)
	Expect(err).ToNot(HaveOccurred())
	Expect(version).ToNot(BeEmpty())

	t.Logf("Grype version: %s", version)
}

func TestGrypeScanner_CheckVersion(t *testing.T) {
	RegisterTestingT(t)

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

func TestParseGrypeVersion(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:  "legacy single line format",
			input: "grype 0.106.0",
			want:  "0.106.0",
		},
		{
			name: "current multiline format",
			input: `Application:         grype
Version:             0.107.0
BuildDate:           2026-01-29T22:10:17Z`,
			want: "0.107.0",
		},
		{
			name:    "invalid output",
			input:   "no version here",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			got, err := parseGrypeVersion(tt.input)
			if tt.wantErr {
				Expect(err).To(HaveOccurred())
				return
			}
			Expect(err).ToNot(HaveOccurred())
			Expect(got).To(Equal(tt.want))
		})
	}
}

func TestGrypeCommandEnv(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name      string
		dbPresent bool
		wantAuto  bool
	}{
		{
			name:      "cold cache allows database update",
			dbPresent: false,
			wantAuto:  false,
		},
		{
			name:      "warm cache skips database update",
			dbPresent: true,
			wantAuto:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			env := grypeCommandEnv(tt.dbPresent)
			Expect(containsString(env, "GRYPE_CHECK_FOR_APP_UPDATE=false")).To(BeTrue())
			Expect(containsString(env, "GRYPE_DB_AUTO_UPDATE=false")).To(Equal(tt.wantAuto))
		})
	}
}

func TestHasGrypeVulnerabilityDB(t *testing.T) {
	RegisterTestingT(t)

	cacheDir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheDir)
	t.Setenv("HOME", t.TempDir())

	Expect(hasGrypeVulnerabilityDB()).To(BeFalse(), "expected no grype DB in empty cache")

	dbPath := filepath.Join(cacheDir, "grype", "db", "6", "vulnerability.db")
	err := os.MkdirAll(filepath.Dir(dbPath), 0o755)
	Expect(err).ToNot(HaveOccurred())
	err = os.WriteFile(dbPath, []byte("db"), 0o644)
	Expect(err).ToNot(HaveOccurred())

	Expect(hasGrypeVulnerabilityDB()).To(BeTrue(), "expected grype DB to be detected")
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
