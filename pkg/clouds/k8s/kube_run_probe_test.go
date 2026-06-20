// SPDX-License-Identifier: MIT
// Copyright (c) 2025 Simple Container

package k8s

import (
	"testing"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/api"
)

// Regression: cloudExtras.startupProbe was silently dropped (no struct field).
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

// Regression: periodSeconds was an unknown key and fell back to kubelet defaults.
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

func TestStartupProbeAbsent(t *testing.T) {
	RegisterTestingT(t)

	result, err := api.ConvertDescriptor(map[string]any{}, &CloudExtras{})

	Expect(err).ToNot(HaveOccurred())
	Expect(result.StartupProbe).To(BeNil())
}
