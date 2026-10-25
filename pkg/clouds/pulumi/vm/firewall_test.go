// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package vm

import (
	"testing"

	. "github.com/onsi/gomega"
	"github.com/samber/lo"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/logger"
	"github.com/simple-container-com/api/pkg/clouds/k3s"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	"github.com/simple-container-com/api/pkg/clouds/vm"
)

// parentStack is a parent stack holding one `vm-compose` template and one
// `k3s-cluster` resource, each pointed at `vmResource`. Pass "" to either to leave
// the reference dangling — which is how the "declared but not onto THIS machine"
// cases below are written.
func parentStack(tplVmResource, k3sVmResource string) api.Stack {
	return api.Stack{
		Name: "test-stack",
		Server: api.ServerDescriptor{
			Templates: map[string]api.StackDescriptor{
				"stack-per-app": {
					Type: vm.TemplateTypeVmCompose,
					Config: api.Config{
						Config: &vm.ComposeTemplateConfig{VmResource: tplVmResource},
					},
				},
				// A template of another type, to make sure the match is on the type
				// and not on "the first template that has a VmResource field".
				"stack-per-app-k8s": {Type: "kubernetes-cloudrun"},
			},
			Resources: api.PerStackResourcesDescriptor{
				Resources: map[string]api.PerEnvResourcesDescriptor{
					"test": {
						Resources: map[string]api.ResourceDescriptor{
							"portal-cluster": {
								Type: k3s.ResourceTypeK3sCluster,
								Name: "portal-cluster",
								Config: api.Config{
									Config: &k3s.K3sClusterConfig{VmResource: k3sVmResource},
								},
							},
						},
					},
					// Another environment's cluster, which must not count: resources
					// are provisioned per environment and a `staging` cluster is not
					// on the `test` machine.
					"staging": {
						Resources: map[string]api.ResourceDescriptor{
							"portal-cluster": {
								Type: k3s.ResourceTypeK3sCluster,
								Name: "portal-cluster",
								Config: api.Config{
									Config: &k3s.K3sClusterConfig{VmResource: "portal-vm"},
								},
							},
						},
					},
				},
			},
		},
	}
}

func firewallPorts(stack api.Stack, cfg *vm.VmConfig) ([]vm.PortRange, error) {
	var ports []vm.PortRange
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		var err error
		ports, err = resolveFirewallPorts(ctx, stack, resourceInput(cfg), cfg,
			pApi.ProvisionParams{Log: logger.New()})
		return err
	}, pulumi.WithMocks("project", "stack", &vmMocks{}))
	return ports, err
}

// The inference itself: a vm resource cannot be asked to know what will be put on
// the machine, but its consumers DECLARE which machine they land on, in the same
// parent stack. Reading that reference is what makes the ports resolvable at
// provision time, which is the whole fix — before it, a smoke reported success and
// the service was reachable only after four rules were added by hand.
func TestResolveFirewallPortsFromDeclaredConsumers(t *testing.T) {
	RegisterTestingT(t)

	cfg := cloudruConfig()
	cfg.Ingress = &vm.VmIngress{Enable: true, Email: "ops@example.com"}

	// Nothing declared onto this machine: the base set only. Note the k3s cluster in
	// `staging` and the template pointing elsewhere are both present in this stack
	// and neither counts.
	ports, err := firewallPorts(parentStack("other-vm", "other-vm"), cfg)
	Expect(err).ToNot(HaveOccurred())
	Expect(labels(ports)).To(Equal([]string{"http", "https", "https-h3", "ssh"}))

	// A `vm-compose` template deploying here: the host-port range opens, and only it.
	ports, err = firewallPorts(parentStack("portal-vm", "other-vm"), cfg)
	Expect(err).ToNot(HaveOccurred())
	Expect(vm.PortsReachable(ports, vm.HostPortRangeStart, vm.ProtocolTcp)).To(BeTrue())
	Expect(vm.PortsReachable(ports, k3s.APIPort, vm.ProtocolTcp)).To(BeFalse(),
		"no k3s cluster is declared on this machine")

	// A `k3s-cluster` resource in THIS environment: the API port and the NodePort
	// range open, and the node-to-node ports open to the cluster rather than to
	// everyone — a second node has to reach them and nobody else may.
	ports, err = firewallPorts(parentStack("other-vm", "portal-vm"), cfg)
	Expect(err).ToNot(HaveOccurred())
	Expect(vm.PortsReachable(ports, k3s.APIPort, vm.ProtocolTcp)).To(BeTrue())
	Expect(vm.PortsReachable(ports, k3s.NodePortRangeStart, vm.ProtocolTcp)).To(BeTrue())
	Expect(vm.PortsReachable(ports, vm.HostPortRangeStart, vm.ProtocolTcp)).To(BeFalse())
	internal := lo.Filter(ports, func(p vm.PortRange, _ int) bool { return p.Internal })
	Expect(labels(internal)).To(Equal([]string{"k3s-etcd", "k3s-flannel-vxlan", "k3s-kubelet"}))

	// Both, which is the live shape: the k3s cluster and the compose stacks share the
	// machine, so the ranges add up rather than one winning.
	ports, err = firewallPorts(parentStack("portal-vm", "portal-vm"), cfg)
	Expect(err).ToNot(HaveOccurred())
	Expect(vm.PortsReachable(ports, k3s.APIPort, vm.ProtocolTcp)).To(BeTrue())
	Expect(vm.PortsReachable(ports, vm.HostPortRangeStart, vm.ProtocolTcp)).To(BeTrue())
	Expect(vm.PortsReachable(ports, 22, vm.ProtocolTcp)).To(BeTrue())

	// `ingressPorts` still adds on top. Inference covers the references SC can read;
	// a port an operator's own process binds on a bare machine is not one of them,
	// and that is exactly what the field is for.
	cfg.IngressPorts = []vm.VmIngressPort{{Port: 9100}}
	ports, err = firewallPorts(parentStack("portal-vm", "portal-vm"), cfg)
	Expect(err).ToNot(HaveOccurred())
	Expect(vm.PortsReachable(ports, 9100, vm.ProtocolTcp)).To(BeTrue())
}

// An empty parent stack must resolve rather than panic. `resolveFirewallPorts` runs
// on every vm provision, including the ones whose parent stack has no templates and
// no sibling resources at all, and a nil map read is the classic way that becomes a
// failed provision instead of an SSH-only machine.
func TestResolveFirewallPortsEmptyParentStack(t *testing.T) {
	RegisterTestingT(t)

	ports, err := firewallPorts(api.Stack{Name: "test-stack"}, cloudruConfig())
	Expect(err).ToNot(HaveOccurred())
	Expect(labels(ports)).To(Equal([]string{"ssh"}))
}

func labels(ports []vm.PortRange) []string {
	return lo.Map(ports, func(p vm.PortRange, _ int) string { return p.Label })
}
