// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package vm

import (
	"strings"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"

	"github.com/simple-container-com/api/pkg/clouds/vm"
)

// The ingress SCRIPTS were covered from the first commit (caddy_scripts_test.go); the
// resource WIRING was not, and that is the whole of why the fault below shipped. These
// tests assert the shape of the commands rather than the text of the scripts.

func ingressConfig() *vm.VmConfig {
	cfg := byoConfig()
	cfg.Ingress = &vm.VmIngress{
		Enable: true,
		Email:  "ops@example.com",
	}
	return cfg
}

// ingressCommands maps each ingress command to its inputs, keyed by the suffix of its
// resource name ("ingress-up", "ingress-teardown", …) rather than the full name, which
// carries the resource and environment.
func ingressCommands(m *vmMocks) map[string]resource.PropertyMap {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[string]resource.PropertyMap{}
	for _, a := range m.created {
		if a.TypeToken != tokenRemoteCommand {
			continue
		}
		if i := strings.Index(a.Name, "ingress-"); i >= 0 {
			name := a.Name[i:]
			if j := strings.Index(name, "--"); j >= 0 {
				name = name[:j]
			}
			out[name] = a.Inputs
		}
	}
	return out
}

// The fault this test exists for, in full, because the shape of it is the lesson:
//
// `ingress-up` carried `Delete: ingressTeardownScript()` and a trigger that moves with
// the configuration. pulumi-command performs a replacement as create-then-delete, so a
// provision that changed one character of `ingress.email` brought Caddy up, logged
// "ingress is up", and two seconds later ran the teardown — container removed,
// /etc/sc/caddy removed, nothing listening on 443 — and exited 0. Re-running the
// provision could not repair it: with the config now unchanged, every ingress command
// was up-to-date and skipped.
//
// Live-caught 2026-10-18 on the RU smoke stand. Nothing in the unit tests of the time
// could have caught it, because none of them looked at the commands.
func TestIngress_TeardownIsNotOnAReplaceableCommand(t *testing.T) {
	RegisterTestingT(t)

	mocks := newVmMocks()
	_, err := provisionVm(ingressConfig(), mocks)
	Expect(err).To(BeNil())

	cmds := ingressCommands(mocks)
	Expect(cmds).To(HaveKey("ingress-up"))
	Expect(cmds).To(HaveKey("ingress-teardown"),
		"the ingress teardown has to live on a command of its own; see the comment above")

	for name, inputs := range cmds {
		_, hasDelete := inputs[resource.PropertyKey("delete")]
		triggers, _ := unwrapSecret(inputs["triggers"])
		hasTriggers := triggers.IsArray() && len(triggers.ArrayValue()) > 0
		Expect(hasDelete && hasTriggers).To(BeFalse(),
			"command %q has both a Delete script and triggers: Pulumi replaces it as "+
				"create-then-delete, so the next change to its inputs will tear down what it "+
				"just created, and report success", name)
	}

	teardown := cmds["ingress-teardown"]
	del, _ := unwrapSecret(teardown["delete"])
	Expect(del.StringValue()).To(Equal(ingressTeardownScript()))
	Expect(teardown).ToNot(HaveKey(resource.PropertyKey("triggers")),
		"a teardown command with triggers can be replaced, and a replacement runs its Delete")
	create, _ := unwrapSecret(teardown["create"])
	Expect(create.StringValue()).To(Equal("true"),
		"the teardown command exists to hold a Delete; its Create must do nothing")
}

// Every ingress command repairs the machine it runs against, so every one of them is
// triggered per run. The three preconditions for that (see TriggersForGuardedChange)
// are the subject of the test above and of the script tests; this one is the
// consequence, and it is the half that a green `sc provision` hid for a day: after the
// teardown fault removed /etc/sc/caddy, a provision with an unchanged config reported
// every ingress command up-to-date and left the machine with no TLS on it.
func TestIngress_EveryCommandRunsEveryProvision(t *testing.T) {
	RegisterTestingT(t)

	mocks := newVmMocks()
	_, err := provisionVm(ingressConfig(), mocks)
	Expect(err).To(BeNil())

	cmds := ingressCommands(mocks)
	for _, name := range []string{"ingress-config", "ingress-env", "ingress-up"} {
		Expect(cmds).To(HaveKey(name))
		triggers, _ := unwrapSecret(cmds[name]["triggers"])
		Expect(triggers.String()).To(ContainSubstring(runNonce),
			"%q is not triggered per run, so a provision cannot repair an ingress that was "+
				"removed or stopped between deploys — it reports green beside it", name)

		// With a per-run trigger, the absence of Update is no longer a latent
		// inconvenience: it would replace the resource on every single provision.
		update, _ := unwrapSecret(cmds[name]["update"])
		create, _ := unwrapSecret(cmds[name]["create"])
		Expect(update.StringValue()).To(Equal(create.StringValue()),
			"%q has no explicit Update equal to its Create", name)
	}
}

// The env file holds the ACME External Account Binding key when there is one. Two
// things have to be true at once, and the triggers are where they conflict: the
// command must re-run every provision (above), and neither the value nor anything
// derived from it may be in the trigger set, which Pulumi stores and prints in a diff.
func TestIngress_EnvFileTriggersOnItsHashAndLogsNothing(t *testing.T) {
	RegisterTestingT(t)

	const mac = "eab-mac-key-value"

	cfg := ingressConfig()
	// EAB only validates alongside a non-default CA, which is the whole case it
	// exists for: a private or enterprise ACME endpoint.
	cfg.Ingress.AcmeCa = "https://acme.example.ru/directory"
	cfg.Ingress.AcmeEabKeyID = "eab-key-id"
	cfg.Ingress.AcmeEabMacKey = mac

	mocks := newVmMocks()
	_, err := provisionVm(cfg, mocks)
	Expect(err).To(BeNil())

	env := ingressCommands(mocks)["ingress-env"]
	Expect(env).ToNot(BeNil())

	triggers, _ := unwrapSecret(env["triggers"])
	Expect(triggers.String()).ToNot(ContainSubstring(mac),
		"the EAB MAC key reached the triggers, which are stored in the clear in the checkpoint")
	create, _ := unwrapSecret(env["create"])
	Expect(create.StringValue()).ToNot(ContainSubstring(mac),
		"the EAB MAC key reached the command string")

	stdin, secret := unwrapSecret(env["stdin"])
	Expect(stdin.StringValue()).To(ContainSubstring(mac),
		"the env file content has to arrive on stdin")
	Expect(secret).To(BeTrue(), "the stdin of the env file command is not marked secret")

	logging, _ := unwrapSecret(env["logging"])
	Expect(logging.StringValue()).To(Equal("none"))
}
