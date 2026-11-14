// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package cmd_secrets

import (
	"strings"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"
)

// clearAgentEnv unsets every marker so a test starts from a "human session"
// baseline — the suite itself is very likely running under one of these.
func clearAgentEnv(t *testing.T) {
	t.Helper()
	for _, m := range agentEnvMarkers {
		t.Setenv(m.env, "")
	}
}

func TestDetectAgentSession(t *testing.T) {
	for _, tc := range []struct {
		name     string
		env      map[string]string
		wantName string
		wantVia  string
	}{
		{name: "no markers", env: nil},
		{name: "CLAUDECODE=1", env: map[string]string{"CLAUDECODE": "1"}, wantName: "Claude Code", wantVia: "CLAUDECODE"},
		{
			name:     "entrypoint only",
			env:      map[string]string{"CLAUDE_CODE_ENTRYPOINT": "cli"},
			wantName: "Claude Code",
			wantVia:  "CLAUDE_CODE_ENTRYPOINT",
		},
		{
			name:     "session id only",
			env:      map[string]string{"CLAUDE_CODE_SESSION_ID": "55e90ee4"},
			wantName: "Claude Code",
			wantVia:  "CLAUDE_CODE_SESSION_ID",
		},
		// An explicitly falsy marker is an opt-out, not a detection.
		{name: "CLAUDECODE=0", env: map[string]string{"CLAUDECODE": "0"}},
		{name: "CLAUDECODE=false", env: map[string]string{"CLAUDECODE": "FALSE"}},
		{name: "CLAUDECODE blank", env: map[string]string{"CLAUDECODE": "   "}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			clearAgentEnv(t)
			for k, v := range tc.env {
				t.Setenv(k, v)
			}

			agent, via := detectAgentSession()
			Expect(agent).To(Equal(tc.wantName))
			Expect(via).To(Equal(tc.wantVia))
		})
	}
}

func TestRefuseInteractiveUnderAgent(t *testing.T) {
	newCmd := func() *cobra.Command {
		root := &cobra.Command{Use: "sc"}
		secrets := &cobra.Command{Use: "secrets"}
		reveal := &cobra.Command{Use: "reveal"}
		secrets.AddCommand(reveal)
		root.AddCommand(secrets)
		return reveal
	}

	t.Run("refuses under an agent without the force flag", func(t *testing.T) {
		RegisterTestingT(t)
		clearAgentEnv(t)
		t.Setenv("CLAUDECODE", "1")

		err := refuseInteractiveUnderAgent(newCmd(), "--force", false)
		Expect(err).To(HaveOccurred())
		// The message has to carry the remedy verbatim: it is the only thing
		// the agent sees, and a runnable command is what makes the retry right.
		Expect(err.Error()).To(ContainSubstring("Claude Code"))
		Expect(err.Error()).To(ContainSubstring("CLAUDECODE"))
		Expect(err.Error()).To(ContainSubstring("sc secrets reveal --force"))
	})

	t.Run("allows under an agent once the force flag is set", func(t *testing.T) {
		RegisterTestingT(t)
		clearAgentEnv(t)
		t.Setenv("CLAUDECODE", "1")

		Expect(refuseInteractiveUnderAgent(newCmd(), "--force", true)).To(Succeed())
	})

	t.Run("allows a human session either way", func(t *testing.T) {
		RegisterTestingT(t)
		clearAgentEnv(t)

		Expect(refuseInteractiveUnderAgent(newCmd(), "--force", false)).To(Succeed())
		Expect(refuseInteractiveUnderAgent(newCmd(), "--force", true)).To(Succeed())
	})
}

// TestSecretsCommandsGuardInteractiveMode runs `add` and `reveal` through cobra
// under an agent session and asserts they fail before touching the provisioner
// — sCmd.Root is nil here, so reaching Cryptor() at all would panic.
func TestSecretsCommandsGuardInteractiveMode(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{name: "reveal", args: []string{"secrets", "reveal"}},
		{name: "add", args: []string{"secrets", "add", "stacks/common/secrets.yaml"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			clearAgentEnv(t)
			t.Setenv("CLAUDECODE", "1")

			sCmd := &secretsCmd{}
			secrets := &cobra.Command{Use: "secrets", SilenceUsage: true, SilenceErrors: true}
			secrets.AddCommand(NewAddCmd(sCmd), NewRevealCmd(sCmd))
			root := &cobra.Command{Use: "sc", SilenceUsage: true, SilenceErrors: true}
			root.AddCommand(secrets)
			root.SetArgs(tc.args)
			root.SetOut(&strings.Builder{})
			root.SetErr(&strings.Builder{})

			err := root.Execute()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("refusing to run"))
			Expect(err.Error()).To(ContainSubstring("--force"))
		})
	}
}
