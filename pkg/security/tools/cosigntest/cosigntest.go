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

	// DelayFirst pauses the first signing invocation.
	DelayFirst time.Duration

	// DelayEach pauses every invocation, confirmation probes included. Set it
	// just under the caller's per-attempt timeout to distinguish a fresh
	// per-attempt deadline from one budget shared across attempts: each
	// invocation fits in its own window, but two cannot fit in a single shared
	// one.
	DelayEach time.Duration

	// ArtifactAlreadyAttached makes the read-only confirmation probe
	// (`cosign verify` / `cosign verify-attestation`) succeed, which is what a
	// redeploy of an unchanged digest sees. The retry loop treats a Rekor
	// conflict confirmed this way as an idempotent success. Default false: the
	// probe fails, so conflicts keep flowing through the retry path.
	ArtifactAlreadyAttached bool

	// ProbeExit overrides the probe's exit status; ProbeStdout overrides what
	// it prints. Zero means "follow ArtifactAlreadyAttached" (0 when attached,
	// 1 otherwise), so a test asking for a specific failure sets it explicitly:
	// 1 for "no matching attestations", 10 for a registry auth error. Together
	// the pair models the cases the boolean cannot: a probe that exits 0 while
	// printing nothing, and one that prints a payload while exiting non-zero.
	ProbeExit   int
	ProbeStdout string

	// Image, when set, is the image reference the probe must target. A probe
	// aimed anywhere else is rejected the way real cosign would reject it.
	Image string
}

// Fake is an installed stub. Calls reports how many times it ran.
type Fake struct{ counter, probes, probeArgs string }

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
	probes := filepath.Join(dir, "probes")
	probeArgs := filepath.Join(dir, "probeargs")

	var b strings.Builder
	b.WriteString("#!/bin/sh\n")
	// Classify the invocation. The confirmation probe is a separate, read-only
	// subcommand; count it apart from sign/attest so existing attempt-count
	// assertions keep measuring signing attempts.
	b.WriteString("case \"$1\" in\n")
	b.WriteString("  verify|verify-attestation) kind=probe ;;\n")
	b.WriteString("  download) kind=legacy ;;\n")
	b.WriteString("  *) kind=op ;;\n")
	b.WriteString("esac\n")

	b.WriteString("if [ \"$kind\" = probe ]; then\n")
	b.WriteString("  p=$(cat " + shellQuote(probes) + " 2>/dev/null || echo 0)\n")
	b.WriteString("  p=$((p+1))\n")
	b.WriteString("  echo $p > " + shellQuote(probes) + "\n")
	b.WriteString("  printf '%s\\n' \"$@\" > " + shellQuote(probeArgs) + "\n")
	b.WriteString("else\n")
	b.WriteString("  n=$(cat " + shellQuote(counter) + " 2>/dev/null || echo 0)\n")
	b.WriteString("  n=$((n+1))\n")
	b.WriteString("  echo $n > " + shellQuote(counter) + "\n")
	b.WriteString("fi\n")

	// Delays apply to probes too. A probe is a real registry round trip in
	// production, so a harness that answers it for free cannot show that the
	// probe shares the caller's timeout model.
	if o.DelayFirst > 0 {
		b.WriteString("if [ \"${n:-0}\" -eq 1 ]; then sleep " + seconds(o.DelayFirst) + "; fi\n")
	}
	if o.DelayEach > 0 {
		b.WriteString("sleep " + seconds(o.DelayEach) + "\n")
	}

	// cosign v3 defaults to the new bundle format and writes nothing to the
	// legacy `sha256-<digest>.sig` tag, so `download signature` and `download
	// attestation` find nothing however the image was signed. Modelled so a
	// regression back to a download-shaped probe fails the suite instead of
	// silently never confirming.
	b.WriteString("if [ \"$kind\" = legacy ]; then\n")
	b.WriteString("  printf '%s\\n' 'Error: no signatures associated with the image' 1>&2\n")
	b.WriteString("  exit 1\n")
	b.WriteString("fi\n")

	writeProbeBranch(&b, o)

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

	return &Fake{counter: counter, probes: probes, probeArgs: probeArgs}
}

// writeProbeBranch emits the confirmation-probe half of the stub. It validates
// the argv the way real cosign would before answering, because a stub that
// answers on "$1" alone lets a probe real cosign rejects, or one aimed at the
// wrong image, read as a clean confirm.
func writeProbeBranch(b *strings.Builder, o Options) {
	exit := o.ProbeExit
	if exit == 0 && !o.ArtifactAlreadyAttached {
		exit = 1
	}

	b.WriteString("if [ \"$kind\" = probe ]; then\n")
	b.WriteString("  sub=$1; shift\n")
	b.WriteString("  ident=0; ptype=0; ref=''\n")
	// Value-taking flags consume their argument, the way real cosign parses.
	// Without that, `verify --key /tmp/k.pub` reads the key path as the image
	// reference and a probe with no image at all looks well formed.
	b.WriteString("  while [ $# -gt 0 ]; do\n")
	b.WriteString("    case \"$1\" in\n")
	b.WriteString("      --key=*|--certificate-identity=*|--certificate-identity-regexp=*) ident=1 ;;\n")
	b.WriteString("      --key|--certificate-identity|--certificate-identity-regexp)\n")
	b.WriteString("        ident=1; if [ $# -gt 1 ]; then shift; fi ;;\n")
	b.WriteString("      --type=*) ptype=1 ;;\n")
	b.WriteString("      --type) ptype=1; if [ $# -gt 1 ]; then shift; fi ;;\n")
	b.WriteString("      --certificate-oidc-issuer|--predicate-type)\n")
	b.WriteString("        if [ $# -gt 1 ]; then shift; fi ;;\n")
	b.WriteString("      -*) ;;\n")
	b.WriteString("      *) ref=$1 ;;\n")
	b.WriteString("    esac\n")
	b.WriteString("    shift\n")
	b.WriteString("  done\n")
	// A verify with no identity flag is rejected by real cosign, and accepting
	// it here would mean the suite blesses a presence-only probe.
	b.WriteString("  if [ \"$ident\" -ne 1 ]; then\n")
	b.WriteString("    printf '%s\\n' 'Error: no key or certificate identity supplied' 1>&2\n")
	b.WriteString("    exit 2\n")
	b.WriteString("  fi\n")
	b.WriteString("  if [ \"$sub\" = verify-attestation ] && [ \"$ptype\" -ne 1 ]; then\n")
	b.WriteString("    printf '%s\\n' 'Error: verify-attestation without --type accepts any predicate' 1>&2\n")
	b.WriteString("    exit 2\n")
	b.WriteString("  fi\n")
	b.WriteString("  if [ -z \"$ref\" ]; then\n")
	b.WriteString("    printf '%s\\n' 'Error: no image reference supplied' 1>&2\n")
	b.WriteString("    exit 2\n")
	b.WriteString("  fi\n")
	if o.Image != "" {
		b.WriteString("  if [ \"$ref\" != " + shellQuote(o.Image) + " ]; then\n")
		b.WriteString("    printf '%s\\n' \"Error: MANIFEST_UNKNOWN: $ref\" 1>&2\n")
		b.WriteString("    exit 2\n")
		b.WriteString("  fi\n")
	}
	if o.ProbeStdout != "" {
		b.WriteString("  printf '%s\\n' " + shellQuote(o.ProbeStdout) + "\n")
	}
	b.WriteString("  exit " + strconv.Itoa(exit) + "\n")
	b.WriteString("fi\n")
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

// Probes returns how many times the read-only confirmation probe
// (`cosign verify …`) ran. Zero when it never ran; unlike Calls, an absent
// counter file is a legitimate result.
func (f *Fake) Probes(t *testing.T) int {
	t.Helper()
	data, err := os.ReadFile(f.probes)
	if os.IsNotExist(err) {
		return 0
	}
	if err != nil {
		t.Fatalf("cosign stub probe counter %s unreadable: %v", f.probes, err)
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatalf("cosign stub probe counter %q is not a number: %v", data, err)
	}
	return n
}

// LastProbeArgs returns the argv of the most recent confirmation probe, one
// element per argument. Nil when no probe ran.
func (f *Fake) LastProbeArgs(t *testing.T) []string {
	t.Helper()
	data, err := os.ReadFile(f.probeArgs)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatalf("cosign stub probe argv %s unreadable: %v", f.probeArgs, err)
	}
	trimmed := strings.TrimSuffix(string(data), "\n")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "\n")
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
