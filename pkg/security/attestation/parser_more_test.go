package attestation

import (
	"encoding/base64"
	"testing"

	. "github.com/onsi/gomega"
)

func b64(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

func TestDecodeFirstPayload(t *testing.T) {
	statement := `{"_type":"https://in-toto.io/Statement/v1","subject":[]}`

	cases := []struct {
		name      string
		output    string
		wantBytes string
		wantErr   string
	}{
		{
			name:      "single base64 envelope",
			output:    `{"payload":"` + b64(statement) + `"}`,
			wantBytes: statement,
		},
		{
			name:      "array of envelopes takes first",
			output:    `[{"payload":"` + b64("first") + `"},{"payload":"` + b64("second") + `"}]`,
			wantBytes: "first",
		},
		{
			name:      "raw-json payload (not base64)",
			output:    `{"payload":` + `"{\"k\":1}"` + `}`,
			wantBytes: `{"k":1}`,
		},
		{
			name:    "empty output",
			output:  "   ",
			wantErr: "empty attestation output",
		},
		{
			name:    "no json at all",
			output:  "Verification succeeded but no payloads here",
			wantErr: "failed to locate JSON attestation payload",
		},
		{
			name:    "payload neither base64 nor json",
			output:  `{"payload":"!!!not-base64!!!"}`,
			wantErr: "neither valid base64 nor valid JSON",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			got, err := DecodeFirstPayload([]byte(tc.output))
			if tc.wantErr != "" {
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(tc.wantErr))
				return
			}
			Expect(err).ToNot(HaveOccurred())
			Expect(string(got)).To(Equal(tc.wantBytes))
		})
	}
}

func TestDecodeFirstPayload_PreambleThenJSON(t *testing.T) {
	RegisterTestingT(t)

	// cosign emits a human-readable preamble before the JSON envelope.
	output := "Verification for example.com/img...\n" +
		"The following checks were performed on each of these signatures:\n" +
		`{"payload":"` + b64("payload-after-preamble") + `"}`

	got, err := DecodeFirstPayload([]byte(output))
	Expect(err).ToNot(HaveOccurred())
	Expect(string(got)).To(Equal("payload-after-preamble"))
}

func TestParseVerifyOutput_NDJSONLines(t *testing.T) {
	RegisterTestingT(t)

	// Multiple JSON objects on separate lines mixed with noise lines.
	output := "noise line\n" +
		`{"payload":"` + b64("a") + `"}` + "\n" +
		"another noise line\n" +
		`{"payload":"` + b64("b") + `"}` + "\n"

	got, err := DecodeFirstPayload([]byte(output))
	Expect(err).ToNot(HaveOccurred())
	Expect(string(got)).To(Equal("a"))
}

func TestParseVerifyJSON_MultipleConcatenatedValues(t *testing.T) {
	RegisterTestingT(t)

	// Concatenated JSON values (decoder stream) — both should be collected.
	raw := []byte(`{"payload":"` + b64("x") + `"}{"payload":"` + b64("y") + `"}`)
	envs, ok, err := parseVerifyJSON(raw)
	Expect(ok).To(BeTrue())
	Expect(err).ToNot(HaveOccurred())
	Expect(envs).To(HaveLen(2))
}

func TestParseVerifyJSON_NonJSONPrefix(t *testing.T) {
	RegisterTestingT(t)

	envs, ok, err := parseVerifyJSON([]byte("not json"))
	Expect(ok).To(BeFalse())
	Expect(err).ToNot(HaveOccurred())
	Expect(envs).To(BeNil())

	envs, ok, _ = parseVerifyJSON(nil)
	Expect(ok).To(BeFalse())
	Expect(envs).To(BeNil())
}

func TestParseVerifyJSON_InvalidJSON(t *testing.T) {
	RegisterTestingT(t)

	_, ok, err := parseVerifyJSON([]byte(`{"payload": `))
	Expect(ok).To(BeTrue())
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("failed to parse attestation JSON"))
}

func TestParseVerifyJSON_EmptyArray(t *testing.T) {
	RegisterTestingT(t)

	// A valid-but-empty JSON array yields no payloads.
	_, ok, err := parseVerifyJSON([]byte(`[]`))
	Expect(ok).To(BeTrue())
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("no attestation payloads found"))
}
