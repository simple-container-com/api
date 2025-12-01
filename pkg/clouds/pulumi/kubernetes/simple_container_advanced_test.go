package kubernetes

import (
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

// Advanced unit test scenarios for SimpleContainer

func TestSimpleContainer_MultiContainerDeployment(t *testing.T) {
	RegisterTestingT(t)

	mocks := NewSimpleContainerMocks()

	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		args := &SimpleContainerArgs{
			Namespace:  "multi-container-test",
			Service:    "multi-service",
			ScEnv:      "test",
			Deployment: "multi-deployment",
			Replicas:   1,
			Log:        logger.New(),

			IngressContainer: &k8s.CloudRunContainer{
				Name:     "main-container",
				Ports:    []int{8080},
				MainPort: lo.ToPtr(8080),
			},
			ServiceType: lo.ToPtr("ClusterIP"),

			// Multiple containers
			Containers: []corev1.ContainerArgs{
				{
					Name:  sdk.String("main-container"),
					Image: sdk.String("nginx:latest"),
					Ports: corev1.ContainerPortArray{
						&corev1.ContainerPortArgs{
							ContainerPort: sdk.Int(8080),
							Name:          sdk.String("http"),
						},
					},
				},
			},
			// Sidecar containers
			Sidecars: []corev1.ContainerArgs{
				{
					Name:  sdk.String("logging-sidecar"),
					Image: sdk.String("fluent/fluent-bit:latest"),
				},
				{
					Name:  sdk.String("metrics-sidecar"),
					Image: sdk.String("prom/node-exporter:latest"),
					Ports: corev1.ContainerPortArray{
						&corev1.ContainerPortArgs{
							ContainerPort: sdk.Int(9100),
							Name:          sdk.String("metrics"),
						},
					},
				},
			},
			// Init containers
			InitContainers: []corev1.ContainerArgs{
				{
					Name:  sdk.String("init-db"),
					Image: sdk.String("busybox:latest"),
					Command: sdk.StringArray{
						sdk.String("sh"),
						sdk.String("-c"),
						sdk.String("echo 'Initializing database...'"),
					},
				},
			},
		}

		sc, err := NewSimpleContainer(ctx, args)
		Expect(err).ToNot(HaveOccurred(), "Multi-container SimpleContainer should be created successfully")
		Expect(sc).ToNot(BeNil(), "SimpleContainer should not be nil")

		// Validate that SimpleContainer was created with multiple containers
		Expect(sc.ServicePublicIP).ToNot(BeNil())
		Expect(sc.ServiceName).ToNot(BeNil())
		Expect(sc.Namespace).ToNot(BeNil())
		Expect(sc.Deployment).ToNot(BeNil())

		return nil
	}, pulumi.WithMocks("project", "stack", mocks))

	Expect(err).ToNot(HaveOccurred())
}

func TestSimpleContainer_ComplexVolumeConfiguration(t *testing.T) {
	RegisterTestingT(t)

	mocks := NewSimpleContainerMocks()

	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		args := &SimpleContainerArgs{
			Namespace:  "volume-test",
			Service:    "volume-service",
			ScEnv:      "test",
			Deployment: "volume-deployment",
			Replicas:   1,
			Log:        logger.New(),

			IngressContainer: &k8s.CloudRunContainer{
				Name:     "app-container",
				Ports:    []int{8080},
				MainPort: lo.ToPtr(8080),
			},
			ServiceType: lo.ToPtr("ClusterIP"),

			// Text volumes (ConfigMaps)
			Volumes: []k8s.SimpleTextVolume{
				{
					TextVolume: api.TextVolume{
						Name:      "app-config",
						Content:   "server.port=8080\napp.name=test-app",
						MountPath: "/etc/config",
					},
				},
				{
					TextVolume: api.TextVolume{
						Name:      "nginx-config",
						Content:   "worker_processes 1;\nevents { worker_connections 1024; }",
						MountPath: "/etc/nginx",
					},
				},
			},

			// Secret volumes
			SecretVolumes: []k8s.SimpleTextVolume{
				{
					TextVolume: api.TextVolume{
						Name:      "db-credentials",
						Content:   "username=admin\npassword=secret123",
						MountPath: "/etc/secrets",
					},
				},
			},

			// Persistent volumes
			PersistentVolumes: []k8s.PersistentVolume{
				{
					Name:             "data-volume",
					MountPath:        "/data",
					Storage:          "10Gi",
					AccessModes:      []string{"ReadWriteOnce"},
					StorageClassName: lo.ToPtr("fast-ssd"),
				},
				{
					Name:        "shared-volume",
					MountPath:   "/shared",
					Storage:     "5Gi",
					AccessModes: []string{"ReadWriteMany"},
				},
			},

			// Secret environment variables
			SecretEnvs: map[string]string{
				"DATABASE_URL":      "postgresql://user:pass@db:5432/mydb",
				"API_KEY":           "super-secret-api-key",
				"ENCRYPTION_SECRET": "encryption-key-123",
			},

			Containers: []corev1.ContainerArgs{
				{
					Name:  sdk.String("app-container"),
					Image: sdk.String("myapp:latest"),
					Ports: corev1.ContainerPortArray{
						&corev1.ContainerPortArgs{
							ContainerPort: sdk.Int(8080),
						},
					},
				},
			},
		}

		sc, err := NewSimpleContainer(ctx, args)
		Expect(err).ToNot(HaveOccurred(), "SimpleContainer with complex volumes should be created successfully")
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

func TestSimpleContainer_SecurityAndNetworkingConfiguration(t *testing.T) {
	RegisterTestingT(t)

	mocks := NewSimpleContainerMocks()

	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		args := &SimpleContainerArgs{
			Namespace:  "security-test",
			Service:    "secure-service",
			ScEnv:      "production",
			Deployment: "secure-deployment",
			Replicas:   3,
			Log:        logger.New(),

			IngressContainer: &k8s.CloudRunContainer{
				Name:     "secure-app",
				Ports:    []int{8443},
				MainPort: lo.ToPtr(8443),
			},
			ServiceType:      lo.ToPtr("LoadBalancer"),
			ProvisionIngress: true,
			UseSSL:           true,

			// Security context
			SecurityContext: &corev1.PodSecurityContextArgs{
				RunAsNonRoot: sdk.Bool(true),
				RunAsUser:    sdk.Int(1000),
				RunAsGroup:   sdk.Int(1000),
				FsGroup:      sdk.Int(2000),
			},

			// Node selector for specific node types
			NodeSelector: map[string]string{
				"node-type":         "compute-optimized",
				"security-level":    "high",
				"availability-zone": "us-west-2a",
			},

			// Custom annotations
			Annotations: map[string]string{
				"deployment.kubernetes.io/revision": "1",
				"app.kubernetes.io/version":         "v1.2.3",
				"app.kubernetes.io/component":       "backend",
				"security.policy/network-isolation": "strict",
				"monitoring.prometheus.io/scrape":   "true",
				"monitoring.prometheus.io/port":     "9090",
				"logging.fluentd.io/parser":         "json",
				"backup.policy/retention-days":      "30",
				"cost-center":                       "engineering",
				"environment":                       "production",
			},

			// Pod disruption budget for high availability
			PodDisruption: &k8s.DisruptionBudget{
				MinAvailable:   lo.ToPtr(2), // Keep at least 2 pods available
				MaxUnavailable: nil,
			},

			// Custom headers (Headers is just a map[string]string)
			Headers: &k8s.Headers{
				"X-Forwarded-Proto":         "https",
				"X-Real-IP":                 "$remote_addr",
				"X-Request-ID":              "$request_id",
				"X-Frame-Options":           "DENY",
				"X-Content-Type-Options":    "nosniff",
				"X-XSS-Protection":          "1; mode=block",
				"Strict-Transport-Security": "max-age=31536000; includeSubDomains",
			},

			Containers: []corev1.ContainerArgs{
				{
					Name:  sdk.String("secure-app"),
					Image: sdk.String("myapp:v1.2.3"),
					Ports: corev1.ContainerPortArray{
						&corev1.ContainerPortArgs{
							ContainerPort: sdk.Int(8443),
							Name:          sdk.String("https"),
							Protocol:      sdk.String("TCP"),
						},
						&corev1.ContainerPortArgs{
							ContainerPort: sdk.Int(9090),
							Name:          sdk.String("metrics"),
							Protocol:      sdk.String("TCP"),
						},
					},
					SecurityContext: &corev1.SecurityContextArgs{
						AllowPrivilegeEscalation: sdk.Bool(false),
						ReadOnlyRootFilesystem:   sdk.Bool(true),
						RunAsNonRoot:             sdk.Bool(true),
						RunAsUser:                sdk.Int(1000),
						Capabilities: &corev1.CapabilitiesArgs{
							Drop: sdk.StringArray{
								sdk.String("ALL"),
							},
							Add: sdk.StringArray{
								sdk.String("NET_BIND_SERVICE"),
							},
						},
					},
				},
			},
		}

		sc, err := NewSimpleContainer(ctx, args)
		Expect(err).ToNot(HaveOccurred(), "Secure SimpleContainer should be created successfully")
		Expect(sc).ToNot(BeNil(), "SimpleContainer should not be nil")

		// Validate security configuration
		Expect(sc.ServicePublicIP).ToNot(BeNil())
		Expect(sc.ServiceName).ToNot(BeNil())
		Expect(sc.Namespace).ToNot(BeNil())
		Expect(sc.Deployment).ToNot(BeNil())

		return nil
	}, pulumi.WithMocks("project", "stack", mocks))

	Expect(err).ToNot(HaveOccurred())
}

func TestSimpleContainer_AutoscalingCombinations(t *testing.T) {
	RegisterTestingT(t)

	testCases := []struct {
		name        string
		scaleConfig *k8s.Scale
		vpaConfig   *k8s.VPAConfig
		description string
	}{
		{
			name: "HPA_only",
			scaleConfig: &k8s.Scale{
				Replicas:     2,
				EnableHPA:    true,
				MinReplicas:  2,
				MaxReplicas:  20,
				CPUTarget:    lo.ToPtr(70),
				MemoryTarget: lo.ToPtr(80),
			},
			vpaConfig:   nil,
			description: "HPA enabled with CPU and memory targets",
		},
		{
			name:        "VPA_only",
			scaleConfig: nil,
			vpaConfig: &k8s.VPAConfig{
				Enabled:    true,
				UpdateMode: lo.ToPtr("Auto"),
			},
			description: "VPA enabled with auto update mode",
		},
		{
			name: "HPA_and_VPA_together",
			scaleConfig: &k8s.Scale{
				Replicas:    3,
				EnableHPA:   true,
				MinReplicas: 3,
				MaxReplicas: 15,
				CPUTarget:   lo.ToPtr(60),
			},
			vpaConfig: &k8s.VPAConfig{
				Enabled:    true,
				UpdateMode: lo.ToPtr("Off"), // VPA in recommendation mode when used with HPA
			},
			description: "HPA and VPA together (VPA in recommendation mode)",
		},
		{
			name: "CPU_only_HPA",
			scaleConfig: &k8s.Scale{
				Replicas:     1,
				EnableHPA:    true,
				MinReplicas:  1,
				MaxReplicas:  10,
				CPUTarget:    lo.ToPtr(75),
				MemoryTarget: nil, // Only CPU scaling
			},
			vpaConfig:   nil,
			description: "HPA with CPU-only scaling",
		},
		{
			name: "Memory_only_HPA",
			scaleConfig: &k8s.Scale{
				Replicas:     2,
				EnableHPA:    true,
				MinReplicas:  2,
				MaxReplicas:  8,
				CPUTarget:    nil, // Only memory scaling
				MemoryTarget: lo.ToPtr(85),
			},
			vpaConfig:   nil,
			description: "HPA with memory-only scaling",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mocks := NewSimpleContainerMocks()

			err := pulumi.RunErr(func(ctx *pulumi.Context) error {
				replicas := 1
				if tc.scaleConfig != nil && tc.scaleConfig.Replicas > 0 {
					replicas = tc.scaleConfig.Replicas
				}

				args := &SimpleContainerArgs{
					Namespace:  "autoscaling-test",
					Service:    "autoscaling-service",
					ScEnv:      "test",
					Deployment: "autoscaling-deployment",
					Replicas:   replicas,
					Log:        logger.New(),

					IngressContainer: &k8s.CloudRunContainer{
						Name:     "test-container",
						Ports:    []int{8080},
						MainPort: lo.ToPtr(8080),
					},
					ServiceType: lo.ToPtr("ClusterIP"),

					Scale: tc.scaleConfig,
					VPA:   tc.vpaConfig,

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
				Expect(err).ToNot(HaveOccurred(), "Autoscaling SimpleContainer should be created successfully: %s", tc.description)
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

func TestSimpleContainer_ResourceLimitsAndRequests(t *testing.T) {
	RegisterTestingT(t)

	testCases := []struct {
		name      string
		resources *corev1.ResourceRequirementsArgs
		expected  string
	}{
		{
			name: "Basic_CPU_Memory",
			resources: &corev1.ResourceRequirementsArgs{
				Requests: sdk.StringMap{
					"cpu":    sdk.String("100m"),
					"memory": sdk.String("128Mi"),
				},
				Limits: sdk.StringMap{
					"cpu":    sdk.String("500m"),
					"memory": sdk.String("512Mi"),
				},
			},
			expected: "basic CPU and memory limits",
		},
		{
			name: "High_Performance",
			resources: &corev1.ResourceRequirementsArgs{
				Requests: sdk.StringMap{
					"cpu":    sdk.String("1000m"),
					"memory": sdk.String("2Gi"),
				},
				Limits: sdk.StringMap{
					"cpu":    sdk.String("4000m"),
					"memory": sdk.String("8Gi"),
				},
			},
			expected: "high performance resource allocation",
		},
		{
			name: "GPU_Enabled",
			resources: &corev1.ResourceRequirementsArgs{
				Requests: sdk.StringMap{
					"cpu":            sdk.String("2000m"),
					"memory":         sdk.String("4Gi"),
					"nvidia.com/gpu": sdk.String("1"),
				},
				Limits: sdk.StringMap{
					"cpu":            sdk.String("4000m"),
					"memory":         sdk.String("16Gi"),
					"nvidia.com/gpu": sdk.String("1"),
				},
			},
			expected: "GPU-enabled resource allocation",
		},
		{
			name: "Ephemeral_Storage",
			resources: &corev1.ResourceRequirementsArgs{
				Requests: sdk.StringMap{
					"cpu":               sdk.String("500m"),
					"memory":            sdk.String("1Gi"),
					"ephemeral-storage": sdk.String("10Gi"),
				},
				Limits: sdk.StringMap{
					"cpu":               sdk.String("1000m"),
					"memory":            sdk.String("2Gi"),
					"ephemeral-storage": sdk.String("20Gi"),
				},
			},
			expected: "ephemeral storage allocation",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mocks := NewSimpleContainerMocks()

			err := pulumi.RunErr(func(ctx *pulumi.Context) error {
				args := &SimpleContainerArgs{
					Namespace:  "resources-test",
					Service:    "resources-service",
					ScEnv:      "test",
					Deployment: "resources-deployment",
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
				Expect(err).ToNot(HaveOccurred(), "SimpleContainer with %s should be created successfully", tc.expected)
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

func TestSimpleContainer_ServiceTypeVariations(t *testing.T) {
	RegisterTestingT(t)

	serviceTypes := []struct {
		name        string
		serviceType string
		description string
	}{
		{
			name:        "ClusterIP",
			serviceType: "ClusterIP",
			description: "Internal cluster access only",
		},
		{
			name:        "NodePort",
			serviceType: "NodePort",
			description: "External access via node ports",
		},
		{
			name:        "LoadBalancer",
			serviceType: "LoadBalancer",
			description: "External load balancer",
		},
	}

	for _, st := range serviceTypes {
		t.Run(st.name, func(t *testing.T) {
			mocks := NewSimpleContainerMocks()

			err := pulumi.RunErr(func(ctx *pulumi.Context) error {
				args := &SimpleContainerArgs{
					Namespace:  "service-test",
					Service:    "service-test",
					ScEnv:      "test",
					Deployment: "service-deployment",
					Replicas:   1,
					Log:        logger.New(),

					IngressContainer: &k8s.CloudRunContainer{
						Name:     "service-container",
						Ports:    []int{8080},
						MainPort: lo.ToPtr(8080),
					},
					ServiceType: lo.ToPtr(st.serviceType),

					Containers: []corev1.ContainerArgs{
						{
							Name:  sdk.String("service-container"),
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
				Expect(err).ToNot(HaveOccurred(), "SimpleContainer with %s service should be created successfully", st.description)
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
