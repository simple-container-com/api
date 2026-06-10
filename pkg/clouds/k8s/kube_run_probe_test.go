package k8s

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/simple-container-com/api/pkg/api"
)

// TestStartupProbeExtraction is the regression test for cloudExtras.startupProbe
// being silently dropped: CloudExtras had no StartupProbe field, so user-supplied
// startup budgets never reached the cluster and pods fell back to the readiness
// probe's (short) window during cold starts.
func TestStartupProbeExtraction(t *testing.T) {
	cloudExtras := map[string]any{
		"startupProbe": map[string]any{
			"httpGet": map[string]any{
				"path": "/health/",
				"port": 8000,
			},
			"initialDelaySeconds": 10,
			"timeoutSeconds":      5,
			"periodSeconds":       15,
			"failureThreshold":    16,
			"successThreshold":    1,
		},
	}

	result, err := api.ConvertDescriptor(cloudExtras, &CloudExtras{})

	require.NoError(t, err)
	require.NotNil(t, result.StartupProbe)
	assert.Equal(t, "/health/", result.StartupProbe.HttpGet.Path)
	assert.Equal(t, 8000, result.StartupProbe.HttpGet.Port)
	assert.Equal(t, 10, *result.StartupProbe.InitialDelaySeconds)
	assert.Equal(t, 5, *result.StartupProbe.TimeoutSeconds)
	assert.Equal(t, 15, *result.StartupProbe.PeriodSeconds)
	assert.Equal(t, 16, *result.StartupProbe.FailureThreshold)
	assert.Equal(t, 1, *result.StartupProbe.SuccessThreshold)
}

// TestProbePeriodSecondsExtraction covers the k8s-native periodSeconds spelling on
// the readiness probe. Previously only the duration-typed `interval` key existed,
// so configs written with periodSeconds silently fell back to the kubelet default.
func TestProbePeriodSecondsExtraction(t *testing.T) {
	cloudExtras := map[string]any{
		"readinessProbe": map[string]any{
			"httpGet": map[string]any{
				"path": "/ready/",
				"port": 8080,
			},
			"periodSeconds":    20,
			"failureThreshold": 3,
		},
	}

	result, err := api.ConvertDescriptor(cloudExtras, &CloudExtras{})

	require.NoError(t, err)
	require.NotNil(t, result.ReadinessProbe)
	assert.Equal(t, 20, *result.ReadinessProbe.PeriodSeconds)
	assert.Equal(t, 3, *result.ReadinessProbe.FailureThreshold)
}

// TestStartupProbeAbsent ensures the zero-value path stays nil.
func TestStartupProbeAbsent(t *testing.T) {
	result, err := api.ConvertDescriptor(map[string]any{}, &CloudExtras{})

	require.NoError(t, err)
	assert.Nil(t, result.StartupProbe)
}
