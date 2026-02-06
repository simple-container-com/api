package signing

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewKeyBasedSigner(t *testing.T) {
	tests := []struct {
		name       string
		privateKey string
		password   string
		timeout    time.Duration
	}{
		{
			name:       "with key file",
			privateKey: "/path/to/cosign.key",
			password:   "secret",
			timeout:    5 * time.Minute,
		},
		{
			name:       "with raw key content",
			privateKey: "-----BEGIN PRIVATE KEY-----\nMIIE...",
			password:   "",
			timeout:    5 * time.Minute,
		},
		{
			name:       "default timeout",
			privateKey: "/path/to/cosign.key",
			password:   "",
			timeout:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			signer := NewKeyBasedSigner(tt.privateKey, tt.password, tt.timeout)
			if signer == nil {
				t.Error("NewKeyBasedSigner() returned nil")
			}
			if signer.PrivateKey != tt.privateKey {
				t.Errorf("PrivateKey = %v, want %v", signer.PrivateKey, tt.privateKey)
			}
			if signer.Password != tt.password {
				t.Errorf("Password = %v, want %v", signer.Password, tt.password)
			}
			if tt.timeout == 0 && signer.Timeout != 5*time.Minute {
				t.Errorf("Timeout = %v, want default 5m", signer.Timeout)
			}
		})
	}
}

func TestKeyBasedSigner_Sign_EmptyKey(t *testing.T) {
	signer := NewKeyBasedSigner("", "", 5*time.Minute)
	ctx := context.Background()

	_, err := signer.Sign(ctx, "test-image:latest")
	if err == nil {
		t.Error("Sign() with empty key should return error")
	}
}

func TestKeyBasedSigner_Sign_WithExistingFile(t *testing.T) {
	// Create a temporary key file
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "test.key")
	keyContent := "-----BEGIN PRIVATE KEY-----\ntest-key-content\n-----END PRIVATE KEY-----"

	if err := os.WriteFile(keyPath, []byte(keyContent), 0o600); err != nil {
		t.Fatalf("Failed to create test key file: %v", err)
	}

	signer := NewKeyBasedSigner(keyPath, "", 5*time.Minute)

	// We can't actually sign without cosign installed, but we can verify the signer was created
	if signer.PrivateKey != keyPath {
		t.Errorf("PrivateKey = %v, want %v", signer.PrivateKey, keyPath)
	}
}

func TestKeyBasedSigner_Sign_WithRawKey(t *testing.T) {
	rawKey := "-----BEGIN PRIVATE KEY-----\ntest-key-content\n-----END PRIVATE KEY-----"
	signer := NewKeyBasedSigner(rawKey, "password123", 5*time.Minute)

	if signer.PrivateKey != rawKey {
		t.Errorf("PrivateKey = %v, want %v", signer.PrivateKey, rawKey)
	}
	if signer.Password != "password123" {
		t.Errorf("Password = %v, want 'password123'", signer.Password)
	}
}
