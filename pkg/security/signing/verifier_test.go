package signing

import (
	"context"
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/gomega"
)

func TestNewKeylessVerifier(t *testing.T) {
	RegisterTestingT(t)

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
			RegisterTestingT(t)
			verifier := NewKeylessVerifier(tt.oidcIssuer, tt.identityRegexp, tt.timeout)
			Expect(verifier).ToNot(BeNil())
			Expect(verifier.OIDCIssuer).To(Equal(tt.oidcIssuer))
			Expect(verifier.IdentityRegexp).To(Equal(tt.identityRegexp))
			if tt.timeout == 0 {
				Expect(verifier.Timeout).To(Equal(2 * time.Minute))
			}
		})
	}
}

func TestNewKeyBasedVerifier(t *testing.T) {
	RegisterTestingT(t)

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
			RegisterTestingT(t)
			verifier := NewKeyBasedVerifier(tt.publicKey, tt.timeout)
			Expect(verifier).ToNot(BeNil())
			Expect(verifier.PublicKey).To(Equal(tt.publicKey))
			if tt.timeout == 0 {
				Expect(verifier.Timeout).To(Equal(2 * time.Minute))
			}
		})
	}
}

func TestVerifier_Verify_InvalidConfig(t *testing.T) {
	RegisterTestingT(t)

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
			RegisterTestingT(t)
			ctx := context.Background()
			_, err := tt.verifier.Verify(ctx, "test-image:latest")
			if tt.wantErr {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(err).ToNot(HaveOccurred())
			}
		})
	}
}

func TestParseCertificateInfo(t *testing.T) {
	RegisterTestingT(t)

	output := "Some verification output"
	info := parseCertificateInfo(output)
	Expect(info).ToNot(BeNil())
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
	RegisterTestingT(t)

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
			RegisterTestingT(t)
			_, err := verifier.VerifyWithPolicy(ctx, "test-image:latest", tt.policy)
			if tt.expectError {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(err).ToNot(HaveOccurred())
			}
		})
	}
}
