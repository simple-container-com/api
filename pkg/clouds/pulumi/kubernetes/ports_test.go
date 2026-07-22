// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package kubernetes

import (
	"testing"

	. "github.com/onsi/gomega"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/samber/lo"

	"github.com/simple-container-com/api/pkg/api/logger"
	"github.com/simple-container-com/api/pkg/clouds/k8s"
)

func TestK8sProtocol(t *testing.T) {
	RegisterTestingT(t)

	// TCP and unspecified stay empty so the protocol field is omitted (byte-for-byte compat).
	Expect(k8sProtocol("")).To(Equal(""))
	Expect(k8sProtocol("tcp")).To(Equal(""))
	Expect(k8sProtocol("TCP")).To(Equal(""))
	// UDP is emitted explicitly, case-insensitively.
	Expect(k8sProtocol("udp")).To(Equal("UDP"))
	Expect(k8sProtocol("UDP")).To(Equal("UDP"))
}

func TestToContainerPortName(t *testing.T) {
	RegisterTestingT(t)

	// TCP/unspecified keep the historical "http-<port>" name.
	Expect(toContainerPortName(k8s.ContainerPort{Port: 7880})).To(Equal("http-7880"))
	Expect(toContainerPortName(k8s.ContainerPort{Port: 7881, Protocol: "TCP"})).To(Equal("http-7881"))
	// UDP ports use a distinct name so mixed-protocol Services have unique names.
	Expect(toContainerPortName(k8s.ContainerPort{Port: 7882, Protocol: "UDP"})).To(Equal("udp-7882"))
}

// livekitPorts models a LiveKit SFU ingress container: TCP signaling (7880),
// TCP ICE/TURN (7881) and a UDP media port (7882).
func livekitPorts() []k8s.ContainerPort {
	return []k8s.ContainerPort{
		{Port: 7880},
		{Port: 7881},
		{Port: 7882, Protocol: "UDP"},
	}
}

func TestDedupePorts(t *testing.T) {
	RegisterTestingT(t)

	// Duplicate port numbers are dropped; the first occurrence wins.
	out := dedupePorts([]k8s.ContainerPort{
		{Port: 3000},
		{Port: 7880},
		{Port: 7880, Protocol: "UDP"}, // dup port number -> dropped
		{Port: 7882, Protocol: "UDP"},
	})
	Expect(out).To(HaveLen(3))
	Expect(out[0].Port).To(Equal(3000))
	Expect(out[1].Port).To(Equal(7880))
	Expect(out[1].Protocol).To(Equal("")) // first (TCP) kept over the later UDP dup
	Expect(out[2].Port).To(Equal(7882))
}

func TestHasMixedProtocols(t *testing.T) {
	RegisterTestingT(t)

	Expect(hasMixedProtocols(nil)).To(BeFalse())
	Expect(hasMixedProtocols([]k8s.ContainerPort{{Port: 80}, {Port: 443}})).To(BeFalse())
	Expect(hasMixedProtocols([]k8s.ContainerPort{{Port: 7882, Protocol: "UDP"}})).To(BeFalse())
	Expect(hasMixedProtocols([]k8s.ContainerPort{{Port: 7880}, {Port: 7882, Protocol: "UDP"}})).To(BeTrue())
}

func TestToContainerPorts_ThreadsProtocol(t *testing.T) {
	RegisterTestingT(t)

	ports := toContainerPorts(livekitPorts())
	Expect(ports).To(HaveLen(3))

	tcp0 := ports[0].(corev1.ContainerPortArgs)
	Expect(tcp0.ContainerPort).To(Equal(sdk.Int(7880)))
	Expect(tcp0.Name).To(Equal(sdk.String("http-7880")))
	Expect(tcp0.Protocol).To(BeNil(), "TCP ports must omit protocol to stay byte-for-byte compatible")

	tcp1 := ports[1].(corev1.ContainerPortArgs)
	Expect(tcp1.Protocol).To(BeNil())

	udp := ports[2].(corev1.ContainerPortArgs)
	Expect(udp.ContainerPort).To(Equal(sdk.Int(7882)))
	Expect(udp.Name).To(Equal(sdk.String("udp-7882")))
	Expect(udp.Protocol).To(Equal(sdk.String("UDP")))
}

func TestToServicePorts_ThreadsProtocol(t *testing.T) {
	RegisterTestingT(t)

	ports := toServicePorts(livekitPorts())
	Expect(ports).To(HaveLen(3))

	tcp0 := ports[0].(corev1.ServicePortArgs)
	Expect(tcp0.Port).To(Equal(sdk.Int(7880)))
	Expect(tcp0.Name).To(Equal(sdk.String("http-7880")))
	Expect(tcp0.Protocol).To(BeNil())

	tcp1 := ports[1].(corev1.ServicePortArgs)
	Expect(tcp1.Port).To(Equal(sdk.Int(7881)))
	Expect(tcp1.Protocol).To(BeNil())

	udp := ports[2].(corev1.ServicePortArgs)
	Expect(udp.Port).To(Equal(sdk.Int(7882)))
	Expect(udp.Name).To(Equal(sdk.String("udp-7882")))
	Expect(udp.Protocol).To(Equal(sdk.String("UDP")))
}

func TestAutoTCPReadinessProbe(t *testing.T) {
	RegisterTestingT(t)

	t.Run("single TCP port gets a TCP probe", func(t *testing.T) {
		RegisterTestingT(t)
		probe, err := autoTCPReadinessProbe(k8s.CloudRunContainer{
			Name:     "web",
			Ports:    k8s.ContainerPorts(8080),
			MainPort: lo.ToPtr(8080),
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(probe).ToNot(BeNil())
		Expect(probe.TcpSocket).ToNot(BeNil())
		Expect(probe.HttpGet).To(BeNil())
	})

	t.Run("single UDP port gets no probe", func(t *testing.T) {
		RegisterTestingT(t)
		probe, err := autoTCPReadinessProbe(k8s.CloudRunContainer{
			Name:     "media",
			Ports:    []k8s.ContainerPort{{Port: 7882, Protocol: "UDP"}},
			MainPort: lo.ToPtr(7882),
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(probe).To(BeNil(), "a UDP-only port has no HTTP/TCP health surface")
	})

	t.Run("mixed TCP+UDP probes the TCP main port", func(t *testing.T) {
		RegisterTestingT(t)
		probe, err := autoTCPReadinessProbe(k8s.CloudRunContainer{
			Name:     "livekit",
			Ports:    livekitPorts(),
			MainPort: lo.ToPtr(7880),
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(probe).ToNot(BeNil())
		Expect(probe.TcpSocket).ToNot(BeNil())
	})

	t.Run("mixed ports with a UDP main port gets no probe", func(t *testing.T) {
		RegisterTestingT(t)
		probe, err := autoTCPReadinessProbe(k8s.CloudRunContainer{
			Name:     "media-main",
			Ports:    livekitPorts(),
			MainPort: lo.ToPtr(7882),
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(probe).To(BeNil())
	})
}

// TestSimpleContainer_UDPLoadBalancer exercises the full ingress path for a
// LiveKit-style container that exposes both TCP and UDP ports behind a
// LoadBalancer Service (a mixed-protocol Service).
func TestSimpleContainer_UDPLoadBalancer(t *testing.T) {
	RegisterTestingT(t)

	mocks := NewSimpleContainerMocks()

	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		args := &SimpleContainerArgs{
			Namespace:  "livekit-test",
			Service:    "livekit",
			ScEnv:      "test",
			Deployment: "livekit-deployment",
			Replicas:   1,
			Log:        logger.New(),

			IngressContainer: &k8s.CloudRunContainer{
				Name:     "livekit",
				Ports:    livekitPorts(),
				MainPort: lo.ToPtr(7880),
			},
			ServiceType: lo.ToPtr("LoadBalancer"),

			Containers: []corev1.ContainerArgs{
				{
					Name:  sdk.String("livekit"),
					Image: sdk.String("livekit/livekit-server:latest"),
					Ports: toContainerPorts(livekitPorts()),
				},
			},
		}

		sc, err := NewSimpleContainer(ctx, args)
		Expect(err).ToNot(HaveOccurred(), "mixed-protocol LoadBalancer container should be created successfully")
		Expect(sc).ToNot(BeNil())
		Expect(sc.Service).ToNot(BeNil(), "a Service must be provisioned for the exposed ports")
		return nil
	}, pulumi.WithMocks("project", "stack", mocks))

	Expect(err).ToNot(HaveOccurred())
}

func TestSimpleContainer_ExtraServicePorts_NonIngressUDP(t *testing.T) {
	RegisterTestingT(t)

	mocks := NewSimpleContainerMocks()

	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		args := &SimpleContainerArgs{
			Namespace:  "voice-test",
			Service:    "voice",
			ScEnv:      "test",
			Deployment: "voice-deployment",
			Replicas:   1,
			Log:        logger.New(),

			// The HTTP backend is the ingress container (TCP only).
			IngressContainer: &k8s.CloudRunContainer{
				Name:     "backend",
				Ports:    []k8s.ContainerPort{{Port: 3000}},
				MainPort: lo.ToPtr(3000),
			},
			ServiceType: lo.ToPtr("LoadBalancer"),
			// The SFU sidecar's ports (incl. UDP media) come from a non-ingress
			// container and must still land on the Service.
			ExtraServicePorts: livekitPorts(),

			Containers: []corev1.ContainerArgs{
				{
					Name:  sdk.String("backend"),
					Image: sdk.String("backend:latest"),
					Ports: toContainerPorts([]k8s.ContainerPort{{Port: 3000}}),
				},
				{
					Name:  sdk.String("livekit"),
					Image: sdk.String("livekit/livekit-server:latest"),
					Ports: toContainerPorts(livekitPorts()),
				},
			},
		}

		sc, err := NewSimpleContainer(ctx, args)
		Expect(err).ToNot(HaveOccurred(), "a Service mixing the ingress TCP port and a sidecar's UDP port should be created")
		Expect(sc).ToNot(BeNil())
		Expect(sc.Service).ToNot(BeNil(), "the non-ingress UDP port must still produce a Service")
		return nil
	}, pulumi.WithMocks("project", "stack", mocks))

	Expect(err).ToNot(HaveOccurred())
}
