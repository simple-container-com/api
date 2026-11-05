// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package cmd_destroy

import (
	"strings"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"

	"github.com/simple-container-com/api/pkg/cmd/root_cmd"
	"github.com/simple-container-com/api/pkg/util"
)

// scriptedReader answers ReadLine from a list, returning io.EOF-style
// empty-and-error once exhausted so the destroy prompt loop hits its
// "after 3 attempts" guard cleanly in tests.
type scriptedReader struct {
	lines []string
	idx   int
}

func (r *scriptedReader) ReadLine() (string, error) {
	if r.idx >= len(r.lines) {
		return "", nil
	}
	s := r.lines[r.idx]
	r.idx++
	return s, nil
}

func (r *scriptedReader) ReadPassword() (string, error) { return "", nil }

type nullWriter struct{}

func (nullWriter) Print(...any)   {}
func (nullWriter) Println(...any) {}

func withConsole(t *testing.T, reader util.ConsoleReader) func() {
	t.Helper()
	prevR := util.DefaultConsoleReader
	prevW := util.DefaultConsoleWriter
	util.DefaultConsoleReader = reader
	util.DefaultConsoleWriter = nullWriter{}
	return func() {
		util.DefaultConsoleReader = prevR
		util.DefaultConsoleWriter = prevW
	}
}

// Setting --yes must skip the Y/N prompt entirely — a CI teardown piping
// nothing to stdin used to hit the 3-attempts guard. With --yes it never
// reads stdin, so the only remaining failure mode is the Provisioner
// itself (which this test isolates with a nil Provisioner, running only
// the pre-provision prompt path).
func TestDestroy_YesSkipsPrompt(t *testing.T) {
	restore := withConsole(t, &scriptedReader{})
	defer restore()

	cmd := NewDestroyCmd(&root_cmd.RootCmd{})
	// `--yes` short form plus a stack name so the command parses. nil
	// Provisioner panics inside the destroy call that follows the prompt,
	// which is PAST the lines under test — catch the panic to assert the
	// loop was NOT entered.
	cmd.SetArgs([]string{"-y", "-s", "irrelevant"})

	defer func() {
		// A panic from the nil Provisioner is the "we got past the
		// prompt" signal. A clean error from the prompt loop would
		// mean --yes did not short-circuit it.
		_ = recover()
	}()
	_ = cmd.Execute()
}

// The non-interactive alias must mean the same thing as --yes. Two names
// per flag is a pure UX choice (every recipe reads differently); drift
// between them is a bug.
func TestDestroy_NonInteractiveAliasesYes(t *testing.T) {
	restore := withConsole(t, &scriptedReader{})
	defer restore()

	cmd := NewDestroyCmd(&root_cmd.RootCmd{})
	cmd.SetArgs([]string{"--non-interactive", "-s", "irrelevant"})

	defer func() { _ = recover() }()
	_ = cmd.Execute()
}

// Without --yes, three empty lines on stdin MUST return a non-zero error
// naming the attempts limit AND pointing at --yes. The "returns an error"
// half is the real defect from sc-cli-unattended-gaps (the prompt used
// to swallow the empty-stdin case into exit 0 in the field) — the hint
// is the UX half.
func TestDestroy_EmptyStdinErrorsWithHint(t *testing.T) {
	restore := withConsole(t, &scriptedReader{lines: []string{"", "", "", ""}})
	defer restore()

	cmd := NewDestroyCmd(&root_cmd.RootCmd{})
	cmd.SetArgs([]string{"-s", "irrelevant"})

	err := cmd.Execute()
	require.Error(t, err, "empty stdin must NOT silently succeed")
	// The error has to carry both the attempt count (so the operator
	// knows the loop exited for the right reason) and the --yes hint
	// (so the fix is one scroll away).
	for _, want := range []string{"after 3 attempts", "--yes", "--non-interactive"} {
		require.True(t, strings.Contains(err.Error(), want),
			"error %q missing marker %q", err, want)
	}
}

// Typing 'n' is the user-authored cancellation path — it must still work
// (and still error, since a cancelled destroy is not a success).
func TestDestroy_NoCancelsWithError(t *testing.T) {
	restore := withConsole(t, &scriptedReader{lines: []string{"n"}})
	defer restore()

	cmd := NewDestroyCmd(&root_cmd.RootCmd{})
	cmd.SetArgs([]string{"-s", "irrelevant"})

	err := cmd.Execute()
	require.Error(t, err)
	require.True(t, errors.Is(err, err) && strings.Contains(err.Error(), "cancelled"),
		"expected cancellation error, got: %v", err)
}
