// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package signing

import (
	"context"
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/security/tools/cosigntest"
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

// attestConfirm is the confirmation probe an SBOM attest carries in production:
// a typed verify-attestation bound to a verification key, with the image
// reference left off for the retry loop to append.
var attestConfirm = &ConfirmProbe{
	Args: []string{"verify-attestation", "--type", "cyclonedx", "--key", "/tmp/cosign.pub"},
	What: "cyclonedx attestation",
}

// isProbe reports whether an argv is the read-only confirmation probe rather
// than the sign/attest under test.
func isProbe(args []string) bool {
	return len(args) > 0 && (args[0] == "verify" || args[0] == "verify-attestation")
}

// probeAbsent wraps a fake exec so the confirmation probe reports the artifact
// as not verifiable. Attempt counts in the wrapped fake then stay about
// sign/attest invocations only.
func probeAbsent(exec execFn) execFn {
	return func(ctx context.Context, name string, args, env []string, timeout time.Duration) (string, string, error) {
		if isProbe(args) {
			return "", "Error: no matching attestations", fmt.Errorf("exit status 1")
		}
		return exec(ctx, name, args, env, timeout)
	}
}

// probePresent is probeAbsent's twin: the probe verifies, which is what a
// redeploy of an unchanged digest sees. It returns empty stdout on purpose.
// cosign writes its verification banner to stderr and prints nothing on stdout
// for `verify`, so a loop that demanded non-empty stdout could never confirm
// against a real cosign.
func probePresent(exec execFn, probes *int) execFn {
	return func(ctx context.Context, name string, args, env []string, timeout time.Duration) (string, string, error) {
		if isProbe(args) {
			if probes != nil {
				*probes++
			}
			return "", "Verification for registry.example.com/team/app --\n", nil
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

	out, _, err := runCosignWithRetry(context.Background(), "sbom attest", attestArgs, nil, time.Minute, attestConfirm, probeAbsent(exec))

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

	_, _, err := runCosignWithRetry(context.Background(), "sbom attest", attestArgs, nil, time.Minute, attestConfirm, probeAbsent(exec))

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

	_, _, err := runCosignWithRetry(context.Background(), "sbom attest", attestArgs, nil, time.Minute, attestConfirm, probeAbsent(exec))

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

	_, _, err := runCosignWithRetry(context.Background(), "sbom attest", attestArgs, nil, time.Minute, attestConfirm, probeAbsent(exec))

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

	_, _, err := runCosignWithRetry(context.Background(), "sbom attest", attestArgs, nil, time.Minute, attestConfirm, probeAbsent(exec))

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

	_, _, err := runCosignWithRetry(context.Background(), "sbom attest", attestArgs, nil, time.Minute, attestConfirm, probeAbsent(exec))

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

	_, _, err := runCosignWithRetry(context.Background(), "sbom attest", attestArgs, nil, time.Minute, attestConfirm, probeAbsent(exec))

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

	_, _, err := runCosignWithRetry(context.Background(), "sbom attest", attestArgs, nil, time.Minute, attestConfirm, probeAbsent(exec))

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

	_, _, err := runCosignWithRetry(ctx, "sbom attest", attestArgs, nil, time.Minute, attestConfirm, probeAbsent(exec))

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

	_, _, _ = runCosignWithRetry(context.Background(), "sbom attest", attestArgs, nil, 90*time.Second, attestConfirm, probeAbsent(exec))

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

	out, _, err := runCosignWithRetry(context.Background(), "sbom attest", attestArgs, nil, time.Minute, attestConfirm, probePresent(exec, &probes))

	Expect(err).ToNot(HaveOccurred(), "a conflict confirmed against the registry is an idempotent success")
	Expect(out).To(BeEmpty(), "no fresh cosign output to report")
	Expect(attempts).To(Equal(1), "confirmation must short-circuit the retry loop, not run it to exhaustion")
	Expect(probes).To(Equal(1))
}

// The probe must ask about the exact artifact the attempt was producing, under
// the identity the caller signs with, against the image the attempt targeted.
// Dropping --type would let an existing SBOM attestation stand in for a missing
// provenance one; dropping the identity flags would accept an artifact produced
// under a rotated key or by an unrelated workflow.
func TestRunCosignWithRetry_ProbeTargetsTheSpecificArtifact(t *testing.T) {
	RegisterTestingT(t)

	const imageRef = "registry.example.com/team/app@sha256:abc"
	keyless := &Config{Keyless: true, IdentityRegexp: "^https://example.test/wf@refs/heads/main$", OIDCIssuer: "https://token.example.test"}
	keyed := &Config{PublicKey: "/tmp/cosign.pub"}

	tests := []struct {
		name      string
		args      []string
		confirm   *ConfirmProbe
		wantProbe []string
	}{
		{
			name:    "keyless sign verifies the signature under the signing identity",
			args:    []string{"sign", "--yes", imageRef},
			confirm: keyless.SignatureConfirmProbe(),
			wantProbe: []string{
				"verify",
				"--certificate-identity-regexp", "^https://example.test/wf@refs/heads/main$",
				"--certificate-oidc-issuer", "https://token.example.test",
				imageRef,
			},
		},
		{
			name:      "key-based sign verifies the signature under the key",
			args:      []string{"sign", "--key", "/tmp/cosign.key", imageRef},
			confirm:   &ConfirmProbe{Args: []string{"verify", "--key", "/tmp/cosign.key"}, What: "signature"},
			wantProbe: []string{"verify", "--key", "/tmp/cosign.key", imageRef},
		},
		{
			name:      "sbom attest verifies the cyclonedx attestation",
			args:      []string{"attest", "--predicate", "/tmp/p.json", "--type", "cyclonedx", "--yes", imageRef},
			confirm:   keyed.AttestationConfirmProbe("cyclonedx"),
			wantProbe: []string{"verify-attestation", "--type", "cyclonedx", "--key", "/tmp/cosign.pub", imageRef},
		},
		{
			name:    "provenance attest verifies its own predicate URI",
			args:    []string{"attest", "--predicate", "/tmp/p.json", "--type", "https://slsa.dev/provenance/v1", "--yes", imageRef},
			confirm: keyed.AttestationConfirmProbe("https://slsa.dev/provenance/v1"),
			wantProbe: []string{
				"verify-attestation", "--type", "https://slsa.dev/provenance/v1",
				"--key", "/tmp/cosign.pub", imageRef,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			var seen []string
			exec := func(_ context.Context, _ string, args, _ []string, _ time.Duration) (string, string, error) {
				if isProbe(args) {
					seen = args
					return "", "", nil
				}
				return "", conflictStderr, fmt.Errorf("exit status 1")
			}

			_, confirmed, err := runCosignWithRetry(context.Background(), "attest", tt.args, nil, time.Minute, tt.confirm, exec)

			Expect(err).ToNot(HaveOccurred())
			Expect(confirmed).To(BeTrue())
			Expect(seen).To(Equal(tt.wantProbe))
		})
	}
}

// The probe argv the loop builds must not alias the caller's ConfirmProbe.
// Appending the image reference in place would grow a shared backing array and
// leave the next probe carrying the previous image.
func TestRunCosignWithRetry_ProbeDoesNotMutateTheCallersArgs(t *testing.T) {
	RegisterTestingT(t)
	noBackoff(t)

	confirm := &ConfirmProbe{Args: []string{"verify", "--key", "/tmp/cosign.pub"}, What: "signature"}
	before := append([]string(nil), confirm.Args...)

	exec := func(_ context.Context, _ string, args, _ []string, _ time.Duration) (string, string, error) {
		if isProbe(args) {
			return "", "", fmt.Errorf("exit status 1")
		}
		return "", conflictStderr, fmt.Errorf("exit status 1")
	}

	_, _, _ = runCosignWithRetry(context.Background(), "sign",
		[]string{"sign", "--yes", "registry.example.com/team/app@sha256:abc"}, nil, time.Minute, confirm, exec)

	Expect(confirm.Args).To(Equal(before), "the loop must clone the probe argv before appending the image")
}

// The probe's exit status is the whole signal. `cosign verify` writes its
// banner to stderr and nothing to stdout, so the earlier rule of "clean exit
// AND non-empty stdout" could never confirm against a real cosign. The
// converse still has to hold: output without a clean exit confirms nothing.
func TestRunCosignWithRetry_ExitCodeIsTheConfirmationSignal(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name         string
		probeStdout  string
		probeErr     error
		wantErr      bool
		wantAttempts int
	}{
		{
			name:         "exit 0 with empty stdout confirms",
			wantAttempts: 1,
		},
		{
			name:         "non-zero exit confirms nothing, however much it prints",
			probeStdout:  `{"payloadType":"application/vnd.in-toto+json","payload":"e30="}`,
			probeErr:     fmt.Errorf("exit status 1"),
			wantErr:      true,
			wantAttempts: maxCosignAttempts,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			noBackoff(t)

			attempts := 0
			exec := func(_ context.Context, _ string, args, _ []string, _ time.Duration) (string, string, error) {
				if isProbe(args) {
					return tt.probeStdout, "", tt.probeErr
				}
				attempts++
				return "", conflictStderr, fmt.Errorf("exit status 1")
			}

			_, confirmed, err := runCosignWithRetry(context.Background(), "sbom attest", attestArgs, nil, time.Minute, attestConfirm, exec)

			if tt.wantErr {
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("createLogEntryConflict"))
				Expect(confirmed).To(BeFalse())
			} else {
				Expect(err).ToNot(HaveOccurred())
				Expect(confirmed).To(BeTrue())
			}
			Expect(attempts).To(Equal(tt.wantAttempts))
		})
	}
}

// A caller that cannot name an identity to verify against passes no probe. It
// must keep the retry-then-report path rather than assume the conflict benign:
// signing an image the caller cannot verify is not the same as having signed it.
func TestRunCosignWithRetry_NilConfirmNeverProbes(t *testing.T) {
	RegisterTestingT(t)
	noBackoff(t)

	attempts, probes := 0, 0
	exec := func(_ context.Context, _ string, args, _ []string, _ time.Duration) (string, string, error) {
		if isProbe(args) {
			probes++
			return "", "", nil
		}
		attempts++
		return "", conflictStderr, fmt.Errorf("exit status 1")
	}

	_, confirmed, err := runCosignWithRetry(context.Background(), "sbom attest", attestArgs, nil, time.Minute, nil, exec)

	Expect(err).To(HaveOccurred())
	Expect(confirmed).To(BeFalse())
	Expect(probes).To(Equal(0))
	Expect(attempts).To(Equal(maxCosignAttempts))
}

// A confirmation on a later attempt still ends the run successfully.
func TestRunCosignWithRetry_ConflictConfirmedOnLaterAttempt(t *testing.T) {
	RegisterTestingT(t)
	noBackoff(t)

	attempts, probes := 0, 0
	exec := func(_ context.Context, _ string, args, _ []string, _ time.Duration) (string, string, error) {
		if isProbe(args) {
			probes++
			if probes < 3 {
				return "", "Error: no matching attestations", fmt.Errorf("exit status 1")
			}
			return "", "", nil
		}
		attempts++
		return "", conflictStderr, fmt.Errorf("exit status 1")
	}

	_, _, err := runCosignWithRetry(context.Background(), "sbom attest", attestArgs, nil, time.Minute, attestConfirm, exec)

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
		if isProbe(args) {
			probes++
			return "", "", fmt.Errorf("probe must not run for a transient failure")
		}
		attempts++
		if attempts == 1 {
			return "", giveUpStderr, fmt.Errorf("exit status 1")
		}
		return "tlog entry created with index: 7", "", nil
	}

	out, _, err := runCosignWithRetry(context.Background(), "sbom attest", attestArgs, nil, time.Minute, attestConfirm, exec)

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

	_, _, err := runCosignWithRetry(context.Background(), "sbom attest", attestArgs, nil, time.Minute, attestConfirm, probeAbsent(exec))

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
		// The three below all matched before the pattern was tightened. The old
		// form allowed 400 arbitrary characters between the two halves with
		// (?s), so a successful log-entry POST followed anywhere in the same
		// stream by an unrelated give-up read as a transparency-log hiccup: a
		// permanent registry-auth failure was retried five times and the
		// operator was sent to the wrong runbook.
		{
			name: "successful log-entry POST followed by a registry give-up",
			output: `Post "https://rekor.sigstore.dev/api/v1/log/entries": 201 Created` + "\n" +
				`pushing signature: Put "https://registry.example.com/v2/team/app/manifests/sha256-abc": unauthorized, giving up after 3 attempt(s)`,
			want: false,
		},
		{
			name: "successful log-entry POST followed by a fulcio give-up",
			output: `Post "https://rekor.sigstore.dev/api/v1/log/entries": 201 Created` + "\n" +
				`Post "https://fulcio.sigstore.dev/api/v2/signingCert": EOF, giving up after 2 attempt(s)`,
			want: false,
		},
		{
			name: "successful log-entry POST followed by an OIDC give-up",
			output: `Post "https://rekor.sigstore.dev/api/v1/log/entries": 201 Created` + "\n" +
				`fetching ambient token: Post "https://oauth2.example.test/token": EOF, giving up after 5 attempt(s)`,
			want: false,
		},
		{
			name:   "a give-up whose quoted URL is a different endpoint on the same host",
			output: `Post "https://rekor.sigstore.dev/api/v1/index/retrieve": EOF, giving up after 2 attempt(s)`,
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

// The probe must not inherit the signing budget. With it, the worst case per
// artifact became maxCosignAttempts x (timeout + timeout) plus backoff: about
// 20 minutes for an attacher on a 2-minute budget, against 6 minutes before
// confirmation existed, and both attachers run per image.
func TestRunCosignWithRetry_ProbeTimeoutIsBoundedIndependently(t *testing.T) {
	RegisterTestingT(t)
	noBackoff(t)

	const signBudget = 5 * time.Minute
	var signTimeouts, probeTimeouts []time.Duration
	exec := func(_ context.Context, _ string, args, _ []string, timeout time.Duration) (string, string, error) {
		if isProbe(args) {
			probeTimeouts = append(probeTimeouts, timeout)
			return "", "", fmt.Errorf("exit status 1")
		}
		signTimeouts = append(signTimeouts, timeout)
		return "", conflictStderr, fmt.Errorf("exit status 1")
	}

	_, _, _ = runCosignWithRetry(context.Background(), "sbom attest", attestArgs, nil, signBudget, attestConfirm, exec)

	Expect(signTimeouts).To(HaveLen(maxCosignAttempts))
	for i, d := range signTimeouts {
		Expect(d).To(Equal(signBudget), "signing attempt %d must keep the full budget", i+1)
	}
	Expect(probeTimeouts).To(HaveLen(maxCosignAttempts))
	for i, d := range probeTimeouts {
		Expect(d).To(Equal(maxConfirmProbeTimeout), "probe %d must be capped independently", i+1)
	}
}

func TestConfirmProbeTimeout(t *testing.T) {
	RegisterTestingT(t)

	Expect(maxConfirmProbeTimeout).To(Equal(30*time.Second), "the cap is what bounds the worst case; pinned deliberately")
	Expect(confirmProbeTimeout(5 * time.Minute)).To(Equal(maxConfirmProbeTimeout))
	Expect(confirmProbeTimeout(2 * time.Minute)).To(Equal(maxConfirmProbeTimeout))
	Expect(confirmProbeTimeout(10*time.Second)).To(Equal(10*time.Second), "a budget under the cap is used as is")
	Expect(confirmProbeTimeout(0)).To(Equal(maxConfirmProbeTimeout))
	Expect(confirmProbeTimeout(-time.Second)).To(Equal(maxConfirmProbeTimeout))
}

// The probe reads the registry. It mints no certificate, so it has no business
// holding the OIDC identity token the signing invocation was given.
func TestRunCosignWithRetry_ProbeDropsTheIdentityToken(t *testing.T) {
	RegisterTestingT(t)
	noBackoff(t)

	signEnv := []string{
		"COSIGN_EXPERIMENTAL=1",
		"SIGSTORE_ID_TOKEN=header.payload.signature",
		"COSIGN_PASSWORD=pw",
		"DOCKER_CONFIG=/tmp/docker",
		"MALFORMED",
	}

	var seenSignEnv, seenProbeEnv []string
	exec := func(_ context.Context, _ string, args, env []string, _ time.Duration) (string, string, error) {
		if isProbe(args) {
			seenProbeEnv = env
			return "", "", nil
		}
		seenSignEnv = env
		return "", conflictStderr, fmt.Errorf("exit status 1")
	}

	_, confirmed, err := runCosignWithRetry(context.Background(), "sbom attest", attestArgs, signEnv, time.Minute, attestConfirm, exec)

	Expect(err).ToNot(HaveOccurred())
	Expect(confirmed).To(BeTrue())
	Expect(seenSignEnv).To(Equal(signEnv), "signing itself still needs the token")
	Expect(seenProbeEnv).To(Equal([]string{"COSIGN_PASSWORD=pw", "DOCKER_CONFIG=/tmp/docker"}))
}

func TestConfig_ConfirmProbes(t *testing.T) {
	RegisterTestingT(t)

	const (
		ident  = "^https://example.test/wf@refs/heads/main$"
		issuer = "https://token.example.test"
	)

	tests := []struct {
		name       string
		cfg        *Config
		wantSign   []string
		wantAttest []string
	}{
		{
			name:       "keyless with a complete identity",
			cfg:        &Config{Keyless: true, IdentityRegexp: ident, OIDCIssuer: issuer},
			wantSign:   []string{"verify", "--certificate-identity-regexp", ident, "--certificate-oidc-issuer", issuer},
			wantAttest: []string{"verify-attestation", "--type", "cyclonedx", "--certificate-identity-regexp", ident, "--certificate-oidc-issuer", issuer},
		},
		{name: "keyless without an issuer cannot attribute a signature", cfg: &Config{Keyless: true, IdentityRegexp: ident}},
		{name: "keyless without an identity regexp cannot attribute a signature", cfg: &Config{Keyless: true, OIDCIssuer: issuer}},
		{
			name:       "key-based prefers the public key",
			cfg:        &Config{PublicKey: "/tmp/cosign.pub", PrivateKey: "/tmp/cosign.key"},
			wantSign:   []string{"verify", "--key", "/tmp/cosign.pub"},
			wantAttest: []string{"verify-attestation", "--type", "cyclonedx", "--key", "/tmp/cosign.pub"},
		},
		{
			name:       "key-based falls back to the signing key, which cosign can derive the public half from",
			cfg:        &Config{PrivateKey: "/tmp/cosign.key"},
			wantSign:   []string{"verify", "--key", "/tmp/cosign.key"},
			wantAttest: []string{"verify-attestation", "--type", "cyclonedx", "--key", "/tmp/cosign.key"},
		},
		{name: "no key material at all", cfg: &Config{}},
		{name: "nil config", cfg: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)

			sign := tt.cfg.SignatureConfirmProbe()
			attest := tt.cfg.AttestationConfirmProbe("cyclonedx")

			if tt.wantSign == nil {
				Expect(sign).To(BeNil(), "a probe that cannot check identity must not exist")
				Expect(attest).To(BeNil())
				return
			}
			Expect(sign).ToNot(BeNil())
			Expect(sign.Args).To(Equal(tt.wantSign))
			Expect(sign.What).To(Equal("signature"))
			Expect(attest).ToNot(BeNil())
			Expect(attest.Args).To(Equal(tt.wantAttest))
			Expect(attest.What).To(Equal("cyclonedx attestation"))
		})
	}

	Expect((&Config{PublicKey: "/tmp/cosign.pub"}).AttestationConfirmProbe("")).To(BeNil(),
		"an untyped verify-attestation accepts any predicate, which is the bug this replaced")
}

// The two exported wrappers, driven through the real exec path rather than an
// injected fake, so the argv actually reaches a process. RunCosignWithRetry is
// the no-confirmation form: it must retry a conflict and never probe.
func TestExportedWrappers_DriveTheRealExecPath(t *testing.T) {
	t.Run("RunCosignWithRetry retries a conflict without probing", func(t *testing.T) {
		RegisterTestingT(t)
		noBackoff(t)

		fake := cosigntest.Install(t, cosigntest.Options{ConflictsBefore: 1})

		out, err := RunCosignWithRetry(context.Background(), "sbom attest",
			[]string{"attest", "--predicate", "/tmp/p.json", "registry.example.com/team/app@sha256:abc"},
			nil, 30*time.Second)

		Expect(err).ToNot(HaveOccurred())
		Expect(out).To(ContainSubstring("tlog entry created with index"))
		Expect(fake.Calls(t)).To(Equal(2))
		Expect(fake.Probes(t)).To(Equal(0), "no probe was supplied, so none may run")
	})

	t.Run("RunCosignWithRetryConfirm confirms a permanent conflict", func(t *testing.T) {
		RegisterTestingT(t)
		noBackoff(t)

		const image = "registry.example.com/team/app@sha256:abc"
		fake := cosigntest.Install(t, cosigntest.Options{
			ConflictsBefore:         99,
			ArtifactAlreadyAttached: true,
			Image:                   image,
		})

		out, err := RunCosignWithRetryConfirm(context.Background(), "sbom attest",
			[]string{"attest", "--predicate", "/tmp/p.json", "--type", "cyclonedx", image},
			nil, 30*time.Second, attestConfirm)

		Expect(err).ToNot(HaveOccurred())
		Expect(out).To(BeEmpty(), "nothing fresh was produced")
		Expect(fake.Calls(t)).To(Equal(1))
		Expect(fake.LastProbeArgs(t)).To(Equal(append(append([]string{}, attestConfirm.Args...), image)))
	})
}
