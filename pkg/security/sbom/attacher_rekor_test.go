// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package sbom

import (
	"context"
	"testing"
	"time"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/security/signing"
	"github.com/simple-container-com/api/pkg/security/tools/cosigntest"
)

const attestTimeout = 5 * time.Second

func rekorTestAttacher() *Attacher {
	return &Attacher{
		SigningConfig: &signing.Config{Enabled: true, Keyless: true, OIDCToken: "a.b.c"},
		Timeout:       attestTimeout,
	}
}

func rekorTestSBOM() *SBOM {
	return NewSBOM(FormatCycloneDXJSON, []byte(`{"bomFormat":"CycloneDX"}`), "sha256:f7ed9277", nil)
}

const rekorTestImage = "registry.example.com/team/app@sha256:f7ed9277c480591d7ec36fe7da13e112b33d898b7687f9bcbcda5c214a242099"

// A Rekor conflict on the first attest must not fail the deploy: a fresh keyless
// invocation mints a new ephemeral cert, so the replayed body differs.
func TestAttach_RetriesRekorConflict(t *testing.T) {
	RegisterTestingT(t)

	fake := cosigntest.Install(t, cosigntest.Options{ConflictsBefore: 1})

	err := rekorTestAttacher().Attach(context.Background(), rekorTestSBOM(), rekorTestImage)

	Expect(err).ToNot(HaveOccurred())
	Expect(fake.Calls(t)).To(Equal(2), "conflict must trigger exactly one retry")
}

// cosign reports through both streams; the conflict must be caught on stdout too.
func TestAttach_RetriesRekorConflictOnStdout(t *testing.T) {
	RegisterTestingT(t)

	fake := cosigntest.Install(t, cosigntest.Options{ConflictsBefore: 1, ConflictOnStdout: true})

	err := rekorTestAttacher().Attach(context.Background(), rekorTestSBOM(), rekorTestImage)

	Expect(err).ToNot(HaveOccurred())
	Expect(fake.Calls(t)).To(Equal(2))
}

// Each retry must get its own full timeout budget. With a single shared deadline
// the slow first attempt starves the rest, cosign is SIGKILLed with empty
// stderr, and the conflict is never classified — so the retry silently does not
// happen and the operator loses the createLogEntryConflict diagnostic.
func TestAttach_EachRetryGetsItsOwnTimeoutBudget(t *testing.T) {
	RegisterTestingT(t)

	fake := cosigntest.Install(t, cosigntest.Options{
		ConflictsBefore: 1,
		DelayEach:       (attestTimeout * 2) / 3,
	})

	err := rekorTestAttacher().Attach(context.Background(), rekorTestSBOM(), rekorTestImage)

	Expect(err).ToNot(HaveOccurred())
	Expect(fake.Calls(t)).To(Equal(2), "the retry must not inherit the exhausted budget")
}

// A conflict on every attempt must still fail: cosign uploads to Rekor before it
// pushes to the registry, so a tlog entry does not prove the attestation landed.
func TestAttach_PersistentConflictStillFails(t *testing.T) {
	RegisterTestingT(t)

	fake := cosigntest.Install(t, cosigntest.Options{ConflictsBefore: 99})

	err := rekorTestAttacher().Attach(context.Background(), rekorTestSBOM(), rekorTestImage)

	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("createLogEntryConflict"))
	Expect(fake.Calls(t)).To(Equal(3))
}

func TestAttach_NoRetryOnOtherErrors(t *testing.T) {
	RegisterTestingT(t)

	fake := cosigntest.Install(t, cosigntest.Options{
		FailStderr: "Error: getting signer: retrieving cert: oidc: token expired",
	})

	err := rekorTestAttacher().Attach(context.Background(), rekorTestSBOM(), rekorTestImage)

	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("token expired"))
	Expect(fake.Calls(t)).To(Equal(1), "a non-conflict error must fail fast")
}

func TestAttach_SucceedsWithoutConflict(t *testing.T) {
	RegisterTestingT(t)

	fake := cosigntest.Install(t, cosigntest.Options{})

	err := rekorTestAttacher().Attach(context.Background(), rekorTestSBOM(), rekorTestImage)

	Expect(err).ToNot(HaveOccurred())
	Expect(fake.Calls(t)).To(Equal(1), "no conflict means no retry")
}

// The retry path must work for key-based signing too, not just keyless.
func TestAttach_KeyBasedRetriesRekorConflict(t *testing.T) {
	RegisterTestingT(t)

	fake := cosigntest.Install(t, cosigntest.Options{ConflictsBefore: 1})
	a := &Attacher{
		SigningConfig: &signing.Config{Enabled: true, Keyless: false, PrivateKey: "/tmp/cosign.key", Password: "pw"},
		Timeout:       attestTimeout,
	}

	err := a.Attach(context.Background(), rekorTestSBOM(), rekorTestImage)

	Expect(err).ToNot(HaveOccurred())
	Expect(fake.Calls(t)).To(Equal(2))
}
