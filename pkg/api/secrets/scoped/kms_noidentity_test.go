// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package scoped

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	kmstypes "github.com/aws/aws-sdk-go-v2/service/kms/types"
	smithy "github.com/aws/smithy-go"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// noAWSIdentity empties every source the default credential chain consults, so a
// test sees what a laptop or a GCP-only CI job with no AWS identity sees.
func noAWSIdentity(t *testing.T) {
	t.Helper()
	missing := filepath.Join(t.TempDir(), "absent")
	for _, k := range []string{
		"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN", "AWS_PROFILE",
		"AWS_WEB_IDENTITY_TOKEN_FILE", "AWS_CONTAINER_CREDENTIALS_FULL_URI",
		"AWS_CONTAINER_CREDENTIALS_RELATIVE_URI", "AWS_ROLE_ARN",
	} {
		t.Setenv(k, "")
	}
	t.Setenv("AWS_CONFIG_FILE", missing)
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", missing)
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
}

func TestNewKMSClient_NoCredentialsFailsFast(t *testing.T) {
	noAWSIdentity(t)
	start := time.Now()
	c, err := newKMSClient(context.Background(), "us-east-1")
	if err == nil || c != nil {
		t.Fatalf("no ambient credentials must be an error, got client=%v err=%v", c, err)
	}
	if !strings.Contains(err.Error(), "no ambient AWS credentials") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !isKMSNoIdentity(err) {
		t.Fatalf("no credentials must read as no identity, not a retry: %v", err)
	}
	if d := time.Since(start); d > 10*time.Second {
		t.Fatalf("credential probe took %v", d)
	}
}

func TestNewKMSClient_StaticCredentialsSucceed(t *testing.T) {
	noAWSIdentity(t)
	t.Setenv("AWS_ACCESS_KEY_ID", "AKIDEXAMPLE")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "secret")
	if c, err := newKMSClient(context.Background(), "us-east-1"); err != nil || c == nil {
		t.Fatalf("static credentials must build a client: %v", err)
	}
}

func sealKMSScope(t *testing.T, dir string, recipients []string, kv ...string) {
	t.Helper()
	sealKMSScopeAs(t, dir, "pr", recipients, kv...)
}

func sealKMSScopeAs(t *testing.T, dir, scope string, recipients []string, kv ...string) {
	t.Helper()
	f, err := NewScopeFile(baseName(dir), scope, recipients)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < len(kv); i += 2 {
		if err := f.Set(kv[i], kv[i+1]); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.Save(ScopeFileNamePath(dir, scope)); err != nil {
		t.Fatal(err)
	}
}

func TestResolveScopedValues_NoAWSIdentityIsSilentSkip(t *testing.T) {
	real := newKMSClient
	withFakeKMS(t, testKMSKeyID)
	dir := t.TempDir()
	sealKMSScope(t, dir, []string{testKMSRecipient()}, "A", "1", "B", "2", "C", "3")

	noAWSIdentity(t)
	probes := 0
	newKMSClient = func(ctx context.Context, region string) (kmsAPI, error) {
		probes++
		return real(ctx, region)
	}
	got, err := ResolveScopedValues(dir, nil)
	if err != nil || len(got) != 0 {
		t.Fatalf("no AWS identity must be a silent skip, got %v, %v", got, err)
	}
	if probes != 1 {
		t.Fatalf("credentials probed %d times; the breaker must hold it to one", probes)
	}
}

func TestResolveScopedValues_SSHWinsWithoutTouchingKMS(t *testing.T) {
	withFakeKMS(t, testKMSKeyID)
	sshAuth, sshPriv := genEd25519Recipient(t)
	dir := t.TempDir()
	sealKMSScope(t, dir, []string{sshAuth, testKMSRecipient()}, "K", "v")

	noAWSIdentity(t)
	newKMSClient = func(context.Context, string) (kmsAPI, error) {
		t.Error("KMS client built for a value an SSH key opens")
		return nil, errors.New("must not be called")
	}
	got, err := ResolveScopedValues(dir, []string{sshPriv})
	if err != nil || got["K"] != "v" {
		t.Fatalf("resolve via SSH = %v, %v", got, err)
	}
}

// credsVanishKMS passes the probe but fails every Decrypt with a credential error.
type credsVanishKMS struct {
	fakeKMS
	decrypts int
}

func (c *credsVanishKMS) Decrypt(context.Context, *kms.DecryptInput, ...func(*kms.Options)) (*kms.DecryptOutput, error) {
	c.decrypts++
	return nil, fmt.Errorf("operation error KMS: Decrypt, get identity: get credentials: failed to refresh cached credentials, %w",
		&kmsCredentialError{Err: errors.New("no EC2 IMDS role found")})
}

func TestResolveScopedValues_CredentialsLostAtDecryptTripBreaker(t *testing.T) {
	withFakeKMS(t, testKMSKeyID)
	dir := t.TempDir()
	sealKMSScope(t, dir, []string{testKMSRecipient()}, "A", "1", "B", "2", "C", "3")
	sealKMSScopeAs(t, dir, "prod", []string{testKMSRecipient()}, "D", "4")

	cli := &credsVanishKMS{}
	newKMSClient = func(context.Context, string) (kmsAPI, error) { return cli, nil }
	got, err := ResolveScopedValues(dir, nil)
	if err != nil || len(got) != 0 {
		t.Fatalf("credential loss at Decrypt must be a silent skip, got %v, %v", got, err)
	}
	if cli.decrypts != 1 {
		t.Fatalf("Decrypt attempted %d times; want exactly once", cli.decrypts)
	}
}

func TestDecryptKMSWrap_CredentialClassification(t *testing.T) {
	r, _ := parseKMSRecipient(testKMSRecipient())
	credErr := func(inner error) error {
		return fmt.Errorf("operation error KMS: Decrypt, get identity: get credentials: %w", &kmsCredentialError{Err: inner})
	}
	for _, tc := range []struct {
		name      string
		err       error
		wantOwned bool
		wantErr   string // "" = no error
	}{
		{"no credentials", credErr(errors.New("no EC2 IMDS role found")), false, ""},
		{"role refused", credErr(&smithy.GenericAPIError{Code: "AccessDenied", Message: "x"}), false, ""},
		{"identity service throttled", credErr(&smithy.GenericAPIError{Code: "ThrottlingException", Message: "x"}), false, "transient"},
		{"identity service 5xx", credErr(&smithy.GenericAPIError{Code: "ServiceUnavailable", Message: "x", Fault: smithy.FaultServer}), false, "transient"},
		{"credential resolution timed out", credErr(context.DeadlineExceeded), false, "transient"},
		{"KMS throttled", &smithy.GenericAPIError{Code: "ThrottlingException", Message: "x"}, false, "transient"},
		{"KMS internal", &kmstypes.KMSInternalException{}, false, "transient"},
		{"untyped network error", errors.New("connection reset"), false, "transient"},
		{"invalid ciphertext", &kmstypes.InvalidCiphertextException{}, true, "KMS rejected the wrap"},
		{"access denied", &smithy.GenericAPIError{Code: "AccessDeniedException", Message: "x"}, false, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dek, owned, err := decryptKMSWrap(context.Background(), &fakeKMS{decryptErr: tc.err}, r, []byte("blob"), "s", "pr", "K")
			if dek != nil || owned != tc.wantOwned {
				t.Fatalf("dek=%v owned=%v; want owned=%v", dek, owned, tc.wantOwned)
			}
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("want a skip, got %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("want error containing %q, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestOpener_CloseClosesCachedGCPClientOnce(t *testing.T) {
	fake := withFakeGCPKMS(t, &fakeGCPKMS{}, testGCPKeyName)
	dir := t.TempDir()
	if err := sealGCP(t, baseName(dir), []string{testGCPRecipient()}, "K", "v").Save(ScopeFileNamePath(dir, "pr")); err != nil {
		t.Fatal(err)
	}
	fake.closes = 0 // sealing closes its own client
	if got, err := ResolveScopedValues(dir, nil); err != nil || got["K"] != "v" {
		t.Fatalf("resolve = %v, %v", got, err)
	}
	if fake.closes != 1 {
		t.Fatalf("ResolveScopedValues closed the Cloud KMS client %d times; want 1", fake.closes)
	}

	o := NewOpener(nil, true)
	if err := o.Close(); err != nil {
		t.Fatalf("Close with no client: %v", err)
	}
	if _, ok := o.gcpKMSClient(context.Background()); !ok {
		t.Fatal("expected a client")
	}
	if err := o.Close(); err != nil || fake.closes != 2 {
		t.Fatalf("Close: err=%v closes=%d; want 2", err, fake.closes)
	}
	if err := o.Close(); err != nil || fake.closes != 2 {
		t.Fatalf("second Close must be a no-op: err=%v closes=%d", err, fake.closes)
	}
}

func TestResolveScopedValues_NoSSHNoKMSSlotsIsEmpty(t *testing.T) {
	sshAuth, _ := genEd25519Recipient(t)
	dir := t.TempDir()
	f, err := NewScopeFile(baseName(dir), "pr", []string{sshAuth})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Set("K", "v"); err != nil {
		t.Fatal(err)
	}
	if err := f.Save(ScopeFileNamePath(dir, "pr")); err != nil {
		t.Fatal(err)
	}
	if got, err := ResolveScopedValues(dir, nil); err != nil || len(got) != 0 {
		t.Fatalf("got %v, %v; want empty", got, err)
	}
}

func TestResolveScopedValues_GCPUnauthenticatedTripsBreaker(t *testing.T) {
	fake := withFakeGCPKMS(t, &fakeGCPKMS{decryptErr: status.Error(codes.Unauthenticated, "no ADC")}, testGCPKeyName)
	dir := t.TempDir()
	// Seal with a working fake, then break Decrypt.
	fake.decryptErr = nil
	for _, scope := range []string{"pr", "prod"} {
		f, err := NewScopeFile(baseName(dir), scope, []string{testGCPRecipient()})
		if err != nil {
			t.Fatal(err)
		}
		if err := f.Set("K-"+scope, "v"); err != nil {
			t.Fatal(err)
		}
		if err := f.Save(ScopeFileNamePath(dir, scope)); err != nil {
			t.Fatal(err)
		}
	}
	fake.decryptErr = status.Error(codes.Unauthenticated, "no ADC")
	fake.decrypts = 0
	got, err := ResolveScopedValues(dir, nil)
	if err != nil || len(got) != 0 {
		t.Fatalf("no GCP identity must be a silent skip: %v, %v", got, err)
	}
	if fake.decrypts != 1 {
		t.Fatalf("Decrypt attempted %d times; the breaker must hold it to one", fake.decrypts)
	}
}

// The Cloud KMS client resolves ADC lazily: construction succeeds with none and the
// first RPC fails Unauthenticated, which decryptGCPKMSWrap must read as no identity.
func TestGCPKMS_RealClientWithoutADCIsNoIdentity(t *testing.T) {
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CLOUDSDK_CONFIG", t.TempDir())
	t.Setenv("GCE_METADATA_HOST", "127.0.0.1:1")
	t.Setenv("GCE_METADATA_TIMEOUT", "1s")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cli, err := newGCPKMSClient(ctx)
	if err != nil {
		t.Fatalf("construction is lazy and must succeed: %v", err)
	}
	defer func() { _ = cli.Close() }()
	r, _ := parseKMSRecipient(testGCPRecipient())
	dek, owned, err := decryptGCPKMSWrap(ctx, cli, r, []byte("blob"), "s", "pr", "K")
	if dek != nil || owned || err != nil {
		t.Fatalf("no ADC must classify as skip, got dek=%v owned=%v err=%v", dek, owned, err)
	}
}

func TestKMSCredentialProvider_TagsFailures(t *testing.T) {
	boom := errors.New("no EC2 IMDS role found")
	p := &kmsCredentialProvider{inner: failingCreds{boom}}
	_, err := p.Retrieve(context.Background())
	var ce *kmsCredentialError
	if !errors.As(err, &ce) || !errors.Is(err, boom) {
		t.Fatalf("failure must be tagged and keep its cause: %v", err)
	}
	if !isKMSNoIdentity(fmt.Errorf("get identity: %w", err)) {
		t.Fatal("a wrapped credential failure must classify as no identity")
	}
}

type failingCreds struct{ err error }

func (f failingCreds) Retrieve(context.Context) (aws.Credentials, error) {
	return aws.Credentials{}, f.err
}

// A profile that does not exist fails config loading; that is no identity too.
func TestNewKMSClient_MissingProfileIsNoIdentity(t *testing.T) {
	noAWSIdentity(t)
	t.Setenv("AWS_PROFILE", "does-not-exist")
	_, err := newKMSClient(context.Background(), "us-east-1")
	if err == nil || !isKMSNoIdentity(err) {
		t.Fatalf("err = %v; want a no-identity error", err)
	}
}
