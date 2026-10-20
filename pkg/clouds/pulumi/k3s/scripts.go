// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package k3s

import (
	"fmt"
	"strings"

	"github.com/simple-container-com/api/pkg/clouds/k3s"
	"github.com/simple-container-com/api/pkg/clouds/vm"
)

const (
	// ArgsMarkerPath records the server arguments SC installed with, on the machine.
	//
	// It exists because the version alone is not enough to decide whether an install
	// can be skipped. The remote half of this path has to be idempotent — a
	// re-provision must not restart a working API server — but a guard that only
	// compares versions would also skip an install whose `--tls-san` or `--disable`
	// set had changed, and the operator would be looking at a cluster that does not
	// match its own config with every layer reporting success. Comparing the full
	// argument string makes "nothing to do" and "reconfigure" distinguishable
	// remotely, which is the only place the question can be answered.
	ArgsMarkerPath = vm.StateDir + "/k3s-server.args"

	// InstallerPath is where the install script is downloaded before it is run.
	// Downloaded and then executed, rather than piped into a shell: a truncated
	// transfer piped into `sh` executes the prefix that arrived, and the prefix of
	// this installer is the part that stops the running service.
	InstallerPath = "/tmp/sc-k3s-install.sh"

	// KubeconfigPath is where k3s writes the admin kubeconfig. Mode 0600, root-owned.
	KubeconfigPath = "/etc/rancher/k3s/k3s.yaml"

	// LocalServerURL is the `server:` URL k3s puts in the kubeconfig it writes. It is
	// correct on the node and useless anywhere else, which is what the rewrite below
	// is for.
	LocalServerURL = "https://127.0.0.1:6443"

	// nodeReadyAttempts × nodeReadyInterval = 300s. A single-node k3s reaches Ready in
	// well under a minute on a warm image; the slack is for a first boot that is also
	// pulling the packaged component images.
	nodeReadyAttempts = 60
	nodeReadyInterval = 5
)

// ServerArgs is the full argument vector SC installs the k3s server with, starting
// with the `server` subcommand. Rendered as one string because that is both what the
// installer takes and what ArgsMarkerPath compares.
func ServerArgs(cfg *k3s.K3sClusterConfig, sans []string, disable []string) string {
	args := []string{"server"}
	for _, san := range sans {
		args = append(args, "--tls-san", san)
	}
	for _, c := range disable {
		args = append(args, "--disable", c)
	}
	args = append(args, cfg.ExtraServerArgs...)
	return strings.Join(args, " ")
}

// InstallScript is the remote half of standing up the control plane.
//
// Everything it does is conditional on the machine not already being in the wanted
// state, and the condition is checked on the machine: Pulumi's own Triggers decide
// whether the command RUNS, this decides whether running it changes anything. Both
// are needed — without Triggers a changed script never runs at all, and without the
// marker a re-run with an unchanged config would restart the API server of a live
// cluster for nothing.
func InstallScript(version, installURL, serverArgs string) string {
	return strings.Join([]string{
		`set -eu`,
		fmt.Sprintf(`WANT_VERSION=%q`, version),
		fmt.Sprintf(`WANT_ARGS=%q`, serverArgs),
		// The skip condition, spelled out: the service is running, the binary is the
		// wanted release, and the arguments on disk are the ones this config asks
		// for. Anything else falls through to a (re)install.
		fmt.Sprintf(`if sudo systemctl is-active --quiet k3s && `+
			`sudo k3s --version 2>/dev/null | grep -qF "$WANT_VERSION" && `+
			`[ -f %[1]s ] && [ "$(sudo cat %[1]s)" = "$WANT_ARGS" ]; then`, ArgsMarkerPath),
		`  echo "k3s $WANT_VERSION already running with the requested configuration"`,
		`  exit 0`,
		`fi`,
		fmt.Sprintf(`curl -fsSL %q -o %s`, installURL, InstallerPath),
		// `sudo env VAR=…` rather than `VAR=… sudo` or a pipe into `sudo sh`: the
		// first form is the only one that reliably survives sudo's env_reset, and
		// INSTALL_K3S_VERSION not arriving means the installer quietly takes the
		// latest release instead of the pinned one.
		//
		// $WANT_ARGS is deliberately unquoted: it is an argument VECTOR and the
		// installer forwards it to the service unit word by word. SC's own elements
		// are validated bare tokens (Validate refuses a SAN or a component name that
		// is anything else); `extraServerArgs` is word-split on spaces, which is the
		// documented behaviour of a pass-through field and the reason each entry
		// should be one flag.
		fmt.Sprintf(`sudo env INSTALL_K3S_VERSION="$WANT_VERSION" sh %s $WANT_ARGS`, InstallerPath),
		fmt.Sprintf(`rm -f %s`, InstallerPath),
		// The installer does NOT guarantee a running service, and this is the line
		// that cost a provision: get.k3s.io diffs the unit and env files it would
		// write against what is on disk and, when they match, enables the unit and
		// prints
		//
		//	[INFO]  No change detected so skipping service start
		//
		// then exits 0. Its idempotence is unit-file-shaped, not service-shaped — so
		// against a machine whose k3s was stopped, killed or k3s-killall'd with an
		// otherwise correct config, the installer repairs nothing and reports
		// success, and the readiness gate 300 s later is the first thing that says
		// so. `enable --now` is the repair: a no-op on a running unit, a start on a
		// stopped one, and it cannot mask a genuine failure because `set -e` takes
		// the script down before the args marker below is written.
		//
		// Reaching this line at all means the skip condition above was false, i.e.
		// something really is wrong — on a first install the installer has already
		// started k3s and this is the no-op case.
		//
		// Live-caught 2026-10-18 on the RU smoke stand: the install re-ran (the
		// per-run trigger worked), printed the line above, wrote the marker, exited
		// 0 — and k3s stayed down, with nothing in its journal between the stop two
		// hours earlier and the failed gate.
		`sudo systemctl enable --now k3s`,
		// Written only after a successful install, so a failed one does not leave a
		// marker claiming the machine is configured.
		fmt.Sprintf(`printf '%%s' "$WANT_ARGS" | sudo tee %s >/dev/null`, ArgsMarkerPath),
		`echo "installed k3s $WANT_VERSION"`,
	}, "\n")
}

// KubeconfigScript prints the admin kubeconfig with its `server:` URL pointed at the
// endpoint the cluster is actually reached on.
//
// The assertions around the rewrite are the point of this function. `sed` reports
// success when it matches nothing, so the straightforward version of this script
// exports a kubeconfig addressed to 127.0.0.1 — which works on the node, works in
// every unit test, and fails only in the child stack's deploy, as a connection
// refused to localhost from a CI runner. Checking both the input and the output turns
// that into a failure here, naming the file (k3s-all-clouds.md §4).
func KubeconfigScript(endpoint string) string {
	wantURL := k3s.KubeconfigURL(endpoint)
	return strings.Join([]string{
		`set -eu`,
		fmt.Sprintf(`test -f %[1]s || { echo "%[1]s does not exist: the k3s server did not start" >&2; exit 1; }`,
			KubeconfigPath),
		fmt.Sprintf(`KC=$(sudo cat %s)`, KubeconfigPath),
		// Only the `server:` line is echoed on failure — the rest of the document is
		// a client certificate and a key.
		fmt.Sprintf(`echo "$KC" | grep -qF %q || { `+
			`echo "unexpected API server URL in the generated kubeconfig: $(echo "$KC" | grep -E '^ *server:')" >&2; `+
			`exit 1; }`, LocalServerURL),
		fmt.Sprintf(`OUT=$(echo "$KC" | sed %q)`, fmt.Sprintf("s|%s|%s|", LocalServerURL, wantURL)),
		fmt.Sprintf(`echo "$OUT" | grep -qF %q || { echo "failed to point the kubeconfig at %s" >&2; exit 1; }`,
			wantURL, wantURL),
		`printf '%s\n' "$OUT"`,
	}, "\n")
}

// NodeReadyScript waits until the cluster's own API says every node is Ready.
//
// Why this exists next to the install command: `systemctl is-active k3s` is true the
// moment the process starts, seconds before the API server serves and well before the
// node registers. A template that deployed on `is-active` would race the first
// `kubectl apply` against a cluster with no node, which fails as an unschedulable pod
// rather than as a readiness problem — the same class of fault as a deploy that waits
// for SSH and then finds no docker (vm-matrix.md §3.4).
func NodeReadyScript() string {
	return strings.Join([]string{
		`set -eu`,
		fmt.Sprintf(`for i in $(seq 1 %d); do`, nodeReadyAttempts),
		`  STATUS=$(sudo k3s kubectl get nodes --no-headers 2>/dev/null | awk '{print $2}' || true)`,
		// NotReady is excluded explicitly because it CONTAINS "Ready": a grep for
		// Ready alone passes on a cluster whose only node is NotReady.
		`  if [ -n "$STATUS" ] && ! echo "$STATUS" | grep -q NotReady && echo "$STATUS" | grep -q Ready; then`,
		`    echo "all nodes ready"`,
		`    exit 0`,
		`  fi`,
		fmt.Sprintf(`  sleep %d`, nodeReadyInterval),
		`done`,
		fmt.Sprintf(`echo "no k3s node became Ready within %ds" >&2`, nodeReadyAttempts*nodeReadyInterval),
		`sudo k3s kubectl get nodes -o wide >&2 || true`,
		`sudo systemctl status k3s --no-pager >&2 || true`,
		`sudo journalctl -u k3s --no-pager -n 50 >&2 || true`,
		`exit 1`,
	}, "\n")
}
