// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package vm

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/samber/lo"

	"github.com/simple-container-com/api/pkg/clouds/k3s"
)

// The ports SC opens on every machine it created, regardless of what is deployed
// onto it.
//
// 22 always — it is the entire delivery path. 80 and 443 only when the machine runs
// the ingress: a host serving stacks on a published port has no business answering
// on 443, and opening it anyway is what makes a later "why is this reachable" audit
// expensive.
//
// 443/udp is HTTP/3, and it is opened because the Caddy compose project publishes it
// (caddy.go — `443:443/udp` alongside the TCP one). A container bound to UDP 443
// behind a firewall that drops it is worse than not serving h3 at all: Caddy still
// advertises Alt-Svc, so every browser spends a connection attempt on a port that
// black-holes before falling back to TCP, and Chrome remembers the broken alt-svc.
const (
	PortLabelSsh      = "ssh"
	PortLabelHttp     = "http"
	PortLabelHttps    = "https"
	PortLabelHttpsH3  = "https-h3"
	ProtocolTcp       = "tcp"
	ProtocolUdp       = "udp"
	maxPort           = 65535
	declaredPortLabel = "declared"
	// internalSpecSuffix marks a node-to-node hole in a rendered port spec, so a
	// reader of the stack output can tell "open" from "open to the cluster".
	internalSpecSuffix = "@nodes"
)

// The presets a vm resource's `ingressPorts` may name instead of spelling out port
// numbers that belong to one of SC's own delivery paths rather than to the
// operator's service.
//
// A preset exists for exactly the ranges an operator cannot reasonably be expected
// to know: that k3s allocates NodePorts from 30000-32767, and that `vm-compose`
// derives a stack's host port by hashing its name into HostPortRangeStart..End. The
// roadmap item this closes (sc-cloudru-vm-no-ingress-ports) was filed because a
// smoke needed both of those ranges opened by hand to be reachable at all.
const (
	// IngressPresetK3s is the k3s API port plus the NodePort range.
	IngressPresetK3s = "k3s"
	// IngressPresetVmCompose is the host-port range `vm-compose` publishes into.
	IngressPresetVmCompose = "vm-compose"
	// IngressPresetHttp is 80 and 443 (on both protocols) — what `ingress.enable`
	// opens, for a machine that serves the web WITHOUT SC's own Caddy in front.
	//
	// The live case it exists for: a k3s stand with `ingress` left off so that k3s
	// keeps 80/443, publishing a Service as `cloudExtras.serviceType: LoadBalancer`
	// through klipper-lb. Kubernetes binds the node's ports and reports success, and
	// the cloud firewall drops the traffic — a Service with an address that nothing
	// can reach.
	IngressPresetHttp = "http"
)

// PortRange is one hole in a machine's firewall: a closed port range on one
// protocol, carrying the label SC names the rule after.
//
// The label is part of the contract, not decoration: on Cloud.ru Advanced it
// becomes the Pulumi resource name of the rule (`<vm>-secgroup-<label>`, so renaming
// one replaces a rule), and on Cloud.ru Evolution it becomes the rule's description,
// which is the ONLY thing a destroy can use to tell SC's rules apart from the ones
// the project's shared default group came with.
type PortRange struct {
	Label    string
	Protocol string
	From     int
	To       int
	// Internal restricts the hole to the other machines of this vm resource rather
	// than opening it to the internet.
	//
	// It exists because a multi-node k3s cluster needs ports that must NOT be
	// public: the kubelet (10250) serves `exec` into any pod on the node, and
	// embedded etcd (2379-2380) is the cluster's entire state. Opening those to
	// 0.0.0.0/0 is a cluster anyone can take over, so the choice is per-range and
	// not per-resource.
	//
	// How it is enforced is the driver's business, because the clouds differ: on
	// Cloud.ru Advanced the rule's source is the machine's own security group, and
	// on Evolution — where SC does not get to choose the group and every VM of the
	// project joins the default one — it is the private subnet's CIDR.
	Internal bool
}

// Covers reports whether this range permits a port on a protocol. Used to tell an
// operator that the host port their stack is about to publish is not open, rather
// than letting them discover it as a timeout against a green deploy.
func (p PortRange) Covers(port int, protocol string) bool {
	return p.Protocol == strings.ToLower(protocol) && port >= p.From && port <= p.To
}

// Spec renders the range as `proto/from` or `proto/from-to`.
func (p PortRange) Spec() string {
	if p.From == p.To {
		return fmt.Sprintf("%s/%d", p.Protocol, p.From)
	}
	return fmt.Sprintf("%s/%d-%d", p.Protocol, p.From, p.To)
}

func (p PortRange) String() string {
	if p.Internal {
		return fmt.Sprintf("%s=%s%s", p.Label, p.Spec(), internalSpecSuffix)
	}
	return fmt.Sprintf("%s=%s", p.Label, p.Spec())
}

// key identifies the hole itself, with the label and the scope left out — two
// entries that open the same ports on the same protocol are one rule however they
// were spelled. Scope is deliberately not part of the identity: see dedupePorts.
func (p PortRange) key() string {
	return p.Spec()
}

// VmIngressPort is one entry of a vm resource's `ingressPorts`: either a preset, or
// a port (optionally a range).
//
// It exists because the `vm` resource genuinely cannot know what will be put on the
// machine. SC infers the presets of the consumers it can see in the same parent
// stack (a `vm-compose` template or a `k3s-cluster` resource naming this vm), and
// this list is how an operator states the rest — the port their own service binds,
// a second service, a UDP listener — without that inference having to be clairvoyant.
type VmIngressPort struct {
	// Preset names a bundle of ranges (see IngressPresetK3s). Mutually exclusive
	// with Port.
	Preset string `json:"preset,omitempty" yaml:"preset,omitempty"`
	// Port is the first (or only) port of the range.
	Port int `json:"port,omitempty" yaml:"port,omitempty"`
	// PortEnd closes an inclusive range that starts at Port. Omit for a single port.
	PortEnd int `json:"portEnd,omitempty" yaml:"portEnd,omitempty"`
	// Protocol is `tcp` (the default) or `udp`.
	Protocol string `json:"protocol,omitempty" yaml:"protocol,omitempty"`
}

func (p VmIngressPort) protocol() string {
	return lo.If(p.Protocol == "", ProtocolTcp).Else(strings.ToLower(p.Protocol))
}

func (p VmIngressPort) portEnd() int {
	return lo.If(p.PortEnd == 0, p.Port).Else(p.PortEnd)
}

// expand turns one entry into the ranges it stands for.
func (p VmIngressPort) expand() []PortRange {
	if p.Preset != "" {
		return presetPorts(p.Preset)
	}
	proto := p.protocol()
	end := p.portEnd()
	label := fmt.Sprintf("%s-%s-%d", declaredPortLabel, proto, p.Port)
	if end != p.Port {
		label = fmt.Sprintf("%s-%s-%d-%d", declaredPortLabel, proto, p.Port, end)
	}
	return []PortRange{{Label: label, Protocol: proto, From: p.Port, To: end}}
}

// validate holds the shape rules. Refusing here is the point: a port accepted and
// not opened is the failure this whole file exists to prevent, and it presents as a
// green deploy nobody can reach.
func (p VmIngressPort) validate(idx int) error {
	at := fmt.Sprintf("ingressPorts[%d]", idx)
	switch {
	case p.Preset != "" && p.Port != 0:
		return errors.Errorf("%s sets both `preset: %s` and `port: %d`: give one or the other",
			at, p.Preset, p.Port)
	case p.Preset != "":
		if p.PortEnd != 0 || p.Protocol != "" {
			return errors.Errorf("%s sets `preset: %s`, so `portEnd`/`protocol` have nothing to apply to; "+
				"a preset carries its own ports and protocols", at, p.Preset)
		}
		if len(presetPorts(p.Preset)) == 0 {
			return errors.Errorf("%s has an unknown `preset: %q`; one of: %s",
				at, p.Preset, strings.Join(KnownIngressPresets(), ", "))
		}
		return nil
	case p.Port == 0:
		return errors.Errorf("%s requires `port` (or `preset`); one of: %s", at, strings.Join(KnownIngressPresets(), ", "))
	}
	if p.Port < 1 || p.Port > maxPort {
		return errors.Errorf("%s has an invalid `port`: %d", at, p.Port)
	}
	if p.PortEnd != 0 && (p.PortEnd < p.Port || p.PortEnd > maxPort) {
		return errors.Errorf("%s has an invalid `portEnd`: %d (must be between `port` (%d) and %d)",
			at, p.PortEnd, p.Port, maxPort)
	}
	if proto := p.protocol(); proto != ProtocolTcp && proto != ProtocolUdp {
		return errors.Errorf("%s has an invalid `protocol`: %q (expected %q or %q)",
			at, p.Protocol, ProtocolTcp, ProtocolUdp)
	}
	return nil
}

// presetPorts resolves a preset name, or nil when it is not one.
func presetPorts(name string) []PortRange {
	switch name {
	case IngressPresetK3s:
		return []PortRange{
			// Public, and both for the same reason: something outside the cluster
			// talks to them. 6443 is what `kubectl` and every SC deploy connect to
			// (and what an agent node joining from another cloud would use), and a
			// NodePort exists precisely to be reached from off-cluster.
			{Label: "k3s-api", Protocol: ProtocolTcp, From: k3s.APIPort, To: k3s.APIPort},
			{
				Label:    "k3s-nodeport",
				Protocol: ProtocolTcp,
				From:     k3s.NodePortRangeStart,
				To:       k3s.NodePortRangeEnd,
			},
			// Node-to-node, and never public. A single-node cluster does not need
			// any of the three; they are in the preset anyway because the failure
			// they prevent is invisible in a one-node smoke and brutal afterwards —
			// a second node joins, `kubectl get nodes` says Ready, and then pods on
			// different nodes cannot reach each other (VXLAN), `kubectl logs`/`exec`
			// against half the cluster hangs (kubelet), and an HA control plane
			// never forms a quorum (etcd).
			{
				Label:    "k3s-flannel-vxlan",
				Protocol: ProtocolUdp,
				From:     k3s.FlannelVxlanPort,
				To:       k3s.FlannelVxlanPort,
				Internal: true,
			},
			{
				Label:    "k3s-kubelet",
				Protocol: ProtocolTcp,
				From:     k3s.KubeletPort,
				To:       k3s.KubeletPort,
				Internal: true,
			},
			{
				Label:    "k3s-etcd",
				Protocol: ProtocolTcp,
				From:     k3s.EtcdClientPort,
				To:       k3s.EtcdPeerPort,
				Internal: true,
			},
		}
	case IngressPresetVmCompose:
		return []PortRange{{
			Label:    "vm-compose-host-ports",
			Protocol: ProtocolTcp,
			From:     HostPortRangeStart,
			To:       HostPortRangeEnd,
		}}
	case IngressPresetHttp:
		// The same three ranges `ingress.enable` opens, deliberately under the same
		// labels: the two collapse into one rule each when both apply, and the rule
		// names stay what they were.
		return []PortRange{
			{Label: PortLabelHttp, Protocol: ProtocolTcp, From: 80, To: 80},
			{Label: PortLabelHttps, Protocol: ProtocolTcp, From: 443, To: 443},
			{Label: PortLabelHttpsH3, Protocol: ProtocolUdp, From: 443, To: 443},
		}
	}
	return nil
}

// KnownIngressPresets names every accepted preset, so a refusal can list them.
func KnownIngressPresets() []string {
	return []string{IngressPresetK3s, IngressPresetVmCompose, IngressPresetHttp}
}

// FirewallPorts is every hole SC opens on a machine it created: the base ports
// above, the resource's own `ingressPorts`, and the presets the caller inferred from
// this vm's consumers in the same parent stack.
//
// Deduplicated by the hole rather than by the label, so a preset inferred from a
// `k3s-cluster` resource and the same preset written out by hand are one rule. The
// result is sorted by label because the label is what the Advanced driver names its
// Pulumi resources after, and an unstable order there is a diff on every preview.
func (r *VmConfig) FirewallPorts(inferredPresets ...string) ([]PortRange, error) {
	ports := []PortRange{{Label: PortLabelSsh, Protocol: ProtocolTcp, From: DefaultSshPort, To: DefaultSshPort}}
	if r.IngressEnabled() {
		ports = append(ports,
			PortRange{Label: PortLabelHttp, Protocol: ProtocolTcp, From: 80, To: 80},
			PortRange{Label: PortLabelHttps, Protocol: ProtocolTcp, From: 443, To: 443},
			PortRange{Label: PortLabelHttpsH3, Protocol: ProtocolUdp, From: 443, To: 443},
		)
	}
	for _, preset := range inferredPresets {
		resolved := presetPorts(preset)
		if len(resolved) == 0 {
			// A caller bug, not an operator one: the inferred presets come from SC's
			// own code reading the parent stack.
			return nil, errors.Errorf("unknown vm ingress preset %q", preset)
		}
		ports = append(ports, resolved...)
	}
	for i, p := range r.IngressPorts {
		if err := p.validate(i); err != nil {
			return nil, err
		}
		ports = append(ports, p.expand()...)
	}
	return dedupePorts(ports), nil
}

// dedupePorts collapses duplicate holes, keeps the first label seen for each, and
// sorts by label.
//
// When the same hole is asked for twice with different scopes, the WIDER scope wins:
// an operator who writes `port: 10250` in `ingressPorts` has asked for it publicly,
// and silently narrowing that to node-to-node because a preset also mentions it
// would be a field accepted and not applied. The refusal this file exists to make
// loud cannot be replaced by a quiet downgrade — if the public hole is a mistake,
// removing the line is the operator's to make.
func dedupePorts(in []PortRange) []PortRange {
	seen := make(map[string]int, len(in))
	out := make([]PortRange, 0, len(in))
	for _, p := range in {
		if at, found := seen[p.key()]; found {
			if !p.Internal {
				out[at].Internal = false
			}
			continue
		}
		seen[p.key()] = len(out)
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Label < out[j].Label })
	return out
}

// FormatPortSpec renders a port list for a stack output, as
// `http=tcp/80,ssh=tcp/22,vm-compose-host-ports=tcp/20000-30000`.
//
// It crosses the stack boundary because the template that publishes a host port
// runs in the CHILD stack and the firewall is described in the parent — so the only
// way for a deploy to say "the port you are publishing is not open" is for the
// parent to export what it opened. A string rather than a structured output because
// a Pulumi stack output of a struct slice round-trips as `[]any` of `map[string]any`
// and every reader would re-parse it anyway.
func FormatPortSpec(ports []PortRange) string {
	return strings.Join(lo.Map(ports, func(p PortRange, _ int) string { return p.String() }), ",")
}

// ParsePortSpec reads back what FormatPortSpec wrote. An entry it cannot parse is
// skipped rather than refused: this is read on the deploy path to produce a warning,
// and a parse error must not be the thing that fails a deploy.
func ParsePortSpec(spec string) []PortRange {
	out := make([]PortRange, 0)
	for _, entry := range strings.Split(spec, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		label, rest, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		proto, portRange, ok := strings.Cut(rest, "/")
		if !ok {
			continue
		}
		internal := strings.HasSuffix(portRange, internalSpecSuffix)
		portRange = strings.TrimSuffix(portRange, internalSpecSuffix)
		fromRaw, toRaw, isRange := strings.Cut(portRange, "-")
		from, err := strconv.Atoi(fromRaw)
		if err != nil {
			continue
		}
		to := from
		if isRange {
			if to, err = strconv.Atoi(toRaw); err != nil {
				continue
			}
		}
		out = append(out, PortRange{Label: label, Protocol: proto, From: from, To: to, Internal: internal})
	}
	return out
}

// PortsReachable reports whether any range makes a port reachable FROM THE
// INTERNET. Node-to-node ranges do not count: the question every caller asks is
// whether a client outside the cloud can connect, and answering "yes, from the
// other nodes" would silence exactly the warning this is called to produce.
func PortsReachable(ports []PortRange, port int, protocol string) bool {
	return lo.SomeBy(ports, func(p PortRange) bool { return !p.Internal && p.Covers(port, protocol) })
}
