// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package cmd_secrets

import (
	"os"
	"strings"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

// Both `secrets add` and `secrets reveal` reach
// secrets.cryptor.ensureDiffAcceptable, which prints a line-by-line diff of the
// PLAINTEXT secret file to the console and then blocks on
// `do you accept changes [Y/N]?`. Inside a coding-agent session that is wrong
// twice over:
//
//   - the diff writes the secret values straight into the agent's transcript,
//     which is persisted on disk and shipped to a model provider — a `reveal`
//     meant to land the plaintext in a gitignored file instead publishes it;
//   - no human is on the other end of the prompt, so after three empty reads
//     the command fails anyway, having already leaked the values.
//
// The force flag makes ensureDiffAcceptable a no-op — it returns before
// printing anything — so the non-interactive path is always available. Refuse
// up front when an agent is detected and name the flag in the error, so the
// agent's next attempt is the correct one.

// agentEnvMarkers maps an environment variable only a coding agent sets to the
// agent it identifies. The Claude Code CLI exports all three into every shell
// it spawns; any one of them is enough, and checking several means a future
// release dropping one does not silently disable the guard.
var agentEnvMarkers = []struct {
	env  string
	name string
}{
	{env: "CLAUDECODE", name: "Claude Code"},
	{env: "CLAUDE_CODE_ENTRYPOINT", name: "Claude Code"},
	{env: "CLAUDE_CODE_SESSION_ID", name: "Claude Code"},
}

// detectAgentSession reports the coding agent running sc and the environment
// variable that gave it away. Both results are empty when sc appears to be
// running under a human.
func detectAgentSession() (agent, via string) {
	for _, m := range agentEnvMarkers {
		// Treat an explicitly falsy value as absent so a wrapper can opt out
		// with CLAUDECODE=0 rather than having to unset it.
		switch v := strings.TrimSpace(os.Getenv(m.env)); {
		case v == "", v == "0", strings.EqualFold(v, "false"):
			continue
		default:
			return m.name, m.env
		}
	}
	return "", ""
}

// refuseInteractiveUnderAgent returns an error when cmd would prompt for secret
// changes while running under a coding agent. force reflects the command's
// non-interactive flag (named by forceFlag): when it is set the diff is never
// printed and no prompt happens, so the guard stands down.
func refuseInteractiveUnderAgent(cmd *cobra.Command, forceFlag string, force bool) error {
	if force {
		return nil
	}
	agent, via := detectAgentSession()
	if agent == "" {
		return nil
	}
	return errors.Errorf(
		"refusing to run %q in interactive mode: %s session detected (%s is set). "+
			"Interactive mode prints a diff of the decrypted secret file to the console and then waits "+
			"for \"do you accept changes [Y/N]?\" — that exposes secret values in the agent transcript "+
			"and stalls on a prompt no one can answer. "+
			"Re-run non-interactively: %s %s",
		cmd.CommandPath(), agent, via, cmd.CommandPath(), forceFlag,
	)
}
