package signing

import (
	"context"
	"testing"
	"time"
)

func TestConfig_Validate(t *testing.T) {
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
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Config.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestConfig_CreateSigner(t *testing.T) {
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
			signer, err := tt.config.CreateSigner(tt.oidcToken)
			if (err != nil) != tt.wantErr {
				t.Errorf("Config.CreateSigner() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && signer == nil {
				t.Error("Config.CreateSigner() returned nil signer without error")
			}
		})
	}
}

func TestConfig_CreateVerifier(t *testing.T) {
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
			verifier, err := tt.config.CreateVerifier()
			if (err != nil) != tt.wantErr {
				t.Errorf("Config.CreateVerifier() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && verifier == nil {
				t.Error("Config.CreateVerifier() returned nil verifier without error")
			}
		})
	}
}

func TestParseDuration(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    time.Duration
		wantErr bool
	}{
		{
			name:    "empty string",
			input:   "",
			want:    5 * time.Minute,
			wantErr: false,
		},
		{
			name:    "valid duration",
			input:   "10m",
			want:    10 * time.Minute,
			wantErr: false,
		},
		{
			name:    "invalid duration",
			input:   "invalid",
			want:    0,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseDuration(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseDuration() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("parseDuration() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSignImage(t *testing.T) {
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
			imageRef:  "test-image:latest",
			oidcToken: "",
			wantErr:   true,
		},
		{
			name: "invalid config",
			config: &Config{
				Enabled: true,
				Keyless: true,
			},
			imageRef:  "test-image:latest",
			oidcToken: "",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := SignImage(ctx, tt.config, tt.imageRef, tt.oidcToken)
			if (err != nil) != tt.wantErr {
				t.Errorf("SignImage() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestVerifyImage(t *testing.T) {
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
			_, err := VerifyImage(ctx, tt.config, tt.imageRef)
			if (err != nil) != tt.wantErr {
				t.Errorf("VerifyImage() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
