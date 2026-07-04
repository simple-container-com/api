// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package scoped

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	"golang.org/x/crypto/ssh"

	"github.com/simple-container-com/api/pkg/api/secrets/ciphers"
)

// genEd25519Recipient returns an SSH authorized-key line and the matching
// unencrypted PEM private key.
func genEd25519Recipient(t *testing.T) (authorized, privatePEM string) {
	t.Helper()
	priv, pub, err := ciphers.GenerateEd25519KeyPair()
	NewWithT(t).Expect(err).NotTo(HaveOccurred())
	sshPub, err := ssh.NewPublicKey(pub)
	NewWithT(t).Expect(err).NotTo(HaveOccurred())
	pem, err := ciphers.MarshalEd25519PrivateKey(priv)
	NewWithT(t).Expect(err).NotTo(HaveOccurred())
	return string(ssh.MarshalAuthorizedKey(sshPub)), pem
}

// genRSARecipient returns an SSH authorized-key line and the matching PEM key.
func genRSARecipient(t *testing.T) (authorized, privatePEM string) {
	t.Helper()
	priv, pub, err := ciphers.GenerateKeyPair(2048)
	NewWithT(t).Expect(err).NotTo(HaveOccurred())
	sshPub, err := ssh.NewPublicKey(pub)
	NewWithT(t).Expect(err).NotTo(HaveOccurred())
	return string(ssh.MarshalAuthorizedKey(sshPub)), ciphers.MarshalRSAPrivateKey(priv)
}

func TestScopeFile_RoundTrip_Ed25519(t *testing.T) {
	RegisterTestingT(t)
	authorized, priv := genEd25519Recipient(t)
	f, err := NewScopeFile("pr", []string{authorized})
	Expect(err).NotTo(HaveOccurred())
	Expect(f.Set("defectdojo-api-key", "s3cr3t-value")).To(Succeed())

	got, err := f.Get("defectdojo-api-key", priv)
	Expect(err).NotTo(HaveOccurred())
	Expect(got).To(Equal("s3cr3t-value"))
}

func TestScopeFile_RoundTrip_RSA(t *testing.T) {
	RegisterTestingT(t)
	authorized, priv := genRSARecipient(t)
	f, err := NewScopeFile("pr", []string{authorized})
	Expect(err).NotTo(HaveOccurred())
	// value longer than one RSA-OAEP chunk to exercise chunking + AAD
	long := ""
	for i := 0; i < 500; i++ {
		long += "x"
	}
	Expect(f.Set("big", long)).To(Succeed())
	got, err := f.Get("big", priv)
	Expect(err).NotTo(HaveOccurred())
	Expect(got).To(Equal(long))
}

func TestScopeFile_MultiRecipient_And_WrongKey(t *testing.T) {
	RegisterTestingT(t)
	authA, privA := genEd25519Recipient(t)
	authB, privB := genRSARecipient(t)
	_, privC := genEd25519Recipient(t) // not a recipient

	f, err := NewScopeFile("pr", []string{authA, authB})
	Expect(err).NotTo(HaveOccurred())
	Expect(f.Set("k", "v")).To(Succeed())

	// both recipients decrypt
	gotA, err := f.Get("k", privA)
	Expect(err).NotTo(HaveOccurred())
	Expect(gotA).To(Equal("v"))
	gotB, err := f.Get("k", privB)
	Expect(err).NotTo(HaveOccurred())
	Expect(gotB).To(Equal("v"))

	// value sealed exactly twice (once per recipient)
	Expect(f.Values["k"]).To(HaveLen(2))

	// a non-recipient key is rejected distinctly
	_, err = f.Get("k", privC)
	Expect(err).To(HaveOccurred())
	Expect(errors.Is(err, ErrRecipientNotAllowed)).To(BeTrue())
}

func TestScopeFile_TransplantResistance_ScopeAndKey(t *testing.T) {
	RegisterTestingT(t)
	authorized, priv := genEd25519Recipient(t)

	// Seal a value in scope "prod".
	prod, err := NewScopeFile("prod", []string{authorized})
	Expect(err).NotTo(HaveOccurred())
	Expect(prod.Set("api-key", "prod-secret")).To(Succeed())
	blob := prod.Values["api-key"]

	// Attacker copies the ciphertext into a "pr"-scoped file (same recipient).
	// AAD binds to scope, so decrypt under scope "pr" must fail.
	pr, err := NewScopeFile("pr", []string{authorized})
	Expect(err).NotTo(HaveOccurred())
	pr.Values["api-key"] = blob
	_, err = pr.Get("api-key", priv)
	Expect(err).To(HaveOccurred())
	Expect(errors.Is(err, ErrRecipientNotAllowed)).To(BeFalse()) // right recipient, wrong binding

	// Attacker moves the ciphertext onto a different key in the SAME scope.
	// AAD binds to key, so this must fail too.
	prod.Values["other-key"] = blob
	_, err = prod.Get("other-key", priv)
	Expect(err).To(HaveOccurred())
}

func TestLoadScopeFile_RejectsRenamedFile(t *testing.T) {
	RegisterTestingT(t)
	authorized, _ := genEd25519Recipient(t)
	f, err := NewScopeFile("prod", []string{authorized})
	Expect(err).NotTo(HaveOccurred())
	Expect(f.Set("k", "v")).To(Succeed())

	dir := t.TempDir()
	// Write the prod-scoped content under a pr filename (the rename attack).
	renamed := filepath.Join(dir, ScopeFileName("pr"))
	Expect(f.Save(renamed)).To(Succeed())

	_, err = LoadScopeFile(renamed)
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("filename"))
}

func TestLoadScopeFile_SaveLoadRoundTrip(t *testing.T) {
	RegisterTestingT(t)
	authorized, priv := genEd25519Recipient(t)
	f, err := NewScopeFile("pr", []string{authorized})
	Expect(err).NotTo(HaveOccurred())
	Expect(f.Set("k", "v")).To(Succeed())

	dir := t.TempDir()
	path := filepath.Join(dir, ScopeFileName("pr"))
	Expect(f.Save(path)).To(Succeed())

	loaded, err := LoadScopeFile(path)
	Expect(err).NotTo(HaveOccurred())
	Expect(loaded.Scope).To(Equal("pr"))
	got, err := loaded.Get("k", priv)
	Expect(err).NotTo(HaveOccurred())
	Expect(got).To(Equal("v"))
}

func TestLoadScopeFile_VersionGuardFailsClosed(t *testing.T) {
	RegisterTestingT(t)
	authorized, _ := genEd25519Recipient(t)
	f, err := NewScopeFile("pr", []string{authorized})
	Expect(err).NotTo(HaveOccurred())
	f.SchemaVersion = CurrentScopesSchemaVersion + 1

	dir := t.TempDir()
	path := filepath.Join(dir, ScopeFileName("pr"))
	Expect(f.Save(path)).To(Succeed())

	_, err = LoadScopeFile(path)
	Expect(err).To(HaveOccurred())
	Expect(errors.Is(err, ErrScopesVersionUnsupported)).To(BeTrue())
}

func TestVerifyConsistency_DetectsUnknownRecipient(t *testing.T) {
	RegisterTestingT(t)
	authA, _ := genEd25519Recipient(t)
	f, err := NewScopeFile("pr", []string{authA})
	Expect(err).NotTo(HaveOccurred())
	Expect(f.Set("k", "v")).To(Succeed())
	Expect(f.VerifyConsistency()).To(Succeed())

	// Inject a value sealed to a recipient not in the declared list.
	f.Values["k"]["SHA256:bogusfingerprint"] = []string{"deadbeef"}
	Expect(f.VerifyConsistency()).NotTo(Succeed())
}

func TestScopes_AllowDisallowRecipients(t *testing.T) {
	RegisterTestingT(t)
	authA, _ := genEd25519Recipient(t)
	authB, _ := genRSARecipient(t)
	s := &Scopes{SchemaVersion: CurrentScopesSchemaVersion, Scopes: map[string]Scope{}}

	added, err := s.Allow("pr", authA)
	Expect(err).NotTo(HaveOccurred())
	Expect(added).To(BeTrue())
	// idempotent (same key with a different comment)
	added, err = s.Allow("pr", authA+" my-comment")
	Expect(err).NotTo(HaveOccurred())
	Expect(added).To(BeFalse())

	_, err = s.Allow("pr", authB)
	Expect(err).NotTo(HaveOccurred())
	recips, err := s.Recipients("pr")
	Expect(err).NotTo(HaveOccurred())
	Expect(recips).To(HaveLen(2))

	removed, err := s.Disallow("pr", authA)
	Expect(err).NotTo(HaveOccurred())
	Expect(removed).To(BeTrue())
	recips, _ = s.Recipients("pr")
	Expect(recips).To(HaveLen(1))

	_, err = s.Recipients("nonexistent")
	Expect(err).To(HaveOccurred())
}

func TestScopes_SaveLoadRoundTrip_And_VersionGuard(t *testing.T) {
	RegisterTestingT(t)
	authA, _ := genEd25519Recipient(t)
	s := &Scopes{Scopes: map[string]Scope{"pr": {Description: "pr scope", Recipients: []string{authA}}}}
	dir := t.TempDir()
	path := filepath.Join(dir, ScopesFileName)
	Expect(s.Save(path)).To(Succeed())

	loaded, err := LoadScopes(path)
	Expect(err).NotTo(HaveOccurred())
	Expect(loaded.Scopes).To(HaveKey("pr"))
	Expect(loaded.SchemaVersion).To(Equal(CurrentScopesSchemaVersion))

	// too-new version fails closed
	loaded.SchemaVersion = CurrentScopesSchemaVersion + 1
	Expect(loaded.Save(path)).To(Succeed())
	_, err = LoadScopes(path)
	Expect(errors.Is(err, ErrScopesVersionUnsupported)).To(BeTrue())
}

func TestLoadScopes_MissingFileIsEmpty(t *testing.T) {
	RegisterTestingT(t)
	s, err := LoadScopes(filepath.Join(t.TempDir(), "nope.yaml"))
	Expect(err).NotTo(HaveOccurred())
	Expect(s.Scopes).To(BeEmpty())
	Expect(s.SchemaVersion).To(Equal(CurrentScopesSchemaVersion))
}

func TestValidation_RejectsUnsafeNames(t *testing.T) {
	RegisterTestingT(t)
	Expect(ValidateScopeName("pr")).To(Succeed())
	Expect(ValidateScopeName("PR")).NotTo(Succeed())      // uppercase
	Expect(ValidateScopeName("pr/prod")).NotTo(Succeed()) // slash
	Expect(ValidateScopeName("pr\x00x")).NotTo(Succeed()) // NUL
	Expect(ValidateSecretKey("defectdojo-api-key")).To(Succeed())
	Expect(ValidateSecretKey("CLOUDFLARE_API_TOKEN")).To(Succeed())
	Expect(ValidateSecretKey("bad key")).NotTo(Succeed())    // space
	Expect(ValidateSecretKey("bad\x00key")).NotTo(Succeed()) // NUL
}

func TestReencrypt_AddAndRemoveRecipient(t *testing.T) {
	RegisterTestingT(t)
	authA, privA := genEd25519Recipient(t)
	authB, privB := genRSARecipient(t)

	f, err := NewScopeFile("pr", []string{authA})
	Expect(err).NotTo(HaveOccurred())
	Expect(f.Set("k", "v")).To(Succeed())

	// Add B by resealing with A's key.
	Expect(f.Reencrypt([]string{authA, authB}, privA)).To(Succeed())
	gotA, err := f.Get("k", privA)
	Expect(err).NotTo(HaveOccurred())
	Expect(gotA).To(Equal("v"))
	gotB, err := f.Get("k", privB)
	Expect(err).NotTo(HaveOccurred())
	Expect(gotB).To(Equal("v"))
	Expect(f.Values["k"]).To(HaveLen(2))

	// Remove B by resealing to A only.
	Expect(f.Reencrypt([]string{authA}, privA)).To(Succeed())
	Expect(f.Values["k"]).To(HaveLen(1))
	_, err = f.Get("k", privB)
	Expect(errors.Is(err, ErrRecipientNotAllowed)).To(BeTrue())
}

func TestReencrypt_NonRecipientKeyFailsAndLeavesFileIntact(t *testing.T) {
	RegisterTestingT(t)
	authA, _ := genEd25519Recipient(t)
	authB, _ := genRSARecipient(t)
	_, privC := genEd25519Recipient(t) // not a recipient

	f, err := NewScopeFile("pr", []string{authA})
	Expect(err).NotTo(HaveOccurred())
	Expect(f.Set("k", "v")).To(Succeed())
	before := f.Values["k"]

	err = f.Reencrypt([]string{authA, authB}, privC)
	Expect(err).To(HaveOccurred())
	// file untouched
	Expect(f.Recipients).To(Equal([]string{authA}))
	Expect(f.Values["k"]).To(Equal(before))
}

func TestListScopeFiles_ExcludesLegacyPlaintext(t *testing.T) {
	RegisterTestingT(t)
	authA, _ := genEd25519Recipient(t)
	scDir := t.TempDir()

	mk := func(stack, base, scope string) {
		dir := StackDir(scDir, stack)
		Expect(os.MkdirAll(dir, 0o755)).To(Succeed())
		if scope == "" {
			Expect(os.WriteFile(filepath.Join(dir, base), []byte("values:\n  k: v\n"), 0o644)).To(Succeed())
			return
		}
		f, err := NewScopeFile(scope, []string{authA})
		Expect(err).NotTo(HaveOccurred())
		Expect(f.Save(filepath.Join(dir, base))).To(Succeed())
	}
	mk("s1", ScopeFileName("pr"), "pr")
	mk("s1", "secrets.yaml", "") // legacy plaintext — must be excluded
	mk("s2", ScopeFileName("prod"), "prod")

	files, err := ListScopeFiles(scDir)
	Expect(err).NotTo(HaveOccurred())
	Expect(files).To(HaveLen(2))
	for _, p := range files {
		Expect(ScopeNameFromFile(p)).NotTo(Equal(""))
	}
}

func TestScopeNameFromFile(t *testing.T) {
	RegisterTestingT(t)
	Expect(ScopeNameFromFile("/x/.sc/stacks/s/secrets.pr.yaml")).To(Equal("pr"))
	Expect(ScopeNameFromFile("/x/secrets.yaml")).To(Equal(""))
	Expect(ScopeNameFromFile("/x/other.pr.yaml")).To(Equal(""))
}
