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
	"strings"
)

// DefaultTrivyVersion is the pinned install version. Bump here to upgrade cluster-wide,
// or override per-scan via SC config (ScanToolConfig.Version) or SC_TRIVY_VERSION env var.
const DefaultTrivyVersion = "0.69.3"

// TrivyScanner implements Scanner interface using Trivy
type TrivyScanner struct {
	installVersion string // exact version to install
	minVersion     string // minimum acceptable (CheckVersion)
}

// NewTrivyScanner creates a new TrivyScanner pinned to DefaultTrivyVersion.
func NewTrivyScanner() *TrivyScanner {
	return &TrivyScanner{
		installVersion: DefaultTrivyVersion,
		minVersion:     DefaultTrivyVersion,
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

	cacheDir, err := ensureTrivyCacheDir()
	if err != nil {
		return nil, err
	}

	// Scan from the local Docker daemon (docker-daemon: prefix).
	// The image must already be present locally — callers are expected to scan
	// BEFORE pushing to the registry so this acts as a fail-gate.
	// Do NOT use --quiet: it suppresses error messages on failure.
	cmd := exec.CommandContext(
		ctx,
		"trivy", "image",
		"--scanners", "vuln",
		"--cache-dir", cacheDir,
		"--format", "json",
	)
	if trivyDBPresent(cacheDir) {
		cmd.Args = append(cmd.Args, "--skip-db-update")
	}
	if trivyJavaDBPresent(cacheDir) {
		cmd.Args = append(cmd.Args, "--skip-java-db-update")
	}
	cmd.Args = append(cmd.Args, "docker-daemon:"+image)
	cmd.Env = append(os.Environ(), "TRIVY_CACHE_DIR="+cacheDir)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("trivy scan failed: %w\nstdout: %s\nstderr: %s",
			err, strings.TrimSpace(stdout.String()), strings.TrimSpace(stderr.String()))
	}

	output := bytes.TrimSpace(stdout.Bytes())
	if len(output) == 0 {
		return nil, fmt.Errorf("trivy produced empty output (stderr: %s)", strings.TrimSpace(stderr.String()))
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
			v.CVSS = extractTrivyCVSS(vuln.CVSS)

			vulns = append(vulns, v)
		}
	}

	// Extract image digest
	imageDigest := ""
	if trivyOutput.Metadata.ImageID != "" {
		imageDigest = extractImageDigestFromTrivy(trivyOutput.Metadata.ImageID)
	}

	result := NewScanResult(imageDigest, ScanToolTrivy, vulns)
	if version, err := t.Version(ctx); err == nil {
		result.Metadata["trivyVersion"] = version
	}

	return result, nil
}

// Version returns the trivy version
func (t *TrivyScanner) Version(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "trivy", "--version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to get trivy version: %w", err)
	}

	return parseTrivyVersion(string(output))
}

// CheckInstalled checks if trivy is installed
func (t *TrivyScanner) CheckInstalled(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "trivy", "--version")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("trivy is not installed or not in PATH. Install from: https://github.com/aquasecurity/trivy")
	}
	return nil
}

// Install installs trivy if not already present using the official install script
func (t *TrivyScanner) Install(ctx context.Context) error {
	if err := t.CheckInstalled(ctx); err == nil {
		return nil // already installed
	}
	fmt.Printf("Installing trivy %s...\n", t.installVersion)
	installDir := "/usr/local/bin"
	if _, err := exec.LookPath("sudo"); err != nil {
		home, _ := os.UserHomeDir()
		installDir = filepath.Join(home, ".local", "bin")
		if err := os.MkdirAll(installDir, 0o755); err != nil {
			return fmt.Errorf("failed to create install directory %s: %w", installDir, err)
		}
	}
	// Download the binary directly from GitHub releases.
	// Asset name format: trivy_VERSION_Linux-64bit.tar.gz (confirmed stable across releases).
	cmd := exec.CommandContext(ctx, "sh", "-c",
		fmt.Sprintf(`set -e
TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT
curl -sSfL "https://github.com/aquasecurity/trivy/releases/download/v%[1]s/trivy_%[1]s_Linux-64bit.tar.gz" \
  -o "$TMP_DIR/trivy.tar.gz"
tar -xzf "$TMP_DIR/trivy.tar.gz" -C "$TMP_DIR" trivy
mv "$TMP_DIR/trivy" %[2]s/trivy`,
			t.installVersion, installDir))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to install trivy: %w", err)
	}
	return t.CheckInstalled(ctx)
}

// CheckVersion checks if trivy meets minimum version requirements
func (t *TrivyScanner) CheckVersion(ctx context.Context) error {
	version, err := t.Version(ctx)
	if err != nil {
		return err
	}

	if !isVersionGreaterOrEqual(version, t.minVersion) {
		return fmt.Errorf("trivy version %s is below minimum required version %s — bump DefaultTrivyVersion or set SC_TRIVY_VERSION", version, t.minVersion)
	}

	return nil
}

// TrivyOutput represents trivy JSON output structure
type TrivyOutput struct {
	Results []struct {
		Vulnerabilities []struct {
			VulnerabilityID  string    `json:"VulnerabilityID"`
			Severity         string    `json:"Severity"`
			PkgName          string    `json:"PkgName"`
			InstalledVersion string    `json:"InstalledVersion"`
			FixedVersion     string    `json:"FixedVersion"`
			Description      string    `json:"Description"`
			References       []string  `json:"References"`
			CVSS             trivyCVSS `json:"CVSS"`
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

type trivyCVSS map[string]trivyCVSSScore

type trivyCVSSScore struct {
	V3Score float64 `json:"V3Score"`
	V2Score float64 `json:"V2Score"`
}

func (c *trivyCVSS) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil
	}

	if trimmed[0] == '{' {
		var values map[string]trivyCVSSScore
		if err := json.Unmarshal(trimmed, &values); err != nil {
			return err
		}
		*c = values
		return nil
	}

	if trimmed[0] == '[' {
		var values []trivyCVSSScore
		if err := json.Unmarshal(trimmed, &values); err != nil {
			return err
		}
		result := make(trivyCVSS, len(values))
		for i, value := range values {
			result[fmt.Sprintf("%d", i)] = value
		}
		*c = result
		return nil
	}

	return fmt.Errorf("unexpected trivy CVSS payload: %s", string(trimmed))
}

func extractTrivyCVSS(cvss trivyCVSS) float64 {
	var best float64
	for _, value := range cvss {
		if value.V3Score > best {
			best = value.V3Score
		}
		if value.V2Score > best {
			best = value.V2Score
		}
	}
	return best
}

func parseTrivyVersion(output string) (string, error) {
	versionPatterns := []*regexp.Regexp{
		regexp.MustCompile(`(?m)^Version:\s*v?(\d+\.\d+\.\d+)\s*$`),
		regexp.MustCompile(`(?m)^trivy\s+v?(\d+\.\d+\.\d+)\s*$`),
		regexp.MustCompile(`v?(\d+\.\d+\.\d+)`),
	}

	for _, pattern := range versionPatterns {
		matches := pattern.FindStringSubmatch(output)
		if len(matches) >= 2 {
			return matches[1], nil
		}
	}

	return "", fmt.Errorf("failed to parse trivy version from: %s", output)
}

func ensureTrivyCacheDir() (string, error) {
	cacheRoot, err := os.UserCacheDir()
	if err != nil {
		cacheRoot = os.TempDir()
	}
	cacheDir := filepath.Join(cacheRoot, "trivy")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", fmt.Errorf("create trivy cache directory: %w", err)
	}
	return cacheDir, nil
}

func trivyDBPresent(cacheDir string) bool {
	return fileExists(filepath.Join(cacheDir, "db", "metadata.json"))
}

func trivyJavaDBPresent(cacheDir string) bool {
	return fileExists(filepath.Join(cacheDir, "java-db", "metadata.json"))
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
