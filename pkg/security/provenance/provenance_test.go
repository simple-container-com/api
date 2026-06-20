// SPDX-License-Identifier: MIT
// Copyright (c) 2025 Simple Container

package provenance

import (
	"testing"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/security/signing"
)

func TestStatementPredicateFromStatementEnvelope(t *testing.T) {
	RegisterTestingT(t)

	statement := &Statement{
		Content: []byte(`{
  "_type":"https://in-toto.io/Statement/v1",
  "predicateType":"https://slsa.dev/provenance/v1",
  "predicate":{"builder":{"id":"builder"}}
}`),
	}

	predicate, err := statement.Predicate()
	Expect(err).ToNot(HaveOccurred())
	Expect(string(predicate)).To(Equal(`{"builder":{"id":"builder"}}`))
}

func TestStatementPredicateFromBarePredicate(t *testing.T) {
	RegisterTestingT(t)

	statement := &Statement{
		Content: []byte(`{"builder":{"id":"builder"}}`),
	}

	predicate, err := statement.Predicate()
	Expect(err).ToNot(HaveOccurred())
	Expect(string(predicate)).To(Equal(`{"builder":{"id":"builder"}}`))
}

func TestAttestationType(t *testing.T) {
	RegisterTestingT(t)

	// Canonical URI form so the value `cosign attest --type ...` writes into
	// the DSSE envelope's predicateType is byte-identical to what
	// `cosign verify-attestation --type ...` matches against. Asymmetry here
	// silently breaks the attach→verify cycle (see PR fixing the SLSA-v1
	// verify-provenance regression).
	Expect(attestationType(FormatSLSAV10)).To(Equal(PredicateTypeSLSAV10))
	Expect(attestationType(FormatSLSAV02)).To(Equal(PredicateTypeSLSAV02))
	Expect(attestationType(FormatSLSAV10)).To(Equal("https://slsa.dev/provenance/v1"))
	Expect(attestationType(FormatSLSAV02)).To(Equal("https://slsa.dev/provenance/v0.2"))
}

func TestAttachAndVerifyUseSamePredicateType(t *testing.T) {
	RegisterTestingT(t)

	// Regression guard: the verify-attestation step in
	// pkg/clouds/pulumi/docker/build_and_push.go hard-coded
	// "https://slsa.dev/provenance/v1" while Attacher.Attach passed the
	// short alias "slsaprovenance1" to `cosign attest --type`. Cosign 3.x
	// changed how aliases resolve on the verify side; only matching URIs
	// are guaranteed to round-trip. Both sides MUST use the same constant.
	attachType := attestationType(FormatSLSAV10)
	verifyType := PredicateTypeSLSAV10
	Expect(attachType).To(Equal(verifyType),
		"attest --type must equal verify-attestation --type (got attest=%q verify=%q)",
		attachType, verifyType)
}

func TestMatchesExpectedEnvelope(t *testing.T) {
	RegisterTestingT(t)

	Expect(matchesExpectedEnvelope(FormatSLSAV10, "https://in-toto.io/Statement/v0.1", "https://slsa.dev/provenance/v1")).To(BeTrue(),
		"should accept cosign v0.1 envelope for v1 provenance")
	Expect(matchesExpectedEnvelope(FormatSLSAV10, "https://in-toto.io/Statement/v1", "https://slsa.dev/provenance/v1")).To(BeTrue(),
		"should accept native v1 envelope for v1 provenance")
	Expect(matchesExpectedEnvelope(FormatSLSAV10, "https://in-toto.io/Statement/v0.1", "https://slsa.dev/provenance/v0.2")).To(BeFalse(),
		"should reject mismatched predicate type")
}

func TestValidateStatementContent(t *testing.T) {
	RegisterTestingT(t)

	statement := []byte(`{
  "_type": "https://in-toto.io/Statement/v1",
  "subject": [
    {
      "name": "docker.io/simplecontainer/test@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
      "digest": {
        "sha256": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
      }
    }
  ],
  "predicateType": "https://slsa.dev/provenance/v1",
  "predicate": {
    "buildDefinition": {
      "resolvedDependencies": [
        {
          "uri": "https://github.com/simple-container-com/api.git",
          "digest": {
            "sha1": "deadbeef"
          }
        }
      ]
    },
    "runDetails": {
      "builder": {
        "id": "https://github.com/simple-container-com/api/actions/runs/123"
      }
    }
  }
}`)

	err := ValidateStatementContent(statement, ValidateOptions{
		ExpectedFormat:    FormatSLSAV10,
		ExpectedDigest:    "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		ExpectedBuilderID: "https://github.com/simple-container-com/api/actions/runs/123",
		ExpectedSourceURI: "https://github.com/simple-container-com/api.git",
		ExpectedCommit:    "deadbeef",
	})
	Expect(err).ToNot(HaveOccurred())
}

func TestValidateStatementContentDigestMismatch(t *testing.T) {
	RegisterTestingT(t)

	statement := []byte(`{
  "subject": [
    {
      "name": "docker.io/simplecontainer/test@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
      "digest": {
        "sha256": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
      }
    }
  ],
  "predicate": {}
}`)

	err := ValidateStatementContent(statement, ValidateOptions{
		ExpectedDigest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	})
	Expect(err).To(HaveOccurred())
}

func TestValidateStatementContentFormatMismatch(t *testing.T) {
	RegisterTestingT(t)

	statement := []byte(`{
  "_type": "https://in-toto.io/Statement/v0.1",
  "predicateType": "https://slsa.dev/provenance/v0.2",
  "subject": [],
  "predicate": {}
}`)

	err := ValidateStatementContent(statement, ValidateOptions{
		ExpectedFormat: FormatSLSAV10,
	})
	Expect(err).To(HaveOccurred())
}

func TestAttacherBuildSigningEnv(t *testing.T) {
	RegisterTestingT(t)

	attacher := &Attacher{}
	Expect(attacher.buildSigningEnv()).To(BeNil())

	attacher.SigningConfig = &signing.Config{
		PrivateKey: "/tmp/cosign.key",
		Password:   "",
	}
	got := attacher.buildSigningEnv()
	Expect(got).To(HaveLen(1))
	Expect(got[0]).To(Equal("COSIGN_PASSWORD="))
}
