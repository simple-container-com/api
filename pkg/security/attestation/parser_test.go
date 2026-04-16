package attestation

import "testing"

func TestDecodeFirstPayloadFromJSONArray(t *testing.T) {
	output := []byte(`[{"payload":"eyJmb28iOiJiYXIifQ=="}]`)

	payload, err := DecodeFirstPayload(output)
	if err != nil {
		t.Fatalf("DecodeFirstPayload() error = %v", err)
	}
	if string(payload) != `{"foo":"bar"}` {
		t.Fatalf("DecodeFirstPayload() = %s", string(payload))
	}
}

func TestDecodeFirstPayloadFromJSONLineWithPreamble(t *testing.T) {
	output := []byte("Verification for image --\nchecks\n" +
		`{"payload":"eyJmb28iOiJiYXIifQ==","payloadType":"application/vnd.in-toto+json"}` + "\n")

	payload, err := DecodeFirstPayload(output)
	if err != nil {
		t.Fatalf("DecodeFirstPayload() error = %v", err)
	}
	if string(payload) != `{"foo":"bar"}` {
		t.Fatalf("DecodeFirstPayload() = %s", string(payload))
	}
}

func TestDecodeFirstPayloadRawJSON(t *testing.T) {
	// Some cosign versions emit payload as raw JSON, not base64.
	output := []byte(`[{"payload":"{\"predicate\":{\"key\":\"value\"}}"}]`)
	payload, err := DecodeFirstPayload(output)
	if err != nil {
		t.Fatalf("DecodeFirstPayload() error = %v", err)
	}
	if string(payload) != `{"predicate":{"key":"value"}}` {
		t.Fatalf("DecodeFirstPayload() = %s", string(payload))
	}
}

func TestDecodeFirstPayloadRejectsGarbage(t *testing.T) {
	// Payload that's neither valid base64 nor valid JSON should error.
	output := []byte(`[{"payload":"not-base64-and-not-json!!!"}]`)
	_, err := DecodeFirstPayload(output)
	if err == nil {
		t.Fatal("DecodeFirstPayload() expected error for garbage payload")
	}
}

func TestDecodeFirstPayloadFailsWithoutJSON(t *testing.T) {
	_, err := DecodeFirstPayload([]byte("Verification output without JSON"))
	if err == nil {
		t.Fatal("DecodeFirstPayload() expected error")
	}
}
