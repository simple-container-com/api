// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

// Package cosigntest installs a scripted stand-in for the cosign binary so
// tests can drive the signing, SBOM and provenance code paths without a real
// cosign install, registry, or signing key.
//
// It exists because three near-identical shell harnesses had accumulated across
// pkg/security; every improvement to one (shell quoting, invocation counting,
// stdout support) had to be repeated in the others.
package cosigntest

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

// RekorConflictStderr reproduces the cosign output shape when Rekor rejects a
// replayed log entry with HTTP 409 createLogEntryConflict.
const RekorConflictStderr = `Error: signing registry.example.com/team/app@sha256:f7ed9277c480591d7ec36fe7da13e112b33d898b7687f9bcbcda5c214a242099: ` +
	`signing bundle: error signing bundle: [POST /api/v1/log/entries][409] createLogEntryConflict ` +
	`{"code":409,"message":"an equivalent entry already exists in the transparency log with UUID 108e9186e8c5677a"}`

// Options configures the stub's behaviour across successive invocations.
type Options struct {
	// ConflictsBefore is how many leading invocations fail with a Rekor
	// conflict. Later invocations succeed.
	ConflictsBefore int

	// ConflictOnStdout emits the conflict marker on stdout rather than stderr.
	ConflictOnStdout bool

	// FailStderr, when non-empty, makes every invocation fail with this stderr
	// instead of a Rekor conflict. ConflictsBefore is then ignored.
	FailStderr string

	// DelayFirst pauses the first invocation.
	DelayFirst time.Duration

	// DelayEach pauses every invocation. Set it just under the caller's
	// per-attempt timeout to distinguish a fresh per-attempt deadline from one
	// budget shared across attempts: each invocation fits in its own window,
	// but two cannot fit in a single shared one.
	DelayEach time.Duration
}

// Fake is an installed stub. Calls reports how many times it ran.
type Fake struct{ counter string }

// Install writes a `cosign` stub into a fresh temp dir and prepends that dir to
// PATH for the duration of the test. The invocation count lives in a file so it
// survives across the separate processes cosign is invoked as.
func Install(t *testing.T, o Options) *Fake {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake cosign harness requires a POSIX shell")
	}

	dir := t.TempDir()
	counter := filepath.Join(dir, "calls")

	var b strings.Builder
	b.WriteString("#!/bin/sh\n")
	b.WriteString("n=$(cat " + shellQuote(counter) + " 2>/dev/null || echo 0)\n")
	b.WriteString("n=$((n+1))\n")
	b.WriteString("echo $n > " + shellQuote(counter) + "\n")

	if o.DelayFirst > 0 {
		b.WriteString("if [ \"$n\" -eq 1 ]; then sleep " + seconds(o.DelayFirst) + "; fi\n")
	}
	if o.DelayEach > 0 {
		b.WriteString("sleep " + seconds(o.DelayEach) + "\n")
	}

	if o.FailStderr != "" {
		b.WriteString("printf '%s\\n' " + shellQuote(o.FailStderr) + " 1>&2\n")
		b.WriteString("exit 1\n")
	} else {
		b.WriteString("if [ \"$n\" -le " + strconv.Itoa(o.ConflictsBefore) + " ]; then\n")
		redirect := " 1>&2"
		if o.ConflictOnStdout {
			// Keep stderr non-empty so a test that passes only because the
			// marker leaked through the other stream cannot pass by accident.
			b.WriteString("  printf '%s\\n' 'Error: attaching attestation' 1>&2\n")
			redirect = ""
		}
		b.WriteString("  printf '%s\\n' " + shellQuote(RekorConflictStderr) + redirect + "\n")
		b.WriteString("  exit 1\n")
		b.WriteString("fi\n")
		b.WriteString("printf '%s\\n' 'tlog entry created with index: 123456'\n")
		b.WriteString("exit 0\n")
	}

	path := filepath.Join(dir, "cosign")
	if err := os.WriteFile(path, []byte(b.String()), 0o755); err != nil {
		t.Fatalf("writing fake cosign: %v", err)
	}
	// Prepend so the stub wins over any real cosign on the runner.
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	return &Fake{counter: counter}
}

// Calls returns how many times the stub ran. It fails the test when the counter
// file is missing, so an assertion cannot pass because the stub never executed.
func (f *Fake) Calls(t *testing.T) int {
	t.Helper()
	data, err := os.ReadFile(f.counter)
	if err != nil {
		t.Fatalf("cosign stub counter %s unreadable — the stub never ran: %v", f.counter, err)
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatalf("cosign stub counter %q is not a number: %v", data, err)
	}
	return n
}

// seconds renders a duration for /bin/sh sleep, which takes a decimal number.
func seconds(d time.Duration) string {
	return strconv.FormatFloat(d.Seconds(), 'f', 3, 64)
}

// shellQuote single-quotes a string for safe embedding in /bin/sh. Temp dir
// paths can contain spaces, which silently broke an earlier harness.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
