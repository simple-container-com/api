// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package k3s

import (
	"strings"
	"testing"

	. "github.com/onsi/gomega"

	cfgK3s "github.com/simple-container-com/api/pkg/clouds/k3s"
	pulumiKubernetes "github.com/simple-container-com/api/pkg/clouds/pulumi/kubernetes"
)

func TestServerArgs(t *testing.T) {
	RegisterTestingT(t)

	args := ServerArgs(
		&cfgK3s.K3sClusterConfig{ExtraServerArgs: []string{"--kube-apiserver-arg=v=2"}},
		[]string{"k8s.example.ru", "203.0.113.10"},
		[]string{"traefik", "servicelb"},
	)

	Expect(args).To(Equal("server " +
		"--tls-san k8s.example.ru --tls-san 203.0.113.10 " +
		"--disable traefik --disable servicelb " +
		"--kube-apiserver-arg=v=2"))
	// `server` first: the installer reads the subcommand off the front of the vector
	// it is given, and an install with the flags but no subcommand starts an agent.
	Expect(strings.HasPrefix(args, "server ")).To(BeTrue())
}

func TestInstallScriptIsIdempotentOnConfigAndNotJustOnVersion(t *testing.T) {
	RegisterTestingT(t)

	script := InstallScript("v1.36.5+k3s1", cfgK3s.DefaultInstallScriptURL, "server --tls-san a")

	// The three halves of the skip condition. Dropping any one of them turns a
	// re-provision into either a pointless control-plane restart (no condition) or a
	// silent no-op against a changed config (version only) — the second being the
	// one that reports green while doing nothing.
	Expect(script).To(ContainSubstring(`sudo systemctl is-active --quiet k3s`))
	Expect(script).To(ContainSubstring(`sudo k3s --version 2>/dev/null | grep -qF "$WANT_VERSION"`))
	Expect(script).To(ContainSubstring(`[ "$(sudo cat ` + ArgsMarkerPath + `)" = "$WANT_ARGS" ]`))

	// The marker is written after the install, never before: a failed install must
	// not leave behind a claim that the machine is configured.
	installAt := strings.Index(script, "sudo env INSTALL_K3S_VERSION")
	markerAt := strings.Index(script, "| sudo tee "+ArgsMarkerPath)
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

	script := NodeReadyScript()

	// "NotReady" contains "Ready", so a grep for Ready alone passes on a cluster
	// whose only node is NotReady — i.e. the gate would open on exactly the state it
	// exists to wait out.
	Expect(script).To(ContainSubstring(`! echo "$STATUS" | grep -q NotReady`))
	Expect(script).To(ContainSubstring(`echo "$STATUS" | grep -q Ready`))
	// An empty node list is not readiness either: `kubectl get nodes` against an API
	// server that is up but has no registered node prints nothing and exits 0.
	Expect(script).To(ContainSubstring(`[ -n "$STATUS" ]`))
	// Diagnostics on the failure path. Without them a timeout here is a bare
	// non-zero exit from a remote command, which says nothing about why.
	Expect(script).To(ContainSubstring("journalctl -u k3s"))
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
}
