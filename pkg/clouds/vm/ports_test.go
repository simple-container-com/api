// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package vm

import (
	"testing"

	. "github.com/onsi/gomega"
	"github.com/samber/lo"

	"github.com/simple-container-com/api/pkg/clouds/k3s"
)

func labels(ports []PortRange) []string {
	return lo.Map(ports, func(p PortRange, _ int) string { return p.Label })
}

// The base set, which every machine SC creates gets regardless of what is deployed
// onto it. The pairing that matters is 443 on BOTH protocols: Caddy publishes
// `443:443/udp` and advertises HTTP/3 via Alt-Svc, so a browser that takes the
// advertisement while the firewall drops UDP stalls instead of falling back.
func TestFirewallPortsBaseSet(t *testing.T) {
	RegisterTestingT(t)

	bare, err := cloudConfig().FirewallPorts()
	Expect(err).ToNot(HaveOccurred())
	Expect(labels(bare)).To(Equal([]string{PortLabelSsh}),
		"without an ingress the machine answers on SSH and whatever the stacks publish")

	withIngress := cloudConfig()
	withIngress.Ingress = &VmIngress{Enable: true, Email: "ops@example.com"}
	ports, err := withIngress.FirewallPorts()
	Expect(err).ToNot(HaveOccurred())
	Expect(labels(ports)).To(Equal([]string{PortLabelHttp, PortLabelHttps, PortLabelHttpsH3, PortLabelSsh}))
	Expect(lo.Filter(ports, func(p PortRange, _ int) bool { return p.From == 443 })).To(HaveLen(2))
	Expect(lo.SomeBy(ports, func(p PortRange) bool { return p.Protocol == ProtocolUdp && p.From == 443 })).
		To(BeTrue(), "443/udp is HTTP/3 and Caddy publishes it")

	// Sorted by label, and that is load-bearing rather than cosmetic: the label is
	// what the Advanced driver names its Pulumi resources after, and what travels
	// inside the spec the Evolution converge hashes. An unstable order is a diff on
	// every preview and a re-converge on a deploy that changed nothing.
	Expect(labels(ports)).To(BeEquivalentTo([]string{"http", "https", "https-h3", "ssh"}))
}

// The two ranges an operator cannot be expected to know, which is the whole reason
// presets exist rather than a longer hard-coded list.
func TestFirewallPortsPresets(t *testing.T) {
	RegisterTestingT(t)

	cfg := cloudConfig()
	ports, err := cfg.FirewallPorts(IngressPresetVmCompose)
	Expect(err).ToNot(HaveOccurred())
	Expect(PortsReachable(ports, HostPortRangeStart, ProtocolTcp)).To(BeTrue())
	Expect(PortsReachable(ports, HostPortRangeEnd, ProtocolTcp)).To(BeTrue())
	Expect(PortsReachable(ports, HostPortRangeEnd+1, ProtocolTcp)).To(BeFalse())
	// The host port of the one live smoke that needed rules added by hand (29440),
	// and the k3s NodePort from the same smoke (31286) — the first is covered by
	// this preset, the second is not, which is why there are two.
	Expect(PortsReachable(ports, 29440, ProtocolTcp)).To(BeTrue())
	Expect(PortsReachable(ports, 31286, ProtocolTcp)).To(BeFalse())

	ports, err = cfg.FirewallPorts(IngressPresetK3s)
	Expect(err).ToNot(HaveOccurred())
	Expect(PortsReachable(ports, k3s.APIPort, ProtocolTcp)).To(BeTrue())
	Expect(PortsReachable(ports, 31286, ProtocolTcp)).To(BeTrue())

	// The node-to-node half. Open to the other machines of the cluster and NOT to
	// the internet — PortsReachable answers the internet question, which is the one
	// every caller of it is asking.
	byLabel := lo.SliceToMap(ports, func(p PortRange) (string, PortRange) { return p.Label, p })
	Expect(byLabel["k3s-kubelet"].Internal).To(BeTrue())
	Expect(PortsReachable(ports, k3s.KubeletPort, ProtocolTcp)).To(BeFalse(),
		"the kubelet reaches into every pod on the node; a public 10250 is a cluster anyone can exec into")
	Expect(byLabel["k3s-flannel-vxlan"].Internal).To(BeTrue())
	Expect(byLabel["k3s-flannel-vxlan"].Protocol).To(Equal(ProtocolUdp),
		"VXLAN is UDP; a tcp rule here is pods that cannot talk across nodes")
	Expect(byLabel["k3s-etcd"]).To(Equal(PortRange{
		Label:    "k3s-etcd",
		Protocol: ProtocolTcp,
		From:     k3s.EtcdClientPort,
		To:       k3s.EtcdPeerPort,
		Internal: true,
	}))

	// The `http` preset is for a machine that serves the web without SC's Caddy —
	// klipper-lb binding the node's 80/443 behind a closed firewall is a Service
	// with an address and no reachability. It opens 443 on BOTH protocols for the
	// same reason the ingress does.
	ports, err = cfg.FirewallPorts(IngressPresetHttp)
	Expect(err).ToNot(HaveOccurred())
	Expect(PortsReachable(ports, 80, ProtocolTcp)).To(BeTrue())
	Expect(PortsReachable(ports, 443, ProtocolTcp)).To(BeTrue())
	Expect(PortsReachable(ports, 443, ProtocolUdp)).To(BeTrue())

	// And it collapses into the ingress's own rules rather than doubling them: the
	// labels are shared on purpose, because on one cloud the label IS the rule's
	// resource name and two rules for one hole is a provision error.
	withIngress := cloudConfig()
	withIngress.Ingress = &VmIngress{Enable: true, Email: "ops@example.com"}
	bare, err := withIngress.FirewallPorts()
	Expect(err).ToNot(HaveOccurred())
	both, err := withIngress.FirewallPorts(IngressPresetHttp)
	Expect(err).ToNot(HaveOccurred())
	Expect(both).To(Equal(bare))

	// An unknown preset from SC's own inference is a caller bug and says so.
	_, err = cfg.FirewallPorts("kubernetes")
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring(`unknown vm ingress preset "kubernetes"`))
}

func TestFirewallPortsDeclaredEntries(t *testing.T) {
	RegisterTestingT(t)

	cfg := cloudConfig()
	cfg.IngressPorts = []VmIngressPort{
		{Port: 8080},
		{Port: 5000, PortEnd: 5010, Protocol: "UDP"},
		{Preset: IngressPresetK3s},
	}
	ports, err := cfg.FirewallPorts()
	Expect(err).ToNot(HaveOccurred())
	Expect(PortsReachable(ports, 8080, ProtocolTcp)).To(BeTrue())
	Expect(PortsReachable(ports, 8080, ProtocolUdp)).To(BeFalse(), "tcp is the default, not both")
	Expect(PortsReachable(ports, 5005, ProtocolUdp)).To(BeTrue())
	Expect(PortsReachable(ports, k3s.APIPort, ProtocolTcp)).To(BeTrue(),
		"a preset is spellable by hand as well as inferred")

	// Asking for the same hole twice — once inferred, once written out — is one
	// rule. Without this the Advanced driver would try to create two Pulumi
	// resources opening the same range.
	ports, err = cfg.FirewallPorts(IngressPresetK3s)
	Expect(err).ToNot(HaveOccurred())
	Expect(lo.Filter(ports, func(p PortRange, _ int) bool { return p.From == k3s.APIPort })).To(HaveLen(1))

	// And when the two spellings disagree about the scope, the WIDER one wins. An
	// operator who writes `port: 10250` has asked for it publicly; narrowing that
	// silently because a preset also mentions it would be a field accepted and not
	// applied, which is the failure this file exists to prevent.
	cfg = cloudConfig()
	cfg.IngressPorts = []VmIngressPort{{Port: k3s.KubeletPort}}
	ports, err = cfg.FirewallPorts(IngressPresetK3s)
	Expect(err).ToNot(HaveOccurred())
	Expect(PortsReachable(ports, k3s.KubeletPort, ProtocolTcp)).To(BeTrue())
	Expect(lo.Filter(ports, func(p PortRange, _ int) bool { return p.From == k3s.KubeletPort })).To(HaveLen(1))
}

// Shape refusals. Each is a line an operator could plausibly write, and the message
// is the whole value: a port accepted and not opened presents as a green deploy that
// nothing can reach.
func TestIngressPortValidation(t *testing.T) {
	RegisterTestingT(t)

	cases := []struct {
		name  string
		entry VmIngressPort
		// A substring of the refusal, not a regexp: these messages are full of
		// backquoted field names and parentheses, and escaping them for a matcher
		// is how an assertion ends up passing against a message it never read.
		errPart string
	}{
		{name: "single port", entry: VmIngressPort{Port: 8080}},
		{name: "range", entry: VmIngressPort{Port: 8080, PortEnd: 8090}},
		{name: "udp", entry: VmIngressPort{Port: 53, Protocol: "udp"}},
		{name: "preset", entry: VmIngressPort{Preset: IngressPresetK3s}},
		{
			name:    "neither",
			entry:   VmIngressPort{},
			errPart: "requires `port` (or `preset`)",
		},
		{
			name:    "both",
			entry:   VmIngressPort{Preset: IngressPresetK3s, Port: 8080},
			errPart: "give one or the other",
		},
		{
			name:    "preset with protocol",
			entry:   VmIngressPort{Preset: IngressPresetK3s, Protocol: "udp"},
			errPart: "a preset carries its own ports and protocols",
		},
		{
			name:    "unknown preset",
			entry:   VmIngressPort{Preset: "k8s"},
			errPart: "has an unknown `preset: \"k8s\"`",
		},
		{
			name:    "port out of range",
			entry:   VmIngressPort{Port: 70000},
			errPart: "has an invalid `port`: 70000",
		},
		{
			name:    "backwards range",
			entry:   VmIngressPort{Port: 8090, PortEnd: 8080},
			errPart: "has an invalid `portEnd`: 8080",
		},
		{
			name:    "unknown protocol",
			entry:   VmIngressPort{Port: 443, Protocol: "sctp"},
			errPart: "has an invalid `protocol`: \"sctp\"",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			RegisterTestingT(t)
			cfg := cloudConfig()
			cfg.IngressPorts = []VmIngressPort{c.entry}
			err := cfg.Validate()
			if c.errPart == "" {
				Expect(err).ToNot(HaveOccurred())
				return
			}
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(c.errPart))
			Expect(err.Error()).To(ContainSubstring("ingressPorts[0]"), "the refusal names the entry")
		})
	}
}

// The two provider-shaped refusals, which follow the same rule as every other field
// in this package: either the cloud can do it or the config is refused.
func TestIngressPortsFollowTheCapability(t *testing.T) {
	RegisterTestingT(t)

	// byo: SC never touches this machine's firewall, so the list would describe
	// something that did not happen.
	byo := byoConfig()
	byo.IngressPorts = []VmIngressPort{{Port: 8080}}
	err := byo.Validate()
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("ingressPorts"))
	Expect(err.Error()).To(ContainSubstring(DriverByo))

	// The premise of the cloud half, so that a newer measurement filling this in is
	// noticed here rather than becoming a refusal nobody can satisfy.
	Expect(Drivers[DriverCloudru].Capabilities.SecurityGroups).To(BeTrue())

	cloud := cloudConfig()
	cloud.IngressPorts = []VmIngressPort{{Port: 8080}}
	Expect(cloud.Validate()).ToNot(HaveOccurred())
}

// The round trip across the stack boundary. The parent stack exports what it opened
// and the child reads it back to tell its operator that the host port it is about to
// publish is shut — so a lossy render is a warning that never fires, or one that
// fires on a port that is in fact open.
func TestPortSpecRoundTrip(t *testing.T) {
	RegisterTestingT(t)

	cfg := cloudConfig()
	cfg.Ingress = &VmIngress{Enable: true, Email: "ops@example.com"}
	ports, err := cfg.FirewallPorts(IngressPresetK3s, IngressPresetVmCompose)
	Expect(err).ToNot(HaveOccurred())

	spec := FormatPortSpec(ports)
	Expect(spec).To(ContainSubstring("ssh=tcp/22"))
	Expect(spec).To(ContainSubstring("vm-compose-host-ports=tcp/20000-30000"))
	Expect(spec).To(ContainSubstring("https-h3=udp/443"))
	// The scope survives too. Without it a child stack would read a node-to-node
	// hole as internet-reachable and stay quiet about a port nobody can reach.
	Expect(spec).To(ContainSubstring("k3s-kubelet=tcp/10250@nodes"))

	Expect(ParsePortSpec(spec)).To(Equal(ports))
	Expect(FormatPortSpec(ParsePortSpec(spec))).To(Equal(spec))

	// Empty is empty, not one malformed entry: an older parent stack exports
	// nothing here, and the caller distinguishes "I cannot see the firewall" from
	// "the port is shut" by the length of this slice.
	Expect(ParsePortSpec("")).To(BeEmpty())

	// Junk is skipped, never fatal. This is read on the deploy path to produce a
	// warning, and a parse error must not be the thing that fails a deploy.
	Expect(ParsePortSpec("garbage,ssh=tcp/22,also=junk,http=tcp/")).To(Equal([]PortRange{
		{Label: PortLabelSsh, Protocol: ProtocolTcp, From: 22, To: 22},
	}))
}
