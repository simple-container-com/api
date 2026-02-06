package signing

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/simple-container-com/api/pkg/security/tools"
)

// KeyBasedSigner implements key-based signing using private keys
type KeyBasedSigner struct {
	PrivateKey string // Path to private key file or key content
	Password   string // Optional password for encrypted keys
	Timeout    time.Duration
}

// NewKeyBasedSigner creates a new key-based signer
func NewKeyBasedSigner(privateKey, password string, timeout time.Duration) *KeyBasedSigner {
	if timeout == 0 {
		timeout = 5 * time.Minute
	}
	return &KeyBasedSigner{
		PrivateKey: privateKey,
		Password:   password,
		Timeout:    timeout,
	}
}

// Sign signs a container image using a private key
func (s *KeyBasedSigner) Sign(ctx context.Context, imageRef string) (*SignResult, error) {
	if s.PrivateKey == "" {
		return nil, fmt.Errorf("private key is required for key-based signing")
	}

	// Check if PrivateKey is a file path or raw key content
	var keyPath string
	var tempFile bool

	if _, err := os.Stat(s.PrivateKey); err == nil {
		// It's an existing file path
		keyPath = s.PrivateKey
	} else {
		// It's raw key content - write to secure temp file
		tmpDir := os.TempDir()
		tmpFile, err := os.CreateTemp(tmpDir, "cosign-key-*.key")
		if err != nil {
			return nil, fmt.Errorf("creating temp key file: %w", err)
		}
		keyPath = tmpFile.Name()
		tempFile = true

		// Write key content and set secure permissions
		if err := os.WriteFile(keyPath, []byte(s.PrivateKey), 0o600); err != nil {
			os.Remove(keyPath)
			return nil, fmt.Errorf("writing temp key file: %w", err)
		}

		// Ensure cleanup
		defer func() {
			os.Remove(keyPath)
		}()
	}

	// Prepare environment variables
	env := []string{}
	if s.Password != "" {
		env = append(env, "COSIGN_PASSWORD="+s.Password)
	}

	// Execute cosign sign command
	args := []string{"sign", "--key", keyPath, imageRef}
	stdout, stderr, err := tools.ExecCommand(ctx, "cosign", args, env, s.Timeout)

	// Clean up temp file immediately after execution
	if tempFile {
		os.Remove(keyPath)
	}

	if err != nil {
		return nil, fmt.Errorf("cosign sign failed: %w\nStderr: %s\nStdout: %s", err, stderr, stdout)
	}

	result := &SignResult{
		SignedAt: time.Now().UTC().Format(time.RFC3339),
	}

	return result, nil
}

// GenerateKeyPair generates a new cosign key pair
func GenerateKeyPair(ctx context.Context, outputDir string, password string) (privateKeyPath, publicKeyPath string, err error) {
	if outputDir == "" {
		outputDir = "."
	}

	// Ensure output directory exists
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return "", "", fmt.Errorf("creating output directory: %w", err)
	}

	privateKeyPath = filepath.Join(outputDir, "cosign.key")
	publicKeyPath = filepath.Join(outputDir, "cosign.pub")

	// Prepare environment
	env := []string{}
	if password != "" {
		env = append(env, "COSIGN_PASSWORD="+password)
	}

	// Execute cosign generate-key-pair
	args := []string{"generate-key-pair"}
	_, stderr, err := tools.ExecCommand(ctx, "cosign", args, env, 30*time.Second)
	if err != nil {
		return "", "", fmt.Errorf("cosign generate-key-pair failed: %w\nStderr: %s", err, stderr)
	}

	// Set secure permissions on private key
	if err := os.Chmod(privateKeyPath, 0o600); err != nil {
		return "", "", fmt.Errorf("setting private key permissions: %w", err)
	}

	return privateKeyPath, publicKeyPath, nil
}
