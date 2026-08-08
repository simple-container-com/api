// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package signing

import (
	"fmt"
	"testing"

	. "github.com/onsi/gomega"
)

// rekorConflictStderr reproduces the cosign stderr shape observed when several
// deploy jobs attest against the public-good Rekor instance at once and its
// upload retry replays a body that already landed.
const rekorConflictStderr = `Error: signing registry.example.com/team/worker@sha256:f7ed9277c480591d7ec36fe7da13e112b33d898b7687f9bcbcda5c214a242099: ` +
	`signing bundle: error signing bundle: [POST /api/v1/log/entries][409] createLogEntryConflict ` +
	`{"code":409,"message":"an equivalent entry already exists in the transparency log with UUID 108e9186e8c5677a2c45c17488e67ac4beb48541bb66419307a9718e225346040648ffdc7942792e"}`

func TestRetryOnRekorConflict_SucceedsOnRetry(t *testing.T) {
	RegisterTestingT(t)

	calls := 0
	err := RetryOnRekorConflict("attest", func() (string, error) {
		calls++
		if calls == 1 {
			return rekorConflictStderr, fmt.Errorf("exit status 1")
		}
		return "", nil
	})

	Expect(err).ToNot(HaveOccurred())
	Expect(calls).To(Equal(2), "a conflict must trigger exactly one retry")
}

func TestRetryOnRekorConflict_NoRetryOnOtherErrors(t *testing.T) {
	RegisterTestingT(t)

	calls := 0
	err := RetryOnRekorConflict("attest", func() (string, error) {
		calls++
		return "Error: GET https://registry.example.com/v2/: unexpected status 409", fmt.Errorf("exit status 1")
	})

	Expect(err).To(HaveOccurred())
	Expect(calls).To(Equal(1), "an unrelated 409 must not be retried")
}

// A deterministic key reproduces the same signature, so every attempt replays an
// identical Rekor body. Exhausting the loop and surfacing the error is correct:
// cosign uploads to Rekor before it pushes to the registry, so a tlog conflict
// does not prove the attestation was attached.
func TestRetryOnRekorConflict_GivesUpAndReportsError(t *testing.T) {
	RegisterTestingT(t)

	calls := 0
	err := RetryOnRekorConflict("attest", func() (string, error) {
		calls++
		return rekorConflictStderr, fmt.Errorf("cosign attest failed: exit status 1")
	})

	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("cosign attest failed"))
	Expect(calls).To(Equal(MaxCosignAttempts))
}

func TestRetryOnRekorConflict_SucceedsFirstTry(t *testing.T) {
	RegisterTestingT(t)

	calls := 0
	err := RetryOnRekorConflict("attest", func() (string, error) {
		calls++
		return "", nil
	})

	Expect(err).ToNot(HaveOccurred())
	Expect(calls).To(Equal(1))
}
