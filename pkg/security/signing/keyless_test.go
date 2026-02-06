package signing

import (
	"context"
	"testing"
	"time"
)

func TestNewKeylessSigner(t *testing.T) {
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
			signer := NewKeylessSigner(tt.oidcToken, tt.timeout)
			if signer == nil {
				t.Error("NewKeylessSigner() returned nil")
			}
			if signer.OIDCToken != tt.oidcToken {
				t.Errorf("OIDCToken = %v, want %v", signer.OIDCToken, tt.oidcToken)
			}
			if tt.timeout == 0 && signer.Timeout != 5*time.Minute {
				t.Errorf("Timeout = %v, want default 5m", signer.Timeout)
			}
		})
	}
}

func TestValidateOIDCToken(t *testing.T) {
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
			err := ValidateOIDCToken(tt.token)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateOIDCToken() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestParseRekorEntry(t *testing.T) {
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
			got := parseRekorEntry(tt.output)
			if got != tt.want {
				t.Errorf("parseRekorEntry() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestKeylessSigner_Sign_EmptyToken(t *testing.T) {
	signer := NewKeylessSigner("", 5*time.Minute)
	ctx := context.Background()

	_, err := signer.Sign(ctx, "test-image:latest")
	if err == nil {
		t.Error("Sign() with empty token should return error")
	}
}

func TestGetRekorEntryFromOutput(t *testing.T) {
	output := "tlog entry created with index: 999"
	expected := "https://rekor.sigstore.dev/api/v1/log/entries?logIndex=999"

	got := GetRekorEntryFromOutput(output)
	if got != expected {
		t.Errorf("GetRekorEntryFromOutput() = %v, want %v", got, expected)
	}
}
