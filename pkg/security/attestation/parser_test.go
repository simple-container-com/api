package attestation

import (
	"testing"

	. "github.com/onsi/gomega"
)

func TestDecodeFirstPayloadFromJSONArray(t *testing.T) {
	RegisterTestingT(t)

	output := []byte(`[{"payload":"eyJmb28iOiJiYXIifQ=="}]`)

	payload, err := DecodeFirstPayload(output)
	Expect(err).ToNot(HaveOccurred())
	Expect(string(payload)).To(Equal(`{"foo":"bar"}`))
}

func TestDecodeFirstPayloadFromJSONLineWithPreamble(t *testing.T) {
	RegisterTestingT(t)

	output := []byte("Verification for image --\nchecks\n" +
		`{"payload":"eyJmb28iOiJiYXIifQ==","payloadType":"application/vnd.in-toto+json"}` + "\n")

	payload, err := DecodeFirstPayload(output)
	Expect(err).ToNot(HaveOccurred())
	Expect(string(payload)).To(Equal(`{"foo":"bar"}`))
}

func TestDecodeFirstPayloadRawJSON(t *testing.T) {
	RegisterTestingT(t)

	// Some cosign versions emit payload as raw JSON, not base64.
	output := []byte(`[{"payload":"{\"predicate\":{\"key\":\"value\"}}"}]`)
	payload, err := DecodeFirstPayload(output)
	Expect(err).ToNot(HaveOccurred())
	Expect(string(payload)).To(Equal(`{"predicate":{"key":"value"}}`))
}

func TestDecodeFirstPayloadRejectsGarbage(t *testing.T) {
	RegisterTestingT(t)

	// Payload that's neither valid base64 nor valid JSON should error.
	output := []byte(`[{"payload":"not-base64-and-not-json!!!"}]`)
	_, err := DecodeFirstPayload(output)
	Expect(err).To(HaveOccurred())
}

func TestDecodeFirstPayloadFailsWithoutJSON(t *testing.T) {
	RegisterTestingT(t)

	_, err := DecodeFirstPayload([]byte("Verification output without JSON"))
	Expect(err).To(HaveOccurred())
}
