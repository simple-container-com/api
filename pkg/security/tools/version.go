package tools

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Version represents a semantic version
type Version struct {
	Major int
	Minor int
	Patch int
	Pre   string
}

// ParseVersion parses a version string into a Version struct
func ParseVersion(versionStr string) (*Version, error) {
	// Remove 'v' prefix if present
	versionStr = strings.TrimPrefix(versionStr, "v")

	// Match semantic version pattern
	re := regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+)(?:-([a-zA-Z0-9.-]+))?`)
	matches := re.FindStringSubmatch(versionStr)

	if len(matches) < 4 {
		return nil, fmt.Errorf("invalid version format: %s", versionStr)
	}

	major, _ := strconv.Atoi(matches[1])
	minor, _ := strconv.Atoi(matches[2])
	patch, _ := strconv.Atoi(matches[3])

	pre := ""
	if len(matches) > 4 {
		pre = matches[4]
	}

	return &Version{
		Major: major,
		Minor: minor,
		Patch: patch,
		Pre:   pre,
	}, nil
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

	// Pre-release versions are considered less than release versions
	if v.Pre != "" && other.Pre == "" {
		return -1
	}
	if v.Pre == "" && other.Pre != "" {
		return 1
	}

	// Both have pre-release, compare lexicographically
	if v.Pre != other.Pre {
		if v.Pre < other.Pre {
			return -1
		}
		return 1
	}

	return 0
}

// MeetsMinimum checks if version meets minimum requirement
func (v *Version) MeetsMinimum(minVersion *Version) bool {
	return v.Compare(minVersion) >= 0
}

// String returns string representation of version
func (v *Version) String() string {
	versionStr := fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
	if v.Pre != "" {
		versionStr += "-" + v.Pre
	}
	return versionStr
}

// ExtractVersionFromOutput extracts version string from command output
func ExtractVersionFromOutput(output, toolName string) string {
	// Common patterns for version output
	patterns := []string{
		// "tool version: v1.2.3"
		toolName + ` version[:\s]+v?(\d+\.\d+\.\d+(?:-[a-zA-Z0-9.-]+)?)`,
		// "tool v1.2.3"
		toolName + ` v?(\d+\.\d+\.\d+(?:-[a-zA-Z0-9.-]+)?)`,
		// "version: v1.2.3" or "Version: v1.2.3"
		`[Vv]ersion[:\s]+v?(\d+\.\d+\.\d+(?:-[a-zA-Z0-9.-]+)?)`,
		// Just "v1.2.3" or "1.2.3"
		`v?(\d+\.\d+\.\d+(?:-[a-zA-Z0-9.-]+)?)`,
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
