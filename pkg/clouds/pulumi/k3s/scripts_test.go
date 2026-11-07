// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package k3s

import (
	"strings"
	"testing"

	. "github.com/onsi/gomega"

	cfgK3s "github.com/simple-container-com/api/pkg/clouds/k3s"
	pulumiKubernetes "github.com/simple-container-com/api/pkg/clouds/pulumi/kubernetes"
	"github.com/simple-container-com/api/pkg/clouds/vm"
)

func TestServerArgs(t *testing.T) {
	RegisterTestingT(t)

	args := ServerArgs(
		&cfgK3s.K3sClusterConfig{ExtraServerArgs: []string{"--kube-apiserver-arg=v=2"}},
		[]string{"k8s.example.ru", "203.0.113.10"},
		[]string{"traefik", "servicelb"},
		JoinSpec{},
	)

	Expect(args).To(Equal("server " +
		"--tls-san k8s.example.ru --tls-san 203.0.113.10 " +
		"--disable traefik --disable servicelb " +
		"--kube-apiserver-arg=v=2"))
	// `server` first: the installer reads the subcommand off the front of the vector
	// it is given, and an install with the flags but no subcommand starts an agent.
	Expect(strings.HasPrefix(args, "server ")).To(BeTrue())
	// The zero JoinSpec is the lone server of a single-server cluster. Neither flag
	// belongs there: `--cluster-init` would hand it an embedded etcd it has no use
	// for (a quorum of one, and a datastore conversion to undo), and `--server`
	// would point it at itself.
	Expect(args).NotTo(ContainSubstring("--cluster-init"))
	Expect(args).NotTo(ContainSubstring("--server"))
}

// The three JoinSpec shapes are the whole topology story, and the flags are not
// interchangeable: `--cluster-init` on a joining server makes it found a SECOND
// cluster of one rather than join the first, which presents as a working provision
// and a split brain.
func TestServerArgsRendersEachTopology(t *testing.T) {
	RegisterTestingT(t)

	cfg := &cfgK3s.K3sClusterConfig{}
	first := ServerArgs(cfg, []string{"a"}, nil, JoinSpec{ClusterInit: true})
	Expect(first).To(HavePrefix("server --cluster-init "))
	Expect(first).NotTo(ContainSubstring("--server "))

	joining := ServerArgs(cfg, []string{"a"}, nil, JoinSpec{ServerURL: "https://203.0.113.10:6443"})
	Expect(joining).To(Equal("server --server https://203.0.113.10:6443 --tls-san a"))
	Expect(joining).NotTo(ContainSubstring("--cluster-init"))

	// An agent gets `--server` and nothing else. `--tls-san`, `--disable` and
	// extraServerArgs are server-only flags and k3s's agent does not ignore them —
	// it exits with an unknown-flag error, so the node never starts.
	agent := AgentArgs(JoinSpec{ServerURL: "https://203.0.113.10:6443"})
	Expect(agent).To(Equal("agent --server https://203.0.113.10:6443"))
	for _, serverOnly := range []string{"--tls-san", "--disable", "--cluster-init"} {
		Expect(agent).NotTo(ContainSubstring(serverOnly))
	}

	// The unit follows the role, because the installer decides which unit to write
	// from the FIRST positional argument it is handed.
	Expect(UnitFor(vm.RoleK3sServer)).To(Equal(ServerUnit))
	Expect(UnitFor(vm.RoleK3sAgent)).To(Equal(AgentUnit))
	Expect(ArgsMarkerPath(ServerUnit)).NotTo(Equal(ArgsMarkerPath(AgentUnit)))
}

// NodeIP is the one addressing decision SC makes for k3s, and the reason it exists
// is that k3s's own default is wrong on a single-NIC VPS: it advertises the
// default-route address, which there is the PUBLIC one, putting etcd peer traffic
// and flannel's unencrypted VXLAN overlay on the internet (vm.VmHost.PrivateAddress).
func TestNodeIPPinsBothAdvertisedAddressesOnAServerAndOnlyNodeIPOnAnAgent(t *testing.T) {
	RegisterTestingT(t)

	cfg := &cfgK3s.K3sClusterConfig{}

	// `--node-ip` alone is not enough on a server: the API server would still
	// advertise its default-route address to its peers, which is the value this
	// exists to avoid. Both flags, both to the same address.
	first := ServerArgs(cfg, []string{"a"}, nil, JoinSpec{ClusterInit: true, NodeIP: "10.0.0.10"})
	Expect(first).To(ContainSubstring("--node-ip 10.0.0.10"))
	Expect(first).To(ContainSubstring("--advertise-address 10.0.0.10"))

	joining := ServerArgs(cfg, []string{"a"}, nil,
		JoinSpec{ServerURL: "https://10.0.0.10:6443", NodeIP: "10.0.0.11"})
	Expect(joining).To(ContainSubstring("--node-ip 10.0.0.11"))
	Expect(joining).To(ContainSubstring("--advertise-address 10.0.0.11"))

	// `--advertise-address` is server-only; an agent handed it exits with an
	// unknown-flag error and the node never starts.
	agent := AgentArgs(JoinSpec{ServerURL: "https://10.0.0.10:6443", NodeIP: "10.0.0.12"})
	Expect(agent).To(Equal("agent --server https://10.0.0.10:6443 --node-ip 10.0.0.12"))
	Expect(agent).NotTo(ContainSubstring("--advertise-address"))

	// An empty NodeIP renders nothing at all, on both roles. That is what keeps a
	// single-node cluster's args marker byte-identical to what it was before this
	// field existed — otherwise every existing control plane reinstalls for no
	// behaviour change.
	Expect(ServerArgs(cfg, []string{"a"}, nil, JoinSpec{ClusterInit: true})).
		NotTo(ContainSubstring("--node-ip"))
	Expect(AgentArgs(JoinSpec{ServerURL: "https://10.0.0.10:6443"})).
		NotTo(ContainSubstring("--node-ip"))
}

// The join token must not reach the argument vector. It would land in the args
// marker on the machine and in the Pulumi checkpoint as part of that string, and a
// token rotation would then make the marker compare unequal — reinstalling a healthy
// control plane for a value the comparison has no business knowing.
func TestTheJoinTokenTravelsInTheEnvironmentAndNotTheArgs(t *testing.T) {
	RegisterTestingT(t)

	const token = "K10deadbeef::server:cafe"
	args := ServerArgs(&cfgK3s.K3sClusterConfig{}, []string{"a"}, nil,
		JoinSpec{ServerURL: "https://203.0.113.10:6443"})
	Expect(args).NotTo(ContainSubstring(token))
	Expect(args).NotTo(ContainSubstring("--token"))

	script := InstallScript("v1.36.5+k3s1", cfgK3s.DefaultInstallScriptURL, args, ServerUnit, token)
	// `sudo env`, for the same reason INSTALL_K3S_VERSION uses it: it is the one
	// form that survives sudo's env_reset.
	Expect(script).To(ContainSubstring(`sudo env INSTALL_K3S_VERSION="$WANT_VERSION" K3S_TOKEN="` + token + `"`))
	Expect(script).To(ContainSubstring(`WANT_ARGS="` + args + `"`))

	// And no token at all when there is none to pass — the first server of a cluster
	// has nothing to authenticate to.
	Expect(InstallScript("v1.36.5+k3s1", cfgK3s.DefaultInstallScriptURL, args, ServerUnit, "")).
		NotTo(ContainSubstring("K3S_TOKEN"))
}

// A destroy has to succeed against a machine whose install never finished — which is
// the common case, since the resource being destroyed is often the one that did not
// come up. The installer writes the uninstaller, so its presence is the only honest
// test of whether k3s was ever here.
func TestUninstallScriptIsRoleAwareAndSucceedsOnAnUntouchedMachine(t *testing.T) {
	RegisterTestingT(t)

	Expect(UninstallScript(ServerUnit)).To(ContainSubstring("/usr/local/bin/k3s-uninstall.sh"))
	Expect(UninstallScript(AgentUnit)).To(ContainSubstring("/usr/local/bin/k3s-agent-uninstall.sh"))
	Expect(UninstallScript(AgentUnit)).NotTo(ContainSubstring("/usr/local/bin/k3s-uninstall.sh"))

	Expect(UninstallScript(ServerUnit)).To(ContainSubstring("exit 0"))
	// The marker lives in SC's own state dir, which the uninstaller knows nothing
	// about. Left behind, it makes the next install's skip condition compare against
	// the arguments of a cluster that no longer exists.
	Expect(UninstallScript(ServerUnit)).To(ContainSubstring("rm -f " + ArgsMarkerPath(ServerUnit)))
}

// The token file is written asynchronously after the server process starts, so a
// read that wins the race returns success and nothing — and every join built from it
// then fails as an authentication error against a cluster that is perfectly healthy.
func TestTokenReadScriptRefusesAnEmptyToken(t *testing.T) {
	RegisterTestingT(t)

	script := TokenReadScript()
	Expect(script).To(ContainSubstring(ServerTokenPath))
	Expect(script).To(ContainSubstring(`[ -n "$TOKEN" ]`))
	// No trailing newline: this value is interpolated into another node's install
	// command, where a stray newline is a token mismatch with no message saying so.
	Expect(script).To(ContainSubstring(`printf '%s' "$TOKEN"`))
	Expect(script).NotTo(ContainSubstring(`echo "$TOKEN"`))
}

func TestInstallScriptIsIdempotentOnConfigAndNotJustOnVersion(t *testing.T) {
	RegisterTestingT(t)

	script := InstallScript("v1.36.5+k3s1", cfgK3s.DefaultInstallScriptURL, "server --tls-san a", ServerUnit, "")

	// The three halves of the skip condition. Dropping any one of them turns a
	// re-provision into either a pointless control-plane restart (no condition) or a
	// silent no-op against a changed config (version only) — the second being the
	// one that reports green while doing nothing.
	Expect(script).To(ContainSubstring(`sudo systemctl is-active --quiet "$UNIT"`))
	Expect(script).To(ContainSubstring(`UNIT="` + ServerUnit + `"`))
	Expect(script).To(ContainSubstring(`sudo k3s --version 2>/dev/null | grep -qF "$WANT_VERSION"`))
	Expect(script).To(ContainSubstring(`[ "$(sudo cat ` + ArgsMarkerPath(ServerUnit) + `)" = "$WANT_ARGS" ]`))

	// The marker is written after the install, never before: a failed install must
	// not leave behind a claim that the machine is configured.
	installAt := strings.Index(script, "sudo env INSTALL_K3S_VERSION")
	markerAt := strings.Index(script, "| sudo tee "+ArgsMarkerPath(ServerUnit))
	Expect(installAt).To(BeNumerically(">", 0))
	Expect(markerAt).To(BeNumerically(">", installAt))

	// The pinned version has to reach the installer through its environment. If it
	// does not, the installer takes the LATEST release and the deploy still reports
	// success — a control-plane version nobody chose.
	Expect(script).To(ContainSubstring(`sudo env INSTALL_K3S_VERSION="$WANT_VERSION" sh ` + InstallerPath))
	Expect(script).To(ContainSubstring(`WANT_VERSION="v1.36.5+k3s1"`))

	// Downloaded, then run. A truncated transfer piped into `sh` executes the prefix
	// that arrived, and this installer's prefix stops the running service.
	Expect(script).To(ContainSubstring(`curl -fsSL "` + cfgK3s.DefaultInstallScriptURL + `" -o ` + InstallerPath))
	Expect(script).NotTo(ContainSubstring("| sh"))
	Expect(script).NotTo(ContainSubstring("| sudo sh"))
}

// The installer's idempotence is unit-file-shaped, not service-shaped: when the unit
// and env files it would write already match what is on disk it enables the unit,
// prints "No change detected so skipping service start" and exits 0. So on a machine
// whose k3s is stopped with an otherwise correct config — killed, k3s-killall'd,
// stopped by an operator — the install repairs nothing, reports success, and the
// readiness gate 300 s later is the first sign. Measured on the RU smoke stand,
// 2026-10-18.
//
// Reaching the start at all means the skip condition was false, so this costs a
// healthy cluster nothing; and it has to come BEFORE the args marker, or a machine
// that cannot start k3s ends up with a marker saying it is configured.
func TestInstallScriptStartsTheServiceTheInstallerDeclinedToStart(t *testing.T) {
	RegisterTestingT(t)

	script := InstallScript("v1.36.5+k3s1", cfgK3s.DefaultInstallScriptURL, "server --tls-san a", ServerUnit, "")

	Expect(script).To(ContainSubstring(`sudo systemctl enable --now "$UNIT"`))

	installAt := strings.Index(script, "sudo env INSTALL_K3S_VERSION")
	startAt := strings.Index(script, `sudo systemctl enable --now "$UNIT"`)
	markerAt := strings.Index(script, "| sudo tee "+ArgsMarkerPath(ServerUnit))
	Expect(startAt).To(BeNumerically(">", installAt),
		"the service start has to follow the installer, which writes the unit file it starts")
	Expect(markerAt).To(BeNumerically(">", startAt),
		"the args marker has to follow the start: a machine where k3s will not start must not "+
			"be left claiming it is configured")
}

func TestKubeconfigScriptAssertsTheRewriteHappened(t *testing.T) {
	RegisterTestingT(t)

	script := KubeconfigScript("k8s.example.ru")
	wantURL := "https://k8s.example.ru:6443"

	Expect(script).To(ContainSubstring("sed \"s|" + LocalServerURL + "|" + wantURL + "|\""))

	// Both assertions, and this is the whole point of the function. `sed` exits 0
	// when it matches nothing, so without them a cluster whose kubeconfig k3s wrote
	// differently (or not yet) exports a kubeconfig addressed to 127.0.0.1 — which
	// passes every test, works on the node, and fails only in the child stack's
	// deploy as a connection refused to localhost from a CI runner.
	Expect(script).To(ContainSubstring(`grep -qF "`+LocalServerURL+`"`),
		"must assert the local URL is present BEFORE rewriting it")
	Expect(script).To(ContainSubstring(`grep -qF "`+wantURL+`"`),
		"must assert the rewritten URL is present AFTER rewriting")

	// The failure path prints the `server:` line and nothing else: the rest of the
	// document is a client certificate and its key.
	Expect(script).To(ContainSubstring(`grep -E '^ *server:'`))
	Expect(script).NotTo(ContainSubstring(`echo "$KC"` + "\n"))
}

func TestNodeReadyScriptDoesNotAcceptNotReady(t *testing.T) {
	RegisterTestingT(t)

	script := NodeReadyScript(1)

	// "NotReady" contains "Ready", so a grep for Ready alone passes on a cluster
	// whose only node is NotReady — i.e. the gate would open on exactly the state it
	// exists to wait out. The count below is anchored for the same reason.
	Expect(script).To(ContainSubstring(`! echo "$STATUS" | grep -q NotReady`))
	Expect(script).To(ContainSubstring(`grep -c '^Ready'`))
	// An empty node list is not readiness either: `kubectl get nodes` against an API
	// server that is up but has no registered node prints nothing and exits 0.
	Expect(script).To(ContainSubstring(`[ -n "$STATUS" ]`))
	// Diagnostics on the failure path. Without them a timeout here is a bare
	// non-zero exit from a remote command, which says nothing about why.
	Expect(script).To(ContainSubstring("journalctl -u k3s"))
}

// The count is what makes it a FLEET gate rather than a node gate. Without it the
// script passes the instant the first server registers, which on a multi-node cluster
// is before any agent has joined — so a deploy ordered after this gate schedules its
// pods onto a cluster of one machine. "No node is NotReady" does not catch it: a node
// that has not registered yet is not NotReady, it is absent.
func TestNodeReadyScriptWaitsForTheWholeFleet(t *testing.T) {
	RegisterTestingT(t)

	script := NodeReadyScript(3)

	Expect(script).To(ContainSubstring(`WANT=3`))
	Expect(script).To(ContainSubstring(`[ "$READY" -ge "$WANT" ]`))
}

// The kubeconfig export name is the one string in this package that has to match
// another package byte for byte: every consumer of a cluster
// (`kubernetes-cloudrun`, the Caddy edge, the helm operators) reads the name
// gcp-gke-autopilot invented. A divergence here is a cluster whose kubeconfig
// nothing can find, and it surfaces in the child stack's deploy as a missing stack
// output rather than in the parent provision that caused it.
func TestKubeconfigExportNameMatchesTheGkeContract(t *testing.T) {
	RegisterTestingT(t)

	Expect(pulumiKubernetes.ToKubeconfigExport("my-cluster")).To(Equal("my-cluster-kubeconfig"))
	// The second export of the pair. A child stack reads it only when one of its
	// workloads declares a `domain`, which is why its absence is the kind of thing
	// that ships green and surfaces weeks later in somebody else's deploy.
	Expect(pulumiKubernetes.ToIngressIpExport("my-cluster")).To(Equal("my-cluster-ingress-ip"))
}
