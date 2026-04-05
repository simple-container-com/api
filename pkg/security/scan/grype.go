package scan

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// GrypeScanner implements Scanner interface using Grype
type GrypeScanner struct {
	minVersion string
}

// NewGrypeScanner creates a new GrypeScanner
func NewGrypeScanner() *GrypeScanner {
	return &GrypeScanner{
		minVersion: "0.106.0",
	}
}

// Tool returns the scanner tool name
func (g *GrypeScanner) Tool() ScanTool {
	return ScanToolGrype
}

// Scan performs vulnerability scanning using grype
func (g *GrypeScanner) Scan(ctx context.Context, image string) (*ScanResult, error) {
	// Check if grype is installed
	if err := g.CheckInstalled(ctx); err != nil {
		return nil, fmt.Errorf("grype not installed: %w", err)
	}

	// Run grype scan
	cmd := exec.CommandContext(ctx, "grype", "--quiet", "-o", "json", "registry:"+image)
	cmd.Env = append(os.Environ(), grypeCommandEnv(hasGrypeVulnerabilityDB())...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("grype scan failed: %w (stderr: %s)", err, strings.TrimSpace(stderr.String()))
	}

	output := bytes.TrimSpace(stdout.Bytes())
	if len(output) == 0 {
		return nil, fmt.Errorf("grype produced empty output (stderr: %s)", strings.TrimSpace(stderr.String()))
	}

	// Parse grype JSON output
	var grypeOutput GrypeOutput
	if err := json.Unmarshal(output, &grypeOutput); err != nil {
		return nil, fmt.Errorf("failed to parse grype output: %w", err)
	}

	// Convert to ScanResult
	vulns := make([]Vulnerability, 0, len(grypeOutput.Matches))
	for _, match := range grypeOutput.Matches {
		vuln := Vulnerability{
			ID:          match.Vulnerability.ID,
			Severity:    normalizeSeverity(match.Vulnerability.Severity),
			Package:     match.Artifact.Name,
			Version:     match.Artifact.Version,
			Description: match.Vulnerability.Description,
			URLs:        extractURLs(match.Vulnerability),
			CVSS:        extractCVSS(match.Vulnerability),
		}

		// Extract fixed version
		if match.Vulnerability.Fix.State == "fixed" {
			for _, version := range match.Vulnerability.Fix.Versions {
				vuln.FixedIn = version
				break
			}
		}

		vulns = append(vulns, vuln)
	}

	// Extract image digest from descriptor
	imageDigest := image
	if grypeOutput.Descriptor.Name != "" {
		imageDigest = extractImageDigestFromGrype(grypeOutput.Descriptor.Name)
	}

	result := NewScanResult(imageDigest, ScanToolGrype, vulns)
	if version, err := g.Version(ctx); err == nil {
		result.Metadata["grypeVersion"] = version
	}

	return result, nil
}

// Version returns the grype version
func (g *GrypeScanner) Version(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "grype", "version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to get grype version: %w", err)
	}

	return parseGrypeVersion(string(output))
}

// CheckInstalled checks if grype is installed
func (g *GrypeScanner) CheckInstalled(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "grype", "version")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("grype is not installed or not in PATH. Install from: https://github.com/anchore/grype")
	}
	return nil
}

// CheckVersion checks if grype meets minimum version requirements
func (g *GrypeScanner) CheckVersion(ctx context.Context) error {
	version, err := g.Version(ctx)
	if err != nil {
		return err
	}

	if !isVersionGreaterOrEqual(version, g.minVersion) {
		return fmt.Errorf("grype version %s is below minimum required version %s", version, g.minVersion)
	}

	return nil
}

// GrypeOutput represents grype JSON output structure
type GrypeOutput struct {
	Matches    []grypeMatch `json:"matches"`
	Descriptor struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"descriptor"`
}

type grypeMatch struct {
	Vulnerability grypeVulnerability `json:"vulnerability"`
	Artifact      grypeArtifact      `json:"artifact"`
}

type grypeVulnerability struct {
	ID          string      `json:"id"`
	Severity    string      `json:"severity"`
	Description string      `json:"description"`
	Fix         grypeFix    `json:"fix"`
	Cvss        []grypeCVSS `json:"cvss"`
	URLs        []string    `json:"urls"`
}

type grypeFix struct {
	State    string   `json:"state"`
	Versions []string `json:"versions"`
}

type grypeCVSS struct {
	Metrics grypeCVSSMetrics `json:"metrics"`
}

type grypeCVSSMetrics struct {
	BaseScore float64 `json:"baseScore"`
}

type grypeArtifact struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// normalizeSeverity normalizes grype severity to our Severity type
func normalizeSeverity(s string) Severity {
	switch strings.ToLower(s) {
	case "critical":
		return SeverityCritical
	case "high":
		return SeverityHigh
	case "medium":
		return SeverityMedium
	case "low":
		return SeverityLow
	case "negligible":
		return SeverityLow
	default:
		return SeverityUnknown
	}
}

// extractURLs extracts URLs from grype vulnerability
func extractURLs(vuln grypeVulnerability) []string {
	return append([]string(nil), vuln.URLs...)
}

// extractCVSS extracts CVSS score from grype vulnerability
func extractCVSS(vuln grypeVulnerability) float64 {
	var best float64
	for _, cvss := range vuln.Cvss {
		if cvss.Metrics.BaseScore > best {
			best = cvss.Metrics.BaseScore
		}
	}
	return best
}

// extractImageDigestFromGrype extracts image digest from grype descriptor name
func extractImageDigestFromGrype(name string) string {
	// Grype descriptor name format: "registry:image@sha256:digest"
	re := regexp.MustCompile(`@(sha256:[a-f0-9]{64})`)
	matches := re.FindStringSubmatch(name)
	if len(matches) >= 2 {
		return matches[1]
	}
	return name
}

// isVersionGreaterOrEqual compares semantic versions
func isVersionGreaterOrEqual(version, minVersion string) bool {
	v1Parts := strings.Split(version, ".")
	v2Parts := strings.Split(minVersion, ".")

	for i := 0; i < len(v1Parts) && i < len(v2Parts); i++ {
		v1, _ := strconv.Atoi(v1Parts[i])
		v2, _ := strconv.Atoi(v2Parts[i])

		if v1 > v2 {
			return true
		}
		if v1 < v2 {
			return false
		}
	}

	return len(v1Parts) >= len(v2Parts)
}

func parseGrypeVersion(output string) (string, error) {
	versionPatterns := []*regexp.Regexp{
		regexp.MustCompile(`(?m)^Version:\s*v?(\d+\.\d+\.\d+)\s*$`),
		regexp.MustCompile(`(?m)^grype\s+v?(\d+\.\d+\.\d+)\s*$`),
		regexp.MustCompile(`v?(\d+\.\d+\.\d+)`),
	}

	for _, pattern := range versionPatterns {
		matches := pattern.FindStringSubmatch(output)
		if len(matches) >= 2 {
			return matches[1], nil
		}
	}

	return "", fmt.Errorf("failed to parse grype version from: %s", output)
}

func grypeCommandEnv(dbPresent bool) []string {
	env := []string{
		"GRYPE_CHECK_FOR_APP_UPDATE=false",
	}
	if dbPresent {
		env = append(env, "GRYPE_DB_AUTO_UPDATE=false")
	}
	return env
}

func hasGrypeVulnerabilityDB() bool {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return false
	}

	matches, err := filepath.Glob(filepath.Join(cacheDir, "grype", "db", "*", "vulnerability.db"))
	return err == nil && len(matches) > 0
}
