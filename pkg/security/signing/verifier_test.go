package signing

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestNewKeylessVerifier(t *testing.T) {
	tests := []struct {
		name           string
		oidcIssuer     string
		identityRegexp string
		timeout        time.Duration
	}{
		{
			name:           "valid keyless verifier",
			oidcIssuer:     "https://token.actions.githubusercontent.com",
			identityRegexp: "^https://github.com/org/.*$",
			timeout:        2 * time.Minute,
		},
		{
			name:           "default timeout",
			oidcIssuer:     "https://token.actions.githubusercontent.com",
			identityRegexp: "^https://github.com/org/.*$",
			timeout:        0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			verifier := NewKeylessVerifier(tt.oidcIssuer, tt.identityRegexp, tt.timeout)
			if verifier == nil {
				t.Fatal("NewKeylessVerifier() returned nil")
			}
			if verifier.OIDCIssuer != tt.oidcIssuer {
				t.Errorf("OIDCIssuer = %v, want %v", verifier.OIDCIssuer, tt.oidcIssuer)
			}
			if verifier.IdentityRegexp != tt.identityRegexp {
				t.Errorf("IdentityRegexp = %v, want %v", verifier.IdentityRegexp, tt.identityRegexp)
			}
			if tt.timeout == 0 && verifier.Timeout != 2*time.Minute {
				t.Errorf("Timeout = %v, want default 2m", verifier.Timeout)
			}
		})
	}
}

func TestNewKeyBasedVerifier(t *testing.T) {
	tests := []struct {
		name      string
		publicKey string
		timeout   time.Duration
	}{
		{
			name:      "valid key verifier",
			publicKey: "/path/to/cosign.pub",
			timeout:   2 * time.Minute,
		},
		{
			name:      "default timeout",
			publicKey: "/path/to/cosign.pub",
			timeout:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			verifier := NewKeyBasedVerifier(tt.publicKey, tt.timeout)
			if verifier == nil {
				t.Fatal("NewKeyBasedVerifier() returned nil")
			}
			if verifier.PublicKey != tt.publicKey {
				t.Errorf("PublicKey = %v, want %v", verifier.PublicKey, tt.publicKey)
			}
			if tt.timeout == 0 && verifier.Timeout != 2*time.Minute {
				t.Errorf("Timeout = %v, want default 2m", verifier.Timeout)
			}
		})
	}
}

func TestVerifier_Verify_InvalidConfig(t *testing.T) {
	tests := []struct {
		name     string
		verifier *Verifier
		wantErr  bool
	}{
		{
			name: "no verification method",
			verifier: &Verifier{
				Timeout: 2 * time.Minute,
			},
			wantErr: true,
		},
		{
			name: "keyless with issuer only",
			verifier: &Verifier{
				OIDCIssuer: "https://token.actions.githubusercontent.com",
				Timeout:    2 * time.Minute,
			},
			wantErr: true,
		},
		{
			name: "keyless with regexp only",
			verifier: &Verifier{
				IdentityRegexp: "^https://github.com/org/.*$",
				Timeout:        2 * time.Minute,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			_, err := tt.verifier.Verify(ctx, "test-image:latest")
			if (err != nil) != tt.wantErr {
				t.Errorf("Verify() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestParseCertificateInfo(t *testing.T) {
	output := "Some verification output"
	info := parseCertificateInfo(output)

	if info == nil {
		t.Error("parseCertificateInfo() returned nil")
	}
}

type mockPolicyChecker struct {
	shouldFail bool
}

func (m *mockPolicyChecker) Check(result *VerifyResult) error {
	if m.shouldFail {
		return fmt.Errorf("policy check failed: policy violation")
	}
	return nil
}

func TestVerifier_VerifyWithPolicy(t *testing.T) {
	verifier := NewKeyBasedVerifier("/path/to/test.pub", 2*time.Minute)
	ctx := context.Background()

	tests := []struct {
		name        string
		policy      PolicyChecker
		expectError bool
	}{
		{
			name:        "nil policy",
			policy:      nil,
			expectError: true, // Will fail because cosign isn't actually running
		},
		{
			name:        "passing policy",
			policy:      &mockPolicyChecker{shouldFail: false},
			expectError: true, // Will fail because cosign isn't actually running
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := verifier.VerifyWithPolicy(ctx, "test-image:latest", tt.policy)
			if (err != nil) != tt.expectError {
				t.Errorf("VerifyWithPolicy() error = %v, expectError %v", err, tt.expectError)
			}
		})
	}
}
