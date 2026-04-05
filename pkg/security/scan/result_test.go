package scan

import "testing"

func TestMergeResultsKeepsDistinctPackages(t *testing.T) {
	first := NewScanResult("sha256:test", ScanToolGrype, []Vulnerability{
		{ID: "CVE-2024-0001", Severity: SeverityHigh, Package: "openssl", Version: "1.1.1"},
	})
	second := NewScanResult("sha256:test", ScanToolTrivy, []Vulnerability{
		{ID: "CVE-2024-0001", Severity: SeverityCritical, Package: "libssl", Version: "1.1.1"},
	})

	merged := MergeResults(first, second)
	if merged == nil {
		t.Fatal("MergeResults returned nil")
	}
	if merged.Summary.Total != 2 {
		t.Fatalf("expected 2 merged vulnerabilities, got %d", merged.Summary.Total)
	}
}

func TestMergeResultsMergesSamePackageFinding(t *testing.T) {
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
	if merged == nil {
		t.Fatal("MergeResults returned nil")
	}
	if merged.Summary.Total != 1 {
		t.Fatalf("expected 1 merged vulnerability, got %d", merged.Summary.Total)
	}

	vuln := merged.Vulnerabilities[0]
	if vuln.Severity != SeverityCritical {
		t.Fatalf("expected critical severity, got %s", vuln.Severity)
	}
	if vuln.FixedIn != "1.1.2" {
		t.Fatalf("expected fixed version to be preserved, got %q", vuln.FixedIn)
	}
	if len(vuln.URLs) != 2 {
		t.Fatalf("expected URLs to be merged, got %d", len(vuln.URLs))
	}
}
