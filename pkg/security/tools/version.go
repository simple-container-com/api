package tools

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// VersionChecker validates tool versions against minimum requirements
type VersionChecker struct {
	registry *ToolRegistry
}

// Version represents a semantic version
type Version struct {
	Major int
	Minor int
	Patch int
	Raw   string
}

// NewVersionChecker creates a new version checker
func NewVersionChecker() *VersionChecker {
	return &VersionChecker{
		registry: NewToolRegistry(),
	}
}

// GetInstalledVersion retrieves the installed version of a tool
func (c *VersionChecker) GetInstalledVersion(ctx context.Context, toolName string) (string, error) {
	tool, err := c.registry.GetTool(toolName)
	if err != nil {
		return "", err
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Run version command
	cmd := exec.CommandContext(ctx, tool.Command, tool.VersionFlag)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to get version for %s: %w (output: %s)", toolName, err, string(output))
	}

	// Extract version from output
	version := c.extractVersion(string(output))
	if version == "" {
		return "", fmt.Errorf("could not extract version from output: %s", string(output))
	}

	return version, nil
}

// ValidateVersion checks if the installed version meets minimum requirements
func (c *VersionChecker) ValidateVersion(toolName, installedVersion string) error {
	tool, err := c.registry.GetTool(toolName)
	if err != nil {
		return err
	}

	if tool.MinVersion == "" {
		// No minimum version specified
		return nil
	}

	// Parse versions
	installed, err := ParseVersion(installedVersion)
	if err != nil {
		return fmt.Errorf("failed to parse installed version %s: %w", installedVersion, err)
	}

	required, err := ParseVersion(tool.MinVersion)
	if err != nil {
		return fmt.Errorf("failed to parse required version %s: %w", tool.MinVersion, err)
	}

	// Compare versions
	if !installed.IsAtLeast(required) {
		return fmt.Errorf("%s version %s is below minimum required version %s", toolName, installedVersion, tool.MinVersion)
	}

	return nil
}

// extractVersion extracts version string from tool output
func (c *VersionChecker) extractVersion(output string) string {
	// Common version patterns:
	// - "version 1.2.3"
	// - "v1.2.3"
	// - "1.2.3"
	// - "tool 1.2.3"

	patterns := []string{
		`v?(\d+\.\d+\.\d+)`,           // Matches v1.2.3 or 1.2.3
		`version\s+v?(\d+\.\d+\.\d+)`, // Matches "version 1.2.3"
		`(\d+\.\d+\.\d+)`,             // Plain version number
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindStringSubmatch(output)
		if len(matches) > 1 {
			return matches[1]
		}
	}

	return ""
}

// ParseVersion parses a version string into a Version struct
func ParseVersion(v string) (*Version, error) {
	// Remove 'v' prefix if present
	v = strings.TrimPrefix(v, "v")

	// Split by dots
	parts := strings.Split(v, ".")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid version format: %s (expected format: X.Y.Z or X.Y)", v)
	}

	// Parse major
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return nil, fmt.Errorf("invalid major version: %s", parts[0])
	}

	// Parse minor
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return nil, fmt.Errorf("invalid minor version: %s", parts[1])
	}

	// Parse patch (optional)
	patch := 0
	if len(parts) >= 3 {
		// Handle patch version with additional suffixes (e.g., "3-beta")
		patchPart := strings.Split(parts[2], "-")[0]
		patch, err = strconv.Atoi(patchPart)
		if err != nil {
			return nil, fmt.Errorf("invalid patch version: %s", parts[2])
		}
	}

	return &Version{
		Major: major,
		Minor: minor,
		Patch: patch,
		Raw:   v,
	}, nil
}

// IsAtLeast returns true if this version is at least the given version
func (v *Version) IsAtLeast(other *Version) bool {
	if v.Major != other.Major {
		return v.Major > other.Major
	}
	if v.Minor != other.Minor {
		return v.Minor > other.Minor
	}
	return v.Patch >= other.Patch
}

// String returns the string representation of the version
func (v *Version) String() string {
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

// Compare compares two versions
// Returns: -1 if v < other, 0 if v == other, 1 if v > other
func (v *Version) Compare(other *Version) int {
	if v.Major != other.Major {
		if v.Major < other.Major {
			return -1
		}
		return 1
	}

	if v.Minor != other.Minor {
		if v.Minor < other.Minor {
			return -1
		}
		return 1
	}

	if v.Patch != other.Patch {
		if v.Patch < other.Patch {
			return -1
		}
		return 1
	}

	return 0
}

// CheckAllToolVersions validates all tool versions
func (c *VersionChecker) CheckAllToolVersions(ctx context.Context, tools []string) error {
	var errors []string

	for _, toolName := range tools {
		version, err := c.GetInstalledVersion(ctx, toolName)
		if err != nil {
			errors = append(errors, fmt.Sprintf("%s: %v", toolName, err))
			continue
		}

		if err := c.ValidateVersion(toolName, version); err != nil {
			errors = append(errors, fmt.Sprintf("%s: %v", toolName, err))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("version check failed:\n  - %s", strings.Join(errors, "\n  - "))
	}

	return nil
}
