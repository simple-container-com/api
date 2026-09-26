// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package scoped

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"cloud.google.com/go/kms/apiv1/kmspb"
	"github.com/googleapis/gax-go/v2"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	testGCPKeyName  = "projects/acme-prod/locations/europe-west1/keyRings/sc/cryptoKeys/scope-pr"
	testGCPKeyName2 = "projects/acme-prod/locations/europe-west1/keyRings/sc/cryptoKeys/scope-other"
)

func testGCPRecipient() string  { return gcpKMSRecipientScheme + testGCPKeyName }
func testGCPRecipient2() string { return gcpKMSRecipientScheme + testGCPKeyName2 }

// fakeGCPBlob stands in for a Cloud KMS ciphertext and carries what the fake needs
// to enforce Cloud KMS's own invariants: the key that made it and the AAD.
type fakeGCPBlob struct {
	Key string `json:"k"`
	AAD []byte `json:"a"`
	PT  []byte `json:"p"`
	Pad string `json:"pad"`
}

// fakeGCPKMS behaves like Cloud KMS for the checks scoped secrets rely on: Decrypt
// rejects a different key or AAD with INVALID_ARGUMENT, an unlisted key with
// PERMISSION_DENIED, and both calls honour and return CRC32C checksums.
type fakeGCPKMS struct {
	allow         map[string]bool
	decryptErr    error
	encryptBadCRC bool
	encryptAs     string // report this key version name instead of the requested key
	decryptBadCRC bool
	decrypts      int
}

func (f *fakeGCPKMS) Encrypt(_ context.Context, in *kmspb.EncryptRequest, _ ...gax.CallOption) (*kmspb.EncryptResponse, error) {
	if in.GetPlaintextCrc32C().GetValue() != crc(in.GetPlaintext()).GetValue() ||
		in.GetAdditionalAuthenticatedDataCrc32C().GetValue() != crc(in.GetAdditionalAuthenticatedData()).GetValue() {
		return nil, status.Error(codes.InvalidArgument, "checksum mismatch")
	}
	ct, _ := json.Marshal(fakeGCPBlob{Key: in.GetName(), AAD: in.GetAdditionalAuthenticatedData(), PT: in.GetPlaintext(), Pad: strings.Repeat("x", 96)})
	name := in.GetName() + "/cryptoKeyVersions/3"
	if f.encryptAs != "" {
		name = f.encryptAs
	}
	ctCRC := crc(ct)
	if f.encryptBadCRC {
		ctCRC = crc([]byte("corrupted in transit"))
	}
	return &kmspb.EncryptResponse{
		Name:                    name,
		Ciphertext:              ct,
		CiphertextCrc32C:        ctCRC,
		VerifiedPlaintextCrc32C: true,
		VerifiedAdditionalAuthenticatedDataCrc32C: true,
	}, nil
}

func (f *fakeGCPKMS) Decrypt(_ context.Context, in *kmspb.DecryptRequest, _ ...gax.CallOption) (*kmspb.DecryptResponse, error) {
	f.decrypts++
	if f.decryptErr != nil {
		return nil, f.decryptErr
	}
	if in.GetCiphertextCrc32C().GetValue() != crc(in.GetCiphertext()).GetValue() {
		return nil, status.Error(codes.InvalidArgument, "ciphertext checksum mismatch")
	}
	var blob fakeGCPBlob
	if err := json.Unmarshal(in.GetCiphertext(), &blob); err != nil || blob.Key != in.GetName() {
		return nil, status.Error(codes.InvalidArgument, "Decryption failed: verify that 'name' refers to the correct CryptoKey.")
	}
	if !f.allow[blob.Key] {
		return nil, status.Error(codes.PermissionDenied, "Permission 'cloudkms.cryptoKeyVersions.useToDecrypt' denied")
	}
	if !bytes.Equal(blob.AAD, in.GetAdditionalAuthenticatedData()) {
		return nil, status.Error(codes.InvalidArgument, "Decryption failed")
	}
	ptCRC := crc(blob.PT)
	if f.decryptBadCRC {
		ptCRC = crc([]byte("corrupted in transit"))
	}
	return &kmspb.DecryptResponse{Plaintext: blob.PT, PlaintextCrc32C: ptCRC}, nil
}

func withFakeGCPKMS(t *testing.T, fake *fakeGCPKMS, allow ...string) *fakeGCPKMS {
	t.Helper()
	fake.allow = map[string]bool{}
	for _, k := range allow {
		fake.allow[k] = true
	}
	prev := newGCPKMSClient
	newGCPKMSClient = func(context.Context) (gcpKMSAPI, error) { return fake, nil }
	t.Cleanup(func() { newGCPKMSClient = prev })
	return fake
}

func sealGCP(t *testing.T, stack string, recipients []string, kv ...string) *ScopeFile {
	t.Helper()
	f, err := NewScopeFile(stack, "pr", recipients)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < len(kv); i += 2 {
		if err := f.Set(kv[i], kv[i+1]); err != nil {
			t.Fatalf("Set %s: %v", kv[i], err)
		}
	}
	return f
}

func TestGCPKMSRecipient_Parse(t *testing.T) {
	r, err := parseKMSRecipient("  " + testGCPRecipient() + "\n")
	if err != nil || r.provider != kmsProviderGCP || r.keyID != testGCPKeyName || r.raw != testGCPRecipient() {
		t.Fatalf("parse = %+v, %v", r, err)
	}
	if _, err := parseKMSRecipient("gcpkms://projects/example.com:acme-prod/locations/global/keyRings/r/cryptoKeys/k"); err != nil {
		t.Fatalf("a domain-scoped project must parse: %v", err)
	}
	if !isKMSRecipient(testGCPRecipient()) || !isKMSRecipientID(testGCPRecipient()) {
		t.Fatal("gcpkms:// must be recognised as a KMS recipient")
	}
	for _, bad := range []string{
		"gcpkms://",
		"gcpkms://projects/p/locations/l/keyRings/r",
		"gcpkms://" + testGCPKeyName + "/cryptoKeyVersions/1", // a version, not a key
		"gcpkms://projects/p/locations/l/keyRings/r/cryptoKeys/k?x=1",
		"gcpkms://projects/../locations/l/keyRings/r/cryptoKeys/k",
		"gcpkms://projects/acme-prod/locations/../keyRings/r/cryptoKeys/k",
		"gcpkms://projects/acme-prod/locations/global/keyRings/r/cryptoKeys/k/../x",
	} {
		if _, err := parseKMSRecipient(bad); err == nil {
			t.Errorf("parse(%q) should fail", bad)
		}
		if err := validateEncryptableRecipient(bad); err == nil {
			t.Errorf("validateEncryptableRecipient(%q) should fail", bad)
		}
	}
}

func TestScopeFile_RoundTrip_GCPKMS(t *testing.T) {
	withFakeGCPKMS(t, &fakeGCPKMS{}, testGCPKeyName)
	f := sealGCP(t, "mystack", []string{testGCPRecipient()}, "API_KEY", "s3cr3t")
	if err := f.VerifyConsistency(); err != nil {
		t.Fatalf("VerifyConsistency: %v", err)
	}
	if _, ok := f.Values["API_KEY"].Wraps[testGCPRecipient()]; !ok {
		t.Fatalf("expected a wrap slot keyed by %q; got %v", testGCPRecipient(), f.Values["API_KEY"].Wraps)
	}
	val, owned, err := f.Open("API_KEY", NewOpener(nil, true))
	if err != nil || !owned || val != "s3cr3t" {
		t.Fatalf("Open via Cloud KMS = %q,%v,%v; want s3cr3t,true,nil", val, owned, err)
	}
	if _, owned, err := f.Open("API_KEY", NewOpener(nil, false)); owned || err != nil {
		t.Fatalf("Open with KMS disabled = owned=%v err=%v; want not-owned, nil", owned, err)
	}
}

func TestGCPKMS_PermissionDeniedIsNotOwned(t *testing.T) {
	withFakeGCPKMS(t, &fakeGCPKMS{} /* this identity may use no key */)
	f := sealGCP(t, "mystack", []string{testGCPRecipient()}, "K", "v")
	if val, owned, err := f.Open("K", NewOpener(nil, true)); owned || err != nil || val != "" {
		t.Fatalf("PERMISSION_DENIED must be a least-privilege skip: %q,%v,%v", val, owned, err)
	}
}

func TestGCPKMS_TransplantAcrossStacksIsIntegrityError(t *testing.T) {
	withFakeGCPKMS(t, &fakeGCPKMS{}, testGCPKeyName)
	fA := sealGCP(t, "stackA", []string{testGCPRecipient()}, "K", "v")
	fB, _ := NewScopeFile("stackB", "pr", []string{testGCPRecipient()})
	fB.Values["K"] = fA.Values["K"]
	if _, owned, err := fB.Open("K", NewOpener(nil, true)); err == nil || !owned {
		t.Fatalf("a wrap bound to stackA must not open under stackB: owned=%v err=%v", owned, err)
	}
}

func TestGCPKMS_WrapMovedToAnotherKeysSlotIsIntegrityError(t *testing.T) {
	// The Decrypt call names the key from the slot id, so a wrap made under key A
	// that is filed under key B's slot is rejected rather than opened by A.
	withFakeGCPKMS(t, &fakeGCPKMS{}, testGCPKeyName, testGCPKeyName2)
	f := sealGCP(t, "s", []string{testGCPRecipient()}, "K", "v")
	ev := f.Values["K"]
	ev.Wraps = map[string][]string{testGCPRecipient2(): ev.Wraps[testGCPRecipient()]}
	f.Values["K"] = ev
	if _, owned, err := f.Open("K", NewOpener(nil, true)); err == nil || !owned {
		t.Fatalf("wrap under the wrong key slot must be an integrity error: owned=%v err=%v", owned, err)
	}
}

func TestGCPKMS_EncryptIntegrityChecks(t *testing.T) {
	withFakeGCPKMS(t, &fakeGCPKMS{encryptBadCRC: true}, testGCPKeyName)
	f, _ := NewScopeFile("s", "pr", []string{testGCPRecipient()})
	if err := f.Set("K", "v"); err == nil || !strings.Contains(err.Error(), "CRC32C") {
		t.Fatalf("a corrupted Encrypt response must be refused: %v", err)
	}
	withFakeGCPKMS(t, &fakeGCPKMS{encryptAs: testGCPKeyName2 + "/cryptoKeyVersions/1"}, testGCPKeyName)
	if err := f.Set("K", "v"); err == nil || !strings.Contains(err.Error(), "not the recipient key") {
		t.Fatalf("a wrap made by another key must be refused: %v", err)
	}
}

func TestDecryptGCPKMSWrap_Classification(t *testing.T) {
	r, _ := parseKMSRecipient(testGCPRecipient())
	for _, tc := range []struct {
		code             codes.Code
		wantOwned, isErr bool
	}{
		{codes.InvalidArgument, true, true},
		{codes.PermissionDenied, false, false},
		{codes.NotFound, false, false},
		{codes.FailedPrecondition, false, false}, // key disabled or destroyed
		{codes.Unauthenticated, false, false},
		{codes.Unavailable, false, true},
		{codes.ResourceExhausted, false, true},
		{codes.DeadlineExceeded, false, true},
		{codes.Internal, false, true},
	} {
		fake := &fakeGCPKMS{decryptErr: status.Error(tc.code, "x")}
		_, owned, err := decryptGCPKMSWrap(context.Background(), fake, r, []byte("blob"), "s", "pr", "K")
		if owned != tc.wantOwned || (err != nil) != tc.isErr {
			t.Errorf("%v: owned=%v err=%v; want owned=%v err=%v", tc.code, owned, err, tc.wantOwned, tc.isErr)
		}
	}
}

func TestResolveScopedValues_GCPKMSRole(t *testing.T) {
	withFakeGCPKMS(t, &fakeGCPKMS{}, testGCPKeyName)
	dir := t.TempDir()
	f := sealGCP(t, baseName(dir), []string{testGCPRecipient()}, "PR_TOKEN", "abc123", "OTHER", "x")
	if err := f.Save(ScopeFileNamePath(dir, "pr")); err != nil {
		t.Fatal(err)
	}
	got, err := ResolveScopedValues(dir, nil)
	if err != nil || got["PR_TOKEN"] != "abc123" || got["OTHER"] != "x" {
		t.Fatalf("resolve via Cloud KMS = %v, %v", got, err)
	}
}

func TestResolveScopedValues_GCPKMSCorruptResponseIsUnavailable(t *testing.T) {
	fake := withFakeGCPKMS(t, &fakeGCPKMS{}, testGCPKeyName)
	dir := t.TempDir()
	if err := sealGCP(t, baseName(dir), []string{testGCPRecipient()}, "K", "v").Save(ScopeFileNamePath(dir, "pr")); err != nil {
		t.Fatal(err)
	}
	fake.decryptBadCRC = true
	_, err := ResolveScopedValues(dir, nil)
	if !errors.Is(err, ErrScopedUnavailable) || errors.Is(err, ErrScopedIntegrity) {
		t.Fatalf("a checksum mismatch in transit is a retry, not tamper: %v", err)
	}
}

func TestResolveScopedValues_NoADCSkipsWithoutReprobing(t *testing.T) {
	withFakeGCPKMS(t, &fakeGCPKMS{}, testGCPKeyName) // seal with a working client
	dir := t.TempDir()
	if err := sealGCP(t, baseName(dir), []string{testGCPRecipient()}, "A", "1", "B", "2", "C", "3").Save(ScopeFileNamePath(dir, "pr")); err != nil {
		t.Fatal(err)
	}
	calls := 0
	newGCPKMSClient = func(context.Context) (gcpKMSAPI, error) {
		calls++
		return nil, errors.New("could not find default credentials")
	}
	got, err := ResolveScopedValues(dir, nil)
	if err != nil || len(got) != 0 {
		t.Fatalf("no ADC must be a silent skip: %v, %v", got, err)
	}
	if calls != 1 {
		t.Fatalf("credentials probed %d times; want once per resolve", calls)
	}
}

func TestResolveScopedValues_MixedSSHAWSAndGCP(t *testing.T) {
	withFakeKMS(t, testKMSKeyID)
	withFakeGCPKMS(t, &fakeGCPKMS{}, testGCPKeyName)
	sshAuth, sshPriv := genEd25519Recipient(t)
	dir := t.TempDir()
	f := sealGCP(t, baseName(dir), []string{sshAuth, testKMSRecipient(), testGCPRecipient()}, "K", "v")
	if err := f.VerifyConsistency(); err != nil {
		t.Fatal(err)
	}
	if err := f.Save(ScopeFileNamePath(dir, "pr")); err != nil {
		t.Fatal(err)
	}
	for name, keys := range map[string][]string{"ssh": {sshPriv}, "kms": nil} {
		got, err := ResolveScopedValues(dir, keys)
		if err != nil || got["K"] != "v" {
			t.Fatalf("%s: resolve = %v, %v", name, got, err)
		}
	}
}

func TestReencrypt_AddGCPKMSRecipient(t *testing.T) {
	fake := withFakeGCPKMS(t, &fakeGCPKMS{}, testGCPKeyName)
	sshAuth, sshPriv := genEd25519Recipient(t)
	f := sealGCP(t, "s", []string{sshAuth}, "K", "v")
	if err := f.Reencrypt([]string{sshAuth, testGCPRecipient()}, NewOpener([]string{sshPriv}, false)); err != nil {
		t.Fatalf("reseal adding a Cloud KMS recipient: %v", err)
	}
	before := fake.decrypts
	if v, owned, err := f.Open("K", NewOpener(nil, true)); !owned || err != nil || v != "v" {
		t.Fatalf("keyless open after reseal = %q,%v,%v", v, owned, err)
	}
	if fake.decrypts != before+1 {
		t.Fatal("the keyless open did not go through Cloud KMS")
	}
}
