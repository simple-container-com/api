// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package vm

import (
	"encoding/base64"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
)

// meshConfig is a two-machine byo fleet with the overlay on, which is the smallest
// shape the mesh is legal in.
func meshConfig() *VmConfig {
	return &VmConfig{
		Driver: DriverByo,
		Hosts: []VmHost{
			{Address: "203.0.113.10", User: "ubuntu", PrivateKey: "k"},
			{Address: "203.0.113.11", User: "ubuntu", PrivateKey: "k"},
		},
		Mesh: &VmMesh{Enable: true},
	}
}

// fakeMeshKey is a value of the right SHAPE — 32 raw bytes in standard base64 — so
// that validateMeshSpec's decode passes. Not a real WireGuard key and not used as one.
func fakeMeshKey(fill byte) string {
	raw := make([]byte, 32)
	for i := range raw {
		raw[i] = fill
	}
	return base64.StdEncoding.EncodeToString(raw)
}

// Address assignment is the property the whole store slice leans on: because machine
// i's address is a pure function of (cidr, i), the certificate store's endpoint can be
// a constant in a rendered Caddyfile instead of a Pulumi Output threaded between
// hosts. So the three things asserted here are determinism, index stability, and that
// the range's capacity is enforced rather than wrapped around.
func TestMeshAddressIn(t *testing.T) {
	RegisterTestingT(t)

	cases := []struct {
		cidr string
		idx  int
		want string
	}{
		// Host addresses start at .1 — the network address itself is not usable.
		{"10.88.0.0/24", 0, "10.88.0.1"},
		{"10.88.0.0/24", 1, "10.88.0.2"},
		{"10.88.0.0/24", 253, "10.88.0.254"},
		{"10.88.0.0/16", 255, "10.88.1.0"},
		{"192.168.40.0/28", 0, "192.168.40.1"},
	}
	for _, c := range cases {
		got, err := MeshAddressIn(c.cidr, c.idx)
		Expect(err).ToNot(HaveOccurred(), "%s[%d]", c.cidr, c.idx)
		Expect(got).To(Equal(c.want), "%s[%d]", c.cidr, c.idx)

		// Determinism, said out loud: a second call must answer the same thing, because
		// a provision that re-addressed a machine would flap the mesh carrying the store.
		again, err := MeshAddressIn(c.cidr, c.idx)
		Expect(err).ToNot(HaveOccurred())
		Expect(again).To(Equal(got))
	}

	// Index stability across a growing fleet. Raising the node count must not move an
	// existing machine — the certificate store lives on index 0 and every other node's
	// Caddyfile names its address.
	for _, count := range []int{2, 3, 7, 50} {
		first, err := MeshAddressIn(DefaultMeshCidr, 0)
		Expect(err).ToNot(HaveOccurred())
		Expect(first).To(Equal("10.88.0.1"), "host 0 must keep its address in a %d-machine fleet", count)
		last, err := MeshAddressIn(DefaultMeshCidr, count-1)
		Expect(err).ToNot(HaveOccurred())
		Expect(last).ToNot(Equal(first))
	}

	// A /28 holds 14 machines. The 15th is refused with the capacity named, and NOT
	// wrapped into the next range — which is what an unchecked add would do, silently
	// handing two fleets the same addresses.
	_, err := MeshAddressIn("192.168.40.0/28", 14)
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("holds 14 machine(s)"))
	Expect(err.Error()).To(ContainSubstring("widen the prefix"))

	_, err = MeshAddressIn("10.88.0.0/24", -1)
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("must not be negative"))

	_, err = MeshAddressIn("not-a-cidr", 0)
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("invalid `mesh.cidr`"))

	// IPv6 is refused with the reason, rather than producing a mesh a node with no
	// IPv6 route cannot join.
	_, err = MeshAddressIn("fd00::/64", 0)
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("IPv4-only"))
}

// The config-tier refusals. Each one is a thing an operator can plausibly write, and
// each would otherwise be accepted and do nothing — which is the failure mode this
// package refuses on principle.
func TestValidateMesh(t *testing.T) {
	RegisterTestingT(t)

	cases := []struct {
		name     string
		mutate   func(*VmConfig)
		errMatch string
	}{
		{
			name:   "a two-machine fleet with the defaults",
			mutate: func(*VmConfig) {},
		},
		{
			name:     "settings without enable are refused, not ignored",
			mutate:   func(c *VmConfig) { c.Mesh = &VmMesh{Cidr: "10.99.0.0/24"} },
			errMatch: "`mesh.cidr` is set without `mesh.enable: true`",
		},
		{
			name:   "an absent block is fine",
			mutate: func(c *VmConfig) { c.Mesh = nil },
		},
		{
			name:     "a mesh of one has no peers",
			mutate:   func(c *VmConfig) { c.Hosts = c.Hosts[:1] },
			errMatch: "`mesh.enable: true` on a 1-machine resource",
		},
		{
			name:     "a host address pinned where a range belongs",
			mutate:   func(c *VmConfig) { c.Mesh.Cidr = "10.88.0.5/24" },
			errMatch: "names a host inside 10.88.0.0/24 rather than the range itself",
		},
		{
			name:     "a port out of range",
			mutate:   func(c *VmConfig) { c.Mesh.Port = 70000 },
			errMatch: "`mesh.port` must be between 1 and 65535",
		},
		{
			name:     "a range that cannot hold the fleet",
			mutate:   func(c *VmConfig) { c.Mesh.Cidr = "10.88.0.0/31" },
			errMatch: "holds 0 machine(s)",
		},
		{
			name:     "a cidr that is not one",
			mutate:   func(c *VmConfig) { c.Mesh.Cidr = "10.88.0.0" },
			errMatch: "invalid `mesh.cidr`",
		},
	}
	for _, c := range cases {
		cfg := meshConfig()
		c.mutate(cfg)
		err := cfg.validateMesh()
		if c.errMatch == "" {
			Expect(err).ToNot(HaveOccurred(), "case %q", c.name)
			continue
		}
		Expect(err).To(HaveOccurred(), "case %q must be refused", c.name)
		Expect(err.Error()).To(ContainSubstring(c.errMatch), "case %q", c.name)
	}
}

// The rendered wg-quick config. Every assertion here is something whose absence
// produces an interface that comes UP and carries nothing — the quiet failure the
// whole mesh slice is built to avoid.
func TestRenderMeshConfig(t *testing.T) {
	RegisterTestingT(t)

	got, err := RenderMeshConfig(MeshSpec{
		Name:        "host-0",
		MeshAddress: "10.88.0.1",
		PrefixLen:   24,
		ListenPort:  51820,
		Peers: []MeshPeer{
			{Name: "host-1", PublicKey: fakeMeshKey('a'), MeshAddress: "10.88.0.2", Endpoint: "203.0.113.11"},
			{Name: "host-2", PublicKey: fakeMeshKey('b'), MeshAddress: "10.88.0.3", Endpoint: "203.0.113.12"},
		},
	})
	Expect(err).ToNot(HaveOccurred())

	// The interface's own address carries the PREFIX, which is what makes wg-quick
	// install a route for the whole overlay rather than for one address.
	Expect(got).To(ContainSubstring("Address = 10.88.0.1/24"))
	Expect(got).To(ContainSubstring("ListenPort = 51820"))

	// The private key is referenced, never inlined. This is the assertion that keeps a
	// secret out of a command string, a preview diff and the Pulumi checkpoint.
	Expect(got).To(ContainSubstring("PostUp = wg set %i private-key " + MeshPrivateKeyPath))
	Expect(got).ToNot(ContainSubstring("PrivateKey ="))

	// A /32 per peer. A wider AllowedIPs would make one peer a catch-all route for
	// addresses it does not own, which fails as "some traffic vanishes" rather than as
	// an error.
	Expect(got).To(ContainSubstring("AllowedIPs = 10.88.0.2/32"))
	Expect(got).To(ContainSubstring("AllowedIPs = 10.88.0.3/32"))
	Expect(got).ToNot(ContainSubstring("AllowedIPs = 0.0.0.0/0"))

	// A peer's endpoint is its PUBLIC address and the fleet's shared listen port.
	Expect(got).To(ContainSubstring("Endpoint = 203.0.113.11:51820"))
	Expect(got).To(ContainSubstring("Endpoint = 203.0.113.12:51820"))

	// On every peer, not only the ones believed to be behind NAT — see MeshKeepalive.
	Expect(strings.Count(got, "PersistentKeepalive = 25")).To(Equal(2))
	Expect(strings.Count(got, "[Peer]")).To(Equal(2))

	// And the address the up script compares the live interface against is parsed back
	// out of this same text, so the two cannot disagree.
	Expect(meshAddressLineOf(got)).To(Equal("10.88.0.1/24"))
}

// Every value in a MeshSpec crossed an SSH connection as a remote command's stdout,
// so none of it is trustworthy for having come from SC's own code.
func TestValidateMeshSpecRefusals(t *testing.T) {
	RegisterTestingT(t)

	valid := MeshSpec{
		Name: "host-0", MeshAddress: "10.88.0.1", PrefixLen: 24, ListenPort: 51820,
		Peers: []MeshPeer{{
			Name: "host-1", PublicKey: fakeMeshKey('a'), MeshAddress: "10.88.0.2", Endpoint: "203.0.113.11",
		}},
	}
	Expect(validateMeshSpec(valid)).ToNot(HaveOccurred())

	cases := []struct {
		name     string
		mutate   func(*MeshSpec)
		errMatch string
	}{
		{"no address", func(s *MeshSpec) { s.MeshAddress = "" }, "has no address"},
		{"prefix 0", func(s *MeshSpec) { s.PrefixLen = 0 }, "invalid prefix length"},
		{"prefix 33", func(s *MeshSpec) { s.PrefixLen = 33 }, "invalid prefix length"},
		{"port 0", func(s *MeshSpec) { s.ListenPort = 0 }, "invalid listen port"},
		// The one that matters most: an interface with no peers comes up and REPORTS
		// SUCCESS while carrying nothing.
		{"no peers", func(s *MeshSpec) { s.Peers = nil }, "has no peers"},
		{"peer without a name", func(s *MeshSpec) { s.Peers[0].Name = "" }, "has no name"},
		{"peer without an address", func(s *MeshSpec) { s.Peers[0].MeshAddress = "" }, "has no overlay address"},
		{"peer without an endpoint", func(s *MeshSpec) { s.Peers[0].Endpoint = "" }, "has no endpoint address"},
		{"peer without a key", func(s *MeshSpec) { s.Peers[0].PublicKey = "" }, "usable WireGuard public key"},
		{
			// A progress line is the realistic bad value here: the keygen script's
			// stdout is read as a key, so anything written to the wrong stream lands
			// in this field.
			name:     "peer whose key is a log line",
			mutate:   func(s *MeshSpec) { s.Peers[0].PublicKey = "wg: /etc/wireguard/sc-mesh.key exists" },
			errMatch: "usable WireGuard public key",
		},
	}
	for _, c := range cases {
		spec := valid
		spec.Peers = append([]MeshPeer{}, valid.Peers...)
		c.mutate(&spec)
		err := validateMeshSpec(spec)
		Expect(err).To(HaveOccurred(), "case %q must be refused", c.name)
		Expect(err.Error()).To(ContainSubstring(c.errMatch), "case %q", c.name)

		// And RenderMeshConfig must not hand back a half-valid file for any of them.
		out, err := RenderMeshConfig(spec)
		Expect(err).To(HaveOccurred(), "case %q", c.name)
		Expect(out).To(BeEmpty(), "case %q", c.name)
	}
}

func TestValidMeshPublicKey(t *testing.T) {
	RegisterTestingT(t)

	Expect(ValidMeshPublicKey(fakeMeshKey('a'))).To(BeTrue())
	Expect(ValidMeshPublicKey("")).To(BeFalse())
	// A trailing newline is the one every shell pipeline produces, and it is the
	// difference between a config wg accepts and one it refuses at `up` time.
	Expect(ValidMeshPublicKey(fakeMeshKey('a') + "\n")).To(BeFalse())
	// Prose: refused on the length and the alphabet both.
	Expect(ValidMeshPublicKey("wg: /etc/wireguard/sc-mesh.key exists")).To(BeFalse())
	// 31 bytes: right syntax, wrong key size.
	Expect(ValidMeshPublicKey(base64.StdEncoding.EncodeToString(make([]byte, 31)))).To(BeFalse())
	// 44 characters of the base64 alphabet ending in `=` pass, because that IS a
	// well-formed encoding of 32 bytes — this function checks the SHAPE of a key and
	// cannot check that the host meant it as one. What keeps a stray message out of
	// the field is the keygen script putting every message on stderr, asserted in
	// TestMeshScripts; this check is the second line, not the first.
	Expect(ValidMeshPublicKey(strings.Repeat("x", 43) + "=")).To(BeTrue())
}

// The three scripts. Asserted on shape rather than byte-for-byte, because what each
// one must not do is the part with a live-caught reason behind it.
func TestMeshScripts(t *testing.T) {
	RegisterTestingT(t)

	keygen := MeshKeygenScript()
	// The key is generated on the host and the only thing on stdout is the PUBLIC key
	// — the caller reads stdout as a key and writes it into every sibling's config.
	Expect(keygen).To(ContainSubstring("wg genkey > " + MeshPrivateKeyPath))
	Expect(keygen).To(ContainSubstring("wg pubkey < " + MeshPrivateKeyPath))
	Expect(keygen).To(ContainSubstring("chmod 0600 " + MeshPrivateKeyPath))
	// Every progress line goes to stderr. A bare `echo` of prose here would be read as
	// a public key by the Pulumi tier.
	for _, line := range strings.Split(keygen, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "echo ") || strings.Contains(trimmed, "; echo ") {
			Expect(trimmed).To(ContainSubstring(">&2"), "every message of the keygen script must go to stderr: %q", line)
		}
	}
	// `[ -s ]`, not `[ -f ]`: a zero-byte key file is what an interrupted redirect
	// leaves, and an existence check would keep it forever.
	Expect(keygen).To(ContainSubstring("[ -s " + MeshPrivateKeyPath + " ]"))
	// The capability probe uses its own link name, so a failed probe cannot leave
	// something behind that looks like the real interface.
	Expect(keygen).To(ContainSubstring(meshProbeInterface))
	Expect(meshProbeInterface).ToNot(Equal(MeshInterface))

	up := MeshUpScript("Address = 10.88.0.1/24\nListenPort = 51820\n")
	// In-place sync on an unchanged address; down/up ONLY when the address moved. A
	// down/up on every provision would interrupt the certificate store the mesh
	// carries, on a provision that may have changed nothing.
	Expect(up).To(ContainSubstring("wg syncconf " + shellQuoteArg(MeshInterface)))
	Expect(up).To(ContainSubstring("wg-quick strip " + shellQuoteArg(MeshInterface)))
	Expect(up).To(ContainSubstring(`want='10.88.0.1/24'`))
	// The config is written to a sibling path and moved, so a half-written file is
	// never what `wg-quick up` reads.
	Expect(up).To(ContainSubstring(MeshConfigPath + ".new"))
	Expect(up).To(ContainSubstring("mv -f " + MeshConfigPath + ".new " + MeshConfigPath))
	// Survives a reboot, best effort.
	Expect(up).To(ContainSubstring("systemctl enable " + shellQuoteArg(MeshUnit)))

	down := MeshTeardownScript()
	// Namespaced to SC's own interface throughout: a hand-made `wg0` on the same host
	// must be untouched, and an operator's tunnel being called `wg0` is exactly why
	// this interface is not.
	Expect(down).To(ContainSubstring("wg-quick down " + shellQuoteArg(MeshInterface)))
	Expect(down).To(ContainSubstring("rm -f " + MeshConfigPath + " " + MeshPrivateKeyPath))
	Expect(down).ToNot(ContainSubstring("wg0"))
	// The package stays: SC installed it on a machine it does not own.
	Expect(down).ToNot(ContainSubstring("apt-get remove"))
	Expect(down).ToNot(ContainSubstring("purge"))
	for _, s := range []string{keygen, up, down} {
		Expect(s).To(HavePrefix("set -eu\n"))
	}
}

// A k3s pool on a hoster with no private network was refused outright, because etcd
// peer traffic and flannel's unencrypted VXLAN would have crossed the public internet
// on a deploy that SUCCEEDS. `mesh.enable: true` is what lifts that refusal — and it
// lifts it rather than loosening it, because SC then owns the node-to-node network.
func TestMeshLiftsThePrivateAddressRequirement(t *testing.T) {
	RegisterTestingT(t)

	pool := func() *VmConfig {
		cfg := meshConfig()
		cfg.Mesh = nil
		cfg.Hosts[0].Role = string(RoleK3sServer)
		cfg.Hosts[1].Role = string(RoleK3sAgent)
		return cfg
	}

	// Without a mesh and without privateAddress: still refused, and the message now
	// names the way out rather than a roadmap item that had not been built.
	err := pool().validateHostRoles()
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("`mesh.enable: true`"))
	Expect(err.Error()).To(ContainSubstring("WireGuard"))
	// The old message pointed at an unbuilt roadmap item and told the operator to
	// declare a compose fleet instead. Both are now wrong advice.
	Expect(err.Error()).ToNot(ContainSubstring("sc-vm-multi-node-clusters"))
	Expect(err.Error()).ToNot(ContainSubstring("not built yet"))

	// With the operator's own private network: accepted, exactly as before.
	priv := pool()
	priv.Hosts[0].PrivateAddress = "10.15.1.216"
	priv.Hosts[1].PrivateAddress = "10.13.2.75"
	Expect(priv.validateHostRoles()).ToNot(HaveOccurred())

	// With a mesh and no privateAddress anywhere: accepted.
	mesh := pool()
	mesh.Mesh = &VmMesh{Enable: true}
	Expect(mesh.validateHostRoles()).ToNot(HaveOccurred())

	// But a collision in the operator's own values stays refused WITH a mesh, which
	// is why that check moved out of the no-private-network branch: whatever else
	// reads `privateAddress` still reads it, and a mesh does not make two machines
	// claiming one address harmless.
	collide := pool()
	collide.Mesh = &VmMesh{Enable: true}
	collide.Hosts[0].PrivateAddress = "10.15.1.216"
	collide.Hosts[1].PrivateAddress = "10.15.1.216"
	err = collide.validateHostRoles()
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("more than one machine"))
}

// The mesh's one deliberately public firewall hole. It is UDP, it is not marked
// internal, and both of those are the point — a peer's endpoint is its public
// address, so there is no inner network to scope the hole to.
func TestFirewallPortsIncludesTheMesh(t *testing.T) {
	RegisterTestingT(t)

	cfg := meshConfig()
	ports, err := cfg.FirewallPorts()
	Expect(err).ToNot(HaveOccurred())

	var found []PortRange
	for _, p := range ports {
		if p.Label == PortLabelMesh {
			found = append(found, p)
		}
	}
	Expect(found).To(HaveLen(1))
	Expect(found[0].Protocol).To(Equal(ProtocolUdp))
	Expect(found[0].From).To(Equal(DefaultMeshPort))
	Expect(found[0].To).To(Equal(DefaultMeshPort))
	Expect(found[0].Internal).To(BeFalse())

	// An overridden port moves the hole rather than adding a second one.
	cfg.Mesh.Port = 51999
	ports, err = cfg.FirewallPorts()
	Expect(err).ToNot(HaveOccurred())
	for _, p := range ports {
		if p.Label == PortLabelMesh {
			Expect(p.From).To(Equal(51999))
		}
		Expect(p.From).ToNot(Equal(DefaultMeshPort))
	}

	// And no hole at all without the overlay, which is every single-machine resource.
	bare := meshConfig()
	bare.Mesh = nil
	ports, err = bare.FirewallPorts()
	Expect(err).ToNot(HaveOccurred())
	for _, p := range ports {
		Expect(p.Label).ToNot(Equal(PortLabelMesh))
	}
}
