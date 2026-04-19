package scan

import (
	"testing"

	. "github.com/onsi/gomega"
)

func TestMergeResultsKeepsDistinctPackages(t *testing.T) {
	RegisterTestingT(t)

	first := NewScanResult("sha256:test", ScanToolGrype, []Vulnerability{
		{ID: "CVE-2024-0001", Severity: SeverityHigh, Package: "openssl", Version: "1.1.1"},
	})
	second := NewScanResult("sha256:test", ScanToolTrivy, []Vulnerability{
		{ID: "CVE-2024-0001", Severity: SeverityCritical, Package: "libssl", Version: "1.1.1"},
	})

	merged := MergeResults(first, second)
	Expect(merged).ToNot(BeNil())
	Expect(merged.Summary.Total).To(Equal(2))
}

func TestMergeResultsMergesSamePackageFinding(t *testing.T) {
	RegisterTestingT(t)

	first := NewScanResult("sha256:test", ScanToolGrype, []Vulnerability{
		{
			ID:          "CVE-2024-0001",
			Severity:    SeverityHigh,
			Package:     "openssl",
			Version:     "1.1.1",
			Description: "first description",
			URLs:        []string{"https://example.com/a"},
		},
	})
	second := NewScanResult("sha256:test", ScanToolTrivy, []Vulnerability{
		{
			ID:       "CVE-2024-0001",
			Severity: SeverityCritical,
			Package:  "openssl",
			Version:  "1.1.1",
			FixedIn:  "1.1.2",
			URLs:     []string{"https://example.com/b"},
			CVSS:     9.8,
		},
	})

	merged := MergeResults(first, second)
	Expect(merged).ToNot(BeNil())
	Expect(merged.Summary.Total).To(Equal(1))

	vuln := merged.Vulnerabilities[0]
	Expect(vuln.Severity).To(Equal(SeverityCritical))
	Expect(vuln.FixedIn).To(Equal("1.1.2"))
	Expect(vuln.URLs).To(HaveLen(2))
}
