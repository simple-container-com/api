// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package kubernetes

import (
	"fmt"
	"strings"

	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/clouds/k8s"
)

func isUDP(proto string) bool {
	return strings.EqualFold(proto, "UDP")
}

// dedupePorts removes duplicate ports by port number, keeping the first
// occurrence (callers pass ingress ports first so they win over ports
// contributed by sibling containers).
func dedupePorts(ports []k8s.ContainerPort) []k8s.ContainerPort {
	seen := make(map[int]bool, len(ports))
	result := make([]k8s.ContainerPort, 0, len(ports))
	for _, p := range ports {
		if seen[p.Port] {
			continue
		}
		seen[p.Port] = true
		result = append(result, p)
	}
	return result
}

// hasMixedProtocols reports whether the port set contains both a TCP (default)
// and a UDP port, i.e. it would produce a mixed-protocol Service.
func hasMixedProtocols(ports []k8s.ContainerPort) bool {
	var tcp, udp bool
	for _, p := range ports {
		if isUDP(p.Protocol) {
			udp = true
		} else {
			tcp = true
		}
	}
	return tcp && udp
}

// portProtocol returns the protocol declared for the given port number on the
// container, or "" (TCP) when the port is not declared.
func portProtocol(container k8s.CloudRunContainer, port int) string {
	for _, p := range container.Ports {
		if p.Port == port {
			return p.Protocol
		}
	}
	return ""
}

// k8sProtocol returns the protocol value to emit on a generated Kubernetes port.
// TCP is the Kubernetes default, so it is returned as an empty string (protocol
// omitted) to keep TCP-only specs byte-for-byte identical to those generated
// before UDP support. Only non-default protocols (UDP) are emitted explicitly.
func k8sProtocol(proto string) string {
	if isUDP(proto) {
		return "UDP"
	}
	return ""
}

// toPortName derives the Kubernetes port name for a container port. TCP and
// unspecified ports keep the historical "http-<port>" name so existing specs do
// not change; UDP ports use a "udp-<port>" name so mixed-protocol Services have
// unique, meaningful port names.
func toContainerPortName(p k8s.ContainerPort) string {
	if isUDP(p.Protocol) {
		return fmt.Sprintf("udp-%d", p.Port)
	}
	return toPortName(p.Port)
}

// toContainerPorts converts the internal container-port model into Kubernetes
// container ports, carrying the protocol through for non-TCP ports.
func toContainerPorts(ports []k8s.ContainerPort) corev1.ContainerPortArray {
	var result corev1.ContainerPortArray
	for _, p := range ports {
		args := corev1.ContainerPortArgs{
			Name:          sdk.String(toContainerPortName(p)),
			ContainerPort: sdk.Int(p.Port),
		}
		if proto := k8sProtocol(p.Protocol); proto != "" {
			args.Protocol = sdk.String(proto)
		}
		result = append(result, args)
	}
	return result
}

// toServicePorts converts the internal container-port model into Kubernetes
// Service ports. A Service that carries both TCP and UDP ports is a
// mixed-protocol Service (GA since Kubernetes 1.26 via the
// MixedProtocolLBService feature) and requires a cluster/cloud LoadBalancer that
// supports it (recent GKE does).
func toServicePorts(ports []k8s.ContainerPort) corev1.ServicePortArray {
	result := corev1.ServicePortArray{}
	for _, p := range ports {
		args := corev1.ServicePortArgs{
			Name: sdk.String(toContainerPortName(p)),
			Port: sdk.Int(p.Port),
		}
		if proto := k8sProtocol(p.Protocol); proto != "" {
			args.Protocol = sdk.String(proto)
		}
		result = append(result, args)
	}
	return result
}
