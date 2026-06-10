package k8s

import (
	"testing"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/api"
)

// TestStartupProbeExtraction is the regression test for cloudExtras.startupProbe
// being silently dropped: CloudExtras had no StartupProbe field, so user-supplied
// startup budgets never reached the cluster and pods fell back to the readiness
// probe's (short) window during cold starts.
func TestStartupProbeExtraction(t *testing.T) {
	RegisterTestingT(t)

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

	Expect(err).ToNot(HaveOccurred())
	Expect(result.StartupProbe).ToNot(BeNil())
	Expect(result.StartupProbe.HttpGet.Path).To(Equal("/health/"))
	Expect(result.StartupProbe.HttpGet.Port).To(Equal(8000))
	Expect(*result.StartupProbe.InitialDelaySeconds).To(Equal(10))
	Expect(*result.StartupProbe.TimeoutSeconds).To(Equal(5))
	Expect(*result.StartupProbe.PeriodSeconds).To(Equal(15))
	Expect(*result.StartupProbe.FailureThreshold).To(Equal(16))
	Expect(*result.StartupProbe.SuccessThreshold).To(Equal(1))
}

// TestProbePeriodSecondsExtraction covers the k8s-native periodSeconds spelling on
// the readiness probe. Previously only the duration-typed `interval` key existed,
// so configs written with periodSeconds silently fell back to the kubelet default.
func TestProbePeriodSecondsExtraction(t *testing.T) {
	RegisterTestingT(t)

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

	Expect(err).ToNot(HaveOccurred())
	Expect(result.ReadinessProbe).ToNot(BeNil())
	Expect(*result.ReadinessProbe.PeriodSeconds).To(Equal(20))
	Expect(*result.ReadinessProbe.FailureThreshold).To(Equal(3))
}

// TestStartupProbeAbsent ensures the zero-value path stays nil.
func TestStartupProbeAbsent(t *testing.T) {
	RegisterTestingT(t)

	result, err := api.ConvertDescriptor(map[string]any{}, &CloudExtras{})

	Expect(err).ToNot(HaveOccurred())
	Expect(result.StartupProbe).To(BeNil())
}
