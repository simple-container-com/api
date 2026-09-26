// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package scoped

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	kmstypes "github.com/aws/aws-sdk-go-v2/service/kms/types"
	smithy "github.com/aws/smithy-go"
)

// fakeKMSBlob is the in-memory stand-in for a KMS ciphertext: it carries the key
// id, the EncryptionContext, and the plaintext so the fake Decrypt can enforce the
// same invariants a real KMS would (key match + context match).
type fakeKMSBlob struct {
	KeyID  string            `json:"k"`
	EncCtx map[string]string `json:"c"`
	PT     []byte            `json:"p"`
	Pad    string            `json:"pad"` // keep the blob comfortably over kmsMinCiphertextLen
}

// fakeKMS implements kmsAPI. allow is the set of key ids the current "role" may
// Decrypt; an unlisted key returns AccessDenied (the "not a recipient" case).
// Encrypt always succeeds (any operator can wrap for a key they name). If decryptErr
// is set, Decrypt returns it unconditionally (used to model transient/error classes).
type fakeKMS struct {
	allow      map[string]bool
	decryptErr error
}

func (f *fakeKMS) Encrypt(_ context.Context, in *kms.EncryptInput, _ ...func(*kms.Options)) (*kms.EncryptOutput, error) {
	raw, err := json.Marshal(fakeKMSBlob{
		KeyID:  aws.ToString(in.KeyId),
		EncCtx: in.EncryptionContext,
		PT:     in.Plaintext,
		Pad:    strings.Repeat("x", 96),
	})
	if err != nil {
		return nil, err
	}
	return &kms.EncryptOutput{CiphertextBlob: raw, KeyId: in.KeyId}, nil
}

func (f *fakeKMS) Decrypt(_ context.Context, in *kms.DecryptInput, _ ...func(*kms.Options)) (*kms.DecryptOutput, error) {
	if f.decryptErr != nil {
		return nil, f.decryptErr
	}
	var blob fakeKMSBlob
	if err := json.Unmarshal(in.CiphertextBlob, &blob); err != nil {
		return nil, &kmstypes.InvalidCiphertextException{} // mangled ciphertext
	}
	if kid := aws.ToString(in.KeyId); kid != "" && kid != blob.KeyID {
		// KeyId pin names a different key than produced the blob: real AWS KMS returns
		// IncorrectKeyException here (a DISTINCT type from InvalidCiphertext).
		return nil, &kmstypes.IncorrectKeyException{}
	}
	if f.allow != nil && !f.allow[blob.KeyID] {
		return nil, &smithy.GenericAPIError{Code: "AccessDeniedException", Message: "not authorized to use " + blob.KeyID}
	}
	if !reflect.DeepEqual(blob.EncCtx, in.EncryptionContext) {
		return nil, &kmstypes.InvalidCiphertextException{} // EncryptionContext (our AAD) mismatch
	}
	return &kms.DecryptOutput{Plaintext: blob.PT, KeyId: aws.String(blob.KeyID)}, nil
}

// withFakeKMS installs a fake KMS client for the duration of a test, restoring the
// real factory on cleanup. No unit test ever reaches AWS.
func withFakeKMS(t *testing.T, allow ...string) {
	t.Helper()
	set := map[string]bool{}
	for _, k := range allow {
		set[k] = true
	}
	prev := newKMSClient
	newKMSClient = func(_ context.Context, _ string) (kmsAPI, error) {
		return &fakeKMS{allow: set}, nil
	}
	t.Cleanup(func() { newKMSClient = prev })
}

// withFakeKMSDecryptErr installs a fake whose Decrypt always returns err (models a
// specific KMS error class); Encrypt still succeeds so a value can be sealed first.
func withFakeKMSDecryptErr(t *testing.T, err error) {
	t.Helper()
	prev := newKMSClient
	newKMSClient = func(_ context.Context, _ string) (kmsAPI, error) {
		return &fakeKMS{decryptErr: err}, nil
	}
	t.Cleanup(func() { newKMSClient = prev })
}

const testKMSKeyID = "alias/sc-test-pr"

func testKMSRecipient() string {
	return "awskms://" + testKMSKeyID + "?region=us-east-1"
}

func TestKMSRecipient_ParseNormalize(t *testing.T) {
	cases := []struct {
		in       string
		wantID   string
		wantErr  bool
		wantNorm string
	}{
		{in: "awskms://alias/foo?region=eu-central-1", wantID: "alias/foo", wantNorm: "awskms://alias/foo?region=eu-central-1"},
		{in: "awskms://arn:aws:kms:us-west-2:123456789012:key/abcd-ef", wantID: "arn:aws:kms:us-west-2:123456789012:key/abcd-ef", wantNorm: "awskms://arn:aws:kms:us-west-2:123456789012:key/abcd-ef?region=us-west-2"},
		{in: "awskms://alias/foo", wantErr: true},         // no region and not an ARN
		{in: "awskms://?region=us-east-1", wantErr: true}, // no key id
		{in: "ssh-ed25519 AAAA...", wantErr: true},        // not a KMS recipient
	}
	for _, c := range cases {
		r, err := parseKMSRecipient(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("parseKMSRecipient(%q): expected error, got %+v", c.in, r)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseKMSRecipient(%q): unexpected error %v", c.in, err)
			continue
		}
		if r.keyID != c.wantID {
			t.Errorf("parseKMSRecipient(%q): keyID=%q want %q", c.in, r.keyID, c.wantID)
		}
		if r.raw != c.wantNorm {
			t.Errorf("parseKMSRecipient(%q): raw=%q want %q", c.in, r.raw, c.wantNorm)
		}
		// recipientID must equal the normalized form (used as the wrap-slot key).
		id, err := recipientID(c.in)
		if err != nil || id != c.wantNorm {
			t.Errorf("recipientID(%q)=%q,%v want %q", c.in, id, err, c.wantNorm)
		}
	}
}

func TestScopeFile_RoundTrip_KMS(t *testing.T) {
	withFakeKMS(t, testKMSKeyID)
	rec := testKMSRecipient()
	f, err := NewScopeFile("mystack", "pr", []string{rec})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Set("API_KEY", "s3cr3t"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := f.VerifyConsistency(); err != nil {
		t.Fatalf("VerifyConsistency: %v", err)
	}
	// The wrap slot is keyed by the normalized KMS URL, not an SSH fingerprint.
	if _, ok := f.Values["API_KEY"].Wraps[rec]; !ok {
		t.Fatalf("expected a KMS wrap slot keyed by %q; got %v", rec, f.Values["API_KEY"].Wraps)
	}
	// Opened via ambient KMS (no SSH key at all).
	val, owned, err := f.Open("API_KEY", NewOpener(nil, true))
	if err != nil || !owned || val != "s3cr3t" {
		t.Fatalf("Open via KMS = %q,%v,%v; want s3cr3t,true,nil", val, owned, err)
	}
	// KMS disabled → not openable (not owned, no error).
	if _, owned, err := f.Open("API_KEY", NewOpener(nil, false)); owned || err != nil {
		t.Fatalf("Open with KMS disabled = owned=%v err=%v; want not-owned, nil", owned, err)
	}
}

func TestKMS_AccessDeniedIsNotOwned(t *testing.T) {
	withFakeKMS(t /* allow nothing → every Decrypt is AccessDenied */)
	f, _ := NewScopeFile("mystack", "pr", []string{testKMSRecipient()})
	// Encrypt is allowed even when Decrypt is not, so Set succeeds.
	if err := f.Set("K", "v"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	val, owned, err := f.Open("K", NewOpener(nil, true))
	if owned || err != nil {
		t.Fatalf("AccessDenied should be a least-privilege skip: got val=%q owned=%v err=%v", val, owned, err)
	}
}

func TestKMS_EncryptionContextBindingIsIntegrityError(t *testing.T) {
	withFakeKMS(t, testKMSKeyID)
	rec := testKMSRecipient()
	// Seal a value in stackA, then transplant it into a file bound to stackB. At
	// decrypt the EncryptionContext is (stackB,pr,K) but the wrap was made for
	// (stackA,pr,K), so KMS rejects it as InvalidCiphertext → integrity (owned+err).
	fA, _ := NewScopeFile("stackA", "pr", []string{rec})
	if err := fA.Set("K", "v"); err != nil {
		t.Fatal(err)
	}
	fB, _ := NewScopeFile("stackB", "pr", []string{rec})
	fB.Values["K"] = fA.Values["K"]
	_, owned, err := fB.Open("K", NewOpener(nil, true))
	if err == nil || !owned {
		t.Fatalf("cross-stack transplant should be owned+integrity error; got owned=%v err=%v", owned, err)
	}
}

func TestKMSErrorClassifiers(t *testing.T) {
	integrity := []error{
		&kmstypes.InvalidCiphertextException{},
		&kmstypes.IncorrectKeyException{},
		&smithy.GenericAPIError{Code: "InvalidCiphertextException"},
		&smithy.GenericAPIError{Code: "IncorrectKeyException"},
	}
	for _, e := range integrity {
		if !isKMSIntegrityError(e) {
			t.Errorf("expected integrity for %T/%v", e, e)
		}
		if isKMSNotAuthorized(e) {
			t.Errorf("integrity error %T must not be classified not-authorized", e)
		}
	}
	notAuthorized := []error{
		&kmstypes.NotFoundException{},
		&kmstypes.DisabledException{},
		&kmstypes.KMSInvalidStateException{},
		&smithy.GenericAPIError{Code: "AccessDeniedException"},
	}
	for _, e := range notAuthorized {
		if !isKMSNotAuthorized(e) {
			t.Errorf("expected not-authorized for %T/%v", e, e)
		}
		if isKMSIntegrityError(e) {
			t.Errorf("not-authorized error %T must not be classified integrity", e)
		}
	}
	transient := []error{
		&smithy.GenericAPIError{Code: "KMSInternalException"},
		&smithy.GenericAPIError{Code: "ThrottlingException"},
		errors.New("dial tcp 10.0.0.1:443: i/o timeout"),
	}
	for _, e := range transient {
		if isKMSIntegrityError(e) || isKMSNotAuthorized(e) {
			t.Errorf("transient error %T must be neither integrity nor not-authorized (surfaced as unavailable)", e)
		}
	}
}

func TestDecryptKMSWrap_Classification(t *testing.T) {
	r, err := parseKMSRecipient(testKMSRecipient())
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name      string
		err       error
		wantOwned bool
		wantErr   bool
	}{
		{"incorrect-key integrity", &kmstypes.IncorrectKeyException{}, true, true},
		{"invalid-ciphertext integrity", &kmstypes.InvalidCiphertextException{}, true, true},
		{"access-denied skip", &smithy.GenericAPIError{Code: "AccessDeniedException"}, false, false},
		{"transient surfaced", &smithy.GenericAPIError{Code: "KMSInternalException"}, false, true},
	}
	for _, c := range cases {
		cli := &fakeKMS{decryptErr: c.err}
		_, owned, derr := decryptKMSWrap(context.Background(), cli, r, []byte("blob"), "s", "pr", "K")
		if owned != c.wantOwned || (derr != nil) != c.wantErr {
			t.Errorf("%s: got owned=%v err=%v; want owned=%v err=%v", c.name, owned, derr, c.wantOwned, c.wantErr)
		}
	}
}

func TestResolveScopedValues_KMSTransientIsUnavailableNotIntegrity(t *testing.T) {
	withFakeKMSDecryptErr(t, &smithy.GenericAPIError{Code: "KMSInternalException"})
	dir := t.TempDir()
	// Seal needs Encrypt (the fake's Encrypt succeeds); only Decrypt errors.
	f, _ := NewScopeFile(baseName(dir), "pr", []string{testKMSRecipient()})
	if err := f.Set("K", "v"); err != nil {
		t.Fatal(err)
	}
	if err := f.Save(ScopeFileNamePath(dir, "pr")); err != nil {
		t.Fatal(err)
	}
	_, err := ResolveScopedValues(dir, nil)
	if err == nil || !errors.Is(err, ErrScopedUnavailable) {
		t.Fatalf("transient KMS error must be ErrScopedUnavailable, got %v", err)
	}
	if errors.Is(err, ErrScopedIntegrity) {
		t.Fatalf("a transient error must NOT be reported as integrity/tamper: %v", err)
	}
}

func TestResolveScopedValues_MalformedKMSSlotSkipped(t *testing.T) {
	withFakeKMS(t, testKMSKeyID)
	dir := t.TempDir()
	f, _ := NewScopeFile(baseName(dir), "pr", []string{testKMSRecipient()})
	if err := f.Set("K", "v"); err != nil {
		t.Fatal(err)
	}
	// Relabel the wrap slot to an unparseable awskms:// id (foreign/corrupt file).
	ev := f.Values["K"]
	ev.Wraps = map[string][]string{"awskms://no-region-here": ev.Wraps[testKMSRecipient()]}
	f.Values["K"] = ev
	if err := f.Save(ScopeFileNamePath(dir, "pr")); err != nil {
		t.Fatal(err)
	}
	// A least-privilege reader must SKIP a slot it cannot even parse, never hard-fail.
	got, err := ResolveScopedValues(dir, nil)
	if err != nil {
		t.Fatalf("malformed foreign KMS slot must be skipped, not error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected nothing resolved, got %v", got)
	}
}

func TestResolveScopedValues_KMSNoCredentialsSkips(t *testing.T) {
	withFakeKMS(t, testKMSKeyID) // seal with a working client
	dir := t.TempDir()
	f, _ := NewScopeFile(baseName(dir), "pr", []string{testKMSRecipient()})
	if err := f.Set("K", "v"); err != nil {
		t.Fatal(err)
	}
	if err := f.Save(ScopeFileNamePath(dir, "pr")); err != nil {
		t.Fatal(err)
	}
	// Now a caller with no ambient AWS credentials: client build fails → skip, no error.
	prev := newKMSClient
	newKMSClient = func(_ context.Context, _ string) (kmsAPI, error) {
		return nil, errors.New("no ambient AWS credentials")
	}
	defer func() { newKMSClient = prev }()
	got, err := ResolveScopedValues(dir, nil)
	if err != nil {
		t.Fatalf("no-credentials must be a skip, not error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected nothing resolved without KMS identity, got %v", got)
	}
}

func TestResolveScopedValues_StrippedWrapIsIntegrity(t *testing.T) {
	authA, privA := genEd25519Recipient(t)
	dir := t.TempDir()
	f, _ := NewScopeFile(baseName(dir), "pr", []string{authA})
	if err := f.Set("A", "va"); err != nil {
		t.Fatal(err)
	}
	if err := f.Set("B", "vb"); err != nil {
		t.Fatal(err)
	}
	// Strip the recipient's wrap from the LATER value (Keys() is sorted: A then B), so
	// value A establishes ownership and value B's missing slot reads as tamper.
	fp, _ := recipientFingerprint(authA)
	ev := f.Values["B"]
	delete(ev.Wraps, fp)
	f.Values["B"] = ev
	if err := f.Save(ScopeFileNamePath(dir, "pr")); err != nil {
		t.Fatal(err)
	}
	_, err := ResolveScopedValues(dir, []string{privA})
	if err == nil || !errors.Is(err, ErrScopedIntegrity) {
		t.Fatalf("a stripped wrap on an owned file must be ErrScopedIntegrity, got %v", err)
	}
}

func TestResolveScopedValues_FirstValueStrippedIsIntegrity(t *testing.T) {
	// A declared SSH recipient whose wrap is stripped from the FIRST (sorted) value
	// must be a hard integrity error, not a silent "not my scope" skip — f.Recipients
	// proves recipiency offline even before any value opens.
	authA, privA := genEd25519Recipient(t)
	dir := t.TempDir()
	f, _ := NewScopeFile(baseName(dir), "pr", []string{authA})
	if err := f.Set("A", "va"); err != nil {
		t.Fatal(err)
	}
	if err := f.Set("B", "vb"); err != nil {
		t.Fatal(err)
	}
	fp, _ := recipientFingerprint(authA)
	ev := f.Values["A"] // "A" sorts first
	delete(ev.Wraps, fp)
	f.Values["A"] = ev
	if err := f.Save(ScopeFileNamePath(dir, "pr")); err != nil {
		t.Fatal(err)
	}
	_, err := ResolveScopedValues(dir, []string{privA})
	if err == nil || !errors.Is(err, ErrScopedIntegrity) {
		t.Fatalf("first-value stripped wrap for a declared recipient must be ErrScopedIntegrity, got %v", err)
	}
}

func TestResolveScopedValues_MixedScope_KMSRoleAndSSH(t *testing.T) {
	withFakeKMS(t, testKMSKeyID)
	sshAuth, sshPriv := genEd25519Recipient(t)
	dir := t.TempDir()
	f, _ := NewScopeFile(baseName(dir), "pr", []string{sshAuth, testKMSRecipient()})
	if err := f.Set("K", "v"); err != nil {
		t.Fatal(err)
	}
	if err := f.Save(ScopeFileNamePath(dir, "pr")); err != nil {
		t.Fatal(err)
	}
	// KMS role (no SSH key) resolves at deploy time.
	got, err := ResolveScopedValues(dir, nil)
	if err != nil || got["K"] != "v" {
		t.Fatalf("KMS role deploy resolve = %v, %v; want K=v", got, err)
	}
	// SSH holder resolves the same value (SSH opens first — no KMS call needed).
	got2, err := ResolveScopedValues(dir, []string{sshPriv})
	if err != nil || got2["K"] != "v" {
		t.Fatalf("SSH holder deploy resolve = %v, %v; want K=v", got2, err)
	}
}

func TestKMSRecipient_GovCloudChinaARN(t *testing.T) {
	cases := map[string]string{
		"awskms://arn:aws-us-gov:kms:us-gov-west-1:123456789012:key/abc": "us-gov-west-1",
		"awskms://arn:aws-cn:kms:cn-north-1:123456789012:key/abc":        "cn-north-1",
	}
	for in, wantRegion := range cases {
		r, err := parseKMSRecipient(in)
		if err != nil {
			t.Errorf("parseKMSRecipient(%q): %v", in, err)
			continue
		}
		if r.region != wantRegion {
			t.Errorf("parseKMSRecipient(%q): region=%q want %q", in, r.region, wantRegion)
		}
	}
}

func TestReencrypt_KMSRecipient(t *testing.T) {
	withFakeKMS(t, testKMSKeyID)
	sshAuth, sshPriv := genEd25519Recipient(t)
	f, err := NewScopeFile("s", "pr", []string{sshAuth, testKMSRecipient()})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Set("K", "v"); err != nil {
		t.Fatal(err)
	}
	// Reseal to KMS-only, decrypting via a KMS-capable opener that holds NO SSH key.
	if err := f.Reencrypt([]string{testKMSRecipient()}, NewOpener(nil, true)); err != nil {
		t.Fatalf("KMS-only reseal (no SSH key) must succeed: %v", err)
	}
	if len(f.Values["K"].Wraps) != 1 {
		t.Fatalf("expected 1 wrap after KMS-only reseal, got %d", len(f.Values["K"].Wraps))
	}
	if v, owned, err := f.Open("K", NewOpener(nil, true)); !owned || err != nil || v != "v" {
		t.Fatalf("KMS-only value should still open: %q,%v,%v", v, owned, err)
	}
	// Reseal back to include SSH, decrypting via KMS this time; SSH holder can then open.
	if err := f.Reencrypt([]string{sshAuth, testKMSRecipient()}, NewOpener(nil, true)); err != nil {
		t.Fatalf("reseal back to mixed via KMS must succeed: %v", err)
	}
	if v, owned, err := f.Open("K", NewOpener([]string{sshPriv}, false)); !owned || err != nil || v != "v" {
		t.Fatalf("SSH holder should open after reseal: %q,%v,%v", v, owned, err)
	}
}

func TestWriteFileAtomic_PermAndNoTempLeak(t *testing.T) {
	dir := t.TempDir()
	p := dir + "/f.yaml"
	if err := writeFileAtomic(p, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o644 {
		t.Fatalf("perm = %o, want 0644", st.Mode().Perm())
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp-") {
			t.Fatalf("temp file leaked: %s", e.Name())
		}
	}
}

func TestScopeFile_MixedSSHAndKMS(t *testing.T) {
	withFakeKMS(t, testKMSKeyID)
	sshAuth, sshPriv := genEd25519Recipient(t)
	recipients := []string{sshAuth, testKMSRecipient()}
	f, err := NewScopeFile("mystack", "pr", recipients)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Set("SHARED", "one-value"); err != nil {
		t.Fatal(err)
	}
	if err := f.VerifyConsistency(); err != nil {
		t.Fatalf("VerifyConsistency mixed scope: %v", err)
	}
	// SSH holder opens (KMS disabled — proves SSH path never needs AWS).
	if val, owned, err := f.Open("SHARED", NewOpener([]string{sshPriv}, false)); !owned || err != nil || val != "one-value" {
		t.Fatalf("SSH open = %q,%v,%v; want one-value,true,nil", val, owned, err)
	}
	// KMS role opens the SAME value (no SSH key).
	if val, owned, err := f.Open("SHARED", NewOpener(nil, true)); !owned || err != nil || val != "one-value" {
		t.Fatalf("KMS open = %q,%v,%v; want one-value,true,nil", val, owned, err)
	}
}

func TestSameRecipients_KMS(t *testing.T) {
	// ARN with explicit region vs derived region → same recipient.
	a := []string{"awskms://arn:aws:kms:us-east-1:1:key/x"}
	b := []string{"awskms://arn:aws:kms:us-east-1:1:key/x?region=us-east-1"}
	if err := SameRecipients(a, b); err != nil {
		t.Errorf("ARN region-derivation should be equal: %v", err)
	}
	// Different region → drift.
	c := []string{"awskms://alias/x?region=us-east-1"}
	d := []string{"awskms://alias/x?region=eu-west-1"}
	if err := SameRecipients(c, d); err == nil {
		t.Error("different regions must be detected as drift")
	}
}

func TestVerifyConsistency_KMS_RejectsShortBlob(t *testing.T) {
	withFakeKMS(t, testKMSKeyID)
	rec := testKMSRecipient()
	f, _ := NewScopeFile("mystack", "pr", []string{rec})
	if err := f.Set("K", "v"); err != nil {
		t.Fatal(err)
	}
	// Replace the KMS wrap with a too-short blob (a plaintext data key would be ~32B).
	ev := f.Values["K"]
	ev.Wraps[rec] = []string{base64.StdEncoding.EncodeToString([]byte("tooshort"))}
	f.Values["K"] = ev
	if err := f.VerifyConsistency(); err == nil || !strings.Contains(err.Error(), "minimum") {
		t.Fatalf("expected KMS short-blob rejection, got %v", err)
	}
}

func TestResolveScopedValues_KMSRole(t *testing.T) {
	withFakeKMS(t, testKMSKeyID)
	dir := t.TempDir()
	f, _ := NewScopeFile(baseName(dir), "pr", []string{testKMSRecipient()})
	if err := f.Set("PR_TOKEN", "abc123"); err != nil {
		t.Fatal(err)
	}
	if err := f.Save(ScopeFileNamePath(dir, "pr")); err != nil {
		t.Fatal(err)
	}
	// A role with KMS access resolves the value (no SSH keys passed).
	got, err := ResolveScopedValues(dir, nil)
	if err != nil {
		t.Fatalf("ResolveScopedValues: %v", err)
	}
	if got["PR_TOKEN"] != "abc123" {
		t.Fatalf("KMS-resolved value = %q; want abc123 (all=%v)", got["PR_TOKEN"], got)
	}
}

func TestResolveScopedValues_KMSAccessDeniedSkips(t *testing.T) {
	withFakeKMS(t /* deny all */)
	dir := t.TempDir()
	f, _ := NewScopeFile(baseName(dir), "pr", []string{testKMSRecipient()})
	if err := f.Set("PR_TOKEN", "abc123"); err != nil {
		t.Fatal(err)
	}
	if err := f.Save(ScopeFileNamePath(dir, "pr")); err != nil {
		t.Fatal(err)
	}
	// No KMS access and no SSH key → the scope is simply not ours; empty, no error.
	got, err := ResolveScopedValues(dir, nil)
	if err != nil {
		t.Fatalf("ResolveScopedValues should skip, not error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected no resolved values, got %v", got)
	}
}

// baseName returns the last path element (the stack name for a temp stack dir).
func baseName(dir string) string {
	parts := strings.Split(strings.TrimRight(dir, "/"), "/")
	return parts[len(parts)-1]
}

// ScopeFileNamePath is a test helper: the scope file path directly inside dir
// (dir already IS the stack directory in these tests).
func ScopeFileNamePath(dir, scope string) string {
	return dir + "/" + ScopeFileName(scope)
}
