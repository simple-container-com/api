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

// Pins the retry bound. Without this every call-count assertion derives from the
// constant itself, so widening 3 -> 5 would go unnoticed.
func TestMaxCosignAttempts_IsPinned(t *testing.T) {
	RegisterTestingT(t)
	Expect(maxCosignAttempts).To(Equal(3))
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

	out, err := runCosignWithRetry(context.Background(), "sbom attest", attestArgs, nil, time.Minute, exec)

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

	_, err := runCosignWithRetry(context.Background(), "sbom attest", attestArgs, nil, time.Minute, exec)

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

	_, err := runCosignWithRetry(context.Background(), "sbom attest", attestArgs, nil, time.Minute, exec)

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

	_, err := runCosignWithRetry(context.Background(), "sbom attest", attestArgs, nil, time.Minute, exec)

	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("token expired"))
	Expect(calls).To(Equal(1))
}

// A fresh invocation always produces a new signature, so exhausting the loop
// means a persistent server-side condition. Surfacing it is correct: cosign
// uploads to Rekor before it pushes to the registry, so a tlog entry does not
// prove the attestation landed.
func TestRunCosignWithRetry_GivesUpAfterMaxAttempts(t *testing.T) {
	RegisterTestingT(t)
	noBackoff(t)

	calls := 0
	exec := func(_ context.Context, _ string, _, _ []string, _ time.Duration) (string, string, error) {
		calls++
		return "", conflictStderr, fmt.Errorf("exit status 1")
	}

	_, err := runCosignWithRetry(context.Background(), "sbom attest", attestArgs, nil, time.Minute, exec)

	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("createLogEntryConflict"))
	Expect(err.Error()).To(ContainSubstring("after 3 attempts"))
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

	_, err := runCosignWithRetry(context.Background(), "sbom attest", attestArgs, nil, time.Minute, exec)

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

	_, err := runCosignWithRetry(context.Background(), "sbom attest", attestArgs, nil, time.Minute, exec)

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

	_, err := runCosignWithRetry(context.Background(), "sbom attest", attestArgs, nil, time.Minute, exec)

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

	_, err := runCosignWithRetry(ctx, "sbom attest", attestArgs, nil, time.Minute, exec)

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

	_, _ = runCosignWithRetry(context.Background(), "sbom attest", attestArgs, nil, 90*time.Second, exec)

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
