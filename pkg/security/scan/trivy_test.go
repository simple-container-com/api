package scan

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/gomega"
)

func TestTrivyScanner_Tool(t *testing.T) {
	RegisterTestingT(t)
	Expect(NewTrivyScanner().Tool()).To(Equal(ScanToolTrivy))
}

func TestNormalizeTrivySeverity(t *testing.T) {
	RegisterTestingT(t)

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
			RegisterTestingT(t)
			Expect(normalizeTrivySeverity(tt.input)).To(Equal(tt.expected))
		})
	}
}

func TestExtractImageDigestFromTrivy(t *testing.T) {
	RegisterTestingT(t)

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
			RegisterTestingT(t)
			Expect(extractImageDigestFromTrivy(tt.input)).To(Equal(tt.expected))
		})
	}
}

func TestTrivyScanner_CheckInstalled(t *testing.T) {
	RegisterTestingT(t)

	scanner := NewTrivyScanner()
	ctx := context.Background()

	err := scanner.CheckInstalled(ctx)
	if err != nil {
		t.Skipf("trivy not installed: %v", err)
	}
}

func TestTrivyScanner_Version(t *testing.T) {
	RegisterTestingT(t)

	scanner := NewTrivyScanner()
	ctx := context.Background()

	if err := scanner.CheckInstalled(ctx); err != nil {
		t.Skipf("trivy not installed: %v", err)
	}

	version, err := scanner.Version(ctx)
	Expect(err).ToNot(HaveOccurred())
	Expect(version).ToNot(BeEmpty())

	t.Logf("Trivy version: %s", version)
}

func TestTrivyScanner_CheckVersion(t *testing.T) {
	RegisterTestingT(t)

	scanner := NewTrivyScanner()
	ctx := context.Background()

	if err := scanner.CheckInstalled(ctx); err != nil {
		t.Skipf("trivy not installed: %v", err)
	}

	err := scanner.CheckVersion(ctx)
	if err != nil {
		t.Logf("CheckVersion() error = %v (this is expected if trivy version is below minimum)", err)
	}
}

func TestParseTrivyVersion(t *testing.T) {
	RegisterTestingT(t)

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
			RegisterTestingT(t)
			got, err := parseTrivyVersion(tt.input)
			if tt.wantErr {
				Expect(err).To(HaveOccurred())
				return
			}
			Expect(err).ToNot(HaveOccurred())
			Expect(got).To(Equal(tt.want))
		})
	}
}

func TestTrivyCVSS_UnmarshalJSON(t *testing.T) {
	RegisterTestingT(t)

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
			RegisterTestingT(t)
			var cvss trivyCVSS
			err := json.Unmarshal([]byte(tt.input), &cvss)
			if tt.wantErr {
				Expect(err).To(HaveOccurred())
				return
			}
			Expect(err).ToNot(HaveOccurred())
			Expect(extractTrivyCVSS(cvss)).To(Equal(tt.want))
		})
	}
}

func TestEnsureTrivyCacheDir(t *testing.T) {
	RegisterTestingT(t)

	cacheRoot := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheRoot)
	t.Setenv("HOME", t.TempDir())

	cacheDir, err := ensureTrivyCacheDir()
	Expect(err).ToNot(HaveOccurred())
	defer cleanupTrivyCacheDir(cacheDir)

	// Per-invocation cache dir lives under <cacheRoot>/trivy/scan-* so
	// concurrent scans can't clobber each other's lock files.
	parent := filepath.Join(cacheRoot, "trivy")
	Expect(cacheDir).To(HavePrefix(parent+string(filepath.Separator)+"scan-"))
	Expect(cacheDir).To(BeADirectory())

	// Second call returns a different directory (thread-safety property).
	cacheDir2, err := ensureTrivyCacheDir()
	Expect(err).ToNot(HaveOccurred())
	defer cleanupTrivyCacheDir(cacheDir2)
	Expect(cacheDir2).ToNot(Equal(cacheDir))

	// Cleanup removes the directory.
	cleanupTrivyCacheDir(cacheDir)
	_, statErr := os.Stat(cacheDir)
	Expect(os.IsNotExist(statErr)).To(BeTrue())
}

func TestTrivyDBPresenceHelpers(t *testing.T) {
	RegisterTestingT(t)

	cacheDir := t.TempDir()

	Expect(trivyDBPresent(cacheDir)).To(BeFalse(), "expected no trivy DB metadata in empty cache")
	Expect(trivyJavaDBPresent(cacheDir)).To(BeFalse(), "expected no trivy Java DB metadata in empty cache")

	dbMeta := filepath.Join(cacheDir, "db", "metadata.json")
	err := os.MkdirAll(filepath.Dir(dbMeta), 0o755)
	Expect(err).ToNot(HaveOccurred())
	err = os.WriteFile(dbMeta, []byte("{}"), 0o644)
	Expect(err).ToNot(HaveOccurred())

	javaMeta := filepath.Join(cacheDir, "java-db", "metadata.json")
	err = os.MkdirAll(filepath.Dir(javaMeta), 0o755)
	Expect(err).ToNot(HaveOccurred())
	err = os.WriteFile(javaMeta, []byte("{}"), 0o644)
	Expect(err).ToNot(HaveOccurred())

	Expect(trivyDBPresent(cacheDir)).To(BeTrue(), "expected trivy DB metadata to be detected")
	Expect(trivyJavaDBPresent(cacheDir)).To(BeTrue(), "expected trivy Java DB metadata to be detected")
}
