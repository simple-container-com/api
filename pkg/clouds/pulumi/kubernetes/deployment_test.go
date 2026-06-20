// SPDX-License-Identifier: MIT
// Copyright (c) 2025 Simple Container

package kubernetes

import (
	"testing"
	"time"

	. "github.com/onsi/gomega"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/samber/lo"

	"github.com/simple-container-com/api/pkg/clouds/k8s"
)

// TestToProbeArgs_WithHeaders tests the toProbeArgs function with HTTP headers
func TestToProbeArgs_WithHeaders(t *testing.T) {
	testCases := []struct {
		name           string
		container      *ContainerImage
		probe          *k8s.CloudRunProbe
		shouldHaveHTTP bool
	}{
		{
			name: "probe with single header",
			container: &ContainerImage{
				Container: k8s.CloudRunContainer{
					Name:     "test-container",
					Ports:    []int{8080},
					MainPort: lo.ToPtr(8080),
				},
			},
			probe: &k8s.CloudRunProbe{
				HttpGet: k8s.ProbeHttpGet{
					Path: "/health",
					Port: 8080,
					HTTPHeaders: []k8s.HTTPHeader{
						{Name: "X-Health-Check", Value: "true"},
					},
				},
				InitialDelaySeconds: lo.ToPtr(10),
				Interval:            lo.ToPtr(5 * time.Second),
			},
			shouldHaveHTTP: true,
		},
		{
			name: "probe with multiple headers",
			container: &ContainerImage{
				Container: k8s.CloudRunContainer{
					Name:     "test-container",
					Ports:    []int{8080},
					MainPort: lo.ToPtr(8080),
				},
			},
			probe: &k8s.CloudRunProbe{
				HttpGet: k8s.ProbeHttpGet{
					Path: "/health",
					Port: 8080,
					HTTPHeaders: []k8s.HTTPHeader{
						{Name: "X-Health-Check", Value: "true"},
						{Name: "Authorization", Value: "Bearer token123"},
						{Name: "X-Custom-Header", Value: "custom-value"},
					},
				},
				InitialDelaySeconds: lo.ToPtr(10),
				Interval:            lo.ToPtr(5 * time.Second),
			},
			shouldHaveHTTP: true,
		},
		{
			name: "probe without headers",
			container: &ContainerImage{
				Container: k8s.CloudRunContainer{
					Name:     "test-container",
					Ports:    []int{8080},
					MainPort: lo.ToPtr(8080),
				},
			},
			probe: &k8s.CloudRunProbe{
				HttpGet: k8s.ProbeHttpGet{
					Path: "/health",
					Port: 8080,
				},
				InitialDelaySeconds: lo.ToPtr(10),
				Interval:            lo.ToPtr(5 * time.Second),
			},
			shouldHaveHTTP: true,
		},
		{
			name: "probe with empty headers slice",
			container: &ContainerImage{
				Container: k8s.CloudRunContainer{
					Name:     "test-container",
					Ports:    []int{8080},
					MainPort: lo.ToPtr(8080),
				},
			},
			probe: &k8s.CloudRunProbe{
				HttpGet: k8s.ProbeHttpGet{
					Path:        "/health",
					Port:        8080,
					HTTPHeaders: []k8s.HTTPHeader{},
				},
				InitialDelaySeconds: lo.ToPtr(10),
				Interval:            lo.ToPtr(5 * time.Second),
			},
			shouldHaveHTTP: true,
		},
		{
			name: "TCP probe without path",
			container: &ContainerImage{
				Container: k8s.CloudRunContainer{
					Name:     "test-container",
					Ports:    []int{8080},
					MainPort: lo.ToPtr(8080),
				},
			},
			probe: &k8s.CloudRunProbe{
				HttpGet: k8s.ProbeHttpGet{
					Port: 8080,
				},
				InitialDelaySeconds: lo.ToPtr(10),
			},
			shouldHaveHTTP: false,
		},
		{
			name: "probe with authorization header",
			container: &ContainerImage{
				Container: k8s.CloudRunContainer{
					Name:     "test-container",
					Ports:    []int{8080},
					MainPort: lo.ToPtr(8080),
				},
			},
			probe: &k8s.CloudRunProbe{
				HttpGet: k8s.ProbeHttpGet{
					Path: "/health",
					Port: 8080,
					HTTPHeaders: []k8s.HTTPHeader{
						{Name: "Authorization", Value: "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9"},
					},
				},
				InitialDelaySeconds: lo.ToPtr(10),
			},
			shouldHaveHTTP: true,
		},
		{
			name: "probe with host header",
			container: &ContainerImage{
				Container: k8s.CloudRunContainer{
					Name:     "test-container",
					Ports:    []int{8080},
					MainPort: lo.ToPtr(8080),
				},
			},
			probe: &k8s.CloudRunProbe{
				HttpGet: k8s.ProbeHttpGet{
					Path: "/health",
					Port: 8080,
					HTTPHeaders: []k8s.HTTPHeader{
						{Name: "Host", Value: "example.com"},
						{Name: "X-Forwarded-Proto", Value: "https"},
					},
				},
				InitialDelaySeconds: lo.ToPtr(10),
			},
			shouldHaveHTTP: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)

			result := toProbeArgs(tc.container, tc.probe)

			// Verify probe was created
			Expect(result).ToNot(BeNil(), "ProbeArgs should not be nil")

			// Check HTTP vs TCP
			if tc.shouldHaveHTTP {
				Expect(result.HttpGet).ToNot(BeNil(), "Should have HTTP GET configured")
				Expect(result.TcpSocket).To(BeNil(), "Should not have TCP socket configured")
			} else {
				Expect(result.HttpGet).To(BeNil(), "Should not have HTTP GET configured")
				Expect(result.TcpSocket).ToNot(BeNil(), "Should have TCP socket configured")
			}

			// Verify timing parameters
			if tc.probe.InitialDelaySeconds != nil {
				Expect(result.InitialDelaySeconds).ToNot(BeNil(), "InitialDelaySeconds should be set")
			}
		})
	}
}

// TestToProbeArgs_PortResolution tests port resolution logic with different configurations
func TestToProbeArgs_PortResolution(t *testing.T) {
	testCases := []struct {
		name              string
		container         *ContainerImage
		probe             *k8s.CloudRunProbe
		shouldHaveHTTPGet bool
	}{
		{
			name: "probe with explicit port",
			container: &ContainerImage{
				Container: k8s.CloudRunContainer{
					Name:  "test-container",
					Ports: []int{8080, 9090},
				},
			},
			probe: &k8s.CloudRunProbe{
				HttpGet: k8s.ProbeHttpGet{
					Path: "/health",
					Port: 9090, // Explicit port
				},
			},
			shouldHaveHTTPGet: true,
		},
		{
			name: "probe uses container MainPort",
			container: &ContainerImage{
				Container: k8s.CloudRunContainer{
					Name:     "test-container",
					Ports:    []int{8080, 9090},
					MainPort: lo.ToPtr(9090),
				},
			},
			probe: &k8s.CloudRunProbe{
				HttpGet: k8s.ProbeHttpGet{
					Path: "/health",
					Port: 0, // No explicit port, should use MainPort
				},
			},
			shouldHaveHTTPGet: true,
		},
		{
			name: "probe uses first container port as fallback",
			container: &ContainerImage{
				Container: k8s.CloudRunContainer{
					Name:  "test-container",
					Ports: []int{8080, 9090},
					// No MainPort set
				},
			},
			probe: &k8s.CloudRunProbe{
				HttpGet: k8s.ProbeHttpGet{
					Path: "/health",
					Port: 0,
				},
			},
			shouldHaveHTTPGet: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)

			result := toProbeArgs(tc.container, tc.probe)

			Expect(result).ToNot(BeNil(), "ProbeArgs should not be nil")

			if tc.shouldHaveHTTPGet {
				Expect(result.HttpGet).ToNot(BeNil(), "Should have HTTP GET configured")
			}
		})
	}
}

// TestToProbeArgs_BackwardCompatibility tests that existing configurations without headers still work
func TestToProbeArgs_BackwardCompatibility(t *testing.T) {
	RegisterTestingT(t)

	container := &ContainerImage{
		Container: k8s.CloudRunContainer{
			Name:     "test-container",
			Ports:    []int{8080},
			MainPort: lo.ToPtr(8080),
		},
	}

	// Test with legacy configuration (no headers)
	probe := &k8s.CloudRunProbe{
		HttpGet: k8s.ProbeHttpGet{
			Path: "/health",
			Port: 8080,
			// HTTPHeaders field not specified
		},
		InitialDelaySeconds: lo.ToPtr(10),
		Interval:            lo.ToPtr(5 * time.Second),
	}

	result := toProbeArgs(container, probe)

	// Verify the probe was created successfully
	Expect(result).ToNot(BeNil(), "ProbeArgs should not be nil")
	Expect(result.HttpGet).ToNot(BeNil(), "Should have HTTP GET configured")
	Expect(result.InitialDelaySeconds).ToNot(BeNil(), "InitialDelaySeconds should not be nil")
}

// TestToProbeArgs_HeaderPreservation tests that headers are properly converted
func TestToProbeArgs_HeaderPreservation(t *testing.T) {
	RegisterTestingT(t)

	container := &ContainerImage{
		Container: k8s.CloudRunContainer{
			Name:     "test-container",
			Ports:    []int{8080},
			MainPort: lo.ToPtr(8080),
		},
	}

	probe := &k8s.CloudRunProbe{
		HttpGet: k8s.ProbeHttpGet{
			Path: "/health",
			Port: 8080,
			HTTPHeaders: []k8s.HTTPHeader{
				{Name: "X-First-Header", Value: "first-value"},
				{Name: "X-Second-Header", Value: "second-value"},
				{Name: "X-Third-Header", Value: "third-value"},
			},
		},
	}

	result := toProbeArgs(container, probe)

	Expect(result).ToNot(BeNil(), "ProbeArgs should not be nil")
	Expect(result.HttpGet).ToNot(BeNil(), "Should have HTTP GET configured")
}

// TestToProbeArgs_PeriodSeconds verifies the precedence between the k8s-native
// periodSeconds field and the legacy duration-typed interval field.
func TestToProbeArgs_PeriodSeconds(t *testing.T) {
	container := &ContainerImage{
		Container: k8s.CloudRunContainer{
			Name:     "test-container",
			Ports:    []int{8080},
			MainPort: lo.ToPtr(8080),
		},
	}

	testCases := []struct {
		name     string
		probe    *k8s.CloudRunProbe
		expected sdk.IntPtrInput
	}{
		{
			name: "periodSeconds used directly",
			probe: &k8s.CloudRunProbe{
				PeriodSeconds: lo.ToPtr(15),
			},
			expected: sdk.IntPtr(15),
		},
		{
			name: "interval converted to seconds when periodSeconds absent",
			probe: &k8s.CloudRunProbe{
				Interval: lo.ToPtr(20 * time.Second),
			},
			expected: sdk.IntPtr(20),
		},
		{
			name: "periodSeconds wins over interval",
			probe: &k8s.CloudRunProbe{
				PeriodSeconds: lo.ToPtr(15),
				Interval:      lo.ToPtr(99 * time.Second),
			},
			expected: sdk.IntPtr(15),
		},
		{
			name:     "neither set leaves kubelet default",
			probe:    &k8s.CloudRunProbe{},
			expected: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)

			result := toProbeArgs(container, tc.probe)

			Expect(result).ToNot(BeNil())
			if tc.expected == nil {
				Expect(result.PeriodSeconds).To(BeNil())
			} else {
				Expect(result.PeriodSeconds).To(Equal(tc.expected))
			}
		})
	}
}
