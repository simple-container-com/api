package k8s

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/simple-container-com/api/pkg/api"
)

// TestPriorityClassNameExtraction tests that priorityClassName is correctly extracted from cloudExtras
func TestPriorityClassNameExtraction(t *testing.T) {
	cloudExtras := map[string]any{
		"priorityClassName": "high-priority",
	}

	result, err := api.ConvertDescriptor(cloudExtras, &CloudExtras{})

	require.NoError(t, err)
	assert.NotNil(t, result.PriorityClassName)
	assert.Equal(t, "high-priority", *result.PriorityClassName)
}

// TestPriorityClassNameNilHandling tests that nil priorityClassName is handled correctly
func TestPriorityClassNameNilHandling(t *testing.T) {
	cloudExtras := map[string]any{}

	result, err := api.ConvertDescriptor(cloudExtras, &CloudExtras{})

	require.NoError(t, err)
	assert.Nil(t, result.PriorityClassName)
}

// TestPriorityClassNameWithSystemCritical tests system-critical priority class
func TestPriorityClassNameWithSystemCritical(t *testing.T) {
	cloudExtras := map[string]any{
		"priorityClassName": "system-cluster-critical",
	}

	result, err := api.ConvertDescriptor(cloudExtras, &CloudExtras{})

	require.NoError(t, err)
	assert.Equal(t, "system-cluster-critical", *result.PriorityClassName)
}

// TestPriorityClassNameWithOtherCloudExtras tests priorityClassName alongside other cloudExtras fields
func TestPriorityClassNameWithOtherCloudExtras(t *testing.T) {
	cloudExtras := map[string]any{
		"priorityClassName": "production-priority",
		"nodeSelector": map[string]any{
			"disktype": "ssd",
		},
		"tolerations": []map[string]any{
			{
				"key":      "key1",
				"operator": "Equal",
				"value":    "value1",
				"effect":   "NoSchedule",
			},
		},
	}

	result, err := api.ConvertDescriptor(cloudExtras, &CloudExtras{})

	require.NoError(t, err)
	assert.Equal(t, "production-priority", *result.PriorityClassName)
	assert.NotNil(t, result.NodeSelector)
	assert.Equal(t, "ssd", result.NodeSelector["disktype"])
	assert.Len(t, result.Tolerations, 1)
}

// TestPriorityClassNameEmptyString tests that empty string priorityClassName is handled
func TestPriorityClassNameEmptyString(t *testing.T) {
	cloudExtras := map[string]any{
		"priorityClassName": "",
	}

	result, err := api.ConvertDescriptor(cloudExtras, &CloudExtras{})

	require.NoError(t, err)
	// Empty string should result in a pointer to empty string, not nil
	assert.NotNil(t, result.PriorityClassName)
	assert.Equal(t, "", *result.PriorityClassName)
}

// TestPriorityClassNameInvalidType tests that invalid type is handled by the converter
func TestPriorityClassNameInvalidType(t *testing.T) {
	cloudExtras := map[string]any{
		"priorityClassName": 123, // Invalid type: YAML converter will convert to string
	}

	result, err := api.ConvertDescriptor(cloudExtras, &CloudExtras{})

	// YAML marshaling converts the integer to string, so no error occurs
	require.NoError(t, err)
	// The value is converted to "123" as a string
	assert.NotNil(t, result.PriorityClassName)
	assert.Equal(t, "123", *result.PriorityClassName)
}

// TestCloudExtrasWithPriorityClassAndAffinity tests priorityClassName with affinity rules
func TestCloudExtrasWithPriorityClassAndAffinity(t *testing.T) {
	cloudExtras := map[string]any{
		"priorityClassName": "high-priority-apps",
		"affinity": map[string]any{
			"nodePool":     "pool-1",
			"computeClass": "Balanced",
		},
	}

	result, err := api.ConvertDescriptor(cloudExtras, &CloudExtras{})

	require.NoError(t, err)
	assert.Equal(t, "high-priority-apps", *result.PriorityClassName)
	assert.NotNil(t, result.Affinity)
	assert.Equal(t, "pool-1", *result.Affinity.NodePool)
	assert.Equal(t, "Balanced", *result.Affinity.ComputeClass)
}

// TestPriorityClassNameWithVPA tests priorityClassName alongside VPA configuration
func TestPriorityClassNameWithVPA(t *testing.T) {
	cloudExtras := map[string]any{
		"priorityClassName": "workload-production",
		"vpa": map[string]any{
			"enabled":    true,
			"updateMode": "Auto",
		},
	}

	result, err := api.ConvertDescriptor(cloudExtras, &CloudExtras{})

	require.NoError(t, err)
	assert.Equal(t, "workload-production", *result.PriorityClassName)
	assert.NotNil(t, result.VPA)
	assert.True(t, result.VPA.Enabled)
	assert.Equal(t, "Auto", *result.VPA.UpdateMode)
}

// TestPriorityClassNameWithProbes tests priorityClassName with health probes
func TestPriorityClassNameWithProbes(t *testing.T) {
	cloudExtras := map[string]any{
		"priorityClassName": "critical-service",
		"readinessProbe": map[string]any{
			"httpGet": map[string]any{
				"path": "/health",
			},
		},
		"livenessProbe": map[string]any{
			"httpGet": map[string]any{
				"path": "/health",
			},
		},
	}

	result, err := api.ConvertDescriptor(cloudExtras, &CloudExtras{})

	require.NoError(t, err)
	assert.Equal(t, "critical-service", *result.PriorityClassName)
	assert.NotNil(t, result.ReadinessProbe)
	assert.NotNil(t, result.LivenessProbe)
}

// TestPriorityClassNameWithEphemeralVolumes tests priorityClassName with ephemeral volumes
func TestPriorityClassNameWithEphemeralVolumes(t *testing.T) {
	cloudExtras := map[string]any{
		"priorityClassName": "high-priority-storage",
		"ephemeralVolumes": []map[string]any{
			{
				"name":      "temp-data",
				"mountPath": "/tmp/data",
				"size":      "100Gi",
			},
		},
	}

	result, err := api.ConvertDescriptor(cloudExtras, &CloudExtras{})

	require.NoError(t, err)
	assert.Equal(t, "high-priority-storage", *result.PriorityClassName)
	assert.Len(t, result.EphemeralVolumes, 1)
	assert.Equal(t, "temp-data", result.EphemeralVolumes[0].Name)
}
