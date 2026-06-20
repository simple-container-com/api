// SPDX-License-Identifier: MIT
// Copyright (c) 2025 Simple Container

package scan

import (
	"testing"

	. "github.com/onsi/gomega"
)

func TestScanResult_ValidateDigest(t *testing.T) {
	RegisterTestingT(t)

	t.Run("freshly constructed result validates", func(t *testing.T) {
		RegisterTestingT(t)
		result := NewScanResult("sha256:abc", ScanToolGrype, []Vulnerability{
			{ID: "CVE-2024-1", Severity: SeverityHigh, Package: "openssl", Version: "1.1.1"},
		})
		Expect(result.ValidateDigest()).To(Succeed())
	})

	t.Run("tampered digest fails validation", func(t *testing.T) {
		RegisterTestingT(t)
		result := NewScanResult("sha256:abc", ScanToolGrype, []Vulnerability{
			{ID: "CVE-2024-1", Severity: SeverityHigh, Package: "openssl", Version: "1.1.1"},
		})
		result.Digest = "sha256:deadbeef"
		err := result.ValidateDigest()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("digest mismatch"))
		Expect(err.Error()).To(ContainSubstring("sha256:deadbeef"))
	})

	t.Run("empty vulnerabilities still produces a stable digest", func(t *testing.T) {
		RegisterTestingT(t)
		result := NewScanResult("sha256:abc", ScanToolTrivy, nil)
		Expect(result.Digest).To(HavePrefix("sha256:"))
		Expect(result.ValidateDigest()).To(Succeed())
	})
}

func TestVulnerabilitySummary_String(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name    string
		summary VulnerabilitySummary
		want    string
	}{
		{
			name:    "no vulnerabilities",
			summary: VulnerabilitySummary{},
			want:    "No vulnerabilities found",
		},
		{
			name:    "with counts",
			summary: VulnerabilitySummary{Critical: 1, High: 2, Medium: 3, Low: 4, Total: 10},
			want:    "Found 1 critical, 2 high, 3 medium, 4 low vulnerabilities",
		},
		{
			name:    "total non-zero but unknown-only still renders",
			summary: VulnerabilitySummary{Unknown: 2, Total: 2},
			want:    "Found 0 critical, 0 high, 0 medium, 0 low vulnerabilities",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			Expect(tt.summary.String()).To(Equal(tt.want))
		})
	}
}

func TestSeverityPriority(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		input Severity
		want  int
	}{
		{SeverityCritical, 4},
		{SeverityHigh, 3},
		{SeverityMedium, 2},
		{SeverityLow, 1},
		{SeverityUnknown, 0},
		{"bogus-default", 0},
	}

	for _, tt := range tests {
		t.Run(string(tt.input), func(t *testing.T) {
			RegisterTestingT(t)
			Expect(severityPriority(tt.input)).To(Equal(tt.want))
		})
	}
}

func TestSummarizeVulnerabilities_AllSeverities(t *testing.T) {
	RegisterTestingT(t)

	vulns := []Vulnerability{
		{Severity: SeverityCritical},
		{Severity: SeverityHigh},
		{Severity: SeverityHigh},
		{Severity: SeverityMedium},
		{Severity: SeverityLow},
		{Severity: SeverityUnknown},
		{Severity: "weird-value-falls-through"},
	}

	summary := summarizeVulnerabilities(vulns)
	Expect(summary.Total).To(Equal(7))
	Expect(summary.Critical).To(Equal(1))
	Expect(summary.High).To(Equal(2))
	Expect(summary.Medium).To(Equal(1))
	Expect(summary.Low).To(Equal(1))
	Expect(summary.Unknown).To(Equal(1))
	// The "weird-value" severity falls through the switch with no counter bumped
	// but Total is still incremented, so the buckets don't sum to Total.
	Expect(summary.Critical + summary.High + summary.Medium + summary.Low + summary.Unknown).To(Equal(6))
}

func TestMergeResults_Empty(t *testing.T) {
	RegisterTestingT(t)
	Expect(MergeResults()).To(BeNil())
}

func TestMergeResults_SkipsNilEntries(t *testing.T) {
	RegisterTestingT(t)

	first := NewScanResult("sha256:img", ScanToolGrype, []Vulnerability{
		{ID: "CVE-2024-9", Severity: SeverityMedium, Package: "zlib", Version: "1.2"},
	})

	merged := MergeResults(nil, first, nil)
	Expect(merged).ToNot(BeNil())
	Expect(merged.ImageDigest).To(Equal("sha256:img"))
	Expect(merged.Tool).To(Equal(ScanToolAll))
	Expect(merged.Summary.Total).To(Equal(1))
	Expect(merged.Metadata).To(HaveKey("mergedTools"))
}

func TestMergeResults_FirstNonEmptyImageDigestWins(t *testing.T) {
	RegisterTestingT(t)

	// First result has no image digest; second supplies one. The merge keeps the
	// first NON-EMPTY value it sees, so the second result's digest is adopted.
	first := NewScanResult("", ScanToolGrype, []Vulnerability{
		{ID: "CVE-A", Severity: SeverityLow, Package: "a", Version: "1"},
	})
	second := NewScanResult("sha256:second", ScanToolTrivy, []Vulnerability{
		{ID: "CVE-B", Severity: SeverityLow, Package: "b", Version: "1"},
	})

	merged := MergeResults(first, second)
	Expect(merged).ToNot(BeNil())
	Expect(merged.ImageDigest).To(Equal("sha256:second"))
}

func TestMergeResults_SortOrder(t *testing.T) {
	RegisterTestingT(t)

	// Distinct packages across severities; expect descending severity order,
	// then ascending package, version, id within a severity.
	result := NewScanResult("sha256:x", ScanToolGrype, []Vulnerability{
		{ID: "CVE-LOW", Severity: SeverityLow, Package: "zpkg", Version: "1"},
		{ID: "CVE-CRIT", Severity: SeverityCritical, Package: "apkg", Version: "1"},
		{ID: "CVE-HIGH-B", Severity: SeverityHigh, Package: "bpkg", Version: "1"},
		{ID: "CVE-HIGH-A", Severity: SeverityHigh, Package: "apkg", Version: "1"},
	})

	merged := MergeResults(result)
	Expect(merged).ToNot(BeNil())
	Expect(merged.Vulnerabilities).To(HaveLen(4))

	got := make([]string, 0, len(merged.Vulnerabilities))
	for _, v := range merged.Vulnerabilities {
		got = append(got, v.ID)
	}
	// critical first, then the two highs (apkg before bpkg by package name), then low.
	Expect(got).To(Equal([]string{"CVE-CRIT", "CVE-HIGH-A", "CVE-HIGH-B", "CVE-LOW"}))
}

func TestMergeResults_SameSeverityTieBreakByVersionThenID(t *testing.T) {
	RegisterTestingT(t)

	// Same package + severity, differing versions then IDs to exercise the deeper
	// tie-break branches of the sort comparator.
	result := NewScanResult("sha256:x", ScanToolGrype, []Vulnerability{
		{ID: "CVE-2", Severity: SeverityHigh, Package: "openssl", Version: "2.0"},
		{ID: "CVE-1", Severity: SeverityHigh, Package: "openssl", Version: "1.0"},
		{ID: "CVE-0", Severity: SeverityHigh, Package: "openssl", Version: "2.0"},
	})

	merged := MergeResults(result)
	got := make([]string, 0, len(merged.Vulnerabilities))
	for _, v := range merged.Vulnerabilities {
		got = append(got, v.Version+"/"+v.ID)
	}
	Expect(got).To(Equal([]string{"1.0/CVE-1", "2.0/CVE-0", "2.0/CVE-2"}))
}

func TestMergeVulnerability_LowerSeverityCandidateDoesNotDowngrade(t *testing.T) {
	RegisterTestingT(t)

	// Same package-level key reported by two scanners; the second has a LOWER
	// severity and a lower CVSS, so neither should override the existing values.
	high := NewScanResult("sha256:x", ScanToolGrype, []Vulnerability{
		{
			ID: "CVE-2024-77", Severity: SeverityCritical, Package: "openssl", Version: "1.1.1",
			FixedIn: "1.1.2", Description: "orig", CVSS: 9.8, URLs: []string{"https://a"},
		},
	})
	low := NewScanResult("sha256:x", ScanToolTrivy, []Vulnerability{
		{
			ID: "CVE-2024-77", Severity: SeverityLow, Package: "openssl", Version: "1.1.1",
			FixedIn: "should-not-overwrite", Description: "should-not-overwrite", CVSS: 2.0,
			URLs: []string{"https://a", "https://b"},
		},
	})

	merged := MergeResults(high, low)
	Expect(merged.Vulnerabilities).To(HaveLen(1))
	v := merged.Vulnerabilities[0]
	Expect(v.Severity).To(Equal(SeverityCritical))
	Expect(v.FixedIn).To(Equal("1.1.2"))
	Expect(v.Description).To(Equal("orig"))
	Expect(v.CVSS).To(Equal(9.8))
	// URLs deduped: only the new "https://b" gets appended.
	Expect(v.URLs).To(ConsistOf("https://a", "https://b"))
}

func TestMergeVulnerability_FillsEmptyFieldsFromCandidate(t *testing.T) {
	RegisterTestingT(t)

	// Existing finding is sparse; candidate (same key) fills FixedIn/Description and
	// raises CVSS + severity.
	sparse := NewScanResult("sha256:x", ScanToolGrype, []Vulnerability{
		{ID: "CVE-2024-88", Severity: SeverityMedium, Package: "curl", Version: "8.0"},
	})
	rich := NewScanResult("sha256:x", ScanToolTrivy, []Vulnerability{
		{
			ID: "CVE-2024-88", Severity: SeverityHigh, Package: "curl", Version: "8.0",
			FixedIn: "8.1", Description: "buffer overflow", CVSS: 8.1, URLs: []string{"https://curl"},
		},
	})

	merged := MergeResults(sparse, rich)
	Expect(merged.Vulnerabilities).To(HaveLen(1))
	v := merged.Vulnerabilities[0]
	Expect(v.Severity).To(Equal(SeverityHigh))
	Expect(v.FixedIn).To(Equal("8.1"))
	Expect(v.Description).To(Equal("buffer overflow"))
	Expect(v.CVSS).To(Equal(8.1))
	Expect(v.URLs).To(ConsistOf("https://curl"))
}

func TestAppendUnique(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name       string
		existing   []string
		candidates []string
		want       []string
	}{
		{
			name:       "appends new values preserving order",
			existing:   []string{"a", "b"},
			candidates: []string{"c", "d"},
			want:       []string{"a", "b", "c", "d"},
		},
		{
			name:       "skips duplicates against existing",
			existing:   []string{"a", "b"},
			candidates: []string{"b", "a"},
			want:       []string{"a", "b"},
		},
		{
			name:       "skips duplicates within candidates",
			existing:   nil,
			candidates: []string{"x", "x", "y"},
			want:       []string{"x", "y"},
		},
		{
			name:       "skips empty-string candidates",
			existing:   []string{"a"},
			candidates: []string{"", "b", ""},
			want:       []string{"a", "b"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			got := appendUnique(tt.existing, tt.candidates...)
			Expect(got).To(Equal(tt.want))
		})
	}

	t.Run("nil inputs yield nil (append of nil slice stays nil)", func(t *testing.T) {
		RegisterTestingT(t)
		Expect(appendUnique(nil)).To(BeNil())
	})
}

func TestVulnerabilityKey(t *testing.T) {
	RegisterTestingT(t)

	v := Vulnerability{ID: "CVE-1", Package: "openssl", Version: "1.1.1"}
	Expect(vulnerabilityKey(v)).To(Equal("CVE-1|openssl|1.1.1"))

	// Same ID, different package -> different key (distinct findings).
	other := Vulnerability{ID: "CVE-1", Package: "libssl", Version: "1.1.1"}
	Expect(vulnerabilityKey(other)).ToNot(Equal(vulnerabilityKey(v)))
}
