package sbom

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// SyftGenerator implements SBOM generation using Syft
type SyftGenerator struct {
	// Timeout for syft commands (default: 5 minutes for large images)
	Timeout time.Duration
}

// NewSyftGenerator creates a new SyftGenerator
func NewSyftGenerator() *SyftGenerator {
	return &SyftGenerator{
		Timeout: 5 * time.Minute,
	}
}

// Generate generates an SBOM using Syft
func (g *SyftGenerator) Generate(ctx context.Context, image string, format Format) (*SBOM, error) {
	if !g.SupportsFormat(format) {
		return nil, fmt.Errorf("format %s not supported by syft", format)
	}

	// Create context with timeout
	timeoutCtx, cancel := context.WithTimeout(ctx, g.Timeout)
	defer cancel()

	// Build syft command: syft registry:IMAGE -o FORMAT
	args := []string{
		fmt.Sprintf("registry:%s", image),
		"-o", string(format),
	}

	cmd := exec.CommandContext(timeoutCtx, "syft", args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Execute syft
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("syft command failed: %w (stderr: %s)", err, stderr.String())
	}

	content := stdout.Bytes()
	if len(content) == 0 {
		return nil, fmt.Errorf("syft produced empty output")
	}

	// Get tool version
	version, err := g.Version(ctx)
	if err != nil {
		version = "unknown"
	}

	// Parse metadata from SBOM content
	metadata := &Metadata{
		ToolName:    "syft",
		ToolVersion: version,
	}

	// Extract package count if format is JSON-based
	if format == FormatCycloneDXJSON || format == FormatSPDXJSON || format == FormatSyftJSON {
		if count, err := g.extractPackageCount(content, format); err == nil {
			metadata.PackageCount = count
		}
	}

	// Extract image digest from syft output or use image reference
	imageDigest := g.extractImageDigest(image, stderr.String())

	return NewSBOM(format, content, imageDigest, metadata), nil
}

// SupportsFormat checks if Syft supports the given format
func (g *SyftGenerator) SupportsFormat(format Format) bool {
	switch format {
	case FormatCycloneDXJSON, FormatCycloneDXXML,
		FormatSPDXJSON, FormatSPDXTagValue,
		FormatSyftJSON:
		return true
	default:
		return false
	}
}

// Version returns the version of Syft
func (g *SyftGenerator) Version(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "syft", "version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to get syft version: %w", err)
	}

	// Parse version from output like "syft 1.41.0"
	versionRegex := regexp.MustCompile(`syft\s+v?(\d+\.\d+\.\d+)`)
	matches := versionRegex.FindSubmatch(output)
	if len(matches) > 1 {
		return string(matches[1]), nil
	}

	// Fallback: try to extract any version-like string
	versionRegex = regexp.MustCompile(`v?(\d+\.\d+\.\d+)`)
	matches = versionRegex.FindSubmatch(output)
	if len(matches) > 1 {
		return string(matches[1]), nil
	}

	return "unknown", fmt.Errorf("could not parse version from: %s", string(output))
}

// extractPackageCount extracts the package count from SBOM content
func (g *SyftGenerator) extractPackageCount(content []byte, format Format) (int, error) {
	switch format {
	case FormatCycloneDXJSON:
		return g.extractCycloneDXPackageCount(content)
	case FormatSPDXJSON:
		return g.extractSPDXPackageCount(content)
	case FormatSyftJSON:
		return g.extractSyftPackageCount(content)
	default:
		return 0, fmt.Errorf("package count extraction not supported for format: %s", format)
	}
}

// extractCycloneDXPackageCount extracts package count from CycloneDX JSON
func (g *SyftGenerator) extractCycloneDXPackageCount(content []byte) (int, error) {
	var data struct {
		Components []interface{} `json:"components"`
	}
	if err := json.Unmarshal(content, &data); err != nil {
		return 0, err
	}
	return len(data.Components), nil
}

// extractSPDXPackageCount extracts package count from SPDX JSON
func (g *SyftGenerator) extractSPDXPackageCount(content []byte) (int, error) {
	var data struct {
		Packages []interface{} `json:"packages"`
	}
	if err := json.Unmarshal(content, &data); err != nil {
		return 0, err
	}
	return len(data.Packages), nil
}

// extractSyftPackageCount extracts package count from Syft JSON
func (g *SyftGenerator) extractSyftPackageCount(content []byte) (int, error) {
	var data struct {
		Artifacts []interface{} `json:"artifacts"`
	}
	if err := json.Unmarshal(content, &data); err != nil {
		return 0, err
	}
	return len(data.Artifacts), nil
}

// extractImageDigest extracts the image digest from syft output
func (g *SyftGenerator) extractImageDigest(image, stderr string) string {
	// Try to extract digest from stderr (syft logs digest there)
	digestRegex := regexp.MustCompile(`sha256:[a-f0-9]{64}`)
	if matches := digestRegex.FindString(stderr); matches != "" {
		return matches
	}

	// If image already contains digest, use it
	if strings.Contains(image, "@sha256:") {
		parts := strings.Split(image, "@")
		if len(parts) == 2 {
			return parts[1]
		}
	}

	// Fallback: use image reference as-is
	return image
}

// CheckInstalled checks if Syft is installed
func CheckInstalled(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "syft", "version")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("syft is not installed or not in PATH. Install from: https://github.com/anchore/syft#installation")
	}
	return nil
}

// CheckVersion checks if Syft version meets minimum requirements
func CheckVersion(ctx context.Context, minVersion string) error {
	g := NewSyftGenerator()
	version, err := g.Version(ctx)
	if err != nil {
		return err
	}

	if !isVersionGreaterOrEqual(version, minVersion) {
		return fmt.Errorf("syft version %s is older than required %s. Please upgrade: https://github.com/anchore/syft#installation", version, minVersion)
	}

	return nil
}

// isVersionGreaterOrEqual compares two semantic versions
func isVersionGreaterOrEqual(current, minimum string) bool {
	currentParts := parseVersion(current)
	minimumParts := parseVersion(minimum)

	for i := 0; i < 3; i++ {
		if currentParts[i] > minimumParts[i] {
			return true
		}
		if currentParts[i] < minimumParts[i] {
			return false
		}
	}
	return true
}

// parseVersion parses a semantic version string into [major, minor, patch]
func parseVersion(version string) [3]int {
	var parts [3]int
	version = strings.TrimPrefix(version, "v")
	components := strings.Split(version, ".")

	for i := 0; i < len(components) && i < 3; i++ {
		if val, err := strconv.Atoi(components[i]); err == nil {
			parts[i] = val
		}
	}

	return parts
}
