package signing

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/gomega"
)

func TestNewKeyBasedSigner(t *testing.T) {
	RegisterTestingT(t)

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
			RegisterTestingT(t)
			signer := NewKeyBasedSigner(tt.privateKey, tt.password, tt.timeout)
			Expect(signer).ToNot(BeNil())
			Expect(signer.PrivateKey).To(Equal(tt.privateKey))
			Expect(signer.Password).To(Equal(tt.password))
			if tt.timeout == 0 {
				Expect(signer.Timeout).To(Equal(5 * time.Minute))
			}
		})
	}
}

func TestKeyBasedSigner_Sign_EmptyKey(t *testing.T) {
	RegisterTestingT(t)

	signer := NewKeyBasedSigner("", "", 5*time.Minute)
	ctx := context.Background()

	_, err := signer.Sign(ctx, "test-image:latest")
	Expect(err).To(HaveOccurred())
}

func TestKeyBasedSigner_Sign_WithExistingFile(t *testing.T) {
	RegisterTestingT(t)

	// Create a temporary key file
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "test.key")
	keyContent := "-----BEGIN PRIVATE KEY-----\ntest-key-content\n-----END PRIVATE KEY-----"

	err := os.WriteFile(keyPath, []byte(keyContent), 0o600)
	Expect(err).ToNot(HaveOccurred())

	signer := NewKeyBasedSigner(keyPath, "", 5*time.Minute)

	// We can't actually sign without cosign installed, but we can verify the signer was created
	Expect(signer.PrivateKey).To(Equal(keyPath))
}

func TestKeyBasedSigner_Sign_WithRawKey(t *testing.T) {
	RegisterTestingT(t)

	rawKey := "-----BEGIN PRIVATE KEY-----\ntest-key-content\n-----END PRIVATE KEY-----"
	signer := NewKeyBasedSigner(rawKey, "password123", 5*time.Minute)

	Expect(signer.PrivateKey).To(Equal(rawKey))
	Expect(signer.Password).To(Equal("password123"))
}

func TestKeyBasedSignerPasswordHandling(t *testing.T) {
	RegisterTestingT(t)

	withoutPassword := NewKeyBasedSigner("/tmp/cosign.key", "", 5*time.Minute)
	Expect(withoutPassword.Password).To(BeEmpty())

	withPassword := NewKeyBasedSigner("/tmp/cosign.key", "secret123", 5*time.Minute)
	Expect(strings.Contains("COSIGN_PASSWORD="+withPassword.Password, "COSIGN_PASSWORD=secret123")).To(BeTrue())
}

func TestKeyBasedSigner_Sign_GivesUpOnPersistentRekorConflict(t *testing.T) {
	RegisterTestingT(t)

	// A deterministic key (e.g. ed25519) reproduces the same signature on
	// retry, so a persistent conflict must exhaust the bounded loop and
	// surface the error — never be treated as success: a tlog entry existing
	// does not prove the signature was attached to the registry.
	calls := 0
	signer := NewKeyBasedSigner("test-key-content", "", time.Second)
	signer.exec = func(ctx context.Context, name string, args []string, env []string, timeout time.Duration) (string, string, error) {
		calls++
		return "", "[POST /api/v1/log/entries][409] createLogEntryConflict", fmt.Errorf("exit status 1")
	}

	_, err := signer.Sign(context.Background(), "registry.example.com/app:1.0.0")

	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("createLogEntryConflict"))
	Expect(calls).To(Equal(maxSignAttempts))
}

func TestKeyBasedSigner_Sign_RetriesOnceOnTransientConflict(t *testing.T) {
	RegisterTestingT(t)

	calls := 0
	signer := NewKeyBasedSigner("test-key-content", "", time.Second)
	signer.exec = func(ctx context.Context, name string, args []string, env []string, timeout time.Duration) (string, string, error) {
		calls++
		if calls == 1 {
			return "", "createLogEntryConflict", fmt.Errorf("exit status 1")
		}
		return "", "", nil
	}

	result, err := signer.Sign(context.Background(), "registry.example.com/app:1.0.0")

	Expect(err).ToNot(HaveOccurred())
	Expect(result).ToNot(BeNil())
	Expect(calls).To(Equal(2))
}
