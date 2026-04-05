package provenance

import (
	"testing"

	"github.com/simple-container-com/api/pkg/security/signing"
)

func TestStatementPredicateFromStatementEnvelope(t *testing.T) {
	statement := &Statement{
		Content: []byte(`{
  "_type":"https://in-toto.io/Statement/v1",
  "predicateType":"https://slsa.dev/provenance/v1",
  "predicate":{"builder":{"id":"builder"}}
}`),
	}

	predicate, err := statement.Predicate()
	if err != nil {
		t.Fatalf("Predicate() error = %v", err)
	}
	if string(predicate) != `{"builder":{"id":"builder"}}` {
		t.Fatalf("Predicate() = %s", string(predicate))
	}
}

func TestStatementPredicateFromBarePredicate(t *testing.T) {
	statement := &Statement{
		Content: []byte(`{"builder":{"id":"builder"}}`),
	}

	predicate, err := statement.Predicate()
	if err != nil {
		t.Fatalf("Predicate() error = %v", err)
	}
	if string(predicate) != `{"builder":{"id":"builder"}}` {
		t.Fatalf("Predicate() = %s", string(predicate))
	}
}

func TestAttestationType(t *testing.T) {
	if got := attestationType(FormatSLSAV10); got != CosignAttestationTypeV10 {
		t.Fatalf("attestationType(v1.0) = %s", got)
	}
	if got := attestationType(FormatSLSAV02); got != CosignAttestationTypeV02 {
		t.Fatalf("attestationType(v0.2) = %s", got)
	}
}

func TestMatchesExpectedEnvelope(t *testing.T) {
	if !matchesExpectedEnvelope(FormatSLSAV10, "https://in-toto.io/Statement/v0.1", "https://slsa.dev/provenance/v1") {
		t.Fatal("matchesExpectedEnvelope() should accept cosign v0.1 envelope for v1 provenance")
	}
	if !matchesExpectedEnvelope(FormatSLSAV10, "https://in-toto.io/Statement/v1", "https://slsa.dev/provenance/v1") {
		t.Fatal("matchesExpectedEnvelope() should accept native v1 envelope for v1 provenance")
	}
	if matchesExpectedEnvelope(FormatSLSAV10, "https://in-toto.io/Statement/v0.1", "https://slsa.dev/provenance/v0.2") {
		t.Fatal("matchesExpectedEnvelope() should reject mismatched predicate type")
	}
}

func TestValidateStatementContent(t *testing.T) {
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

	if err := ValidateStatementContent(statement, ValidateOptions{
		ExpectedFormat:    FormatSLSAV10,
		ExpectedDigest:    "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		ExpectedBuilderID: "https://github.com/simple-container-com/api/actions/runs/123",
		ExpectedSourceURI: "https://github.com/simple-container-com/api.git",
		ExpectedCommit:    "deadbeef",
	}); err != nil {
		t.Fatalf("ValidateStatementContent() unexpected error = %v", err)
	}
}

func TestValidateStatementContentDigestMismatch(t *testing.T) {
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
	if err == nil {
		t.Fatal("ValidateStatementContent() expected error for digest mismatch")
	}
}

func TestValidateStatementContentFormatMismatch(t *testing.T) {
	statement := []byte(`{
  "_type": "https://in-toto.io/Statement/v0.1",
  "predicateType": "https://slsa.dev/provenance/v0.2",
  "subject": [],
  "predicate": {}
}`)

	err := ValidateStatementContent(statement, ValidateOptions{
		ExpectedFormat: FormatSLSAV10,
	})
	if err == nil {
		t.Fatal("ValidateStatementContent() expected error for format mismatch")
	}
}

func TestAttacherBuildSigningEnv(t *testing.T) {
	attacher := &Attacher{}
	if got := attacher.buildSigningEnv(); got != nil {
		t.Fatalf("buildSigningEnv() = %v, want nil", got)
	}

	attacher.SigningConfig = &signing.Config{
		PrivateKey: "/tmp/cosign.key",
		Password:   "",
	}
	got := attacher.buildSigningEnv()
	if len(got) != 1 || got[0] != "COSIGN_PASSWORD=" {
		t.Fatalf("buildSigningEnv() = %v, want [COSIGN_PASSWORD=]", got)
	}
}
