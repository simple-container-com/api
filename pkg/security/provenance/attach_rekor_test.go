// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package provenance

import (
	"context"
	"testing"
	"time"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/security/signing"
	"github.com/simple-container-com/api/pkg/security/tools/cosigntest"
)

const provAttestTimeout = 5 * time.Second

const provTestImage = "registry.example.com/team/app@sha256:e6ba56b60370949f74b515333ea56a827c1775002aa8b808e37797d1f4304309"

func provTestStatement() *Statement {
	predicate := []byte(`{"buildDefinition":{"buildType":"https://simple-container.com/build/v1"}}`)
	return NewStatement(FormatSLSAV10, predicate, provTestImage, &Metadata{BuilderID: "sc"})
}

func provTestAttacher() *Attacher {
	return &Attacher{
		SigningConfig: &signing.Config{Enabled: true, Keyless: true, OIDCToken: "a.b.c"},
		Timeout:       provAttestTimeout,
	}
}

// Provenance attest and SBOM attest run against the same digest, so provenance
// needs the same conflict tolerance.
func TestProvenanceAttach_RetriesRekorConflict(t *testing.T) {
	RegisterTestingT(t)

	fake := cosigntest.Install(t, cosigntest.Options{ConflictsBefore: 1})

	err := provTestAttacher().Attach(context.Background(), provTestStatement(), provTestImage)

	Expect(err).ToNot(HaveOccurred())
	Expect(fake.Calls(t)).To(Equal(2), "conflict must trigger exactly one retry")
}

func TestProvenanceAttach_RetriesRekorConflictOnStdout(t *testing.T) {
	RegisterTestingT(t)

	fake := cosigntest.Install(t, cosigntest.Options{ConflictsBefore: 1, ConflictOnStdout: true})

	err := provTestAttacher().Attach(context.Background(), provTestStatement(), provTestImage)

	Expect(err).ToNot(HaveOccurred())
	Expect(fake.Calls(t)).To(Equal(2))
}

// See the sbom twin: a shared deadline starves the retry and erases the conflict.
func TestProvenanceAttach_EachRetryGetsItsOwnTimeoutBudget(t *testing.T) {
	RegisterTestingT(t)

	fake := cosigntest.Install(t, cosigntest.Options{
		ConflictsBefore: 1,
		DelayEach:       (provAttestTimeout * 2) / 3,
	})

	err := provTestAttacher().Attach(context.Background(), provTestStatement(), provTestImage)

	Expect(err).ToNot(HaveOccurred())
	Expect(fake.Calls(t)).To(Equal(2), "the retry must not inherit the exhausted budget")
}

func TestProvenanceAttach_PersistentConflictStillFails(t *testing.T) {
	RegisterTestingT(t)

	fake := cosigntest.Install(t, cosigntest.Options{ConflictsBefore: 99})

	err := provTestAttacher().Attach(context.Background(), provTestStatement(), provTestImage)

	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("createLogEntryConflict"))
	Expect(fake.Calls(t)).To(Equal(3))
}

func TestProvenanceAttach_NoRetryOnOtherErrors(t *testing.T) {
	RegisterTestingT(t)

	fake := cosigntest.Install(t, cosigntest.Options{
		FailStderr: "Error: getting signer: retrieving cert: oidc: token expired",
	})

	err := provTestAttacher().Attach(context.Background(), provTestStatement(), provTestImage)

	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("token expired"))
	Expect(fake.Calls(t)).To(Equal(1), "a non-conflict error must fail fast")
}

func TestProvenanceAttach_SucceedsWithoutConflict(t *testing.T) {
	RegisterTestingT(t)

	fake := cosigntest.Install(t, cosigntest.Options{})

	err := provTestAttacher().Attach(context.Background(), provTestStatement(), provTestImage)

	Expect(err).ToNot(HaveOccurred())
	Expect(fake.Calls(t)).To(Equal(1))
}
