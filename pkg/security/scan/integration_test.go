package scan

import (
	"context"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/gomega"
)

// Integration tests that run real scanner commands
// These tests will skip if the required tools are not installed

func TestGrypeScanner_Scan_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	RegisterTestingT(t)

	scanner := NewGrypeScanner()
	ctx := context.Background()

	// Check if grype is installed
	if err := scanner.CheckInstalled(ctx); err != nil {
		t.Skipf("grype not installed: %v", err)
	}

	// Use a small, well-known image for testing
	testImage := "alpine:3.17"

	t.Logf("Running grype scan on %s (this may take a while)...", testImage)

	result, err := scanner.Scan(ctx, testImage)
	if err != nil && isEnvironmentalScanError(err) {
		t.Skipf("Skipping grype integration test due to environment constraints: %v", err)
	}
	Expect(err).ToNot(HaveOccurred())

	Expect(result).ToNot(BeNil())
	Expect(result.Tool).To(Equal(ScanToolGrype))
	Expect(result.ScannedAt.IsZero()).To(BeFalse())
	Expect(result.Digest).ToNot(BeEmpty())

	// Validate summary
	t.Logf("Scan summary: %s", result.Summary.String())
	t.Logf("Total vulnerabilities: %d", result.Summary.Total)
	t.Logf("Critical: %d, High: %d, Medium: %d, Low: %d",
		result.Summary.Critical, result.Summary.High, result.Summary.Medium, result.Summary.Low)

	// Validate vulnerabilities have required fields
	for i, vuln := range result.Vulnerabilities {
		if i >= 5 {
			break // Just check first 5
		}
		Expect(vuln.ID).ToNot(BeEmpty(), "vulnerability %d: ID is empty", i)
		Expect(vuln.Package).ToNot(BeEmpty(), "vulnerability %d: Package is empty", i)
		Expect(vuln.Version).ToNot(BeEmpty(), "vulnerability %d: Version is empty", i)
		Expect(vuln.Severity).ToNot(BeEmpty(), "vulnerability %d: Severity is empty", i)

		t.Logf("Sample vuln %d: %s - %s (%s) in %s@%s",
			i, vuln.ID, vuln.Severity, vuln.Package, vuln.Version, vuln.FixedIn)
	}
}

func TestTrivyScanner_Scan_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	RegisterTestingT(t)

	scanner := NewTrivyScanner()
	ctx := context.Background()

	// Check if trivy is installed
	if err := scanner.CheckInstalled(ctx); err != nil {
		t.Skipf("trivy not installed: %v", err)
	}

	// Use a small, well-known image for testing
	testImage := "alpine:3.17"

	t.Logf("Running trivy scan on %s (this may take a while)...", testImage)

	result, err := scanner.Scan(ctx, testImage)
	if err != nil && isEnvironmentalScanError(err) {
		t.Skipf("Skipping trivy integration test due to environment constraints: %v", err)
	}
	Expect(err).ToNot(HaveOccurred())

	Expect(result).ToNot(BeNil())
	Expect(result.Tool).To(Equal(ScanToolTrivy))
	Expect(result.ScannedAt.IsZero()).To(BeFalse())

	// Validate summary
	t.Logf("Scan summary: %s", result.Summary.String())
	t.Logf("Total vulnerabilities: %d", result.Summary.Total)

	// Validate vulnerabilities have required fields
	for i, vuln := range result.Vulnerabilities {
		if i >= 5 {
			break // Just check first 5
		}
		Expect(vuln.ID).ToNot(BeEmpty(), "vulnerability %d: ID is empty", i)
		Expect(vuln.Package).ToNot(BeEmpty(), "vulnerability %d: Package is empty", i)
		Expect(vuln.Severity).ToNot(BeEmpty(), "vulnerability %d: Severity is empty", i)

		t.Logf("Sample vuln %d: %s - %s (%s) in %s",
			i, vuln.ID, vuln.Severity, vuln.Package, vuln.Version)
	}
}

func TestMergeResults_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	RegisterTestingT(t)

	grypeScanner := NewGrypeScanner()
	trivyScanner := NewTrivyScanner()
	ctx := context.Background()

	// Check if both scanners are installed
	grypeInstalled := grypeScanner.CheckInstalled(ctx) == nil
	trivyInstalled := trivyScanner.CheckInstalled(ctx) == nil

	if !grypeInstalled && !trivyInstalled {
		t.Skip("neither grype nor trivy is installed")
	}

	testImage := "alpine:3.17"

	var results []*ScanResult

	// Run grype if available
	if grypeInstalled {
		t.Log("Running grype scan...")
		result, err := grypeScanner.Scan(ctx, testImage)
		if err != nil {
			t.Logf("Grype scan failed: %v", err)
		} else {
			results = append(results, result)
			t.Logf("Grype found %d vulnerabilities", result.Summary.Total)
		}
	}

	// Run trivy if available
	if trivyInstalled {
		t.Log("Running trivy scan...")
		result, err := trivyScanner.Scan(ctx, testImage)
		if err != nil {
			t.Logf("Trivy scan failed: %v", err)
		} else {
			results = append(results, result)
			t.Logf("Trivy found %d vulnerabilities", result.Summary.Total)
		}
	}

	if len(results) < 2 {
		t.Skip("need at least 2 scan results to test merging")
	}

	// Merge results
	merged := MergeResults(results...)
	Expect(merged).ToNot(BeNil())
	Expect(merged.Tool).To(Equal(ScanToolAll))

	t.Logf("Merged result: %s", merged.Summary.String())
	t.Logf("Total after deduplication: %d", merged.Summary.Total)

	// Merged result should have <= sum of individual results (due to deduplication)
	totalBefore := 0
	for _, r := range results {
		totalBefore += r.Summary.Total
	}

	Expect(merged.Summary.Total).To(BeNumerically("<=", totalBefore))

	t.Logf("Deduplication: %d -> %d vulnerabilities", totalBefore, merged.Summary.Total)
}

func TestPolicyEnforcer_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	RegisterTestingT(t)

	scanner := NewGrypeScanner()
	ctx := context.Background()

	// Check if grype is installed
	if err := scanner.CheckInstalled(ctx); err != nil {
		t.Skipf("grype not installed: %v", err)
	}

	testImage := "alpine:3.17"

	result, err := scanner.Scan(ctx, testImage)
	if err != nil && isEnvironmentalScanError(err) {
		t.Skipf("Skipping policy integration test due to environment constraints: %v", err)
	}
	Expect(err).ToNot(HaveOccurred())

	// Test different policy levels
	policies := []struct {
		failOn      Severity
		shouldBlock bool
	}{
		{SeverityCritical, result.Summary.HasCritical()},
		{SeverityHigh, result.Summary.HasCritical() || result.Summary.HasHigh()},
		{SeverityMedium, result.Summary.HasCritical() || result.Summary.HasHigh() || result.Summary.HasMedium()},
		{SeverityLow, result.Summary.Total > 0},
	}

	for _, policy := range policies {
		t.Run(string(policy.failOn), func(t *testing.T) {
			RegisterTestingT(t)
			config := &Config{
				FailOn: policy.failOn,
			}
			enforcer := NewPolicyEnforcer(config)

			err := enforcer.Enforce(result)
			blocked := (err != nil)

			Expect(blocked).To(Equal(policy.shouldBlock),
				"policy %s: blocked = %v, want %v (summary: %s)",
				policy.failOn, blocked, policy.shouldBlock, result.Summary.String())

			if blocked {
				t.Logf("Policy %s correctly blocked: %v", policy.failOn, err)
			} else {
				t.Logf("Policy %s correctly allowed deployment", policy.failOn)
			}
		})
	}
}

func TestScanResult_ValidateDigest_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	RegisterTestingT(t)

	scanner := NewGrypeScanner()
	ctx := context.Background()

	// Check if grype is installed
	if err := scanner.CheckInstalled(ctx); err != nil {
		t.Skipf("grype not installed: %v", err)
	}

	testImage := "alpine:3.17"

	result, err := scanner.Scan(ctx, testImage)
	if err != nil && isEnvironmentalScanError(err) {
		t.Skipf("Skipping digest integration test due to environment constraints: %v", err)
	}
	Expect(err).ToNot(HaveOccurred())

	// Validate digest
	Expect(result.ValidateDigest()).ToNot(HaveOccurred())

	// Test with modified result
	originalDigest := result.Digest
	result.Digest = "sha256:invalid"
	Expect(result.ValidateDigest()).To(HaveOccurred())
	result.Digest = originalDigest
}

func TestVulnerabilitySummary_Methods(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name    string
		summary VulnerabilitySummary
		want    string
	}{
		{
			name: "with vulnerabilities",
			summary: VulnerabilitySummary{
				Critical: 3,
				High:     12,
				Medium:   45,
				Low:      103,
				Total:    163,
			},
			want: "Found 3 critical, 12 high, 45 medium, 103 low vulnerabilities",
		},
		{
			name:    "no vulnerabilities",
			summary: VulnerabilitySummary{},
			want:    "No vulnerabilities found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			Expect(tt.summary.String()).To(Equal(tt.want))
		})
	}
}

func TestNewScanResult(t *testing.T) {
	RegisterTestingT(t)

	vulns := []Vulnerability{
		{ID: "CVE-2023-0001", Severity: SeverityCritical, Package: "pkg1"},
		{ID: "CVE-2023-0002", Severity: SeverityHigh, Package: "pkg2"},
		{ID: "CVE-2023-0003", Severity: SeverityMedium, Package: "pkg3"},
	}

	result := NewScanResult("sha256:test", ScanToolGrype, vulns)

	Expect(result.ImageDigest).To(Equal("sha256:test"))
	Expect(result.Tool).To(Equal(ScanToolGrype))
	Expect(result.Vulnerabilities).To(HaveLen(3))
	Expect(result.Summary.Total).To(Equal(3))
	Expect(result.Summary.Critical).To(Equal(1))
	Expect(result.ScannedAt.IsZero()).To(BeFalse())
	Expect(result.Digest).ToNot(BeEmpty())

	// Test digest calculation consistency
	digest1 := result.calculateDigest()
	time.Sleep(10 * time.Millisecond)
	digest2 := result.calculateDigest()

	Expect(digest1).To(Equal(digest2))
}

func isEnvironmentalScanError(err error) bool {
	if err == nil {
		return false
	}

	message := strings.ToLower(err.Error())
	environmentalFailures := []string{
		"read-only file system",
		"unable to initialize cache",
		"failed to fetch latest version",
		"lookup ",
		"dial tcp",
		"socket: operation not permitted",
		"operation not permitted",
		"connection refused",
		"timeout",
		"no such host",
		"cannot connect to the docker daemon",
		"failed to catalog",
		"manifest unknown",
	}

	for _, marker := range environmentalFailures {
		if strings.Contains(message, marker) {
			return true
		}
	}

	return false
}
