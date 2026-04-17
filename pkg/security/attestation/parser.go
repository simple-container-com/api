package attestation

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

type verifyEnvelope struct {
	Payload string `json:"payload"`
}

// DecodeFirstPayload extracts the first attestation payload from cosign
// verify-attestation output. Recent cosign versions emit a human-readable
// preamble followed by JSON lines, while older versions emitted JSON directly.
func DecodeFirstPayload(output []byte) ([]byte, error) {
	envelopes, err := parseVerifyOutput(output)
	if err != nil {
		return nil, err
	}
	if len(envelopes) == 0 {
		return nil, fmt.Errorf("no attestations found")
	}

	payload, err := base64.StdEncoding.DecodeString(envelopes[0].Payload)
	if err == nil {
		return payload, nil
	}

	// Some cosign versions emit the payload as raw JSON instead of base64.
	// Accept it only if it looks like valid JSON.
	raw := []byte(envelopes[0].Payload)
	if json.Valid(raw) {
		return raw, nil
	}

	return nil, fmt.Errorf("attestation payload is neither valid base64 nor valid JSON: %w", err)
}

func parseVerifyOutput(output []byte) ([]verifyEnvelope, error) {
	trimmed := bytes.TrimSpace(output)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("empty attestation output")
	}

	if envelopes, ok, err := parseVerifyJSON(trimmed); ok {
		if err != nil {
			return nil, err
		}
		return envelopes, nil
	}

	var lastErr error
	if start := bytes.IndexAny(trimmed, "{["); start >= 0 {
		if envelopes, ok, err := parseVerifyJSON(trimmed[start:]); ok {
			if err != nil {
				lastErr = err
			} else {
				return envelopes, nil
			}
		}
	}

	var envelopes []verifyEnvelope
	for _, line := range strings.Split(string(trimmed), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "{") && !strings.HasPrefix(line, "[") {
			continue
		}

		parsed, ok, err := parseVerifyJSON([]byte(line))
		if !ok {
			continue
		}
		if err != nil {
			lastErr = err
			continue
		}
		envelopes = append(envelopes, parsed...)
	}

	if len(envelopes) == 0 {
		if lastErr != nil {
			return nil, lastErr
		}
		return nil, fmt.Errorf("failed to locate JSON attestation payload in cosign output")
	}

	return envelopes, nil
}

func parseVerifyJSON(raw []byte) ([]verifyEnvelope, bool, error) {
	if len(raw) == 0 {
		return nil, false, nil
	}

	if raw[0] != '{' && raw[0] != '[' {
		return nil, false, nil
	}

	dec := json.NewDecoder(bytes.NewReader(raw))
	var envelopes []verifyEnvelope

	for {
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			if err == io.EOF {
				break
			}
			return nil, true, fmt.Errorf("failed to parse attestation JSON: %w", err)
		}

		var batch []verifyEnvelope
		if err := json.Unmarshal(value, &batch); err == nil {
			envelopes = append(envelopes, batch...)
			continue
		}

		var envelope verifyEnvelope
		if err := json.Unmarshal(value, &envelope); err == nil {
			envelopes = append(envelopes, envelope)
			continue
		}

		return nil, true, fmt.Errorf("failed to parse attestation JSON value")
	}

	if len(envelopes) == 0 {
		return nil, true, fmt.Errorf("no attestation payloads found in JSON output")
	}

	return envelopes, true, nil
}
