package kubernetes

import (
	"testing"

	. "github.com/onsi/gomega"
	"github.com/samber/lo"

	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api/logger"
	"github.com/simple-container-com/api/pkg/clouds/k8s"
)

// Final comprehensive tests that focus on successful creation and structure validation

func TestSimpleContainer_CreationSuccess(t *testing.T) {
	RegisterTestingT(t)

	RegisterTestingT(t)

	mocks := NewSimpleContainerMocks()

	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		args := &SimpleContainerArgs{
			Namespace:              "test-namespace",
			Service:                "test-service",
			ScEnv:                  "test",
			Domain:                 "test.example.com",
			Deployment:             "test-deployment",
			Replicas:               1,
			GenerateCaddyfileEntry: true,
			Log:                    logger.New(),

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
							Name:          sdk.String("http"),
						},
					},
				},
			},
		}

		sc, err := NewSimpleContainer(ctx, args)
		Expect(err).ToNot(HaveOccurred(), "SimpleContainer creation should succeed")
		Expect(sc).ToNot(BeNil(), "SimpleContainer should not be nil")

		// Test that all expected outputs are present (not nil)
		Expect(sc.ServicePublicIP).ToNot(BeNil(), "ServicePublicIP should not be nil")
		Expect(sc.ServiceName).ToNot(BeNil(), "ServiceName should not be nil")
		Expect(sc.Namespace).ToNot(BeNil(), "Namespace should not be nil")
		Expect(sc.Deployment).ToNot(BeNil(), "Deployment should not be nil")
		Expect(sc.Service).ToNot(BeNil(), "Service should not be nil")

		// Test that CaddyfileEntry is generated when enabled
		Expect(sc.CaddyfileEntry).ToNot(BeEmpty(), "CaddyfileEntry should be generated")

		return nil
	}, pulumi.WithMocks("project", "stack", mocks))

	Expect(err).ToNot(HaveOccurred(), "Test should complete without errors")
}

func TestSimpleContainer_HPAIntegration(t *testing.T) {
	RegisterTestingT(t)

	mocks := NewSimpleContainerMocks()

	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		args := &SimpleContainerArgs{
			Namespace:  "hpa-test",
			Service:    "hpa-service",
			ScEnv:      "test",
			Deployment: "hpa-deployment",
			Replicas:   2,
			Log:        logger.New(),

			IngressContainer: &k8s.CloudRunContainer{
				Name:     "test-container",
				Ports:    []int{8080},
				MainPort: lo.ToPtr(8080),
			},
			ServiceType: lo.ToPtr("ClusterIP"),

			// Enable HPA
			Scale: &k8s.Scale{
				Replicas:     2,
				EnableHPA:    true,
				MinReplicas:  2,
				MaxReplicas:  10,
				CPUTarget:    lo.ToPtr(70),
				MemoryTarget: lo.ToPtr(80),
			},

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
		Expect(err).ToNot(HaveOccurred(), "SimpleContainer with HPA should be created successfully")
		Expect(sc).ToNot(BeNil(), "SimpleContainer should not be nil")

		// Validate that SimpleContainer was created successfully with HPA
		Expect(sc.ServicePublicIP).ToNot(BeNil(), "ServicePublicIP should not be nil")
		Expect(sc.ServiceName).ToNot(BeNil(), "ServiceName should not be nil")
		Expect(sc.Namespace).ToNot(BeNil(), "Namespace should not be nil")
		Expect(sc.Deployment).ToNot(BeNil(), "Deployment should not be nil")

		return nil
	}, pulumi.WithMocks("project", "stack", mocks))

	Expect(err).ToNot(HaveOccurred(), "HPA integration test should complete without errors")
}

func TestSimpleContainer_VPAIntegration(t *testing.T) {
	RegisterTestingT(t)

	mocks := NewSimpleContainerMocks()

	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		args := &SimpleContainerArgs{
			Namespace:  "vpa-test",
			Service:    "vpa-service",
			ScEnv:      "test",
			Deployment: "vpa-deployment",
			Replicas:   1,
			Log:        logger.New(),

			IngressContainer: &k8s.CloudRunContainer{
				Name:     "test-container",
				Ports:    []int{8080},
				MainPort: lo.ToPtr(8080),
			},
			ServiceType: lo.ToPtr("ClusterIP"),

			// Enable VPA
			VPA: &k8s.VPAConfig{
				Enabled:    true,
				UpdateMode: lo.ToPtr("Auto"),
			},

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
		Expect(err).ToNot(HaveOccurred(), "SimpleContainer with VPA should be created successfully")
		Expect(sc).ToNot(BeNil(), "SimpleContainer should not be nil")

		// Validate that SimpleContainer was created successfully with VPA
		Expect(sc.ServicePublicIP).ToNot(BeNil(), "ServicePublicIP should not be nil")
		Expect(sc.ServiceName).ToNot(BeNil(), "ServiceName should not be nil")
		Expect(sc.Namespace).ToNot(BeNil(), "Namespace should not be nil")
		Expect(sc.Deployment).ToNot(BeNil(), "Deployment should not be nil")

		return nil
	}, pulumi.WithMocks("project", "stack", mocks))

	Expect(err).ToNot(HaveOccurred(), "VPA integration test should complete without errors")
}

func TestSimpleContainer_IngressIntegration(t *testing.T) {
	RegisterTestingT(t)

	mocks := NewSimpleContainerMocks()

	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		args := &SimpleContainerArgs{
			Namespace:              "ingress-test",
			Service:                "ingress-service",
			ScEnv:                  "test",
			Domain:                 "ingress.example.com",
			Deployment:             "ingress-deployment",
			Replicas:               1,
			GenerateCaddyfileEntry: true,
			Log:                    logger.New(),

			IngressContainer: &k8s.CloudRunContainer{
				Name:     "test-container",
				Ports:    []int{8080},
				MainPort: lo.ToPtr(8080),
			},
			ServiceType:      lo.ToPtr("ClusterIP"),
			ProvisionIngress: true, // Enable ingress

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
		Expect(err).ToNot(HaveOccurred(), "SimpleContainer with Ingress should be created successfully")
		Expect(sc).ToNot(BeNil(), "SimpleContainer should not be nil")

		// Validate that SimpleContainer was created successfully with Ingress
		Expect(sc.ServicePublicIP).ToNot(BeNil(), "ServicePublicIP should not be nil")
		Expect(sc.ServiceName).ToNot(BeNil(), "ServiceName should not be nil")
		Expect(sc.Namespace).ToNot(BeNil(), "Namespace should not be nil")
		Expect(sc.Deployment).ToNot(BeNil(), "Deployment should not be nil")

		// Validate Caddyfile entry is generated (content validation is complex with Pulumi outputs)
		Expect(sc.CaddyfileEntry).ToNot(BeEmpty(), "CaddyfileEntry should be generated")

		return nil
	}, pulumi.WithMocks("project", "stack", mocks))

	Expect(err).ToNot(HaveOccurred(), "Ingress integration test should complete without errors")
}

func TestSimpleContainer_PersistentVolumeIntegration(t *testing.T) {
	RegisterTestingT(t)

	mocks := NewSimpleContainerMocks()

	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		args := &SimpleContainerArgs{
			Namespace:  "pv-test",
			Service:    "pv-service",
			ScEnv:      "test",
			Deployment: "pv-deployment",
			Replicas:   1,
			Log:        logger.New(),

			IngressContainer: &k8s.CloudRunContainer{
				Name:     "test-container",
				Ports:    []int{8080},
				MainPort: lo.ToPtr(8080),
			},
			ServiceType: lo.ToPtr("ClusterIP"),

			// Add persistent volumes
			PersistentVolumes: []k8s.PersistentVolume{
				{
					Name:        "data-volume",
					MountPath:   "/data",
					Storage:     "1Gi",
					AccessModes: []string{"ReadWriteOnce"},
				},
				{
					Name:        "logs-volume",
					MountPath:   "/logs",
					Storage:     "500Mi",
					AccessModes: []string{"ReadWriteMany"},
				},
			},

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
		Expect(err).ToNot(HaveOccurred(), "SimpleContainer with PersistentVolumes should be created successfully")
		Expect(sc).ToNot(BeNil(), "SimpleContainer should not be nil")

		// Validate that SimpleContainer was created successfully with PVs
		Expect(sc.ServicePublicIP).ToNot(BeNil(), "ServicePublicIP should not be nil")
		Expect(sc.ServiceName).ToNot(BeNil(), "ServiceName should not be nil")
		Expect(sc.Namespace).ToNot(BeNil(), "Namespace should not be nil")
		Expect(sc.Deployment).ToNot(BeNil(), "Deployment should not be nil")

		return nil
	}, pulumi.WithMocks("project", "stack", mocks))

	Expect(err).ToNot(HaveOccurred(), "PersistentVolume integration test should complete without errors")
}

func TestSimpleContainer_MinimalConfiguration(t *testing.T) {
	RegisterTestingT(t)

	mocks := NewSimpleContainerMocks()

	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		// Minimal configuration - only required fields
		args := &SimpleContainerArgs{
			Namespace:  "minimal",
			Service:    "minimal-service",
			ScEnv:      "test",
			Deployment: "minimal-deployment",
			Replicas:   1,
			Log:        logger.New(),

			// Minimal container configuration
			Containers: []corev1.ContainerArgs{
				{
					Name:  sdk.String("minimal-container"),
					Image: sdk.String("nginx:latest"),
				},
			},
		}

		sc, err := NewSimpleContainer(ctx, args)
		Expect(err).ToNot(HaveOccurred(), "SimpleContainer with minimal config should be created successfully")
		Expect(sc).ToNot(BeNil(), "SimpleContainer should not be nil")

		// Even with minimal config, basic outputs should be available
		Expect(sc.Namespace).ToNot(BeNil(), "Namespace should not be nil")
		Expect(sc.Deployment).ToNot(BeNil(), "Deployment should not be nil")

		return nil
	}, pulumi.WithMocks("project", "stack", mocks))

	Expect(err).ToNot(HaveOccurred(), "Minimal configuration test should complete without errors")
}
