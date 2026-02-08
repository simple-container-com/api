package scan

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
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
	cmd := exec.CommandContext(ctx, "grype", "registry:"+image, "-o", "json")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("grype scan failed: %w (output: %s)", err, string(output))
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
	imageDigest := ""
	if grypeOutput.Descriptor.Name != "" {
		imageDigest = extractImageDigestFromGrype(grypeOutput.Descriptor.Name)
	}

	result := NewScanResult(imageDigest, ScanToolGrype, vulns)
	result.Metadata["grypeVersion"] = grypeOutput.Descriptor.Version

	return result, nil
}

// Version returns the grype version
func (g *GrypeScanner) Version(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "grype", "version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to get grype version: %w", err)
	}

	// Parse version from output (format: "grype 0.106.0")
	re := regexp.MustCompile(`grype\s+(\d+\.\d+\.\d+)`)
	matches := re.FindStringSubmatch(string(output))
	if len(matches) < 2 {
		return "", fmt.Errorf("failed to parse grype version from: %s", string(output))
	}

	return matches[1], nil
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
	Matches []struct {
		Vulnerability struct {
			ID          string `json:"id"`
			Severity    string `json:"severity"`
			Description string `json:"description"`
			Fix         struct {
				State    string   `json:"state"`
				Versions []string `json:"versions"`
			} `json:"fix"`
			Cvss []struct {
				Metrics struct {
					BaseScore float64 `json:"baseScore"`
				} `json:"metrics"`
			} `json:"cvss"`
			URLs []string `json:"urls"`
		} `json:"vulnerability"`
		Artifact struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"artifact"`
	} `json:"matches"`
	Descriptor struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"descriptor"`
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
func extractURLs(vuln interface{}) []string {
	// Try to extract URLs from vulnerability struct
	v, ok := vuln.(struct {
		ID          string   `json:"id"`
		Severity    string   `json:"severity"`
		Description string   `json:"description"`
		Fix         struct{} `json:"fix"`
		Cvss        []struct {
			Metrics struct {
				BaseScore float64 `json:"baseScore"`
			} `json:"metrics"`
		} `json:"cvss"`
		URLs []string `json:"urls"`
	})
	if !ok {
		return []string{}
	}
	return v.URLs
}

// extractCVSS extracts CVSS score from grype vulnerability
func extractCVSS(vuln interface{}) float64 {
	// Try to extract CVSS from vulnerability struct
	v, ok := vuln.(struct {
		ID          string   `json:"id"`
		Severity    string   `json:"severity"`
		Description string   `json:"description"`
		Fix         struct{} `json:"fix"`
		Cvss        []struct {
			Metrics struct {
				BaseScore float64 `json:"baseScore"`
			} `json:"metrics"`
		} `json:"cvss"`
		URLs []string `json:"urls"`
	})
	if !ok {
		return 0.0
	}

	if len(v.Cvss) > 0 {
		return v.Cvss[0].Metrics.BaseScore
	}
	return 0.0
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
