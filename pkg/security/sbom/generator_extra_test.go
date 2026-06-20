package sbom

import (
	"encoding/base64"
	"encoding/json"
	"testing"

	. "github.com/onsi/gomega"
)

func TestSBOMValidateDigest(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name    string
		mutate  func(s *SBOM)
		want    bool
	}{
		{
			name:   "Untampered content matches digest",
			mutate: func(s *SBOM) {},
			want:   true,
		},
		{
			name:   "Tampered content fails digest",
			mutate: func(s *SBOM) { s.Content = []byte("tampered") },
			want:   false,
		},
		{
			name:   "Tampered digest fails validation",
			mutate: func(s *SBOM) { s.Digest = "deadbeef" },
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			s := NewSBOM(FormatCycloneDXJSON, []byte(`{"bomFormat":"CycloneDX"}`), "img", &Metadata{ToolName: "syft"})
			tt.mutate(s)
			Expect(s.ValidateDigest()).To(Equal(tt.want))
		})
	}
}

func TestSBOMSize(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name    string
		content []byte
		want    int
	}{
		{"Empty content", []byte{}, 0},
		{"Nil content", nil, 0},
		{"Short content", []byte("abc"), 3},
		{"JSON content", []byte(`{"a":1}`), 7},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			s := NewSBOM(FormatSyftJSON, tt.content, "img", nil)
			Expect(s.Size()).To(Equal(tt.want))
		})
	}
}

func TestNewSBOMComputesDigestAndFields(t *testing.T) {
	RegisterTestingT(t)

	content := []byte(`{"components":[]}`)
	meta := &Metadata{ToolName: "syft", ToolVersion: "1.0.0", PackageCount: 0}
	s := NewSBOM(FormatCycloneDXJSON, content, "sha256:abc", meta)

	Expect(s.Format).To(Equal(FormatCycloneDXJSON))
	Expect(s.Content).To(Equal(content))
	Expect(s.ImageDigest).To(Equal("sha256:abc"))
	Expect(s.Metadata).To(Equal(meta))
	Expect(s.Digest).To(HaveLen(64)) // hex-encoded sha256
	Expect(s.ValidateDigest()).To(BeTrue())
	Expect(s.GeneratedAt.IsZero()).To(BeFalse())
}

func TestFormatString(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name   string
		format Format
		want   string
	}{
		{"CycloneDX JSON", FormatCycloneDXJSON, "cyclonedx-json"},
		{"SPDX JSON", FormatSPDXJSON, "spdx-json"},
		{"Syft JSON", FormatSyftJSON, "syft-json"},
		{"Arbitrary value passes through", Format("custom-thing"), "custom-thing"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			Expect(tt.format.String()).To(Equal(tt.want))
		})
	}
}

func TestExtractPackageCount(t *testing.T) {
	RegisterTestingT(t)

	g := NewSyftGenerator()

	tests := []struct {
		name    string
		format  Format
		content string
		want    int
		wantErr bool
	}{
		{"CycloneDX components", FormatCycloneDXJSON, `{"components":[{"name":"a"},{"name":"b"}]}`, 2, false},
		{"SPDX packages", FormatSPDXJSON, `{"packages":[{"name":"a"}]}`, 1, false},
		{"Syft artifacts", FormatSyftJSON, `{"artifacts":[{"name":"a"},{"name":"b"},{"name":"c"}]}`, 3, false},
		{"Unsupported format errors", FormatCycloneDXXML, `<xml/>`, 0, true},
		{"SPDX tag-value unsupported errors", FormatSPDXTagValue, `nope`, 0, true},
		{"CycloneDX invalid JSON errors", FormatCycloneDXJSON, `{bad`, 0, true},
		{"SPDX invalid JSON errors", FormatSPDXJSON, `{bad`, 0, true},
		{"Syft invalid JSON errors", FormatSyftJSON, `{bad`, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			got, err := g.extractPackageCount([]byte(tt.content), tt.format)
			if tt.wantErr {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(err).ToNot(HaveOccurred())
				Expect(got).To(Equal(tt.want))
			}
		})
	}
}

func TestExtractSyftPackageCountInvalidJSON(t *testing.T) {
	RegisterTestingT(t)

	g := NewSyftGenerator()
	_, err := g.extractSyftPackageCount([]byte(`{not valid`))
	Expect(err).To(HaveOccurred())
}

// makeCosignEnvelope builds a cosign verify-attestation style envelope line:
// a JSON object whose base64-encoded "payload" is the in-toto statement.
func makeCosignEnvelope(statement interface{}) []byte {
	stmtBytes, _ := json.Marshal(statement)
	envelope := map[string]string{
		"payload": base64.StdEncoding.EncodeToString(stmtBytes),
	}
	out, _ := json.Marshal(envelope)
	return out
}

func TestParseAttestationOutput(t *testing.T) {
	RegisterTestingT(t)

	a := &Attacher{}

	t.Run("Valid base64 envelope with predicate", func(t *testing.T) {
		RegisterTestingT(t)
		predicate := json.RawMessage(`{"bomFormat":"CycloneDX","components":[]}`)
		statement := map[string]json.RawMessage{"predicate": predicate}
		output := makeCosignEnvelope(statement)

		sbom, err := a.parseAttestationOutput(output, FormatCycloneDXJSON, "registry.io/app@sha256:"+
			"abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890")
		Expect(err).ToNot(HaveOccurred())
		Expect(sbom).ToNot(BeNil())
		Expect(sbom.Format).To(Equal(FormatCycloneDXJSON))
		Expect(string(sbom.Content)).To(Equal(string(predicate)))
		Expect(sbom.ImageDigest).To(Equal("sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"))
		Expect(sbom.Metadata).ToNot(BeNil())
		Expect(sbom.Metadata.ToolName).To(Equal("syft"))
		Expect(sbom.Metadata.ToolVersion).To(Equal("unknown"))
	})

	t.Run("Image without digest falls back to image reference", func(t *testing.T) {
		RegisterTestingT(t)
		statement := map[string]json.RawMessage{"predicate": json.RawMessage(`{"x":1}`)}
		output := makeCosignEnvelope(statement)

		sbom, err := a.parseAttestationOutput(output, FormatSPDXJSON, "app:v1")
		Expect(err).ToNot(HaveOccurred())
		Expect(sbom.ImageDigest).To(Equal("app:v1"))
		Expect(sbom.Format).To(Equal(FormatSPDXJSON))
	})

	t.Run("Empty output errors via DecodeFirstPayload", func(t *testing.T) {
		RegisterTestingT(t)
		_, err := a.parseAttestationOutput([]byte("   "), FormatCycloneDXJSON, "app:v1")
		Expect(err).To(HaveOccurred())
	})

	t.Run("Payload not valid JSON errors on unmarshal", func(t *testing.T) {
		RegisterTestingT(t)
		// Envelope whose decoded payload is not a JSON object.
		envelope := map[string]string{
			"payload": base64.StdEncoding.EncodeToString([]byte("not json at all")),
		}
		out, _ := json.Marshal(envelope)
		_, err := a.parseAttestationOutput(out, FormatCycloneDXJSON, "app:v1")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to parse attestation payload"))
	})

	t.Run("Statement missing predicate errors", func(t *testing.T) {
		RegisterTestingT(t)
		statement := map[string]string{"subject": "something"}
		output := makeCosignEnvelope(statement)
		_, err := a.parseAttestationOutput(output, FormatCycloneDXJSON, "app:v1")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("no predicate"))
	})

	t.Run("Statement with explicit null predicate errors", func(t *testing.T) {
		RegisterTestingT(t)
		// Build a statement whose predicate is literally null.
		stmt := []byte(`{"predicate":null}`)
		envelope := map[string]string{"payload": base64.StdEncoding.EncodeToString(stmt)}
		out, _ := json.Marshal(envelope)
		_, err := a.parseAttestationOutput(out, FormatCycloneDXJSON, "app:v1")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("no predicate"))
	})

	t.Run("Raw JSON payload (non-base64) is accepted", func(t *testing.T) {
		RegisterTestingT(t)
		// Envelope whose "payload" field is a JSON string holding raw JSON rather
		// than base64. The inner string contains braces/quotes which are not valid
		// base64, so DecodeFirstPayload's base64 decode fails and it falls back to
		// treating the payload as raw JSON (json.Valid branch).
		rawStatement := map[string]json.RawMessage{"predicate": json.RawMessage(`{"k":"v"}`)}
		stmtBytes, _ := json.Marshal(rawStatement)
		// Marshal the whole envelope so the inner JSON is correctly escaped.
		envelope := map[string]string{"payload": string(stmtBytes)}
		out, _ := json.Marshal(envelope)

		sbom, err := a.parseAttestationOutput(out, FormatSyftJSON, "app:v1")
		Expect(err).ToNot(HaveOccurred())
		Expect(sbom).ToNot(BeNil())
		Expect(string(sbom.Content)).To(Equal(`{"k":"v"}`))
		Expect(sbom.Format).To(Equal(FormatSyftJSON))
	})
}
