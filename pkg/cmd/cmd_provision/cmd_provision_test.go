// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package cmd_provision

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/simple-container-com/api/pkg/cmd/root_cmd"
)

// Running `sc provision -e <env>` must FAIL with a sentence that
// explains parent stacks are not per-env — before any work runs. The
// alternative (accept-and-ignore) silently papers over a mental-model
// mismatch; the recipe caller then watches the next `sc deploy` fail
// for an unrelated reason and spends 20 minutes on the wrong hypothesis.
// From sc-cli-unattended-gaps 2026-10-24 (Cloud.ru smoke: every recipe
// written from a deploy example reaches for -e).
func TestProvision_EnvFlagIsRejectedWithClearMessage(t *testing.T) {
	cmd := NewProvisionCmd(&root_cmd.RootCmd{})
	cmd.SetArgs([]string{"-e", "staging", "-s", "irrelevant"})
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true

	err := cmd.Execute()
	require.Error(t, err)
	// The message has to carry the mechanism ("parent stacks are not
	// per-env") AND the fix ("drop the flag") in the same error, so
	// the operator does not have to grep docs after the failure.
	for _, want := range []string{"not applicable", "parent", "not per-env", "sc provision -s"} {
		require.True(t, strings.Contains(err.Error(), want),
			"error %q missing marker %q", err, want)
	}
}

// Long form of -e must behave identically — cobra's --environment path
// must not bypass the guard. One test per spelling keeps a regression
// that only trips one of them visible.
func TestProvision_EnvironmentLongFlagIsRejected(t *testing.T) {
	cmd := NewProvisionCmd(&root_cmd.RootCmd{})
	cmd.SetArgs([]string{"--environment", "staging", "-s", "irrelevant"})
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true

	err := cmd.Execute()
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "parent"),
		"long-form -e must reject with the same message, got: %v", err)
}
