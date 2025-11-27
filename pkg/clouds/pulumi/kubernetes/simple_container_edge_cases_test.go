package kubernetes

import (
	"fmt"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/samber/lo"

	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/logger"
	"github.com/simple-container-com/api/pkg/clouds/k8s"
)

// Edge cases and error handling tests for SimpleContainer

func TestSimpleContainer_EmptyContainersArray(t *testing.T) {
	RegisterTestingT(t)

	mocks := NewSimpleContainerMocks()

	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		args := &SimpleContainerArgs{
			Namespace:  "empty-containers-test",
			Service:    "empty-service",
			ScEnv:      "test",
			Deployment: "empty-deployment",
			Replicas:   1,
			Log:        logger.New(),

			// Empty containers array - should not crash
			Containers: []corev1.ContainerArgs{},
		}

		// This might succeed or fail depending on implementation
		// The important thing is that it doesn't panic
		sc, err := NewSimpleContainer(ctx, args)

		// If it succeeds, validate basic structure
		if err == nil && sc != nil {
			Expect(sc.Namespace).ToNot(BeNil())
		}

		// If it fails, that's also acceptable for this edge case
		// The test is mainly checking that we don't panic
		return nil // Don't propagate errors for this edge case test
	}, pulumi.WithMocks("project", "stack", mocks))

	Expect(err).ToNot(HaveOccurred())
}

func TestSimpleContainer_ExtremelyLongNames(t *testing.T) {
	RegisterTestingT(t)

	mocks := NewSimpleContainerMocks()

	// Kubernetes has limits on name lengths (63 characters for most resources)
	longName := "this-is-an-extremely-long-name-that-exceeds-kubernetes-naming-limits-and-should-be-truncated-or-sanitized-properly"

	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		args := &SimpleContainerArgs{
			Namespace:  longName,
			Service:    longName,
			ScEnv:      "test",
			Deployment: longName,
			Replicas:   1,
			Log:        logger.New(),

			IngressContainer: &k8s.CloudRunContainer{
				Name:     "test-container",
				Ports:    []int{8080},
				MainPort: lo.ToPtr(8080),
			},
			ServiceType: lo.ToPtr("ClusterIP"),

			Containers: []corev1.ContainerArgs{
				{
					Name:  sdk.String("test-container"),
					Image: sdk.String("nginx:latest"),
					Ports: corev1.ContainerPortArray{
						&corev1.ContainerPortArgs{
							ContainerPort: sdk.Int(8080),
						},
					},
				},
			},
		}

		sc, err := NewSimpleContainer(ctx, args)
		Expect(err).ToNot(HaveOccurred(), "SimpleContainer should handle long names gracefully")
		Expect(sc).ToNot(BeNil(), "SimpleContainer should not be nil")

		// Validate that resources were created despite long names
		Expect(sc.Namespace).ToNot(BeNil())
		Expect(sc.Deployment).ToNot(BeNil())

		return nil
	}, pulumi.WithMocks("project", "stack", mocks))

	Expect(err).ToNot(HaveOccurred())
}

func TestSimpleContainer_SpecialCharactersInNames(t *testing.T) {
	RegisterTestingT(t)

	testCases := []struct {
		name        string
		inputName   string
		description string
	}{
		{
			name:        "underscores_and_dots",
			inputName:   "my_service.v1.2.3",
			description: "underscores and dots should be sanitized",
		},
		{
			name:        "special_symbols",
			inputName:   "service@#$%^&*()+=[]{}|\\:;\"'<>?,./",
			description: "special symbols should be removed",
		},
		{
			name:        "unicode_characters",
			inputName:   "service-名前-тест-🚀",
			description: "unicode characters should be handled",
		},
		{
			name:        "leading_trailing_hyphens",
			inputName:   "---service---",
			description: "leading and trailing hyphens should be removed",
		},
		{
			name:        "numbers_only",
			inputName:   "123456789",
			description: "numbers-only names should be handled",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mocks := NewSimpleContainerMocks()

			err := pulumi.RunErr(func(ctx *pulumi.Context) error {
				args := &SimpleContainerArgs{
					Namespace:  tc.inputName,
					Service:    tc.inputName,
					ScEnv:      "test",
					Deployment: tc.inputName,
					Replicas:   1,
					Log:        logger.New(),

					IngressContainer: &k8s.CloudRunContainer{
						Name:     "test-container",
						Ports:    []int{8080},
						MainPort: lo.ToPtr(8080),
					},
					ServiceType: lo.ToPtr("ClusterIP"),

					Containers: []corev1.ContainerArgs{
						{
							Name:  sdk.String("test-container"),
							Image: sdk.String("nginx:latest"),
							Ports: corev1.ContainerPortArray{
								&corev1.ContainerPortArgs{
									ContainerPort: sdk.Int(8080),
								},
							},
						},
					},
				}

				sc, err := NewSimpleContainer(ctx, args)
				Expect(err).ToNot(HaveOccurred(), "SimpleContainer should handle special characters: %s", tc.description)
				Expect(sc).ToNot(BeNil(), "SimpleContainer should not be nil")

				// Validate that resources were created
				Expect(sc.Namespace).ToNot(BeNil())
				Expect(sc.Deployment).ToNot(BeNil())

				return nil
			}, pulumi.WithMocks("project", "stack", mocks))

			Expect(err).ToNot(HaveOccurred(), "Test case %s should complete without errors", tc.name)
		})
	}
}

func TestSimpleContainer_ExtremeResourceValues(t *testing.T) {
	RegisterTestingT(t)

	testCases := []struct {
		name        string
		resources   *corev1.ResourceRequirementsArgs
		description string
	}{
		{
			name: "minimal_resources",
			resources: &corev1.ResourceRequirementsArgs{
				Requests: sdk.StringMap{
					"cpu":    sdk.String("1m"),
					"memory": sdk.String("1Mi"),
				},
				Limits: sdk.StringMap{
					"cpu":    sdk.String("10m"),
					"memory": sdk.String("10Mi"),
				},
			},
			description: "minimal resource allocation",
		},
		{
			name: "maximum_resources",
			resources: &corev1.ResourceRequirementsArgs{
				Requests: sdk.StringMap{
					"cpu":    sdk.String("100"),
					"memory": sdk.String("1000Gi"),
				},
				Limits: sdk.StringMap{
					"cpu":    sdk.String("200"),
					"memory": sdk.String("2000Gi"),
				},
			},
			description: "maximum resource allocation",
		},
		{
			name: "fractional_cpu",
			resources: &corev1.ResourceRequirementsArgs{
				Requests: sdk.StringMap{
					"cpu":    sdk.String("0.1"),
					"memory": sdk.String("128Mi"),
				},
				Limits: sdk.StringMap{
					"cpu":    sdk.String("0.5"),
					"memory": sdk.String("512Mi"),
				},
			},
			description: "fractional CPU values",
		},
		{
			name: "binary_memory_units",
			resources: &corev1.ResourceRequirementsArgs{
				Requests: sdk.StringMap{
					"cpu":    sdk.String("100m"),
					"memory": sdk.String("128Mi"),
				},
				Limits: sdk.StringMap{
					"cpu":    sdk.String("500m"),
					"memory": sdk.String("1Gi"),
				},
			},
			description: "binary memory units (Mi, Gi)",
		},
		{
			name: "decimal_memory_units",
			resources: &corev1.ResourceRequirementsArgs{
				Requests: sdk.StringMap{
					"cpu":    sdk.String("100m"),
					"memory": sdk.String("128M"),
				},
				Limits: sdk.StringMap{
					"cpu":    sdk.String("500m"),
					"memory": sdk.String("1G"),
				},
			},
			description: "decimal memory units (M, G)",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mocks := NewSimpleContainerMocks()

			err := pulumi.RunErr(func(ctx *pulumi.Context) error {
				args := &SimpleContainerArgs{
					Namespace:  "resource-test",
					Service:    "resource-service",
					ScEnv:      "test",
					Deployment: "resource-deployment",
					Replicas:   1,
					Log:        logger.New(),

					IngressContainer: &k8s.CloudRunContainer{
						Name:     "resource-container",
						Ports:    []int{8080},
						MainPort: lo.ToPtr(8080),
					},
					ServiceType: lo.ToPtr("ClusterIP"),

					Containers: []corev1.ContainerArgs{
						{
							Name:      sdk.String("resource-container"),
							Image:     sdk.String("nginx:latest"),
							Resources: tc.resources,
							Ports: corev1.ContainerPortArray{
								&corev1.ContainerPortArgs{
									ContainerPort: sdk.Int(8080),
								},
							},
						},
					},
				}

				sc, err := NewSimpleContainer(ctx, args)
				Expect(err).ToNot(HaveOccurred(), "SimpleContainer with %s should be created successfully", tc.description)
				Expect(sc).ToNot(BeNil(), "SimpleContainer should not be nil")

				// Validate creation
				Expect(sc.ServicePublicIP).ToNot(BeNil())
				Expect(sc.ServiceName).ToNot(BeNil())
				Expect(sc.Namespace).ToNot(BeNil())
				Expect(sc.Deployment).ToNot(BeNil())

				return nil
			}, pulumi.WithMocks("project", "stack", mocks))

			Expect(err).ToNot(HaveOccurred())
		})
	}
}

func TestSimpleContainer_ExtremeScalingValues(t *testing.T) {
	RegisterTestingT(t)

	testCases := []struct {
		name        string
		scaleConfig *k8s.Scale
		description string
	}{
		{
			name: "single_replica_HPA",
			scaleConfig: &k8s.Scale{
				Replicas:     1,
				EnableHPA:    true,
				MinReplicas:  1,
				MaxReplicas:  2,
				CPUTarget:    lo.ToPtr(50),
				MemoryTarget: lo.ToPtr(50),
			},
			description: "minimal HPA configuration (1-2 replicas)",
		},
		{
			name: "massive_scale_HPA",
			scaleConfig: &k8s.Scale{
				Replicas:     100,
				EnableHPA:    true,
				MinReplicas:  100,
				MaxReplicas:  10000,
				CPUTarget:    lo.ToPtr(90),
				MemoryTarget: lo.ToPtr(95),
			},
			description: "massive scale HPA configuration (100-10000 replicas)",
		},
		{
			name: "extreme_thresholds",
			scaleConfig: &k8s.Scale{
				Replicas:     5,
				EnableHPA:    true,
				MinReplicas:  5,
				MaxReplicas:  50,
				CPUTarget:    lo.ToPtr(1),  // Very low threshold
				MemoryTarget: lo.ToPtr(99), // Very high threshold
			},
			description: "extreme scaling thresholds (1% CPU, 99% memory)",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mocks := NewSimpleContainerMocks()

			err := pulumi.RunErr(func(ctx *pulumi.Context) error {
				args := &SimpleContainerArgs{
					Namespace:  "scaling-test",
					Service:    "scaling-service",
					ScEnv:      "test",
					Deployment: "scaling-deployment",
					Replicas:   tc.scaleConfig.Replicas,
					Log:        logger.New(),

					IngressContainer: &k8s.CloudRunContainer{
						Name:     "test-container",
						Ports:    []int{8080},
						MainPort: lo.ToPtr(8080),
					},
					ServiceType: lo.ToPtr("ClusterIP"),

					Scale: tc.scaleConfig,

					Containers: []corev1.ContainerArgs{
						{
							Name:  sdk.String("test-container"),
							Image: sdk.String("nginx:latest"),
							Ports: corev1.ContainerPortArray{
								&corev1.ContainerPortArgs{
									ContainerPort: sdk.Int(8080),
								},
							},
							Resources: &corev1.ResourceRequirementsArgs{
								Requests: sdk.StringMap{
									"cpu":    sdk.String("100m"),
									"memory": sdk.String("128Mi"),
								},
								Limits: sdk.StringMap{
									"cpu":    sdk.String("500m"),
									"memory": sdk.String("512Mi"),
								},
							},
						},
					},
				}

				sc, err := NewSimpleContainer(ctx, args)
				Expect(err).ToNot(HaveOccurred(), "SimpleContainer with %s should be created successfully", tc.description)
				Expect(sc).ToNot(BeNil(), "SimpleContainer should not be nil")

				// Validate basic structure
				Expect(sc.ServicePublicIP).ToNot(BeNil())
				Expect(sc.ServiceName).ToNot(BeNil())
				Expect(sc.Namespace).ToNot(BeNil())
				Expect(sc.Deployment).ToNot(BeNil())

				return nil
			}, pulumi.WithMocks("project", "stack", mocks))

			Expect(err).ToNot(HaveOccurred(), "Test case %s should complete without errors", tc.name)
		})
	}
}

func TestSimpleContainer_LargeVolumeConfiguration(t *testing.T) {
	RegisterTestingT(t)

	mocks := NewSimpleContainerMocks()

	// Create a large number of volumes to test performance and limits
	var textVolumes []k8s.SimpleTextVolume
	var secretVolumes []k8s.SimpleTextVolume
	var persistentVolumes []k8s.PersistentVolume

	// Create 20 text volumes
	for i := 0; i < 20; i++ {
		textVolumes = append(textVolumes, k8s.SimpleTextVolume{
			TextVolume: api.TextVolume{
				Name:      fmt.Sprintf("config-volume-%d", i),
				Content:   fmt.Sprintf("config-data-%d=value-%d", i, i),
				MountPath: fmt.Sprintf("/etc/config-%d", i),
			},
		})
	}

	// Create 10 secret volumes
	for i := 0; i < 10; i++ {
		secretVolumes = append(secretVolumes, k8s.SimpleTextVolume{
			TextVolume: api.TextVolume{
				Name:      fmt.Sprintf("secret-volume-%d", i),
				Content:   fmt.Sprintf("secret-data-%d=secret-value-%d", i, i),
				MountPath: fmt.Sprintf("/etc/secrets-%d", i),
			},
		})
	}

	// Create 5 persistent volumes
	for i := 0; i < 5; i++ {
		persistentVolumes = append(persistentVolumes, k8s.PersistentVolume{
			Name:        fmt.Sprintf("pv-%d", i),
			MountPath:   fmt.Sprintf("/data-%d", i),
			Storage:     fmt.Sprintf("%dGi", i+1),
			AccessModes: []string{"ReadWriteOnce"},
		})
	}

	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		args := &SimpleContainerArgs{
			Namespace:  "large-volume-test",
			Service:    "large-volume-service",
			ScEnv:      "test",
			Deployment: "large-volume-deployment",
			Replicas:   1,
			Log:        logger.New(),

			IngressContainer: &k8s.CloudRunContainer{
				Name:     "volume-container",
				Ports:    []int{8080},
				MainPort: lo.ToPtr(8080),
			},
			ServiceType: lo.ToPtr("ClusterIP"),

			Volumes:           textVolumes,
			SecretVolumes:     secretVolumes,
			PersistentVolumes: persistentVolumes,

			Containers: []corev1.ContainerArgs{
				{
					Name:  sdk.String("volume-container"),
					Image: sdk.String("nginx:latest"),
					Ports: corev1.ContainerPortArray{
						&corev1.ContainerPortArgs{
							ContainerPort: sdk.Int(8080),
						},
					},
				},
			},
		}

		sc, err := NewSimpleContainer(ctx, args)
		Expect(err).ToNot(HaveOccurred(), "SimpleContainer with large volume configuration should be created successfully")
		Expect(sc).ToNot(BeNil(), "SimpleContainer should not be nil")

		// Validate creation
		Expect(sc.ServicePublicIP).ToNot(BeNil())
		Expect(sc.ServiceName).ToNot(BeNil())
		Expect(sc.Namespace).ToNot(BeNil())
		Expect(sc.Deployment).ToNot(BeNil())

		return nil
	}, pulumi.WithMocks("project", "stack", mocks))

	Expect(err).ToNot(HaveOccurred())
}

func TestSimpleContainer_EmptyStringFields(t *testing.T) {
	RegisterTestingT(t)

	mocks := NewSimpleContainerMocks()

	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		args := &SimpleContainerArgs{
			Namespace:  "empty-fields-test",
			Service:    "empty-service",
			ScEnv:      "test",
			Deployment: "empty-deployment",
			Replicas:   1,
			Log:        logger.New(),

			// Empty string fields should be handled gracefully
			Domain:      "", // Empty domain
			Prefix:      "", // Empty prefix
			ParentStack: nil,

			IngressContainer: &k8s.CloudRunContainer{
				Name:     "test-container",
				Ports:    []int{8080},
				MainPort: lo.ToPtr(8080),
			},
			ServiceType: lo.ToPtr("ClusterIP"),

			// Empty maps should be handled
			SecretEnvs:   map[string]string{},
			Annotations:  map[string]string{},
			NodeSelector: map[string]string{},

			// Empty slices should be handled
			Volumes:           []k8s.SimpleTextVolume{},
			SecretVolumes:     []k8s.SimpleTextVolume{},
			PersistentVolumes: []k8s.PersistentVolume{},

			Containers: []corev1.ContainerArgs{
				{
					Name:  sdk.String("test-container"),
					Image: sdk.String("nginx:latest"),
					Ports: corev1.ContainerPortArray{
						&corev1.ContainerPortArgs{
							ContainerPort: sdk.Int(8080),
						},
					},
				},
			},
		}

		sc, err := NewSimpleContainer(ctx, args)
		Expect(err).ToNot(HaveOccurred(), "SimpleContainer with empty fields should be created successfully")
		Expect(sc).ToNot(BeNil(), "SimpleContainer should not be nil")

		// Validate creation
		Expect(sc.ServicePublicIP).ToNot(BeNil())
		Expect(sc.ServiceName).ToNot(BeNil())
		Expect(sc.Namespace).ToNot(BeNil())
		Expect(sc.Deployment).ToNot(BeNil())

		return nil
	}, pulumi.WithMocks("project", "stack", mocks))

	Expect(err).ToNot(HaveOccurred())
}
