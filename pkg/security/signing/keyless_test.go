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

func TestIsRekorConflict(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name   string
		output string
		want   bool
	}{
		{
			name: "cosign bundle 409 createLogEntryConflict",
			output: `signing registry.example.com/app@sha256:a7c43eb1700be291e4ea2bc146c8d5d23118f1d9: signing bundle: error signing bundle: ` +
				`[POST /api/v1/log/entries][409] createLogEntryConflict {"code":409,"message":"an equivalent entry already exists in the transparency log with UUID 108e9186e8c5677a"}`,
			want: true,
		},
		{
			name:   "bare conflict marker",
			output: "createLogEntryConflict",
			want:   true,
		},
		{
			name:   "409 against the rekor entries endpoint",
			output: "[POST /api/v1/log/entries][409] something else",
			want:   true,
		},
		{
			name:   "unrelated 409 from a registry",
			output: "GET https://registry.example.com/v2/: unexpected status 409",
			want:   false,
		},
		{
			name:   "fulcio auth failure",
			output: "error signing: getting signer: getting key from Fulcio: retrieving cert: oidc: token expired",
			want:   false,
		},
		{
			name:   "empty",
			output: "",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			Expect(isRekorConflict(tt.output)).To(Equal(tt.want))
		})
	}
}

func TestKeylessSigner_Sign_RetriesOnRekorConflict(t *testing.T) {
	RegisterTestingT(t)

	origExec := execCommand
	defer func() { execCommand = origExec }()

	conflictStderr := `signing bundle: error signing bundle: [POST /api/v1/log/entries][409] createLogEntryConflict {"code":409,"message":"an equivalent entry already exists in the transparency log"}`

	calls := 0
	execCommand = func(ctx context.Context, name string, args []string, env []string, timeout time.Duration) (string, string, error) {
		calls++
		Expect(name).To(Equal("cosign"))
		Expect(args[0]).To(Equal("sign"))
		if calls == 1 {
			return "", conflictStderr, fmt.Errorf("exit status 1")
		}
		return "tlog entry created with index: 123456", "", nil
	}

	signer := NewKeylessSigner("a.b.c", time.Second)
	result, err := signer.Sign(context.Background(), "registry.example.com/app:1.0.0")

	Expect(err).ToNot(HaveOccurred())
	Expect(calls).To(Equal(2), "conflict must trigger exactly one retry")
	Expect(result.RekorEntry).To(Equal("https://rekor.sigstore.dev/api/v1/log/entries?logIndex=123456"))
}

func TestKeylessSigner_Sign_NoRetryOnOtherErrors(t *testing.T) {
	RegisterTestingT(t)

	origExec := execCommand
	defer func() { execCommand = origExec }()

	calls := 0
	execCommand = func(ctx context.Context, name string, args []string, env []string, timeout time.Duration) (string, string, error) {
		calls++
		return "", "error signing: getting signer: oidc: token expired", fmt.Errorf("exit status 1")
	}

	signer := NewKeylessSigner("a.b.c", time.Second)
	_, err := signer.Sign(context.Background(), "registry.example.com/app:1.0.0")

	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("token expired"))
	Expect(calls).To(Equal(1), "non-conflict errors must not be retried")
}

func TestKeylessSigner_Sign_GivesUpAfterMaxConflictAttempts(t *testing.T) {
	RegisterTestingT(t)

	origExec := execCommand
	defer func() { execCommand = origExec }()

	calls := 0
	execCommand = func(ctx context.Context, name string, args []string, env []string, timeout time.Duration) (string, string, error) {
		calls++
		return "", "[POST /api/v1/log/entries][409] createLogEntryConflict", fmt.Errorf("exit status 1")
	}

	signer := NewKeylessSigner("a.b.c", time.Second)
	_, err := signer.Sign(context.Background(), "registry.example.com/app:1.0.0")

	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("createLogEntryConflict"))
	Expect(calls).To(Equal(maxSignAttempts))
}
