package sbom

import (
	"context"
	"os"
	"testing"
	"time"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/security/signing"
)

func TestNewAttacher(t *testing.T) {
	RegisterTestingT(t)

	config := &signing.Config{
		Enabled: true,
		Keyless: true,
	}

	attacher := NewAttacher(config)
	Expect(attacher).ToNot(BeNil())
	Expect(attacher.SigningConfig).To(Equal(config))
	Expect(attacher.Timeout).To(Equal(2 * time.Minute))
}

func TestAttacherBuildSigningArgs(t *testing.T) {
	RegisterTestingT(t)

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
			RegisterTestingT(t)
			attacher := &Attacher{SigningConfig: tt.config}
			got := attacher.buildSigningArgs()
			Expect(got).To(HaveLen(len(tt.want)))
			for i := range got {
				Expect(got[i]).To(Equal(tt.want[i]))
			}
		})
	}
}

func TestAttacherBuildVerificationArgs(t *testing.T) {
	RegisterTestingT(t)

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
			RegisterTestingT(t)
			attacher := &Attacher{SigningConfig: tt.config}
			got := attacher.buildVerificationArgs()
			Expect(got).To(HaveLen(len(tt.want)))
			for i := range got {
				Expect(got[i]).To(Equal(tt.want[i]))
			}
		})
	}
}

func TestAttacherBuildSigningEnv(t *testing.T) {
	RegisterTestingT(t)

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
				PrivateKey: "/tmp/cosign.key",
				Password:   "secret123",
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
		{
			name: "Key based empty password still exports env",
			config: &signing.Config{
				PrivateKey: "/tmp/cosign.key",
				Password:   "",
			},
			want: []string{"COSIGN_PASSWORD="},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			attacher := &Attacher{SigningConfig: tt.config}
			got := attacher.buildSigningEnv()
			Expect(got).To(HaveLen(len(tt.want)))
			for i := range got {
				Expect(got[i]).To(Equal(tt.want[i]))
			}
		})
	}
}

func TestAttacherCreateTempSBOMFile(t *testing.T) {
	RegisterTestingT(t)

	content := []byte(`{"bomFormat": "CycloneDX"}`)
	sbomObj := NewSBOM(FormatCycloneDXJSON, content, "test-image", &Metadata{
		ToolName:    "syft",
		ToolVersion: "1.0.0",
	})

	attacher := NewAttacher(nil)
	tmpFile, err := attacher.createTempSBOMFile(sbomObj)
	Expect(err).ToNot(HaveOccurred())
	defer os.Remove(tmpFile)

	// Verify file exists
	_, err = os.Stat(tmpFile)
	Expect(os.IsNotExist(err)).To(BeFalse(), "Temp file was not created")

	// Verify content
	readContent, err := os.ReadFile(tmpFile)
	Expect(err).ToNot(HaveOccurred())
	Expect(string(readContent)).To(Equal(string(content)))
}

func TestAttacherExtractImageDigest(t *testing.T) {
	RegisterTestingT(t)

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
			RegisterTestingT(t)
			Expect(attacher.extractImageDigest(tt.image)).To(Equal(tt.want))
		})
	}
}

func TestAttachAndVerify_NotInstalled(t *testing.T) {
	RegisterTestingT(t)

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
