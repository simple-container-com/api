// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package cosigntest

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/security/tools"
)

// The harness is what three retry suites assert against, so a silent break in
// it (a dropped counter, a marker on the wrong stream, a path that needs
// quoting) turns those suites green for the wrong reason. Drive the stub the
// way production drives cosign — tools.ExecCommand, which is exactly what
// signing.RunCosignWithRetry calls — so stdout and stderr stay separated here
// the same way they do there.

const stubTimeout = 10 * time.Second

func runStub(args ...string) (stdout, stderr string, err error) {
	return tools.ExecCommand(context.Background(), "cosign", args, nil, stubTimeout)
}

func TestInstall_StubWinsOnPathAndReportsSuccess(t *testing.T) {
	RegisterTestingT(t)

	fake := Install(t, Options{})

	resolved, err := exec.LookPath("cosign")
	Expect(err).ToNot(HaveOccurred())
	Expect(resolved).To(Equal(filepath.Join(filepath.Dir(fake.counter), "cosign")),
		"the stub must resolve ahead of any real cosign on the runner")

	stdout, stderr, err := runStub("attest", "--yes")
	Expect(err).ToNot(HaveOccurred())
	Expect(stdout).To(ContainSubstring("tlog entry created with index"))
	Expect(stderr).To(BeEmpty())
	Expect(fake.Calls(t)).To(Equal(1))
}

func TestInstall_ConflictsBeforeFailsThenSucceeds(t *testing.T) {
	RegisterTestingT(t)

	fake := Install(t, Options{ConflictsBefore: 2})

	for attempt := 1; attempt <= 2; attempt++ {
		stdout, stderr, err := runStub("attest")
		Expect(err).To(HaveOccurred(), "invocation %d must fail", attempt)
		Expect(stderr).To(ContainSubstring("createLogEntryConflict"))
		Expect(stdout).ToNot(ContainSubstring("createLogEntryConflict"),
			"the conflict must not leak onto stdout when ConflictOnStdout is unset")
	}

	stdout, _, err := runStub("attest")
	Expect(err).ToNot(HaveOccurred(), "invocation 3 is past ConflictsBefore")
	Expect(stdout).To(ContainSubstring("tlog entry created with index"))
	Expect(fake.Calls(t)).To(Equal(3), "the counter must survive across the three processes")
}

func TestInstall_ConflictOnStdoutKeepsStderrNonEmptyWithoutTheMarker(t *testing.T) {
	RegisterTestingT(t)

	Install(t, Options{ConflictsBefore: 1, ConflictOnStdout: true})

	stdout, stderr, err := runStub("attest")
	Expect(err).To(HaveOccurred())
	Expect(stdout).To(ContainSubstring("createLogEntryConflict"))
	// Both halves matter: a caller that classifies only stderr must still see
	// output, and it must not be the marker — otherwise a stdout-classification
	// bug passes because the marker was on stderr too.
	Expect(stderr).ToNot(BeEmpty())
	Expect(stderr).ToNot(ContainSubstring("createLogEntryConflict"))
}

func TestInstall_FailStderrIsTerminalAndIgnoresConflictsBefore(t *testing.T) {
	RegisterTestingT(t)

	fake := Install(t, Options{FailStderr: "Error: UNAUTHORIZED: authentication required", ConflictsBefore: 1})

	for attempt := 1; attempt <= 2; attempt++ {
		_, stderr, err := runStub("attest")
		Expect(err).To(HaveOccurred(), "invocation %d must fail", attempt)
		Expect(stderr).To(ContainSubstring("UNAUTHORIZED"))
		Expect(stderr).ToNot(ContainSubstring("createLogEntryConflict"),
			"FailStderr must suppress the retryable conflict entirely")
	}
	Expect(fake.Calls(t)).To(Equal(2))
}

// A stderr body carrying a single quote is what broke an earlier harness: the
// generated /bin/sh script stopped parsing and every invocation failed for the
// wrong reason.
func TestInstall_FailStderrSurvivesShellQuoting(t *testing.T) {
	RegisterTestingT(t)

	body := `Error: signing 'registry.example.com/team/app': it's fatal`
	Install(t, Options{FailStderr: body})

	_, stderr, err := runStub("sign")
	Expect(err).To(HaveOccurred())
	Expect(stderr).To(ContainSubstring(body), "the quoted body must reach stderr verbatim")
}

func TestInstall_DelayEachAppliesToEveryInvocation(t *testing.T) {
	RegisterTestingT(t)

	const delay = 300 * time.Millisecond
	Install(t, Options{DelayEach: delay})

	for attempt := 1; attempt <= 2; attempt++ {
		start := time.Now()
		_, _, err := runStub("attest")
		Expect(err).ToNot(HaveOccurred())
		Expect(time.Since(start)).To(BeNumerically(">=", delay),
			"invocation %d must get its own delay, not a shared one", attempt)
	}
}

func TestInstall_DelayFirstAppliesOnlyToTheFirstInvocation(t *testing.T) {
	RegisterTestingT(t)

	const delay = 700 * time.Millisecond
	Install(t, Options{DelayFirst: delay})

	start := time.Now()
	_, _, err := runStub("attest")
	Expect(err).ToNot(HaveOccurred())
	first := time.Since(start)

	start = time.Now()
	_, _, err = runStub("attest")
	Expect(err).ToNot(HaveOccurred())
	second := time.Since(start)

	Expect(first).To(BeNumerically(">=", delay))
	Expect(second).To(BeNumerically("<", delay), "only invocation 1 is delayed")
}

func TestSeconds_RendersDecimalsPosixSleepAccepts(t *testing.T) {
	RegisterTestingT(t)

	// `sleep 150ms` is a Go duration string, not a number — /bin/sh rejects it.
	for _, tt := range []struct {
		in   time.Duration
		want string
	}{
		{0, "0.000"},
		{150 * time.Millisecond, "0.150"},
		{1500 * time.Millisecond, "1.500"},
		{2 * time.Second, "2.000"},
	} {
		Expect(seconds(tt.in)).To(Equal(tt.want))
	}
}

func TestShellQuote_QuotesForPosixShell(t *testing.T) {
	RegisterTestingT(t)

	for _, tt := range []struct {
		name string
		in   string
		want string
	}{
		{"plain", "abc", `'abc'`},
		{"with space", "/tmp/go build/cosign", `'/tmp/go build/cosign'`},
		{"single quote", "it's", `'it'\''s'`},
		{"empty", "", `''`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			Expect(shellQuote(tt.in)).To(Equal(tt.want))

			// Round-trip through the shell the generated stub actually runs
			// under: string equality alone would not catch a quoting form that
			// /bin/sh re-splits or expands.
			out, err := exec.Command("/bin/sh", "-c", "printf '%s' "+shellQuote(tt.in)).Output()
			Expect(err).ToNot(HaveOccurred())
			Expect(string(out)).To(Equal(tt.in))
		})
	}
}

func TestShellQuote_NeutralisesShellMetacharacters(t *testing.T) {
	RegisterTestingT(t)

	// A temp dir or stderr body is untrusted input as far as the generated
	// script is concerned; quoting must stop it executing.
	out, err := exec.Command("/bin/sh", "-c", "printf '%s' "+shellQuote("$(echo pwned)`id`;rm -rf /")).Output()
	Expect(err).ToNot(HaveOccurred())
	Expect(string(out)).To(Equal("$(echo pwned)`id`;rm -rf /"))
}

func TestRekorConflictStderr_MatchesWhatCallersClassifyOn(t *testing.T) {
	RegisterTestingT(t)

	// The retry classifier anchors on the swagger error shape, not on "409"
	// alone. If the fixture drifts off that shape the retry suites stop
	// exercising the retry path and silently assert the failure branch.
	Expect(RekorConflictStderr).To(ContainSubstring("[POST /api/v1/log/entries][409]"))
	Expect(RekorConflictStderr).To(ContainSubstring("createLogEntryConflict"))
}

// The probe half of the stub used to answer on "$1" alone. That is why the
// cosign-v3 breakage was invisible: a probe real cosign rejects, or one aimed
// at another image, still read as a clean confirm. These pin the validation.
func TestInstall_ProbeRejectsAnArgvRealCosignWouldReject(t *testing.T) {
	RegisterTestingT(t)

	Install(t, Options{ArtifactAlreadyAttached: true, Image: "registry.example.com/team/app@sha256:abc"})

	for _, tt := range []struct {
		name string
		args []string
		want string
	}{
		{
			name: "verify without an identity to verify against",
			args: []string{"verify", "registry.example.com/team/app@sha256:abc"},
			want: "no key or certificate identity",
		},
		{
			name: "verify-attestation without a predicate type",
			args: []string{"verify-attestation", "--key", "/tmp/k.pub", "registry.example.com/team/app@sha256:abc"},
			want: "without --type",
		},
		{
			name: "no image reference at all",
			args: []string{"verify", "--key", "/tmp/k.pub"},
			want: "no image reference",
		},
		{
			name: "aimed at a different image",
			args: []string{"verify", "--key", "/tmp/k.pub", "registry.example.com/team/other@sha256:def"},
			want: "MANIFEST_UNKNOWN",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			_, stderr, err := runStub(tt.args...)
			Expect(err).To(HaveOccurred())
			Expect(stderr).To(ContainSubstring(tt.want))
		})
	}

	// The well-formed probe still confirms, so the cases above fail for the
	// reason stated rather than because nothing can ever pass.
	_, _, err := runStub("verify", "--key", "/tmp/k.pub", "registry.example.com/team/app@sha256:abc")
	Expect(err).ToNot(HaveOccurred())
}

// cosign v3 defaults to the new bundle format and writes nothing to the legacy
// signature tag, so `download` finds nothing however the image was signed.
func TestInstall_LegacyDownloadNeverConfirms(t *testing.T) {
	RegisterTestingT(t)

	fake := Install(t, Options{ArtifactAlreadyAttached: true})

	for _, args := range [][]string{
		{"download", "signature", "registry.example.com/team/app@sha256:abc"},
		{"download", "attestation", "--predicate-type", "cyclonedx", "registry.example.com/team/app@sha256:abc"},
	} {
		_, stderr, err := runStub(args...)
		Expect(err).To(HaveOccurred(), "cosign v3 answers %v with nothing", args)
		Expect(stderr).To(ContainSubstring("no signatures associated"))
	}
	Expect(fake.Probes(t)).To(Equal(0), "a download is not a confirmation probe")
}

func TestInstall_ProbeExitAndProbeStdoutAreIndependent(t *testing.T) {
	RegisterTestingT(t)

	probeArgs := []string{"verify", "--key", "/tmp/k.pub", "registry.example.com/team/app@sha256:abc"}

	t.Run("exit 0 with empty stdout", func(t *testing.T) {
		RegisterTestingT(t)
		Install(t, Options{ArtifactAlreadyAttached: true})
		stdout, _, err := runStub(probeArgs...)
		Expect(err).ToNot(HaveOccurred())
		Expect(stdout).To(BeEmpty(), "real `cosign verify` prints its banner to stderr")
	})

	t.Run("registry auth failure", func(t *testing.T) {
		RegisterTestingT(t)
		Install(t, Options{ProbeExit: 10})
		_, _, err := runStub(probeArgs...)
		Expect(err).To(HaveOccurred())
	})

	t.Run("payload on stdout with a non-zero exit", func(t *testing.T) {
		RegisterTestingT(t)
		Install(t, Options{ProbeExit: 1, ProbeStdout: `{"payload":"e30="}`})
		stdout, _, err := runStub(probeArgs...)
		Expect(err).To(HaveOccurred(), "output is not confirmation")
		Expect(stdout).To(ContainSubstring("payload"))
	})
}

// The probe branch sits below the delay scripting now. Before, probes cost zero
// simulated time, so no test could show the probe sharing the timeout model.
func TestInstall_DelayEachAppliesToProbesToo(t *testing.T) {
	RegisterTestingT(t)

	const delay = 300 * time.Millisecond
	fake := Install(t, Options{DelayEach: delay, ArtifactAlreadyAttached: true})

	start := time.Now()
	_, _, err := runStub("verify", "--key", "/tmp/k.pub", "registry.example.com/team/app@sha256:abc")
	Expect(err).ToNot(HaveOccurred())
	Expect(time.Since(start)).To(BeNumerically(">=", delay))
	Expect(fake.Probes(t)).To(Equal(1))
}

func TestFake_LastProbeArgsRecordsTheWholeArgv(t *testing.T) {
	RegisterTestingT(t)

	fake := Install(t, Options{ArtifactAlreadyAttached: true})
	Expect(fake.LastProbeArgs(t)).To(BeNil(), "no probe has run yet")

	args := []string{"verify-attestation", "--type", "cyclonedx", "--key", "/tmp/k.pub", "registry.example.com/team/app@sha256:abc"}
	_, _, err := runStub(args...)
	Expect(err).ToNot(HaveOccurred())
	Expect(fake.LastProbeArgs(t)).To(Equal(args))
}
