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

func TestDecodeFirstPayloadFailsWithoutJSON(t *testing.T) {
	_, err := DecodeFirstPayload([]byte("Verification output without JSON"))
	if err == nil {
		t.Fatal("DecodeFirstPayload() expected error")
	}
}
