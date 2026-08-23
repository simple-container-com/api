// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package signing

import (
	"context"
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/gomega"
)

func TestNewKeylessSigner(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name      string
		oidcToken string
		timeout   time.Duration
		wantErr   bool
	}{
		{
			name:      "valid token",
			oidcToken: "valid.oidc.token",
			timeout:   5 * time.Minute,
			wantErr:   false,
		},
		{
			name:      "empty token",
			oidcToken: "",
			timeout:   5 * time.Minute,
			wantErr:   false, // Constructor doesn't validate, Sign() does
		},
		{
			name:      "default timeout",
			oidcToken: "valid.oidc.token",
			timeout:   0,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			signer := NewKeylessSigner(tt.oidcToken, tt.timeout)
			Expect(signer).ToNot(BeNil())
			Expect(signer.OIDCToken).To(Equal(tt.oidcToken))
			if tt.timeout == 0 {
				Expect(signer.Timeout).To(Equal(5 * time.Minute))
			}
		})
	}
}

func TestValidateOIDCToken(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name    string
		token   string
		wantErr bool
	}{
		{
			name:    "valid JWT token",
			token:   "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.signature",
			wantErr: false,
		},
		{
			name:    "empty token",
			token:   "",
			wantErr: true,
		},
		{
			name:    "invalid format - 2 parts",
			token:   "header.payload",
			wantErr: true,
		},
		{
			name:    "invalid format - 4 parts",
			token:   "header.payload.signature.extra",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			err := ValidateOIDCToken(tt.token)
			if tt.wantErr {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(err).ToNot(HaveOccurred())
			}
		})
	}
}

func TestParseRekorEntry(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name   string
		output string
		want   string
	}{
		{
			name:   "direct URL",
			output: "Successfully uploaded to https://rekor.sigstore.dev/api/v1/log/entries/abcd1234",
			want:   "https://rekor.sigstore.dev/api/v1/log/entries/abcd1234",
		},
		{
			name:   "index reference",
			output: "tlog entry created with index: 123456789",
			want:   "https://rekor.sigstore.dev/api/v1/log/entries?logIndex=123456789",
		},
		{
			name:   "no rekor entry",
			output: "Some other output without rekor information",
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			Expect(parseRekorEntry(tt.output)).To(Equal(tt.want))
		})
	}
}

func TestKeylessSigner_Sign_EmptyToken(t *testing.T) {
	RegisterTestingT(t)

	signer := NewKeylessSigner("", 5*time.Minute)
	ctx := context.Background()

	_, err := signer.Sign(ctx, "test-image:latest")
	Expect(err).To(HaveOccurred())
}

func TestGetRekorEntryFromOutput(t *testing.T) {
	RegisterTestingT(t)

	output := "tlog entry created with index: 999"
	expected := "https://rekor.sigstore.dev/api/v1/log/entries?logIndex=999"

	Expect(GetRekorEntryFromOutput(output)).To(Equal(expected))
}

func TestKeylessSigner_Sign_RetriesOnRekorConflict(t *testing.T) {
	RegisterTestingT(t)
	noBackoff(t)

	conflictStderr := `signing bundle: error signing bundle: [POST /api/v1/log/entries][409] createLogEntryConflict {"code":409,"message":"an equivalent entry already exists in the transparency log"}`

	calls := 0
	signer := NewKeylessSigner("a.b.c", time.Second)
	signer.exec = probeAbsent(func(ctx context.Context, name string, args []string, env []string, timeout time.Duration) (string, string, error) {
		calls++
		Expect(name).To(Equal("cosign"))
		Expect(args[0]).To(Equal("sign"))
		if calls == 1 {
			return "", conflictStderr, fmt.Errorf("exit status 1")
		}
		return "tlog entry created with index: 123456", "", nil
	})
	result, err := signer.Sign(context.Background(), "registry.example.com/app:1.0.0")

	Expect(err).ToNot(HaveOccurred())
	Expect(calls).To(Equal(2), "conflict must trigger exactly one retry")
	Expect(result.RekorEntry).To(Equal("https://rekor.sigstore.dev/api/v1/log/entries?logIndex=123456"))
}

func TestKeylessSigner_Sign_NoRetryOnOtherErrors(t *testing.T) {
	RegisterTestingT(t)

	calls := 0
	signer := NewKeylessSigner("a.b.c", time.Second)
	signer.exec = func(ctx context.Context, name string, args []string, env []string, timeout time.Duration) (string, string, error) {
		calls++
		return "", "error signing: getting signer: oidc: token expired", fmt.Errorf("exit status 1")
	}
	_, err := signer.Sign(context.Background(), "registry.example.com/app:1.0.0")

	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("token expired"))
	Expect(calls).To(Equal(1), "non-conflict errors must not be retried")
}

func TestKeylessSigner_Sign_GivesUpAfterMaxConflictAttempts(t *testing.T) {
	RegisterTestingT(t)
	noBackoff(t)

	calls := 0
	signer := NewKeylessSigner("a.b.c", time.Second)
	signer.exec = probeAbsent(func(ctx context.Context, name string, args []string, env []string, timeout time.Duration) (string, string, error) {
		calls++
		return "", "[POST /api/v1/log/entries][409] createLogEntryConflict", fmt.Errorf("exit status 1")
	})
	_, err := signer.Sign(context.Background(), "registry.example.com/app:1.0.0")

	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("createLogEntryConflict"))
	Expect(calls).To(Equal(maxCosignAttempts))
}

// A redeploy of an unchanged digest: the signature is already on the image, so
// the conflict is the expected outcome and the deploy must not fail on it.
func TestKeylessSigner_Sign_ConfirmedConflictSucceeds(t *testing.T) {
	RegisterTestingT(t)
	noBackoff(t)

	calls, probes := 0, 0
	signer := NewKeylessSigner("a.b.c", time.Second)
	// Confirmation needs an identity to verify against; Config.CreateSigner
	// plumbs these from the signing config in production.
	signer.IdentityRegexp = "^https://example.test/wf@refs/heads/main$"
	signer.OIDCIssuer = "https://token.example.test"
	signer.exec = probePresent(func(ctx context.Context, name string, args []string, env []string, timeout time.Duration) (string, string, error) {
		calls++
		return "", "[POST /api/v1/log/entries][409] createLogEntryConflict", fmt.Errorf("exit status 1")
	}, &probes)

	result, err := signer.Sign(context.Background(), "registry.example.com/app@sha256:f7ed9277c480591d7ec36fe7da13e112b33d898b7687f9bcbcda5c214a242099")

	Expect(err).ToNot(HaveOccurred())
	Expect(result).ToNot(BeNil())
	Expect(calls).To(Equal(1))
	Expect(probes).To(Equal(1))
	Expect(result.Confirmed).To(BeTrue(), "the signature was confirmed, not freshly made")
	Expect(result.RekorEntry).To(BeEmpty(), "no new transparency-log entry exists to point at")
}

// A keyless signer with no certificate identity cannot tell its own signature
// from anyone else's, so it must not confirm. The conflict is retried and then
// reported, which is the pre-confirmation behaviour.
func TestKeylessSigner_Sign_WithoutAnIdentityDoesNotConfirm(t *testing.T) {
	RegisterTestingT(t)
	noBackoff(t)

	calls, probes := 0, 0
	signer := NewKeylessSigner("a.b.c", time.Second)
	signer.exec = probePresent(func(ctx context.Context, name string, args []string, env []string, timeout time.Duration) (string, string, error) {
		calls++
		return "", "[POST /api/v1/log/entries][409] createLogEntryConflict", fmt.Errorf("exit status 1")
	}, &probes)

	_, err := signer.Sign(context.Background(), "registry.example.com/app@sha256:f7ed9277c480591d7ec36fe7da13e112b33d898b7687f9bcbcda5c214a242099")

	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("createLogEntryConflict"))
	Expect(calls).To(Equal(maxCosignAttempts))
	Expect(probes).To(Equal(0), "there is nothing to verify against")
}

// CreateSigner is where the identity reaches the signer. Without this wiring
// the confirmation path is dead code in every production keyless run.
func TestConfig_CreateSigner_PlumbsTheKeylessIdentity(t *testing.T) {
	RegisterTestingT(t)

	cfg := &Config{
		Enabled:        true,
		Keyless:        true,
		OIDCIssuer:     "https://token.example.test",
		IdentityRegexp: "^https://example.test/wf@refs/heads/main$",
	}
	signer, err := cfg.CreateSigner("a.b.c")
	Expect(err).ToNot(HaveOccurred())

	keyless, ok := signer.(*KeylessSigner)
	Expect(ok).To(BeTrue())
	Expect(keyless.IdentityRegexp).To(Equal(cfg.IdentityRegexp))
	Expect(keyless.OIDCIssuer).To(Equal(cfg.OIDCIssuer))
	Expect(keyless.confirmProbe()).ToNot(BeNil())
}
