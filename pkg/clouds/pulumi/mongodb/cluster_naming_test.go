package mongodb

import (
	"testing"

	"github.com/samber/lo"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/mongodb"
)

// Test cases that represent existing clusters using version 1 naming strategy
func TestToClusterName_ExistingClusters(t *testing.T) {
	tests := []struct {
		name         string
		stackName    string
		resourceName string
		expected     string
		description  string
	}{
		{
			name:         "short stack and resource name",
			stackName:    "integrail",
			resourceName: "mongo-main",
			expected:     "integrail---main--test", // Version 1: TrimStringMiddle behavior
			description:  "Existing clusters use version 1 TrimStringMiddle logic",
		},
		{
			name:         "medium length names",
			stackName:    "integrail",
			resourceName: "mongodb-customers",
			expected:     "integrail---mers--test", // Version 1: TrimStringMiddle behavior
			description:  "Existing clusters preserve legacy truncation",
		},
		{
			name:         "long stack name",
			stackName:    "very-long-stack-name-that-exceeds-limits",
			resourceName: "mongodb-atlas-cluster",
			expected:     "very-long---ster--test", // Version 1: TrimStringMiddle behavior
			description:  "Long existing cluster names use legacy logic",
		},
		{
			name:         "your actual case from server.yaml",
			stackName:    "integrail",
			resourceName: "mongodb-pool-dedicated-1",
			expected:     "integrail---ed-1--test", // Version 1: TrimStringMiddle behavior
			description:  "Real existing cluster uses legacy naming",
		},
		{
			name:         "another potential conflict case",
			stackName:    "integrail",
			resourceName: "mongodb-pool-dedicated-2",
			expected:     "integrail---ed-2--test", // Version 1: TrimStringMiddle behavior
			description:  "Legacy naming may have conflicts - hence need for version 2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Existing clusters use version 1 naming strategy
			config := &mongodb.AtlasConfig{
				NamingStrategyVersion: lo.ToPtr(1),
			}
			input := createTestResourceInput(tt.resourceName, config)
			result := toClusterName(tt.stackName, input)

			if result != tt.expected {
				t.Errorf("toClusterName() = %v, want %v", result, tt.expected)
				t.Errorf("Description: %s", tt.description)
			}

			// Document existing behavior - version 1 may exceed 21-char limit
			t.Logf("Version 1 behavior: name=%q, length=%d", result, len(result))
			if len(result) > 21 {
				t.Logf("NOTE: Version 1 produces names exceeding MongoDB Atlas 21-char limit (hence version 2)")
			}
		})
	}
}

func TestToClusterName_NamingStrategyVersions(t *testing.T) {
	tests := []struct {
		name         string
		stackName    string
		resourceName string
		version      *int // nil = default (2), 1 = legacy, 2 = new
		expected     string
		description  string
	}{
		{
			name:         "version 1 - legacy behavior preserved",
			stackName:    "integrail",
			resourceName: "mongodb-pool-dedicated-1",
			version:      lo.ToPtr(1),
			expected:     "integrail---ed-1--test", // Exact old TrimStringMiddle result
			description:  "Version 1 preserves exact legacy naming for existing clusters",
		},
		{
			name:         "version 1 - short name",
			stackName:    "integrail",
			resourceName: "mongo-main",
			version:      lo.ToPtr(1),
			expected:     "integrail---main--test", // Legacy TrimStringMiddle result
			description:  "Version 1 uses TrimStringMiddle even for shorter names",
		},
		{
			name:         "version 2 - improved naming",
			stackName:    "integrail",
			resourceName: "mongodb-pool-dedicated-1",
			version:      lo.ToPtr(2),
			expected:     "integrail--mongodb-3b05", // Hash-based for consistency
			description:  "Version 2 uses hash-based naming for consistency and conflict resolution",
		},
		{
			name:         "version 2 - hash for long names",
			stackName:    "very-long-stack-name-that-exceeds-limits",
			resourceName: "mongodb-cluster-production",
			version:      lo.ToPtr(2),
			expected:     "very-long-stack-na-374f", // Hash-based truncation
			description:  "Version 2 uses hash-based truncation for very long names",
		},
		{
			name:         "default version (no version specified)",
			stackName:    "integrail",
			resourceName: "mongodb-pool-dedicated-2",
			version:      nil,                       // Should default to version 2
			expected:     "integrail--mongodb-3b28", // Version 2 hash-based behavior
			description:  "Default behavior uses version 2 naming strategy",
		},
		{
			name:         "version 2 - conflict resolution",
			stackName:    "integrail",
			resourceName: "mongodb-pool-dedicated-3",
			version:      lo.ToPtr(2),
			expected:     "integrail--mongodb-3b4b", // Different hash prevents conflicts
			description:  "Version 2 resolves naming conflicts with unique hashes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &mongodb.AtlasConfig{
				NamingStrategyVersion: tt.version,
			}
			input := createTestResourceInput(tt.resourceName, config)
			result := toClusterName(tt.stackName, input)

			if result != tt.expected {
				t.Errorf("toClusterName() = %v, want %v", result, tt.expected)
				t.Errorf("Description: %s", tt.description)
			}

			// All results should fit MongoDB Atlas 23-character limit
			if len(result) > 23 {
				t.Errorf("Result %q exceeds MongoDB Atlas 23-character limit (got %d chars)", result, len(result))
			}

			t.Logf("Version %v: %s -> %s (len=%d)",
				ptrIntValue(tt.version, 2), tt.resourceName, result, len(result))
		})
	}
}

func TestToClusterName_WithCustomClusterName(t *testing.T) {
	tests := []struct {
		name              string
		stackName         string
		resourceName      string
		customClusterName string
		expected          string
		description       string
	}{
		{
			name:              "custom name within limits",
			stackName:         "integrail",
			resourceName:      "mongodb-pool-dedicated-1",
			customClusterName: "pool-dedicated-1",
			expected:          "pool-dedicated-1",
			description:       "Custom name used directly when within limits",
		},
		{
			name:              "custom name too long gets truncated",
			stackName:         "integrail",
			resourceName:      "mongodb-pool-dedicated-1",
			customClusterName: "very-long-custom-cluster-name-that-exceeds-limits",
			expected:          "very-long---eds-limits", // TrimStringMiddle behavior
			description:       "Long custom names get TrimStringMiddle treatment",
		},
		{
			name:              "empty custom name falls back to default logic",
			stackName:         "integrail",
			resourceName:      "mongodb-pool-dedicated-1",
			customClusterName: "",
			expected:          "integrail--mongodb-3b05", // Falls back to improved default
			description:       "Empty custom name uses improved default logic",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &mongodb.AtlasConfig{
				ClusterName: tt.customClusterName, // Using existing clusterName field
			}
			input := createTestResourceInput(tt.resourceName, config)
			result := toClusterName(tt.stackName, input)

			if result != tt.expected {
				t.Errorf("toClusterName() = %v, want %v", result, tt.expected)
				t.Errorf("Description: %s", tt.description)
			}

			// Document custom name behavior
			t.Logf("Custom name behavior: name=%q, length=%d", result, len(result))
			if len(result) > 21 {
				t.Logf("WARNING: Custom name exceeds MongoDB Atlas 21-char limit")
			}
		})
	}
}

func TestToClusterName_ConflictDetection(t *testing.T) {
	// Test cases that demonstrate the conflict issue
	conflictCases := []struct {
		stackName     string
		resourceName1 string
		resourceName2 string
		description   string
	}{
		{
			stackName:     "integrail",
			resourceName1: "mongodb-pool-dedicated-1",
			resourceName2: "mongodb-pool-dedicated-2",
			description:   "Different resources produce same truncated name",
		},
		{
			stackName:     "very-long-stack-name",
			resourceName1: "mongodb-cluster-production",
			resourceName2: "mongodb-cluster-staging",
			description:   "Long stack name causes resource name conflicts",
		},
	}

	for _, tc := range conflictCases {
		t.Run(tc.description, func(t *testing.T) {
			input1 := createTestResourceInput(tc.resourceName1, nil)
			input2 := createTestResourceInput(tc.resourceName2, nil)

			result1 := toClusterName(tc.stackName, input1)
			result2 := toClusterName(tc.stackName, input2)

			if result1 == result2 {
				t.Logf("CONFLICT DETECTED: Both %q and %q produce cluster name %q",
					tc.resourceName1, tc.resourceName2, result1)
				t.Logf("This is the exact issue we need to solve with improved naming")
			} else {
				t.Logf("No conflict: %q -> %q, %q -> %q",
					tc.resourceName1, result1, tc.resourceName2, result2)
			}
		})
	}
}

func TestToProjectName_Baseline(t *testing.T) {
	// Test the project name function that toClusterName depends on
	tests := []struct {
		name         string
		stackName    string
		resourceName string
		expected     string
	}{
		{
			name:         "basic project name",
			stackName:    "integrail",
			resourceName: "mongodb-main",
			expected:     "integrail--mongodb-main--test",
		},
		{
			name:         "your actual case",
			stackName:    "integrail",
			resourceName: "mongodb-pool-dedicated-1",
			expected:     "integrail--mongodb-pool-dedicated-1--test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := createTestResourceInput(tt.resourceName, nil)
			result := toProjectName(tt.stackName, input)

			if result != tt.expected {
				t.Errorf("toProjectName() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// Helper function to create test resource input
func createTestResourceInput(resourceName string, config *mongodb.AtlasConfig) api.ResourceInput {
	var configWrapper api.Config
	if config != nil {
		configWrapper = api.Config{Config: config}
	}

	return api.ResourceInput{
		Descriptor: &api.ResourceDescriptor{
			Name:   resourceName,
			Type:   mongodb.ResourceTypeMongodbAtlas,
			Config: configWrapper,
		},
		StackParams: &api.StackParams{
			StackName:   "test-stack",
			Environment: "test",
		},
	}
}

// Helper function for pointer handling in tests
func ptrIntValue(p *int, defaultValue int) int {
	if p == nil {
		return defaultValue
	}
	return *p
}
