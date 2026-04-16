package signing

import (
	"context"
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
