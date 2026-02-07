package scan

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

// TrivyScanner implements Scanner interface using Trivy
type TrivyScanner struct {
	minVersion string
}

// NewTrivyScanner creates a new TrivyScanner
func NewTrivyScanner() *TrivyScanner {
	return &TrivyScanner{
		minVersion: "0.68.2",
	}
}

// Tool returns the scanner tool name
func (t *TrivyScanner) Tool() ScanTool {
	return ScanToolTrivy
}

// Scan performs vulnerability scanning using trivy
func (t *TrivyScanner) Scan(ctx context.Context, image string) (*ScanResult, error) {
	// Check if trivy is installed
	if err := t.CheckInstalled(ctx); err != nil {
		return nil, fmt.Errorf("trivy not installed: %w", err)
	}

	// Run trivy scan
	cmd := exec.CommandContext(ctx, "trivy", "image", "--format", "json", image)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("trivy scan failed: %w (output: %s)", err, string(output))
	}

	// Parse trivy JSON output
	var trivyOutput TrivyOutput
	if err := json.Unmarshal(output, &trivyOutput); err != nil {
		return nil, fmt.Errorf("failed to parse trivy output: %w", err)
	}

	// Convert to ScanResult
	vulns := []Vulnerability{}
	for _, result := range trivyOutput.Results {
		for _, vuln := range result.Vulnerabilities {
			v := Vulnerability{
				ID:          vuln.VulnerabilityID,
				Severity:    normalizeTrivySeverity(vuln.Severity),
				Package:     vuln.PkgName,
				Version:     vuln.InstalledVersion,
				FixedIn:     vuln.FixedVersion,
				Description: vuln.Description,
				URLs:        vuln.References,
			}

			// Extract CVSS score
			if vuln.CVSS != nil {
				for _, cvss := range vuln.CVSS {
					if cvss.V3Score > 0 {
						v.CVSS = cvss.V3Score
						break
					}
				}
			}

			vulns = append(vulns, v)
		}
	}

	// Extract image digest
	imageDigest := ""
	if trivyOutput.Metadata.ImageID != "" {
		imageDigest = extractImageDigestFromTrivy(trivyOutput.Metadata.ImageID)
	}

	result := NewScanResult(imageDigest, ScanToolTrivy, vulns)
	result.Metadata["trivyVersion"] = trivyOutput.Metadata.Version

	return result, nil
}

// Version returns the trivy version
func (t *TrivyScanner) Version(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "trivy", "--version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to get trivy version: %w", err)
	}

	// Parse version from output (format: "Version: 0.68.2")
	re := regexp.MustCompile(`Version:\s*(\d+\.\d+\.\d+)`)
	matches := re.FindStringSubmatch(string(output))
	if len(matches) < 2 {
		return "", fmt.Errorf("failed to parse trivy version from: %s", string(output))
	}

	return matches[1], nil
}

// CheckInstalled checks if trivy is installed
func (t *TrivyScanner) CheckInstalled(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "trivy", "--version")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("trivy is not installed or not in PATH. Install from: https://github.com/aquasecurity/trivy")
	}
	return nil
}

// CheckVersion checks if trivy meets minimum version requirements
func (t *TrivyScanner) CheckVersion(ctx context.Context) error {
	version, err := t.Version(ctx)
	if err != nil {
		return err
	}

	if !isVersionGreaterOrEqual(version, t.minVersion) {
		return fmt.Errorf("trivy version %s is below minimum required version %s", version, t.minVersion)
	}

	return nil
}

// TrivyOutput represents trivy JSON output structure
type TrivyOutput struct {
	Results []struct {
		Vulnerabilities []struct {
			VulnerabilityID  string   `json:"VulnerabilityID"`
			Severity         string   `json:"Severity"`
			PkgName          string   `json:"PkgName"`
			InstalledVersion string   `json:"InstalledVersion"`
			FixedVersion     string   `json:"FixedVersion"`
			Description      string   `json:"Description"`
			References       []string `json:"References"`
			CVSS             []struct {
				V3Score float64 `json:"V3Score"`
			} `json:"CVSS"`
		} `json:"Vulnerabilities"`
	} `json:"Results"`
	Metadata struct {
		Version string `json:"Version"`
		ImageID string `json:"ImageID"`
	} `json:"Metadata"`
}

// normalizeTrivySeverity normalizes trivy severity to our Severity type
func normalizeTrivySeverity(s string) Severity {
	switch strings.ToUpper(s) {
	case "CRITICAL":
		return SeverityCritical
	case "HIGH":
		return SeverityHigh
	case "MEDIUM":
		return SeverityMedium
	case "LOW":
		return SeverityLow
	default:
		return SeverityUnknown
	}
}

// extractImageDigestFromTrivy extracts image digest from trivy metadata
func extractImageDigestFromTrivy(imageID string) string {
	// Trivy imageID format: "sha256:digest" or similar
	if strings.HasPrefix(imageID, "sha256:") {
		return imageID
	}
	return ""
}
