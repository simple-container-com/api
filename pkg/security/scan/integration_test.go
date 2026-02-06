package scan

import (
	"context"
	"testing"
	"time"
)

// Integration tests that run real scanner commands
// These tests will skip if the required tools are not installed

func TestGrypeScanner_Scan_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

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
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}

	if result == nil {
		t.Fatal("Scan() returned nil result")
	}

	// Validate result structure
	if result.Tool != ScanToolGrype {
		t.Errorf("result.Tool = %s, want %s", result.Tool, ScanToolGrype)
	}

	if result.ScannedAt.IsZero() {
		t.Error("result.ScannedAt is zero")
	}

	if result.Digest == "" {
		t.Error("result.Digest is empty")
	}

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

		if vuln.ID == "" {
			t.Errorf("vulnerability %d: ID is empty", i)
		}
		if vuln.Package == "" {
			t.Errorf("vulnerability %d: Package is empty", i)
		}
		if vuln.Version == "" {
			t.Errorf("vulnerability %d: Version is empty", i)
		}
		if vuln.Severity == "" {
			t.Errorf("vulnerability %d: Severity is empty", i)
		}

		t.Logf("Sample vuln %d: %s - %s (%s) in %s@%s",
			i, vuln.ID, vuln.Severity, vuln.Package, vuln.Version, vuln.FixedIn)
	}
}

func TestTrivyScanner_Scan_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

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
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}

	if result == nil {
		t.Fatal("Scan() returned nil result")
	}

	// Validate result structure
	if result.Tool != ScanToolTrivy {
		t.Errorf("result.Tool = %s, want %s", result.Tool, ScanToolTrivy)
	}

	if result.ScannedAt.IsZero() {
		t.Error("result.ScannedAt is zero")
	}

	// Validate summary
	t.Logf("Scan summary: %s", result.Summary.String())
	t.Logf("Total vulnerabilities: %d", result.Summary.Total)

	// Validate vulnerabilities have required fields
	for i, vuln := range result.Vulnerabilities {
		if i >= 5 {
			break // Just check first 5
		}

		if vuln.ID == "" {
			t.Errorf("vulnerability %d: ID is empty", i)
		}
		if vuln.Package == "" {
			t.Errorf("vulnerability %d: Package is empty", i)
		}
		if vuln.Severity == "" {
			t.Errorf("vulnerability %d: Severity is empty", i)
		}

		t.Logf("Sample vuln %d: %s - %s (%s) in %s",
			i, vuln.ID, vuln.Severity, vuln.Package, vuln.Version)
	}
}

func TestMergeResults_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

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

	if merged == nil {
		t.Fatal("MergeResults returned nil")
	}

	if merged.Tool != ScanToolAll {
		t.Errorf("merged.Tool = %s, want %s", merged.Tool, ScanToolAll)
	}

	t.Logf("Merged result: %s", merged.Summary.String())
	t.Logf("Total after deduplication: %d", merged.Summary.Total)

	// Merged result should have <= sum of individual results (due to deduplication)
	totalBefore := 0
	for _, r := range results {
		totalBefore += r.Summary.Total
	}

	if merged.Summary.Total > totalBefore {
		t.Errorf("merged total %d > sum of individual totals %d", merged.Summary.Total, totalBefore)
	}

	t.Logf("Deduplication: %d → %d vulnerabilities", totalBefore, merged.Summary.Total)
}

func TestPolicyEnforcer_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	scanner := NewGrypeScanner()
	ctx := context.Background()

	// Check if grype is installed
	if err := scanner.CheckInstalled(ctx); err != nil {
		t.Skipf("grype not installed: %v", err)
	}

	testImage := "alpine:3.17"

	result, err := scanner.Scan(ctx, testImage)
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}

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
			config := &Config{
				FailOn: policy.failOn,
			}
			enforcer := NewPolicyEnforcer(config)

			err := enforcer.Enforce(result)
			blocked := (err != nil)

			if blocked != policy.shouldBlock {
				t.Errorf("policy %s: blocked = %v, want %v (summary: %s)",
					policy.failOn, blocked, policy.shouldBlock, result.Summary.String())
			}

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

	scanner := NewGrypeScanner()
	ctx := context.Background()

	// Check if grype is installed
	if err := scanner.CheckInstalled(ctx); err != nil {
		t.Skipf("grype not installed: %v", err)
	}

	testImage := "alpine:3.17"

	result, err := scanner.Scan(ctx, testImage)
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}

	// Validate digest
	if err := result.ValidateDigest(); err != nil {
		t.Errorf("ValidateDigest() error = %v", err)
	}

	// Test with modified result
	originalDigest := result.Digest
	result.Digest = "sha256:invalid"
	if err := result.ValidateDigest(); err == nil {
		t.Error("ValidateDigest() should fail with invalid digest")
	}
	result.Digest = originalDigest
}

func TestVulnerabilitySummary_Methods(t *testing.T) {
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
			got := tt.summary.String()
			if got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNewScanResult(t *testing.T) {
	vulns := []Vulnerability{
		{ID: "CVE-2023-0001", Severity: SeverityCritical, Package: "pkg1"},
		{ID: "CVE-2023-0002", Severity: SeverityHigh, Package: "pkg2"},
		{ID: "CVE-2023-0003", Severity: SeverityMedium, Package: "pkg3"},
	}

	result := NewScanResult("sha256:test", ScanToolGrype, vulns)

	if result.ImageDigest != "sha256:test" {
		t.Errorf("ImageDigest = %s, want sha256:test", result.ImageDigest)
	}

	if result.Tool != ScanToolGrype {
		t.Errorf("Tool = %s, want %s", result.Tool, ScanToolGrype)
	}

	if len(result.Vulnerabilities) != 3 {
		t.Errorf("len(Vulnerabilities) = %d, want 3", len(result.Vulnerabilities))
	}

	if result.Summary.Total != 3 {
		t.Errorf("Summary.Total = %d, want 3", result.Summary.Total)
	}

	if result.Summary.Critical != 1 {
		t.Errorf("Summary.Critical = %d, want 1", result.Summary.Critical)
	}

	if result.ScannedAt.IsZero() {
		t.Error("ScannedAt is zero")
	}

	if result.Digest == "" {
		t.Error("Digest is empty")
	}

	// Test digest calculation consistency
	digest1 := result.calculateDigest()
	time.Sleep(10 * time.Millisecond)
	digest2 := result.calculateDigest()

	if digest1 != digest2 {
		t.Error("Digest calculation is not consistent")
	}
}
