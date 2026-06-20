package reporting

import (
	"strings"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/security/scan"
)

func TestBuildScanResultsComment(t *testing.T) {
	RegisterTestingT(t)

	t.Run("nil result emits placeholder", func(t *testing.T) {
		RegisterTestingT(t)
		out := BuildScanResultsComment("image:tag", nil, nil)
		Expect(out).To(ContainSubstring(scanCommentMarker))
		Expect(out).To(ContainSubstring("No scan results were produced."))
		// Should not render the severity table when there is no result.
		Expect(out).ToNot(ContainSubstring("| Severity | Count |"))
	})

	t.Run("clean result renders summary table and no findings line", func(t *testing.T) {
		RegisterTestingT(t)
		result := &scan.ScanResult{
			ImageDigest: "sha256:abc123",
			Tool:        scan.ScanToolGrype,
			Summary: scan.VulnerabilitySummary{
				Medium: 2, Low: 1, Total: 3,
			},
		}
		out := BuildScanResultsComment("registry/app:1.0", result, nil)

		Expect(out).To(HavePrefix(scanCommentMarker))
		Expect(out).To(ContainSubstring("Image: `registry/app:1.0`"))
		Expect(out).To(ContainSubstring("Digest: `sha256:abc123`"))
		Expect(out).To(ContainSubstring("Scanners: `grype`"))
		Expect(out).To(ContainSubstring("| Critical | 0 |"))
		Expect(out).To(ContainSubstring("| Medium | 2 |"))
		Expect(out).To(ContainSubstring("| Total | 3 |"))
		// No critical/high findings -> the "none" line, not the findings table.
		Expect(out).To(ContainSubstring("No critical or high vulnerabilities were found."))
		Expect(out).ToNot(ContainSubstring("### Top Critical/High Findings"))
	})

	t.Run("omits digest line when empty", func(t *testing.T) {
		RegisterTestingT(t)
		result := &scan.ScanResult{Tool: scan.ScanToolTrivy}
		out := BuildScanResultsComment("img", result, nil)
		Expect(out).ToNot(ContainSubstring("Digest:"))
	})

	t.Run("renders top findings table with fixed-version fallback", func(t *testing.T) {
		RegisterTestingT(t)
		result := &scan.ScanResult{
			Tool: scan.ScanToolGrype,
			Summary: scan.VulnerabilitySummary{
				Critical: 1, High: 1, Total: 2,
			},
			Vulnerabilities: []scan.Vulnerability{
				{ID: "CVE-2024-0001", Severity: scan.SeverityCritical, Package: "openssl", Version: "1.1.1", FixedIn: "1.1.1k"},
				{ID: "CVE-2024-0002", Severity: scan.SeverityHigh, Package: "zlib", Version: "1.2.11"}, // no FixedIn -> "-"
				{ID: "CVE-2024-0003", Severity: scan.SeverityMedium, Package: "curl", Version: "7.0"},  // filtered out
			},
		}
		out := BuildScanResultsComment("img", result, nil)

		Expect(out).To(ContainSubstring("### Top Critical/High Findings"))
		Expect(out).To(ContainSubstring("| CRITICAL | `CVE-2024-0001` | `openssl` | `1.1.1` | `1.1.1k` |"))
		// Empty FixedIn renders as a dash.
		Expect(out).To(ContainSubstring("| HIGH | `CVE-2024-0002` | `zlib` | `1.2.11` | `-` |"))
		// Medium severity must not appear in the top-findings table.
		Expect(out).ToNot(ContainSubstring("CVE-2024-0003"))
	})

	t.Run("renders upload section with success and failure states", func(t *testing.T) {
		RegisterTestingT(t)
		result := &scan.ScanResult{Tool: scan.ScanToolGrype}
		uploads := []*UploadSummary{
			{Target: "defectdojo", Success: true, URL: "https://dd/test/1"},
			{Target: "s3", Success: false},
		}
		out := BuildScanResultsComment("img", result, uploads)

		Expect(out).To(ContainSubstring("### Report Uploads"))
		Expect(out).To(ContainSubstring("- `defectdojo`: uploaded (https://dd/test/1)"))
		Expect(out).To(ContainSubstring("- `s3`: failed"))
	})

	t.Run("no upload section when uploads empty", func(t *testing.T) {
		RegisterTestingT(t)
		out := BuildScanResultsComment("img", &scan.ScanResult{Tool: scan.ScanToolGrype}, nil)
		Expect(out).ToNot(ContainSubstring("### Report Uploads"))
	})
}

func TestScanTools(t *testing.T) {
	RegisterTestingT(t)

	t.Run("nil result returns nil", func(t *testing.T) {
		RegisterTestingT(t)
		Expect(scanTools(nil)).To(BeNil())
	})

	t.Run("mergedTools as []scan.ScanTool sorted", func(t *testing.T) {
		RegisterTestingT(t)
		result := &scan.ScanResult{
			Metadata: map[string]interface{}{
				"mergedTools": []scan.ScanTool{scan.ScanToolTrivy, scan.ScanToolGrype},
			},
		}
		Expect(scanTools(result)).To(Equal([]string{"grype", "trivy"}))
	})

	t.Run("mergedTools as []interface{} skips non-string and empty entries", func(t *testing.T) {
		RegisterTestingT(t)
		result := &scan.ScanResult{
			Metadata: map[string]interface{}{
				"mergedTools": []interface{}{"trivy", "", 7, "grype"},
			},
		}
		Expect(scanTools(result)).To(Equal([]string{"grype", "trivy"}))
	})

	t.Run("falls back to single Tool when no merged metadata", func(t *testing.T) {
		RegisterTestingT(t)
		result := &scan.ScanResult{Tool: scan.ScanToolTrivy}
		Expect(scanTools(result)).To(Equal([]string{"trivy"}))
	})

	t.Run("returns nil when no tool and no metadata", func(t *testing.T) {
		RegisterTestingT(t)
		Expect(scanTools(&scan.ScanResult{})).To(BeNil())
	})

	t.Run("metadata present but mergedTools missing falls back to Tool", func(t *testing.T) {
		RegisterTestingT(t)
		result := &scan.ScanResult{
			Tool:     scan.ScanToolGrype,
			Metadata: map[string]interface{}{"other": "value"},
		}
		Expect(scanTools(result)).To(Equal([]string{"grype"}))
	})
}

func TestTopFindings(t *testing.T) {
	RegisterTestingT(t)

	t.Run("nil result returns nil", func(t *testing.T) {
		RegisterTestingT(t)
		Expect(topFindings(nil, 5)).To(BeNil())
	})

	t.Run("non-positive limit returns nil", func(t *testing.T) {
		RegisterTestingT(t)
		result := &scan.ScanResult{
			Vulnerabilities: []scan.Vulnerability{
				{ID: "CVE-1", Severity: scan.SeverityCritical, Package: "a"},
			},
		}
		Expect(topFindings(result, 0)).To(BeNil())
		Expect(topFindings(result, -1)).To(BeNil())
	})

	t.Run("filters to critical+high and sorts by severity then package then id", func(t *testing.T) {
		RegisterTestingT(t)
		result := &scan.ScanResult{
			Vulnerabilities: []scan.Vulnerability{
				{ID: "CVE-300", Severity: scan.SeverityHigh, Package: "zlib"},
				{ID: "CVE-100", Severity: scan.SeverityCritical, Package: "openssl"},
				{ID: "CVE-200", Severity: scan.SeverityMedium, Package: "curl"}, // dropped
				{ID: "CVE-050", Severity: scan.SeverityHigh, Package: "aaa"},
				{ID: "CVE-051", Severity: scan.SeverityHigh, Package: "aaa"}, // same pkg, id tiebreak
				{ID: "CVE-400", Severity: scan.SeverityLow, Package: "x"},    // dropped
			},
		}
		got := topFindings(result, 10)
		ids := make([]string, 0, len(got))
		for _, v := range got {
			ids = append(ids, v.ID)
		}
		// critical first; then high sorted by package (aaa<zlib), within aaa by id.
		Expect(ids).To(Equal([]string{"CVE-100", "CVE-050", "CVE-051", "CVE-300"}))
	})

	t.Run("applies the limit after sorting", func(t *testing.T) {
		RegisterTestingT(t)
		result := &scan.ScanResult{
			Vulnerabilities: []scan.Vulnerability{
				{ID: "CVE-1", Severity: scan.SeverityHigh, Package: "b"},
				{ID: "CVE-2", Severity: scan.SeverityCritical, Package: "c"},
				{ID: "CVE-3", Severity: scan.SeverityHigh, Package: "a"},
			},
		}
		got := topFindings(result, 2)
		Expect(got).To(HaveLen(2))
		Expect(got[0].ID).To(Equal("CVE-2")) // critical
		Expect(got[1].ID).To(Equal("CVE-3")) // high, package "a" < "b"
	})

	t.Run("no critical or high yields empty slice", func(t *testing.T) {
		RegisterTestingT(t)
		result := &scan.ScanResult{
			Vulnerabilities: []scan.Vulnerability{
				{ID: "CVE-1", Severity: scan.SeverityMedium, Package: "a"},
				{ID: "CVE-2", Severity: scan.SeverityLow, Package: "b"},
			},
		}
		Expect(topFindings(result, 5)).To(HaveLen(0))
	})
}

func TestSeverityRank(t *testing.T) {
	RegisterTestingT(t)

	cases := []struct {
		name     string
		severity scan.Severity
		want     int
	}{
		{"critical", scan.SeverityCritical, 4},
		{"high", scan.SeverityHigh, 3},
		{"medium", scan.SeverityMedium, 2},
		{"low", scan.SeverityLow, 1},
		{"unknown", scan.SeverityUnknown, 0},
		{"unrecognized", scan.Severity("bogus"), 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			Expect(severityRank(tc.severity)).To(Equal(tc.want))
		})
	}
}

// guard against accidental marker drift; PR-comment update logic keys off it.
func TestScanCommentMarkerStable(t *testing.T) {
	RegisterTestingT(t)
	Expect(strings.HasPrefix(BuildScanResultsComment("x", nil, nil), scanCommentMarker)).To(BeTrue())
}
