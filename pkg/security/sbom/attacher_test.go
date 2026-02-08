package sbom

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/simple-container-com/api/pkg/security/signing"
)

func TestNewAttacher(t *testing.T) {
	config := &signing.Config{
		Enabled: true,
		Keyless: true,
	}

	attacher := NewAttacher(config)
	if attacher == nil {
		t.Fatal("NewAttacher() returned nil")
	}
	if attacher.SigningConfig != config {
		t.Errorf("SigningConfig not set correctly")
	}
	if attacher.Timeout != 2*time.Minute {
		t.Errorf("Expected timeout of 2 minutes, got %v", attacher.Timeout)
	}
}

func TestAttacherBuildSigningArgs(t *testing.T) {
	tests := []struct {
		name   string
		config *signing.Config
		want   []string
	}{
		{
			name:   "Nil config",
			config: nil,
			want:   []string{},
		},
		{
			name: "Keyless signing",
			config: &signing.Config{
				Keyless: true,
			},
			want: []string{"--yes"},
		},
		{
			name: "Key-based signing",
			config: &signing.Config{
				Keyless:    false,
				PrivateKey: "/path/to/key",
			},
			want: []string{"--key", "/path/to/key"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attacher := &Attacher{SigningConfig: tt.config}
			got := attacher.buildSigningArgs()

			if len(got) != len(tt.want) {
				t.Errorf("buildSigningArgs() returned %d args, want %d", len(got), len(tt.want))
				return
			}

			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("buildSigningArgs()[%d] = %v, want %v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestAttacherBuildVerificationArgs(t *testing.T) {
	tests := []struct {
		name   string
		config *signing.Config
		want   []string
	}{
		{
			name:   "Nil config",
			config: nil,
			want:   []string{},
		},
		{
			name: "Keyless verification with certificate",
			config: &signing.Config{
				Keyless:        true,
				IdentityRegexp: "user@example.com",
				OIDCIssuer:     "https://token.actions.githubusercontent.com",
			},
			want: []string{"--certificate-identity-regexp", "user@example.com", "--certificate-oidc-issuer", "https://token.actions.githubusercontent.com"},
		},
		{
			name: "Key-based verification",
			config: &signing.Config{
				Keyless:   false,
				PublicKey: "/path/to/pub.key",
			},
			want: []string{"--key", "/path/to/pub.key"},
		},
		{
			name: "Keyless without certificate",
			config: &signing.Config{
				Keyless: true,
			},
			want: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attacher := &Attacher{SigningConfig: tt.config}
			got := attacher.buildVerificationArgs()

			if len(got) != len(tt.want) {
				t.Errorf("buildVerificationArgs() returned %d args, want %d", len(got), len(tt.want))
				return
			}

			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("buildVerificationArgs()[%d] = %v, want %v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestAttacherBuildSigningEnv(t *testing.T) {
	tests := []struct {
		name   string
		config *signing.Config
		want   []string
	}{
		{
			name:   "Nil config",
			config: nil,
			want:   []string{},
		},
		{
			name: "With password",
			config: &signing.Config{
				Password: "secret123",
			},
			want: []string{"COSIGN_PASSWORD=secret123"},
		},
		{
			name: "Without password",
			config: &signing.Config{
				Keyless: true,
			},
			want: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attacher := &Attacher{SigningConfig: tt.config}
			got := attacher.buildSigningEnv()

			if len(got) != len(tt.want) {
				t.Errorf("buildSigningEnv() returned %d env vars, want %d", len(got), len(tt.want))
				return
			}

			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("buildSigningEnv()[%d] = %v, want %v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestAttacherCreateTempSBOMFile(t *testing.T) {
	content := []byte(`{"bomFormat": "CycloneDX"}`)
	sbomObj := NewSBOM(FormatCycloneDXJSON, content, "test-image", &Metadata{
		ToolName:    "syft",
		ToolVersion: "1.0.0",
	})

	attacher := NewAttacher(nil)
	tmpFile, err := attacher.createTempSBOMFile(sbomObj)
	if err != nil {
		t.Fatalf("createTempSBOMFile() error = %v", err)
	}
	defer os.Remove(tmpFile)

	// Verify file exists
	if _, err := os.Stat(tmpFile); os.IsNotExist(err) {
		t.Errorf("Temp file was not created")
	}

	// Verify content
	readContent, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatalf("Failed to read temp file: %v", err)
	}
	if string(readContent) != string(content) {
		t.Errorf("Temp file content = %v, want %v", string(readContent), string(content))
	}
}

func TestAttacherExtractImageDigest(t *testing.T) {
	attacher := &Attacher{}

	tests := []struct {
		name  string
		image string
		want  string
	}{
		{
			name:  "Image with digest",
			image: "myapp@sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
			want:  "sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
		},
		{
			name:  "Image without digest",
			image: "myapp:v1.0",
			want:  "myapp:v1.0",
		},
		{
			name:  "Image with partial digest",
			image: "registry.io/myapp@sha256:1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
			want:  "sha256:1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := attacher.extractImageDigest(tt.image)
			if got != tt.want {
				t.Errorf("extractImageDigest() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAttachAndVerify_NotInstalled(t *testing.T) {
	// These tests will skip or fail if cosign is not installed
	ctx := context.Background()

	sbomContent := []byte(`{"bomFormat": "CycloneDX"}`)
	sbomObj := NewSBOM(FormatCycloneDXJSON, sbomContent, "test:v1", &Metadata{
		ToolName:    "syft",
		ToolVersion: "1.0.0",
	})

	config := &signing.Config{
		Enabled: true,
		Keyless: true,
	}

	attacher := NewAttacher(config)

	// Test Attach - will fail if cosign not installed
	err := attacher.Attach(ctx, sbomObj, "test:v1")
	if err != nil {
		t.Logf("Attach failed (expected if cosign not installed): %v", err)
	}

	// Test Verify - will fail if cosign not installed or no attestation
	_, err = attacher.Verify(ctx, "test:v1", FormatCycloneDXJSON)
	if err != nil {
		t.Logf("Verify failed (expected if cosign not installed or no attestation): %v", err)
	}
}
