// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package signing

import (
	"context"
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/gomega"
)

// conflictStderr reproduces the cosign stderr shape observed when several deploy
// jobs attest against the public-good Rekor instance at once and cosign's own
// upload retry replays a body that already landed.
const conflictStderr = `Error: signing registry.example.com/team/worker@sha256:f7ed9277c480591d7ec36fe7da13e112b33d898b7687f9bcbcda5c214a242099: ` +
	`signing bundle: error signing bundle: [POST /api/v1/log/entries][409] createLogEntryConflict ` +
	`{"code":409,"message":"an equivalent entry already exists in the transparency log with UUID 108e9186e8c5677a"}`

// attestArgs is a representative argv; args[0] drives the error prefix.
var attestArgs = []string{"attest", "--predicate", "/tmp/p.json", "registry.example.com/team/app"}

// noBackoff removes the retry delay so tests do not sleep. Backoff timing is
// covered separately by TestCosignBackoff_IsBoundedAndJittered.
func noBackoff(t *testing.T) {
	t.Helper()
	orig := cosignBaseBackoff
	cosignBaseBackoff = 0
	t.Cleanup(func() { cosignBaseBackoff = orig })
}

// probeAbsent wraps a fake exec so the read-only presence probe
// (`cosign download signature|attestation`) reports nothing attached. Attempt
// counts in the wrapped fake then stay about sign/attest invocations only.
func probeAbsent(exec execFn) execFn {
	return func(ctx context.Context, name string, args, env []string, timeout time.Duration) (string, string, error) {
		if len(args) > 0 && args[0] == "download" {
			return "", "Error: no matching attestations", fmt.Errorf("exit status 1")
		}
		return exec(ctx, name, args, env, timeout)
	}
}

// probePresent is probeAbsent's twin: the probe reports the artifact already on
// the image, which is what a redeploy of an unchanged digest sees.
func probePresent(exec execFn, probes *int) execFn {
	return func(ctx context.Context, name string, args, env []string, timeout time.Duration) (string, string, error) {
		if len(args) > 0 && args[0] == "download" {
			if probes != nil {
				*probes++
			}
			return `{"payloadType":"application/vnd.in-toto+json","payload":"e30="}`, "", nil
		}
		return exec(ctx, name, args, env, timeout)
	}
}

// Pins the retry bound. Without this every call-count assertion derives from the
// constant itself, so widening 5 -> 7 would go unnoticed.
func TestMaxCosignAttempts_IsPinned(t *testing.T) {
	RegisterTestingT(t)
	Expect(maxCosignAttempts).To(Equal(5))
}

func TestRunCosignWithRetry_SucceedsOnRetry(t *testing.T) {
	RegisterTestingT(t)
	noBackoff(t)

	calls := 0
	exec := func(_ context.Context, _ string, _, _ []string, _ time.Duration) (string, string, error) {
		calls++
		if calls == 1 {
			return "", conflictStderr, fmt.Errorf("exit status 1")
		}
		return "tlog entry created with index: 123456", "", nil
	}

	out, err := runCosignWithRetry(context.Background(), "sbom attest", attestArgs, nil, time.Minute, probeAbsent(exec))

	Expect(err).ToNot(HaveOccurred())
	Expect(out).To(Equal("tlog entry created with index: 123456"))
	Expect(calls).To(Equal(2), "a conflict must trigger exactly one retry")
}

// The conflict can arrive on stdout; classification must cover both streams.
func TestRunCosignWithRetry_ClassifiesConflictOnStdout(t *testing.T) {
	RegisterTestingT(t)
	noBackoff(t)

	calls := 0
	exec := func(_ context.Context, _ string, _, _ []string, _ time.Duration) (string, string, error) {
		calls++
		if calls == 1 {
			return conflictStderr, "Error: attaching attestation", fmt.Errorf("exit status 1")
		}
		return "ok", "", nil
	}

	_, err := runCosignWithRetry(context.Background(), "sbom attest", attestArgs, nil, time.Minute, probeAbsent(exec))

	Expect(err).ToNot(HaveOccurred())
	Expect(calls).To(Equal(2), "a conflict on stdout must be retried too")
}

// Regression guard against cross-stream widening: a registry-side 409 on stderr
// plus a Rekor entry URL on stdout must NOT read as a tlog conflict. Neither
// stream carries the marker alone, and a registry conflict is not retryable.
func TestRunCosignWithRetry_RegistryConflictPlusRekorURLIsNotAConflict(t *testing.T) {
	RegisterTestingT(t)
	noBackoff(t)

	calls := 0
	exec := func(_ context.Context, _ string, _, _ []string, _ time.Duration) (string, string, error) {
		calls++
		return "tlog entry created: https://rekor.sigstore.dev/api/v1/log/entries?logIndex=77",
			"pushing signature: PUT https://registry.example.com/v2/team/app/manifests/sha256-abc: unexpected status 409 Conflict",
			fmt.Errorf("exit status 1")
	}

	_, err := runCosignWithRetry(context.Background(), "sbom attest", attestArgs, nil, time.Minute, probeAbsent(exec))

	Expect(err).To(HaveOccurred())
	Expect(calls).To(Equal(1), "a registry 409 must fail fast, not retry")
}

func TestRunCosignWithRetry_NoRetryOnOtherErrors(t *testing.T) {
	RegisterTestingT(t)
	noBackoff(t)

	calls := 0
	exec := func(_ context.Context, _ string, _, _ []string, _ time.Duration) (string, string, error) {
		calls++
		return "", "error signing: getting signer: oidc: token expired", fmt.Errorf("exit status 1")
	}

	_, err := runCosignWithRetry(context.Background(), "sbom attest", attestArgs, nil, time.Minute, probeAbsent(exec))

	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("token expired"))
	Expect(calls).To(Equal(1))
}

// A conflict the registry cannot confirm means the artifact is genuinely not
// attached, so exhausting the loop must still fail. Surfacing it is correct:
// cosign uploads to Rekor before it pushes to the registry, so a tlog entry on
// its own does not prove the attestation landed.
func TestRunCosignWithRetry_GivesUpAfterMaxAttempts(t *testing.T) {
	RegisterTestingT(t)
	noBackoff(t)

	calls := 0
	exec := func(_ context.Context, _ string, _, _ []string, _ time.Duration) (string, string, error) {
		calls++
		return "", conflictStderr, fmt.Errorf("exit status 1")
	}

	_, err := runCosignWithRetry(context.Background(), "sbom attest", attestArgs, nil, time.Minute, probeAbsent(exec))

	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("createLogEntryConflict"))
	Expect(err.Error()).To(ContainSubstring("after 5 attempts"))
	Expect(err.Error()).To(ContainSubstring("cosign attest failed"), "prefix comes from args[0]")
	Expect(calls).To(Equal(maxCosignAttempts))
}

// The operator must still get the conflict text when a later attempt dies for an
// unrelated reason — that string is what the troubleshooting docs key on.
func TestRunCosignWithRetry_SurfacesFirstConflictNotLastError(t *testing.T) {
	RegisterTestingT(t)
	noBackoff(t)

	calls := 0
	exec := func(_ context.Context, _ string, _, _ []string, _ time.Duration) (string, string, error) {
		calls++
		if calls == 1 {
			return "", conflictStderr, fmt.Errorf("exit status 1")
		}
		return "", "", fmt.Errorf("signal: killed")
	}

	_, err := runCosignWithRetry(context.Background(), "sbom attest", attestArgs, nil, time.Minute, probeAbsent(exec))

	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("signal: killed"))
	Expect(calls).To(Equal(2), "a non-conflict error stops the loop")
}

func TestRunCosignWithRetry_SucceedsOnFinalAllowedAttempt(t *testing.T) {
	RegisterTestingT(t)
	noBackoff(t)

	calls := 0
	exec := func(_ context.Context, _ string, _, _ []string, _ time.Duration) (string, string, error) {
		calls++
		if calls < maxCosignAttempts {
			return "", conflictStderr, fmt.Errorf("exit status 1")
		}
		return "ok", "", nil
	}

	_, err := runCosignWithRetry(context.Background(), "sbom attest", attestArgs, nil, time.Minute, probeAbsent(exec))

	Expect(err).ToNot(HaveOccurred())
	Expect(calls).To(Equal(maxCosignAttempts))
}

// A successful run whose output happens to contain conflict text must not be
// reclassified — success short-circuits before classification.
func TestRunCosignWithRetry_ConflictTextOnSuccessIsIgnored(t *testing.T) {
	RegisterTestingT(t)
	noBackoff(t)

	calls := 0
	exec := func(_ context.Context, _ string, _, _ []string, _ time.Duration) (string, string, error) {
		calls++
		return conflictStderr, "", nil
	}

	_, err := runCosignWithRetry(context.Background(), "sbom attest", attestArgs, nil, time.Minute, probeAbsent(exec))

	Expect(err).ToNot(HaveOccurred())
	Expect(calls).To(Equal(1))
}

func TestRunCosignWithRetry_StopsOnCancelledContext(t *testing.T) {
	RegisterTestingT(t)
	noBackoff(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	calls := 0
	exec := func(_ context.Context, _ string, _, _ []string, _ time.Duration) (string, string, error) {
		calls++
		return "", conflictStderr, fmt.Errorf("exit status 1")
	}

	_, err := runCosignWithRetry(ctx, "sbom attest", attestArgs, nil, time.Minute, probeAbsent(exec))

	Expect(err).To(MatchError(context.Canceled))
	Expect(calls).To(Equal(0), "a cancelled context must not invoke cosign")
}

// Each attempt must receive the full per-attempt timeout, not a slice of a
// shared budget.
func TestRunCosignWithRetry_EachAttemptGetsFullTimeout(t *testing.T) {
	RegisterTestingT(t)
	noBackoff(t)

	var seen []time.Duration
	exec := func(_ context.Context, _ string, _, _ []string, timeout time.Duration) (string, string, error) {
		seen = append(seen, timeout)
		return "", conflictStderr, fmt.Errorf("exit status 1")
	}

	_, _ = runCosignWithRetry(context.Background(), "sbom attest", attestArgs, nil, 90*time.Second, probeAbsent(exec))

	Expect(seen).To(HaveLen(maxCosignAttempts))
	for i, d := range seen {
		Expect(d).To(Equal(90*time.Second), "attempt %d got a reduced budget", i+1)
	}
}

func TestIsRekorConflict(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name   string
		output string
		want   bool
	}{
		{name: "cosign bundle 409 createLogEntryConflict", output: conflictStderr, want: true},
		{name: "bare conflict marker", output: "createLogEntryConflict", want: true},
		{name: "swagger shape without the marker word", output: "[POST /api/v1/log/entries][409] something else", want: true},
		{name: "unrelated 409 from a registry", output: "GET https://registry.example.com/v2/: unexpected status 409", want: false},
		{
			name:   "registry 409 alongside a rekor entry URL",
			output: "unexpected status 409 Conflict; tlog entry: https://rekor.sigstore.dev/api/v1/log/entries?logIndex=77",
			want:   false,
		},
		{
			name:   "image ref carrying the endpoint path",
			output: "Error: signing registry.example.com/team/api/v1/log/entries@sha256:abc: unexpected status 409",
			want:   false,
		},
		{name: "fulcio auth failure", output: "error signing: getting key from Fulcio: oidc: token expired", want: false},
		{name: "empty", output: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			Expect(isRekorConflict(tt.output)).To(Equal(tt.want))
		})
	}
}

func TestCosignBackoff_IsBoundedAndJittered(t *testing.T) {
	RegisterTestingT(t)

	for attempt := 2; attempt <= 6; attempt++ {
		for i := 0; i < 50; i++ {
			d := cosignBackoff(attempt)
			Expect(d).To(BeNumerically(">=", time.Duration(0)))
			Expect(d).To(BeNumerically("<", cosignMaxBackoff))
		}
	}
}

func TestCosignBackoff_ZeroBaseDisablesDelay(t *testing.T) {
	RegisterTestingT(t)
	noBackoff(t)

	Expect(cosignBackoff(2)).To(Equal(time.Duration(0)))
	Expect(cosignBackoff(5)).To(Equal(time.Duration(0)))
}

// giveUpStderr reproduces cosign exhausting its own HTTP retry budget against
// the Rekor log-entries endpoint, the shape seen when several deploys of the
// same digest hit the public-good instance together.
const giveUpStderr = `Error: signing registry.example.com/team/worker@sha256:f7ed9277c480591d7ec36fe7da13e112b33d898b7687f9bcbcda5c214a242099: ` +
	`signing bundle: Post "https://rekor.sigstore.dev/api/v1/log/entries": ` +
	`net/http: TLS handshake timeout, giving up after 2 attempt(s)`

// The whole point of the fix: an entry that Rekor already holds, for an
// artifact the registry already carries, is the desired end state. It must not
// fail the deploy, and it must not burn the retry budget getting there.
func TestRunCosignWithRetry_ConfirmedConflictIsSuccess(t *testing.T) {
	RegisterTestingT(t)
	noBackoff(t)

	attempts, probes := 0, 0
	exec := func(_ context.Context, _ string, _, _ []string, _ time.Duration) (string, string, error) {
		attempts++
		return "", conflictStderr, fmt.Errorf("exit status 1")
	}

	out, err := runCosignWithRetry(context.Background(), "sbom attest", attestArgs, nil, time.Minute, probePresent(exec, &probes))

	Expect(err).ToNot(HaveOccurred(), "a conflict confirmed against the registry is an idempotent success")
	Expect(out).To(BeEmpty(), "no fresh cosign output to report")
	Expect(attempts).To(Equal(1), "confirmation must short-circuit the retry loop, not run it to exhaustion")
	Expect(probes).To(Equal(1))
}

// The probe must ask about the exact artifact the attempt was producing.
// Dropping --predicate-type would let an existing SBOM attestation stand in for
// a missing provenance one.
func TestRunCosignWithRetry_ProbeTargetsTheSpecificArtifact(t *testing.T) {
	RegisterTestingT(t)
	noBackoff(t)

	tests := []struct {
		name      string
		args      []string
		wantProbe []string
	}{
		{
			name:      "keyless sign probes the signature",
			args:      []string{"sign", "--yes", "registry.example.com/team/app@sha256:abc"},
			wantProbe: []string{"download", "signature", "registry.example.com/team/app@sha256:abc"},
		},
		{
			name:      "key-based sign probes the signature",
			args:      []string{"sign", "--key", "/tmp/cosign.key", "registry.example.com/team/app@sha256:abc"},
			wantProbe: []string{"download", "signature", "registry.example.com/team/app@sha256:abc"},
		},
		{
			name: "sbom attest probes the cyclonedx attestation",
			args: []string{"attest", "--predicate", "/tmp/p.json", "--type", "cyclonedx", "--yes", "registry.example.com/team/app@sha256:abc"},
			wantProbe: []string{
				"download", "attestation", "--predicate-type", "cyclonedx",
				"registry.example.com/team/app@sha256:abc",
			},
		},
		{
			name: "provenance attest probes its own predicate URI",
			args: []string{"attest", "--predicate", "/tmp/p.json", "--type", "https://slsa.dev/provenance/v1", "--yes", "registry.example.com/team/app@sha256:abc"},
			wantProbe: []string{
				"download", "attestation", "--predicate-type", "https://slsa.dev/provenance/v1",
				"registry.example.com/team/app@sha256:abc",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			var seen []string
			exec := func(_ context.Context, _ string, args, _ []string, _ time.Duration) (string, string, error) {
				if len(args) > 0 && args[0] == "download" {
					seen = args
					return `{"payload":"e30="}`, "", nil
				}
				return "", conflictStderr, fmt.Errorf("exit status 1")
			}

			_, err := runCosignWithRetry(context.Background(), "attest", tt.args, nil, time.Minute, exec)

			Expect(err).ToNot(HaveOccurred())
			Expect(seen).To(Equal(tt.wantProbe))
		})
	}
}

// Fail-closed: the probe only confirms on a clean exit AND real output. A
// cosign build that exits 0 while printing nothing must not turn a genuine
// signing failure into a green deploy.
func TestRunCosignWithRetry_EmptyProbeOutputIsNotConfirmation(t *testing.T) {
	RegisterTestingT(t)
	noBackoff(t)

	attempts := 0
	exec := func(_ context.Context, _ string, args, _ []string, _ time.Duration) (string, string, error) {
		if len(args) > 0 && args[0] == "download" {
			return "   \n", "", nil
		}
		attempts++
		return "", conflictStderr, fmt.Errorf("exit status 1")
	}

	_, err := runCosignWithRetry(context.Background(), "sbom attest", attestArgs, nil, time.Minute, exec)

	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("createLogEntryConflict"))
	Expect(attempts).To(Equal(maxCosignAttempts))
}

// A confirmation on a later attempt still ends the run successfully.
func TestRunCosignWithRetry_ConflictConfirmedOnLaterAttempt(t *testing.T) {
	RegisterTestingT(t)
	noBackoff(t)

	attempts, probes := 0, 0
	exec := func(_ context.Context, _ string, args, _ []string, _ time.Duration) (string, string, error) {
		if len(args) > 0 && args[0] == "download" {
			probes++
			if probes < 3 {
				return "", "Error: no matching attestations", fmt.Errorf("exit status 1")
			}
			return `{"payload":"e30="}`, "", nil
		}
		attempts++
		return "", conflictStderr, fmt.Errorf("exit status 1")
	}

	_, err := runCosignWithRetry(context.Background(), "sbom attest", attestArgs, nil, time.Minute, exec)

	Expect(err).ToNot(HaveOccurred())
	Expect(attempts).To(Equal(3))
	Expect(probes).To(Equal(3))
}

// Before the fix this fell straight through to the fail-fast branch, so one
// slow transparency log broke the deploy on the first try.
func TestRunCosignWithRetry_RetriesTransientRekorGiveUp(t *testing.T) {
	RegisterTestingT(t)
	noBackoff(t)

	attempts, probes := 0, 0
	exec := func(_ context.Context, _ string, args, _ []string, _ time.Duration) (string, string, error) {
		if len(args) > 0 && args[0] == "download" {
			probes++
			return "", "", fmt.Errorf("probe must not run for a transient failure")
		}
		attempts++
		if attempts == 1 {
			return "", giveUpStderr, fmt.Errorf("exit status 1")
		}
		return "tlog entry created with index: 7", "", nil
	}

	out, err := runCosignWithRetry(context.Background(), "sbom attest", attestArgs, nil, time.Minute, exec)

	Expect(err).ToNot(HaveOccurred())
	Expect(out).To(Equal("tlog entry created with index: 7"))
	Expect(attempts).To(Equal(2))
	Expect(probes).To(Equal(0), "nothing was uploaded, so there is nothing to confirm")
}

func TestRunCosignWithRetry_PersistentGiveUpStillFails(t *testing.T) {
	RegisterTestingT(t)
	noBackoff(t)

	attempts := 0
	exec := func(_ context.Context, _ string, _, _ []string, _ time.Duration) (string, string, error) {
		attempts++
		return "", giveUpStderr, fmt.Errorf("exit status 1")
	}

	_, err := runCosignWithRetry(context.Background(), "sbom attest", attestArgs, nil, time.Minute, probeAbsent(exec))

	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("giving up after 2 attempt"))
	Expect(err.Error()).To(ContainSubstring("after 5 attempts"))
	Expect(attempts).To(Equal(maxCosignAttempts))
}

func TestIsRekorTransient(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name   string
		output string
		want   bool
	}{
		{name: "cosign gave up posting the log entry", output: giveUpStderr, want: true},
		{
			name:   "give-up on a self-hosted rekor",
			output: `signing bundle: Post "https://tlog.internal.example/api/v1/log/entries": EOF, giving up after 3 attempt(s)`,
			want:   true,
		},
		{
			name:   "give-up against fulcio is not a log-entry failure",
			output: `Post "https://fulcio.sigstore.dev/api/v2/signingCert": EOF, giving up after 2 attempt(s)`,
			want:   false,
		},
		{
			name:   "registry give-up is not a log-entry failure",
			output: `Put "https://registry.example.com/v2/team/app/manifests/sha256-abc": EOF, giving up after 4 attempt(s)`,
			want:   false,
		},
		{
			name:   "log-entry POST that succeeded",
			output: `Post "https://rekor.sigstore.dev/api/v1/log/entries": 201 Created`,
			want:   false,
		},
		{name: "a plain conflict is not the transient class", output: conflictStderr, want: false},
		{name: "empty", output: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			Expect(isRekorTransient(tt.output)).To(Equal(tt.want))
		})
	}
}

func TestProbeFor(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name string
		args []string
		ok   bool
		what string
	}{
		{name: "sign", args: []string{"sign", "--yes", "img"}, ok: true, what: "signature"},
		{name: "untyped attest", args: []string{"attest", "--predicate", "p", "img"}, ok: true, what: "attestation"},
		{
			name: "typed attest", args: []string{"attest", "--type=cyclonedx", "img"},
			ok: true, what: "cyclonedx attestation",
		},
		{name: "unknown subcommand gets no probe", args: []string{"copy", "a", "b"}, ok: false},
		{name: "verify gets no probe", args: []string{"verify", "--key", "k", "img"}, ok: false},
		{name: "argv too short", args: []string{"sign"}, ok: false},
		{name: "trailing flag is not an image ref", args: []string{"sign", "--yes"}, ok: false},
		{name: "empty argv", args: nil, ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			probe, ok := probeFor(tt.args)
			Expect(ok).To(Equal(tt.ok))
			if tt.ok {
				Expect(probe.what).To(Equal(tt.what))
				Expect(probe.args[0]).To(Equal("download"))
				Expect(probe.args[len(probe.args)-1]).To(Equal("img"))
			}
		})
	}
}
