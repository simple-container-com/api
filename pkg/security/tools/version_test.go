package tools

import (
	"testing"
)

func TestParseVersion(t *testing.T) {
	tests := []struct {
		name        string
		versionStr  string
		expected    *Version
		expectError bool
	}{
		{
			name:       "Simple version",
			versionStr: "1.2.3",
			expected:   &Version{Major: 1, Minor: 2, Patch: 3, Pre: ""},
		},
		{
			name:       "Version with v prefix",
			versionStr: "v2.0.1",
			expected:   &Version{Major: 2, Minor: 0, Patch: 1, Pre: ""},
		},
		{
			name:       "Version with pre-release",
			versionStr: "1.2.3-beta.1",
			expected:   &Version{Major: 1, Minor: 2, Patch: 3, Pre: "beta.1"},
		},
		{
			name:       "Version with v and pre-release",
			versionStr: "v3.0.2-rc1",
			expected:   &Version{Major: 3, Minor: 0, Patch: 2, Pre: "rc1"},
		},
		{
			name:        "Invalid version",
			versionStr:  "invalid",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseVersion(tt.versionStr)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("ParseVersion failed: %v", err)
			}

			if result.Major != tt.expected.Major {
				t.Errorf("Expected major %d, got %d", tt.expected.Major, result.Major)
			}

			if result.Minor != tt.expected.Minor {
				t.Errorf("Expected minor %d, got %d", tt.expected.Minor, result.Minor)
			}

			if result.Patch != tt.expected.Patch {
				t.Errorf("Expected patch %d, got %d", tt.expected.Patch, result.Patch)
			}

			if result.Pre != tt.expected.Pre {
				t.Errorf("Expected pre '%s', got '%s'", tt.expected.Pre, result.Pre)
			}
		})
	}
}

func TestVersion_Compare(t *testing.T) {
	tests := []struct {
		name     string
		v1       *Version
		v2       *Version
		expected int
	}{
		{
			name:     "Equal versions",
			v1:       &Version{Major: 1, Minor: 2, Patch: 3},
			v2:       &Version{Major: 1, Minor: 2, Patch: 3},
			expected: 0,
		},
		{
			name:     "v1 > v2 (major)",
			v1:       &Version{Major: 2, Minor: 0, Patch: 0},
			v2:       &Version{Major: 1, Minor: 9, Patch: 9},
			expected: 1,
		},
		{
			name:     "v1 < v2 (major)",
			v1:       &Version{Major: 1, Minor: 0, Patch: 0},
			v2:       &Version{Major: 2, Minor: 0, Patch: 0},
			expected: -1,
		},
		{
			name:     "v1 > v2 (minor)",
			v1:       &Version{Major: 1, Minor: 3, Patch: 0},
			v2:       &Version{Major: 1, Minor: 2, Patch: 9},
			expected: 1,
		},
		{
			name:     "v1 < v2 (minor)",
			v1:       &Version{Major: 1, Minor: 2, Patch: 0},
			v2:       &Version{Major: 1, Minor: 3, Patch: 0},
			expected: -1,
		},
		{
			name:     "v1 > v2 (patch)",
			v1:       &Version{Major: 1, Minor: 2, Patch: 4},
			v2:       &Version{Major: 1, Minor: 2, Patch: 3},
			expected: 1,
		},
		{
			name:     "v1 < v2 (patch)",
			v1:       &Version{Major: 1, Minor: 2, Patch: 3},
			v2:       &Version{Major: 1, Minor: 2, Patch: 4},
			expected: -1,
		},
		{
			name:     "Pre-release < release",
			v1:       &Version{Major: 1, Minor: 2, Patch: 3, Pre: "beta"},
			v2:       &Version{Major: 1, Minor: 2, Patch: 3},
			expected: -1,
		},
		{
			name:     "Release > pre-release",
			v1:       &Version{Major: 1, Minor: 2, Patch: 3},
			v2:       &Version{Major: 1, Minor: 2, Patch: 3, Pre: "rc1"},
			expected: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.v1.Compare(tt.v2)
			if result != tt.expected {
				t.Errorf("Expected %d, got %d", tt.expected, result)
			}
		})
	}
}

func TestVersion_MeetsMinimum(t *testing.T) {
	tests := []struct {
		name     string
		current  *Version
		minimum  *Version
		expected bool
	}{
		{
			name:     "Meets minimum (equal)",
			current:  &Version{Major: 1, Minor: 2, Patch: 3},
			minimum:  &Version{Major: 1, Minor: 2, Patch: 3},
			expected: true,
		},
		{
			name:     "Meets minimum (greater)",
			current:  &Version{Major: 1, Minor: 3, Patch: 0},
			minimum:  &Version{Major: 1, Minor: 2, Patch: 3},
			expected: true,
		},
		{
			name:     "Does not meet minimum",
			current:  &Version{Major: 1, Minor: 2, Patch: 0},
			minimum:  &Version{Major: 1, Minor: 2, Patch: 3},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.current.MeetsMinimum(tt.minimum)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestVersion_String(t *testing.T) {
	tests := []struct {
		name     string
		version  *Version
		expected string
	}{
		{
			name:     "Simple version",
			version:  &Version{Major: 1, Minor: 2, Patch: 3},
			expected: "1.2.3",
		},
		{
			name:     "Version with pre-release",
			version:  &Version{Major: 1, Minor: 2, Patch: 3, Pre: "beta.1"},
			expected: "1.2.3-beta.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.version.String()
			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

func TestExtractVersionFromOutput(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		toolName string
		expected string
	}{
		{
			name:     "Cosign version output",
			output:   "cosign version: v3.0.2",
			toolName: "cosign",
			expected: "3.0.2",
		},
		{
			name:     "Syft version output",
			output:   "syft v1.41.0",
			toolName: "syft",
			expected: "1.41.0",
		},
		{
			name:     "Grype version output",
			output:   "grype version: 0.106.0",
			toolName: "grype",
			expected: "0.106.0",
		},
		{
			name:     "Generic version output",
			output:   "Version: 2.5.1",
			toolName: "tool",
			expected: "2.5.1",
		},
		{
			name:     "No version found",
			output:   "No version information",
			toolName: "tool",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractVersionFromOutput(tt.output, tt.toolName)
			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}
