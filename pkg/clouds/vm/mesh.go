// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package vm

import (
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"net"
	"strings"

	"github.com/pkg/errors"
	"github.com/samber/lo"
)

// The names SC gives its own WireGuard tunnel on a machine.
//
// `sc-mesh` and not `wg0`, which is not cosmetic: `wg0` is what every hand-written
// WireGuard tutorial calls the operator's own tunnel, and a byo machine may well
// already have one. An SC-managed interface that silently took that name would bring
// down the operator's access to the box it is provisioning, and `wg-quick down wg0`
// in a teardown would do it deliberately. Everything here is namespaced so that
// SC's mesh and whatever else the machine runs can coexist, and so that the teardown
// can only ever remove SC's own.
const (
	// MeshInterface is the link name, which must be ≤ 15 bytes (IFNAMSIZ) — `sc-mesh`
	// is 7.
	MeshInterface = "sc-mesh"
	// MeshConfigPath is the wg-quick config. 0600: it names every peer, and on a
	// host that inlined its key it would hold one (SC's does not — see
	// MeshPrivateKeyPath).
	MeshConfigPath = "/etc/wireguard/" + MeshInterface + ".conf"
	// MeshPrivateKeyPath is where the machine keeps the key it generated itself.
	//
	// The key is generated ON the host (`wg genkey`) and never leaves it: the
	// provisioning step's stdout is the PUBLIC key only, so no private key enters
	// the Pulumi checkpoint, a command's input set, or a preview diff. Persistence
	// is this file being present, not SC having stored the value — which also means
	// a re-imaged machine simply generates a new key on the next provision and the
	// peers learn it in the same run, with nothing to push back at it.
	//
	// The config references it through `PostUp = wg set %i private-key …` rather than
	// inlining it, so that the rendered config is a pure function of the fleet's
	// public facts and SC never has to template a secret into a file.
	MeshPrivateKeyPath = "/etc/wireguard/" + MeshInterface + ".key"
	// MeshUnit is the systemd unit wg-quick generates for the interface.
	MeshUnit = "wg-quick@" + MeshInterface

	// DefaultMeshCidr is the overlay's address range.
	//
	// 10.88.0.0/24 is a deliberate choice of an UNFASHIONABLE corner of RFC 1918:
	// it has to not collide with the machines' real private networking (the clouds
	// hand out 10.0.x, 10.13-10.15.x, 172.31.x, 192.168.x), with docker's default
	// bridge (172.17.0.0/16) or its compose networks (172.18+), or with k3s's pod
	// and service CIDRs (10.42.0.0/16 and 10.43.0.0/16 — the two that would be a
	// genuinely confusing outage). A collision is the one failure here SC cannot
	// detect, so the field is overridable and this is only a default.
	DefaultMeshCidr = "10.88.0.0/24"
	// DefaultMeshPort is WireGuard's IANA-registered port.
	DefaultMeshPort = 51820

	// MeshKeepalive keeps a NAT binding open from both ends, in seconds.
	//
	// Set on every peer rather than only on the ones believed to be behind NAT,
	// because SC cannot know which those are and the cost of being wrong is
	// asymmetric: a keepalive on a directly-addressable peer is one 32-byte packet
	// every 25 s, and a missing one on a NATed peer is a tunnel that works until the
	// first idle minute and then silently stops carrying the certificate store.
	MeshKeepalive = 25
)

// VmMesh is an SC-managed WireGuard overlay across the machines of one vm resource.
//
// It exists because two things the fleet needs — a shared certificate store and a
// multi-node k3s pool — both require the machines to reach each other on an address
// that is not the public internet, and a large part of the hosting market provides
// no private networking at all. Without it those topologies are either refused (see
// VmHost.PrivateAddress) or quietly insecure.
//
// What it is not: a substitute for a cloud's own private network. Where one exists,
// `privateAddress` is cheaper, faster and already trusted by the cloud's firewall —
// the mesh is for the machines that have no such thing, and for the case where the
// machines are in different clouds entirely.
type VmMesh struct {
	Enable bool `json:"enable" yaml:"enable"`
	// Cidr is the overlay's range. Machine i gets host address i+1 within it
	// (MeshAddress), so a /24 holds 254 machines.
	//
	// Override it when it collides with something on the machines — see
	// DefaultMeshCidr for the list of ranges the default was chosen to avoid, and
	// note that changing it later re-addresses every node, which flaps the mesh once.
	Cidr string `json:"cidr,omitempty" yaml:"cidr,omitempty"`
	// Port is the UDP port every machine listens on. It is open to the INTERNET and
	// not to the fleet — a peer's endpoint is its public address, so there is no
	// inner network for the hole to be scoped to.
	//
	// That is sound, and it is worth saying why rather than leaving it to look like
	// an oversight: WireGuard sends no reply at all to a packet that does not
	// authenticate against a known peer's key, so the port is invisible to a scan
	// and unreachable to anyone without a key in the fleet's own config. It is the
	// one piece of the mesh whose exposure is deliberate.
	Port int `json:"port,omitempty" yaml:"port,omitempty"`
}

// MeshEnabled reports whether SC should stand up the overlay on this resource.
func (r *VmConfig) MeshEnabled() bool {
	return r.Mesh != nil && r.Mesh.Enable
}

// MeshCidr is the overlay range, defaulted.
func (r *VmConfig) MeshCidr() string {
	if r.Mesh != nil && r.Mesh.Cidr != "" {
		return r.Mesh.Cidr
	}
	return DefaultMeshCidr
}

// MeshPort is the listen port, defaulted.
func (r *VmConfig) MeshPort() int {
	if r.Mesh != nil && r.Mesh.Port > 0 {
		return r.Mesh.Port
	}
	return DefaultMeshPort
}

// MeshPrefixLen is the CIDR's prefix length, which is what goes on the interface's
// own `Address` line so that wg-quick installs a route for the whole overlay.
func (r *VmConfig) MeshPrefixLen() (int, error) {
	_, netw, err := net.ParseCIDR(r.MeshCidr())
	if err != nil {
		return 0, errors.Wrapf(err, "invalid `mesh.cidr` %q", r.MeshCidr())
	}
	ones, _ := netw.Mask.Size()
	return ones, nil
}

// MeshAddress is machine i's address on the overlay.
//
// A pure function of the config — the CIDR and the index — and that is the property
// that earns its keep. The certificate store runs on machine 0 and every other
// machine has to be told where it is; because this is computable, the store's
// endpoint is a constant in the rendered Caddyfile instead of a Pulumi Output
// threaded from one host's provisioning step into every other host's.
//
// Index-stable by construction, so raising the node count never re-addresses an
// existing machine. Host addresses start at .1 (the network address itself is not
// usable), so machine i is network+i+1.
func (r *VmConfig) MeshAddress(i int) (string, error) {
	return MeshAddressIn(r.MeshCidr(), i)
}

// MeshAddressIn is MeshAddress with the range passed explicitly, for callers that
// have the CIDR but not the whole config.
func MeshAddressIn(cidr string, i int) (string, error) {
	if i < 0 {
		return "", errors.Errorf("mesh host index must not be negative, got %d", i)
	}
	_, netw, err := net.ParseCIDR(cidr)
	if err != nil {
		return "", errors.Wrapf(err, "invalid `mesh.cidr` %q", cidr)
	}
	base := netw.IP.To4()
	if base == nil {
		return "", errors.Errorf(
			"`mesh.cidr` %q is IPv6; the mesh is IPv4-only, because a fleet member with no IPv6 route "+
				"would be unreachable on it with nothing to fall back to", cidr,
		)
	}
	ones, bits := netw.Mask.Size()
	// -2: the network address and the broadcast address are not host addresses.
	usable := (uint64(1) << uint(bits-ones)) - 2
	if uint64(i)+1 > usable {
		return "", errors.Errorf(
			"`mesh.cidr` %q holds %d machine(s) and this fleet needs at least %d; widen the prefix",
			cidr, usable, i+1,
		)
	}
	addr := binary.BigEndian.Uint32(base) + uint32(i) + 1
	var out [4]byte
	binary.BigEndian.PutUint32(out[:], addr)
	return net.IP(out[:]).String(), nil
}

// validateMesh holds the mesh's config rules.
//
// Called from both the byo and the cloud path, and deliberately after the host/node
// checks on each: every refusal here can name the fleet's real size.
func (r *VmConfig) validateMesh() error {
	if r.Mesh == nil {
		return nil
	}
	if !r.Mesh.Enable {
		// The file's standing rule — support a field or refuse it. A `mesh:` block
		// with settings and no `enable: true` is config an operator believes applied.
		var set []string
		if r.Mesh.Cidr != "" {
			set = append(set, "cidr")
		}
		if r.Mesh.Port != 0 {
			set = append(set, "port")
		}
		if len(set) > 0 {
			return errors.Errorf(
				"`mesh.%s` is set without `mesh.enable: true`: the overlay is not built, so nothing would "+
					"read %s", strings.Join(set, "`, `mesh."), lo.If(len(set) == 1, "it").Else("them"),
			)
		}
		return nil
	}
	if p := r.MeshPort(); p < 1 || p > maxPort {
		return errors.Errorf("`mesh.port` must be between 1 and %d, got %d", maxPort, p)
	}
	ip, netw, err := net.ParseCIDR(r.MeshCidr())
	if err != nil {
		return errors.Wrapf(err, "invalid `mesh.cidr` %q (expected something like %s)", r.MeshCidr(), DefaultMeshCidr)
	}
	// `10.88.0.5/24` parses, and ParseCIDR would hand back the 10.88.0.0/24 network
	// while the operator believes they pinned an address. Refused rather than
	// normalised, because the two readings differ by one host and the operator's is
	// the wrong one.
	if !ip.Equal(netw.IP) {
		return errors.Errorf(
			"`mesh.cidr` %q names a host inside %s rather than the range itself; write the network address "+
				"(SC assigns each machine an address within it — see `mesh.cidr`)", r.MeshCidr(), netw.String(),
		)
	}
	count := r.NodeCount()
	if count < 2 {
		// A mesh of one has no peers and nothing to carry. Refused rather than
		// ignored for the usual reason, and with the usual remedy named: a resource
		// that was a fleet and is now a single machine needs the fleet-shaped blocks
		// removed, not silently disabled.
		return errors.Errorf(
			"`mesh.enable: true` on a %d-machine resource: the overlay exists so the machines of a FLEET can "+
				"reach each other, and a mesh of one has no peers. Remove the `mesh:` block, or declare more "+
				"machines", count,
		)
	}
	if _, err := r.MeshAddress(count - 1); err != nil {
		return err
	}
	return nil
}

// MeshPeer is one other machine of the fleet, as the local machine's config needs to
// name it.
type MeshPeer struct {
	// Name is the host's SC name, written into the config as a comment so that
	// `wg show` output and the file can be read side by side.
	Name string
	// PublicKey is the peer's WireGuard public key, as the peer's own `wg genkey |
	// wg pubkey` printed it.
	PublicKey string
	// MeshAddress is the peer's overlay address — its `AllowedIPs`, as a /32. A /32
	// and not the whole range: every peer owns exactly one address, and a wider
	// AllowedIPs would make one peer a catch-all route for addresses it does not own.
	MeshAddress string
	// Endpoint is the peer's PUBLIC address, where its UDP port is reached. There is
	// no inner network to use here — building one is what this whole mesh is for.
	Endpoint string
}

// MeshSpec is the whole input of RenderMeshConfig: the local machine's own facts
// plus its peers.
type MeshSpec struct {
	Name        string
	MeshAddress string
	PrefixLen   int
	ListenPort  int
	Peers       []MeshPeer
}

// RenderMeshConfig renders a wg-quick config for one machine.
//
// Pure, and that is why it is here rather than in the provisioning tier: the file's
// content is the fleet's public facts and nothing else, so it can be tested without
// a Pulumi context and diffed in review.
//
// The private key is NOT in it. `PostUp` loads it from MeshPrivateKeyPath, which
// keeps a secret out of a rendered file and lets the machine own the only copy. Note
// that `wg-quick strip` — what `wg syncconf` is fed on a re-provision — drops PostUp
// lines, which is correct here: a running interface already holds its key.
func RenderMeshConfig(spec MeshSpec) (string, error) {
	if err := validateMeshSpec(spec); err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString("# Managed by Simple Container. Changes are overwritten on the next provision.\n")
	b.WriteString(fmt.Sprintf("# Interface %s carries this vm resource's node-to-node traffic.\n", MeshInterface))
	b.WriteString("[Interface]\n")
	b.WriteString(fmt.Sprintf("Address = %s/%d\n", spec.MeshAddress, spec.PrefixLen))
	b.WriteString(fmt.Sprintf("ListenPort = %d\n", spec.ListenPort))
	// The key stays in its own 0600 file; see MeshPrivateKeyPath.
	b.WriteString(fmt.Sprintf("PostUp = wg set %%i private-key %s\n", MeshPrivateKeyPath))
	for _, p := range spec.Peers {
		b.WriteString("\n[Peer]\n")
		b.WriteString(fmt.Sprintf("# %s\n", p.Name))
		b.WriteString(fmt.Sprintf("PublicKey = %s\n", p.PublicKey))
		b.WriteString(fmt.Sprintf("AllowedIPs = %s/32\n", p.MeshAddress))
		b.WriteString(fmt.Sprintf("Endpoint = %s:%d\n", p.Endpoint, spec.ListenPort))
		b.WriteString(fmt.Sprintf("PersistentKeepalive = %d\n", MeshKeepalive))
	}
	return b.String(), nil
}

// validateMeshSpec refuses a config that would be syntactically wrong or silently
// partial.
//
// Every value here crossed an SSH connection as the stdout of a remote command, so
// "it came from our own code" is not true of any of it: a public key is whatever the
// far side printed. A stray warning line pasted into `PublicKey =` makes wg-quick
// refuse the whole interface — which is the good case — and an empty peer list makes
// it come up isolated and report success, which is the bad one.
func validateMeshSpec(spec MeshSpec) error {
	if spec.MeshAddress == "" {
		return errors.Errorf("mesh config for %q has no address", spec.Name)
	}
	if spec.PrefixLen < 1 || spec.PrefixLen > 32 {
		return errors.Errorf("mesh config for %q has an invalid prefix length %d", spec.Name, spec.PrefixLen)
	}
	if spec.ListenPort < 1 || spec.ListenPort > maxPort {
		return errors.Errorf("mesh config for %q has an invalid listen port %d", spec.Name, spec.ListenPort)
	}
	if len(spec.Peers) == 0 {
		return errors.Errorf(
			"mesh config for %q has no peers: an interface with none comes up, reports success and carries "+
				"nothing, so it is refused here rather than discovered later", spec.Name,
		)
	}
	for i, p := range spec.Peers {
		switch {
		case p.Name == "":
			return errors.Errorf("mesh peer %d of %q has no name", i, spec.Name)
		case p.MeshAddress == "":
			return errors.Errorf("mesh peer %q of %q has no overlay address", p.Name, spec.Name)
		case p.Endpoint == "":
			return errors.Errorf("mesh peer %q of %q has no endpoint address", p.Name, spec.Name)
		case !ValidMeshPublicKey(p.PublicKey):
			// The value, not just its absence: see the function comment.
			return errors.Errorf(
				"mesh peer %q of %q did not report a usable WireGuard public key (a 44-character base64 "+
					"value); the machine's key generation step printed something else, which is where to look",
				p.Name, spec.Name,
			)
		}
	}
	return nil
}

// meshProbeInterface is the throwaway link name the capability check creates and
// removes. A name of its own so that a failed probe can never leave something behind
// that looks like the real interface.
const meshProbeInterface = MeshInterface + "-probe"

// MeshKeygenScript is pass 1 of standing the overlay up on one machine: get
// `wireguard-tools`, prove the kernel can carry a WireGuard link, and generate the
// machine's keypair if it does not already have one.
//
// STDOUT IS THE PUBLIC KEY AND NOTHING ELSE. Every other word this script says goes
// to stderr, because the caller reads stdout as a key and pastes it into every
// sibling's config — a progress line on the wrong stream becomes a `PublicKey =`
// value. The caller checks the shape too (ValidMeshPublicKey), so a mistake here
// fails the provision rather than producing a mesh that cannot handshake.
//
// The private key never leaves the machine. It is generated here, into a 0600 file
// owned by root, and persistence is that file being present rather than SC having
// stored the value — so no private key enters the Pulumi checkpoint, a command's
// input set, a preview diff or a CI log. A re-imaged machine generates a new one and
// its peers learn the new public key in the same provision, with nothing to push
// back at it.
//
// No `Delete` belongs on the command that runs this (see MeshTeardownScript for what
// the teardown does remove): this installs a package on a machine SC does not own,
// and the precedent is ByoDockerInstallScript's — uninstalling an operator's
// packages because a stack was destroyed is not SC's business.
func MeshKeygenScript() string {
	q := shellQuoteArg
	return strings.Join([]string{
		`set -eu`,
		// Already there is the common case on every provision after the first.
		`if command -v wg >/dev/null 2>&1; then`,
		`  echo "wireguard-tools is already installed" >&2`,
		`else`,
		`  echo "installing wireguard-tools..." >&2`,
		`  if command -v apt-get >/dev/null 2>&1; then`,
		`    sudo DEBIAN_FRONTEND=noninteractive apt-get update -qq >&2`,
		`    sudo DEBIAN_FRONTEND=noninteractive apt-get install -y -qq wireguard-tools >&2`,
		`  elif command -v dnf >/dev/null 2>&1; then sudo dnf install -y -q wireguard-tools >&2`,
		`  elif command -v yum >/dev/null 2>&1; then sudo yum install -y -q wireguard-tools >&2`,
		`  elif command -v apk >/dev/null 2>&1; then sudo apk add --no-progress wireguard-tools >&2`,
		`  elif command -v pacman >/dev/null 2>&1; then sudo pacman -S --noconfirm --quiet wireguard-tools >&2`,
		`  else`,
		`    echo "cannot install wireguard-tools: no apt-get, dnf, yum, apk or pacman on this host. ` +
			`Install the wireguard-tools package by hand, or remove 'mesh.enable' and give each host a ` +
			`'privateAddress' on a network they already share" >&2`,
		`    exit 1`,
		`  fi`,
		`fi`,
		`command -v wg >/dev/null 2>&1 || { echo "wireguard-tools is still absent after the install; ` +
			`install it by hand" >&2; exit 1; }`,
		`command -v wg-quick >/dev/null 2>&1 || { echo "wg is installed but wg-quick is not; on some ` +
			`distributions they are separate packages and SC needs both" >&2; exit 1; }`,
		// The capability check. Cheap (a link created and removed), and it moves the
		// failure from a confusing `wg-quick up` error in pass 2 to a sentence here.
		// A container, a VPS on an old OpenVZ-style kernel, or a hardened kernel
		// without the module is a real case, and so is the userspace fallback — which
		// SC does NOT install, because that path has never been run on the stand and
		// an unverified install is worse than an accurate refusal.
		`if sudo ip link add dev ` + q(meshProbeInterface) + ` type wireguard >/dev/null 2>&1; then`,
		`  sudo ip link del dev ` + q(meshProbeInterface) + ` >/dev/null 2>&1 || true`,
		`else`,
		`  command -v wireguard-go >/dev/null 2>&1 || { echo "this kernel cannot create a WireGuard ` +
			`interface and wireguard-go is not installed, so there is no userspace fallback either. ` +
			`Install wireguard-go (wg-quick uses it automatically when the kernel module is absent), use a ` +
			`host whose kernel has the module, or remove 'mesh.enable'" >&2; exit 1; }`,
		`  echo "kernel module unavailable; wg-quick will fall back to wireguard-go" >&2`,
		`fi`,
		`sudo install -d -m 0700 /etc/wireguard`,
		// `[ -s ]` and not `[ -f ]`: a zero-byte key file is what an interrupted
		// `wg genkey` redirect leaves behind, and it would be kept forever by an
		// existence check while failing every `wg set`.
		//
		// Run inside one `sudo sh -c` because the redirect has to happen as root: the
		// file is 0600 root-owned, so a redirect by the login user cannot write it
		// and a later `sudo wg pubkey < file` cannot read it.
		`sudo sh -c '[ -s ` + MeshPrivateKeyPath + ` ] || { umask 077; wg genkey > ` + MeshPrivateKeyPath + `; }'`,
		`sudo chmod 0600 ` + MeshPrivateKeyPath,
		`sudo sh -c 'wg pubkey < ` + MeshPrivateKeyPath + `'`,
	}, "\n") + "\n"
}

// MeshUpScript is pass 2: write this machine's peer list and apply it.
//
// `conf` is the output of RenderMeshConfig and is NOT a secret — the private key is
// referenced by path, never inlined — so it travels in the command string like the
// Caddyfile rather than over stdin.
//
// The applying half is the part with a decision in it. A running interface is
// updated with `wg syncconf`, which replaces the peer set in place and does not
// touch established sessions; `wg-quick down && up` is used only when the
// interface's own ADDRESS changed, because that is the one thing syncconf cannot
// apply. The reason is not speed: the mesh carries the fleet's certificate store, so
// a down/up on every provision is a real interruption of it, on a provision that may
// have changed nothing and that reports success either way. A flap that brief would
// be invisible in every log SC writes and visible only as an occasional failed
// renewal weeks later.
func MeshUpScript(conf string) string {
	q := shellQuoteArg
	newPath := MeshConfigPath + ".new"
	return strings.Join([]string{
		`set -eu`,
		`command -v wg-quick >/dev/null 2>&1 || { echo "wireguard-tools is not installed on this host; ` +
			`the mesh key-generation step should have installed it" >&2; exit 1; }`,
		`sudo install -d -m 0700 /etc/wireguard`,
		// Written to a sibling path and moved, like every other config this package
		// lays down: a half-written file is what `wg-quick up` would otherwise read.
		// `tee` rather than a redirect because the target directory is root-owned.
		`printf %s ` + q(encodeMeshFile(conf)) + ` | base64 -d | sudo tee ` + newPath + ` >/dev/null`,
		`sudo chmod 0600 ` + newPath,
		`want=` + q(meshAddressLineOf(conf)),
		`if sudo wg show ` + q(MeshInterface) + ` >/dev/null 2>&1; then`,
		`  have="$(ip -4 -o addr show dev ` + q(MeshInterface) +
			` 2>/dev/null | awk '{print $4}' | head -n1 || true)"`,
		`else`,
		`  have=""`,
		`fi`,
		`sudo mv -f ` + newPath + ` ` + MeshConfigPath,
		`if [ -n "$have" ] && [ "$have" = "$want" ]; then`,
		// strip removes wg-quick's own directives (Address, PostUp, …) and leaves
		// what `wg syncconf` takes. Through a temp file rather than process
		// substitution, which is a bashism and this runs under sh.
		`  tmp="$(mktemp)"`,
		`  trap 'rm -f "$tmp"' EXIT`,
		`  sudo wg-quick strip ` + q(MeshInterface) + ` > "$tmp"`,
		`  sudo wg syncconf ` + q(MeshInterface) + ` "$tmp"`,
		`  echo "mesh ` + MeshInterface + ` updated in place (peers synced, sessions kept)"`,
		`elif [ -n "$have" ]; then`,
		`  echo "mesh ` + MeshInterface + ` is addressed $have and should be $want; restarting it" >&2`,
		`  sudo wg-quick down ` + q(MeshInterface) + ` >/dev/null 2>&1 || true`,
		`  sudo wg-quick up ` + q(MeshInterface) + ` >&2`,
		`  echo "mesh ` + MeshInterface + ` restarted on $want"`,
		`else`,
		`  sudo wg-quick up ` + q(MeshInterface) + ` >&2`,
		`  echo "mesh ` + MeshInterface + ` is up on $want"`,
		`fi`,
		// Survives a reboot. Best effort, like the docker install's: a machine with
		// no systemd still has a working interface for this provision's lifetime, and
		// failing the provision over the unit would be refusing a mesh that works.
		`if command -v systemctl >/dev/null 2>&1; then`,
		`  sudo systemctl enable ` + q(MeshUnit) + ` >/dev/null 2>&1 || ` +
			`echo "could not enable ` + MeshUnit + `; the mesh will not survive a reboot of this host" >&2`,
		`fi`,
	}, "\n") + "\n"
}

// MeshTeardownScript removes SC's interface, its config and its key when the vm
// resource is destroyed.
//
// What it does NOT remove is the `wireguard-tools` package, for the reason
// ByoDockerInstallScript's missing Delete gives: SC installed it on a machine it
// does not own, and the operator may be using it. Everything removed here is
// namespaced to `sc-mesh` (see MeshInterface) — a hand-made `wg0` on the same host is
// untouched.
func MeshTeardownScript() string {
	q := shellQuoteArg
	return strings.Join([]string{
		`set -eu`,
		`if command -v wg-quick >/dev/null 2>&1 && [ -f ` + MeshConfigPath + ` ]; then`,
		`  sudo wg-quick down ` + q(MeshInterface) + ` >/dev/null 2>&1 || true`,
		`fi`,
		`if command -v systemctl >/dev/null 2>&1; then`,
		`  sudo systemctl disable ` + q(MeshUnit) + ` >/dev/null 2>&1 || true`,
		`fi`,
		// The key goes with it: unlike docker, this is SC's own key for SC's own
		// interface and is worth nothing once the interface is gone. A later
		// provision generates a fresh one.
		`sudo rm -f ` + MeshConfigPath + ` ` + MeshPrivateKeyPath,
		`echo "removed the ` + MeshInterface + ` mesh (wireguard-tools left installed)"`,
	}, "\n") + "\n"
}

// meshAddressLineOf extracts the `Address = …` value from a rendered config, which
// is what the up script compares the live interface against.
//
// Parsed out of the rendered text rather than passed in beside it so that the two
// cannot disagree: the comparison decides between an in-place sync and a restart,
// and a stale value there means either a flap on every provision or an interface
// left on the wrong address.
func meshAddressLineOf(conf string) string {
	for _, line := range strings.Split(conf, "\n") {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "Address"); ok {
			if v, found := strings.CutPrefix(strings.TrimSpace(rest), "="); found {
				return strings.TrimSpace(v)
			}
		}
	}
	return ""
}

// encodeMeshFile is base64 for the same reason the ingress config tree uses it: the
// content crosses an sh command line, and a config file with `$`, newlines and `=`
// in it has no safe single-quoted spelling.
func encodeMeshFile(content string) string {
	return base64.StdEncoding.EncodeToString([]byte(content))
}

// ValidMeshPublicKey reports whether s is shaped like a WireGuard key: 32 raw bytes
// in standard base64, which is always 44 characters ending in `=`.
//
// Checked rather than trusted because the value arrives as a remote command's
// stdout. Length alone is not enough — a 44-character log line would pass — so the
// decode is done too.
func ValidMeshPublicKey(s string) bool {
	if len(s) != 44 || !strings.HasSuffix(s, "=") {
		return false
	}
	raw, err := base64.StdEncoding.DecodeString(s)
	return err == nil && len(raw) == 32
}
