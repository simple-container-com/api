package api

import (
	"encoding/json"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestSigningDescriptor_PasswordNotSerializedToJSON verifies that the cosign
// private key passphrase is never written out when a SigningDescriptor is
// marshaled to JSON (e.g., debug logging, config dumps, cache key hashing).
func TestSigningDescriptor_PasswordNotSerializedToJSON(t *testing.T) {
	cfg := &SigningDescriptor{
		Enabled:    true,
		PrivateKey: ".keys/cosign.key",
		Password:   "super-secret-passphrase",
	}

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var roundtrip map[string]interface{}
	if err := json.Unmarshal(data, &roundtrip); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if _, ok := roundtrip["password"]; ok {
		t.Error("SigningDescriptor.Password must not appear in JSON output (json:\"-\" tag missing or wrong)")
	}

	// Confirm other fields are still present.
	if _, ok := roundtrip["privateKey"]; !ok {
		t.Error("SigningDescriptor.PrivateKey should be present in JSON")
	}
}

// TestDefectDojoDescriptor_Sanitize verifies that the API key is replaced with
// a placeholder in the returned copy, while the original remains unchanged.
func TestDefectDojoDescriptor_Sanitize(t *testing.T) {
	t.Run("redacts non-empty api key", func(t *testing.T) {
		d := &DefectDojoDescriptor{
			Enabled:      true,
			URL:          "https://dojo.example.com",
			APIKey:       "very-secret-key",
			EngagementID: 42,
		}
		sanitized := d.Sanitize()
		if sanitized.APIKey != "[REDACTED]" {
			t.Errorf("Sanitize() APIKey = %q, want [REDACTED]", sanitized.APIKey)
		}
		if d.APIKey != "very-secret-key" {
			t.Errorf("Sanitize() mutated original APIKey to %q", d.APIKey)
		}
		if sanitized.EngagementID != d.EngagementID {
			t.Errorf("Sanitize() changed EngagementID: got %d", sanitized.EngagementID)
		}
	})

	t.Run("empty api key stays empty", func(t *testing.T) {
		d := &DefectDojoDescriptor{}
		sanitized := d.Sanitize()
		if sanitized.APIKey != "" {
			t.Errorf("Sanitize() empty APIKey = %q, want empty string", sanitized.APIKey)
		}
	})
}

// TestSigningDescriptor_PasswordLoadableFromYAML verifies that the Password field
// can still be loaded from YAML stack configs (yaml tag is preserved, unlike json).
func TestSigningDescriptor_PasswordLoadableFromYAML(t *testing.T) {
	raw := []byte("enabled: true\nprivateKey: .keys/cosign.key\npassword: my-passphrase\n")

	var cfg SigningDescriptor
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}

	if cfg.Password != "my-passphrase" {
		t.Errorf("SigningDescriptor.Password from YAML = %q, want %q", cfg.Password, "my-passphrase")
	}
}

// TestSigningDescriptor_PasswordNotRestoredFromJSON verifies that unmarshaling
// JSON into a SigningDescriptor never populates the Password field, even when
// an attacker-controlled or legacy config file includes the key.
func TestSigningDescriptor_PasswordNotRestoredFromJSON(t *testing.T) {
	raw := `{"enabled":true,"privateKey":".keys/cosign.key","password":"injected-secret"}`

	var cfg SigningDescriptor
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if cfg.Password != "" {
		t.Errorf("SigningDescriptor.Password populated from JSON = %q, want empty string", cfg.Password)
	}
}
