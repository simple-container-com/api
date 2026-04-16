package signing

import (
	"context"
	"testing"
	"time"

	. "github.com/onsi/gomega"
)

func TestConfig_Validate(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name: "disabled config",
			config: &Config{
				Enabled: false,
			},
			wantErr: false,
		},
		{
			name: "valid keyless config",
			config: &Config{
				Enabled:        true,
				Keyless:        true,
				OIDCIssuer:     "https://token.actions.githubusercontent.com",
				IdentityRegexp: "^https://github.com/org/.*$",
			},
			wantErr: false,
		},
		{
			name: "valid key-based config",
			config: &Config{
				Enabled:    true,
				Keyless:    false,
				PrivateKey: "/path/to/cosign.key",
			},
			wantErr: false,
		},
		{
			name: "keyless missing issuer",
			config: &Config{
				Enabled:        true,
				Keyless:        true,
				IdentityRegexp: "^https://github.com/org/.*$",
			},
			wantErr: true,
		},
		{
			name: "keyless missing identity",
			config: &Config{
				Enabled:    true,
				Keyless:    true,
				OIDCIssuer: "https://token.actions.githubusercontent.com",
			},
			wantErr: true,
		},
		{
			name: "key-based missing private key",
			config: &Config{
				Enabled: true,
				Keyless: false,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			err := tt.config.Validate()
			if tt.wantErr {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(err).ToNot(HaveOccurred())
			}
		})
	}
}

func TestConfig_CreateSigner(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name      string
		config    *Config
		oidcToken string
		wantErr   bool
	}{
		{
			name: "keyless with token",
			config: &Config{
				Enabled:        true,
				Keyless:        true,
				OIDCIssuer:     "https://token.actions.githubusercontent.com",
				IdentityRegexp: "^https://github.com/org/.*$",
				Timeout:        "5m",
			},
			oidcToken: "valid.oidc.token",
			wantErr:   false,
		},
		{
			name: "keyless without token",
			config: &Config{
				Enabled: true,
				Keyless: true,
			},
			oidcToken: "",
			wantErr:   true,
		},
		{
			name: "key-based with key",
			config: &Config{
				Enabled:    true,
				Keyless:    false,
				PrivateKey: "/path/to/cosign.key",
				Password:   "secret",
			},
			oidcToken: "",
			wantErr:   false,
		},
		{
			name: "key-based without key",
			config: &Config{
				Enabled: true,
				Keyless: false,
			},
			oidcToken: "",
			wantErr:   true,
		},
		{
			name: "invalid timeout",
			config: &Config{
				Enabled: true,
				Keyless: true,
				Timeout: "invalid",
			},
			oidcToken: "valid.oidc.token",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			signer, err := tt.config.CreateSigner(tt.oidcToken)
			if tt.wantErr {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(err).ToNot(HaveOccurred())
				Expect(signer).ToNot(BeNil())
			}
		})
	}
}

func TestConfig_CreateVerifier(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name: "keyless verifier",
			config: &Config{
				Keyless:        true,
				OIDCIssuer:     "https://token.actions.githubusercontent.com",
				IdentityRegexp: "^https://github.com/org/.*$",
			},
			wantErr: false,
		},
		{
			name: "key-based verifier",
			config: &Config{
				Keyless:   false,
				PublicKey: "/path/to/cosign.pub",
			},
			wantErr: false,
		},
		{
			name: "keyless missing issuer",
			config: &Config{
				Keyless:        true,
				IdentityRegexp: "^https://github.com/org/.*$",
			},
			wantErr: true,
		},
		{
			name: "key-based missing public key",
			config: &Config{
				Keyless: false,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			verifier, err := tt.config.CreateVerifier()
			if tt.wantErr {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(err).ToNot(HaveOccurred())
				Expect(verifier).ToNot(BeNil())
			}
		})
	}
}

func TestParseDuration(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name    string
		input   string
		want    time.Duration
		wantErr bool
	}{
		{
			name:  "empty string",
			input: "",
			want:  5 * time.Minute,
		},
		{
			name:  "valid duration",
			input: "10m",
			want:  10 * time.Minute,
		},
		{
			name:    "invalid duration",
			input:   "invalid",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			got, err := parseDuration(tt.input)
			if tt.wantErr {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(err).ToNot(HaveOccurred())
				Expect(got).To(Equal(tt.want))
			}
		})
	}
}

func TestSignImage(t *testing.T) {
	RegisterTestingT(t)
	ctx := context.Background()

	tests := []struct {
		name      string
		config    *Config
		imageRef  string
		oidcToken string
		wantErr   bool
	}{
		{
			name: "signing disabled",
			config: &Config{
				Enabled: false,
			},
			imageRef: "test-image:latest",
			wantErr:  true,
		},
		{
			name: "invalid config",
			config: &Config{
				Enabled: true,
				Keyless: true,
			},
			imageRef: "test-image:latest",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			_, err := SignImage(ctx, tt.config, tt.imageRef, tt.oidcToken)
			if tt.wantErr {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(err).ToNot(HaveOccurred())
			}
		})
	}
}

func TestVerifyImage(t *testing.T) {
	RegisterTestingT(t)
	ctx := context.Background()

	tests := []struct {
		name     string
		config   *Config
		imageRef string
		wantErr  bool
	}{
		{
			name: "invalid config",
			config: &Config{
				Keyless: true,
			},
			imageRef: "test-image:latest",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			_, err := VerifyImage(ctx, tt.config, tt.imageRef)
			if tt.wantErr {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(err).ToNot(HaveOccurred())
			}
		})
	}
}

func TestCreateSigner_OIDCTokenFallback(t *testing.T) {
	RegisterTestingT(t)

	cfg := &Config{
		Enabled:   true,
		Keyless:   true,
		OIDCToken: "eyJ0eXAiOiJKV1QiLCJhbGciOiJSUzI1NiJ9.eyJpc3MiOiJ0ZXN0In0.dGVzdA",
	}
	// Empty oidcToken param should fall back to cfg.OIDCToken
	signer, err := cfg.CreateSigner("")
	Expect(err).ToNot(HaveOccurred())
	Expect(signer).ToNot(BeNil())
}

func TestCreateSigner_ParamTakesPrecedence(t *testing.T) {
	RegisterTestingT(t)

	cfg := &Config{
		Enabled:   true,
		Keyless:   true,
		OIDCToken: "eyJvbGQiOiJ0b2tlbiJ9.eyJpc3MiOiJ0ZXN0In0.dGVzdA",
	}
	paramToken := "eyJuZXciOiJ0b2tlbiJ9.eyJpc3MiOiJ0ZXN0In0.dGVzdA"
	signer, err := cfg.CreateSigner(paramToken)
	Expect(err).ToNot(HaveOccurred())
	// Verify the param token was used (it's a KeylessSigner)
	ks, ok := signer.(*KeylessSigner)
	Expect(ok).To(BeTrue(), "expected KeylessSigner")
	Expect(ks.OIDCToken).To(Equal(paramToken))
}

func TestCreateSigner_KeylessNoToken(t *testing.T) {
	RegisterTestingT(t)

	cfg := &Config{
		Enabled: true,
		Keyless: true,
	}
	_, err := cfg.CreateSigner("")
	Expect(err).To(HaveOccurred())
}
