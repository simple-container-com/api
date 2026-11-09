// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package k3s

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/onsi/gomega"

	cfgK3s "github.com/simple-container-com/api/pkg/clouds/k3s"
	"github.com/simple-container-com/api/pkg/clouds/vm"
)

// runSh runs a script under /bin/sh and returns its combined output and exit code.
// Shelling out is the point: a generated script is only ever executed by a shell,
// so a Go test comparing strings cannot tell a working retry from one that is a
// syntax error or an infinite loop.
func runSh(t *testing.T, script string) (string, int) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "script.sh")
	if err := os.WriteFile(path, []byte(script), 0o600); err != nil {
		t.Fatalf("writing script: %v", err)
	}
	out, err := exec.Command("/bin/sh", path).CombinedOutput()
	code := 0
	var exit *exec.ExitError
	if err != nil {
		if ok := asExitError(err, &exit); !ok {
			t.Fatalf("running script: %v\n%s", err, out)
		}
		code = exit.ExitCode()
	}
	return string(out), code
}

func asExitError(err error, target **exec.ExitError) bool {
	e, ok := err.(*exec.ExitError)
	if ok {
		*target = e
	}
	return ok
}

// A command that fails and then succeeds must succeed overall, and must say so:
// an operator whose network needed three attempts has no other signal that it did.
func TestScRetrySucceedsAfterTransientFailures(t *testing.T) {
	RegisterTestingT(t)

	// `flaky` fails twice by counting attempts in a file, then succeeds. sc_wait is
	// overridden after the function is defined so the test does not sleep 15s.
	out, code := runSh(t, strings.Join([]string{
		"set -eu",
		scRetryFunc,
		"COUNT=0",
		"flaky() { COUNT=$((COUNT + 1)); [ \"$COUNT\" -ge 3 ]; }",
		// Re-define sleep as a no-op: the backoff is the thing under test, not the
		// wall clock.
		"sleep() { :; }",
		"sc_retry flaky",
		"echo \"attempts=$COUNT\"",
	}, "\n"))

	Expect(code).To(Equal(0))
	Expect(out).To(ContainSubstring("attempts=3"))
	Expect(out).To(ContainSubstring("sc: attempt 1 of 5 failed, retrying in 5s"))
	Expect(out).To(ContainSubstring("sc: attempt 2 of 5 failed, retrying in 10s"))
	Expect(out).ToNot(ContainSubstring("giving up"))
}

// The loop is bounded, and the bound is the thing that makes it safe to wrap a
// download in: a genuinely unreachable host fails the provision rather than
// hanging until the command's timeout.
func TestScRetryGivesUpAfterTheBound(t *testing.T) {
	RegisterTestingT(t)

	out, code := runSh(t, strings.Join([]string{
		"set -eu",
		scRetryFunc,
		"COUNT=0",
		"always_fails() { COUNT=$((COUNT + 1)); return 7; }",
		"sleep() { :; }",
		"if sc_retry always_fails; then echo UNEXPECTED_SUCCESS; fi",
		"echo \"attempts=$COUNT\"",
	}, "\n"))

	Expect(code).To(Equal(0)) // the `if` consumes the failure
	Expect(out).ToNot(ContainSubstring("UNEXPECTED_SUCCESS"))
	Expect(out).To(ContainSubstring("attempts=5"))
	Expect(out).To(ContainSubstring("sc: giving up after 5 attempts"))
}

// Under `set -eu` — which every generated script sets — an exhausted retry has to
// take the script down. Otherwise the install would fall through to
// `systemctl enable --now` and the args marker, and write a marker claiming a
// machine is configured with a binary that never downloaded.
func TestScRetryExhaustionFailsTheScriptUnderSetE(t *testing.T) {
	RegisterTestingT(t)

	out, code := runSh(t, strings.Join([]string{
		"set -eu",
		scRetryFunc,
		"sleep() { :; }",
		"sc_retry false",
		"echo REACHED_THE_NEXT_LINE",
	}, "\n"))

	Expect(code).ToNot(Equal(0))
	Expect(out).ToNot(ContainSubstring("REACHED_THE_NEXT_LINE"))
}

// Every script SC sends to a machine has to parse, and the args vector travelling
// through sc_retry unquoted is the part most likely to break that. `sh -n` is a
// parse-only check, so it is safe to run against a script that would install k3s.
func TestGeneratedScriptsParseUnderSh(t *testing.T) {
	RegisterTestingT(t)

	cfg := &cfgK3s.K3sClusterConfig{ExtraServerArgs: []string{"--kube-apiserver-arg=v=2"}}
	serverArgs := ServerArgs(cfg, []string{"k8s.example.ru", "10.88.0.1"}, []string{"traefik"},
		JoinSpec{ClusterInit: true, NodeIP: "10.88.0.1"})
	agentArgs := AgentArgs(JoinSpec{ServerURL: "https://10.88.0.1:6443", NodeIP: "10.88.0.2"})

	url := cfgK3s.DefaultInstallScriptURL
	for name, script := range map[string]string{
		"server install":         InstallScript("v1.36.5+k3s1", url, serverArgs, ServerUnit, ""),
		"joining server install": InstallScript("v1.36.5+k3s1", url, serverArgs, ServerUnit, "K10token::server:secret"),
		"agent install":          InstallScript("v1.36.5+k3s1", url, agentArgs, AgentUnit, "K10token::server:secret"),
		"uninstall server":       UninstallScript(UnitFor(vm.RoleK3sServer)),
		"uninstall agent":        UninstallScript(UnitFor(vm.RoleK3sAgent)),
		"kubeconfig read":        KubeconfigScript("10.88.0.1"),
		"node readiness":         NodeReadyScript(3),
		"join token read":        TokenReadScript(),
	} {
		t.Run(name, func(t *testing.T) {
			RegisterTestingT(t)
			path := filepath.Join(t.TempDir(), "s.sh")
			Expect(os.WriteFile(path, []byte(script), 0o600)).To(Succeed())
			out, err := exec.Command("/bin/sh", "-n", path).CombinedOutput()
			Expect(err).ToNot(HaveOccurred(), "%s does not parse: %s", name, out)
		})
	}
}

// The two steps that reach the network are the ones wrapped, and the retry
// definition precedes them — a script that called sc_retry before defining it
// would fail on the first install of every cluster.
func TestInstallScriptRetriesBothDownloadBearingSteps(t *testing.T) {
	RegisterTestingT(t)

	script := InstallScript("v1.36.5+k3s1", cfgK3s.DefaultInstallScriptURL, "server", ServerUnit, "")

	Expect(script).To(ContainSubstring("sc_retry curl -fsSL"))
	Expect(script).To(ContainSubstring("sc_retry sudo env INSTALL_K3S_VERSION="))
	Expect(strings.Index(script, "sc_retry()")).To(BeNumerically("<", strings.Index(script, "sc_retry curl")))
	// The skip condition must stay ahead of both, so a machine already in the
	// wanted state still makes no network request at all.
	Expect(strings.Index(script, "already running as")).To(BeNumerically("<", strings.Index(script, "sc_retry curl")))
}
