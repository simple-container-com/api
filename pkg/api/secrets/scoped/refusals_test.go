// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package scoped

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cloud.google.com/go/kms/apiv1/kmspb"
	"github.com/googleapis/gax-go/v2"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func wantErr(t *testing.T, name string, err error, contains string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), contains) {
		t.Errorf("%s: err = %v; want it to mention %q", name, err, contains)
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestNewScopeFileRefusals(t *testing.T) {
	r, _ := genEd25519Recipient(t)
	_, err := NewScopeFile("app", "Bad Scope", []string{r})
	wantErr(t, "bad scope", err, "invalid scope name")
	_, err = NewScopeFile(" ", "pr", []string{r})
	wantErr(t, "no stack", err, "no stack")
	_, err = NewScopeFile("app", "pr", nil)
	wantErr(t, "no recipients", err, "no recipients")
}

func TestLoadScopeFileRefusals(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "app")
	_, err := LoadScopeFile(filepath.Join(dir, "secrets.pr.yaml"))
	wantErr(t, "missing", err, "failed to read")

	cases := []struct{ name, file, body, want string }{
		{"not yaml", "secrets.pr.yaml", "values: [", "failed to parse"},
		{"bad scope name", "secrets.pr.yaml", "schemaVersion: 1\nstack: app\nscope: '../x'\n", "invalid scope name"},
		{"no stack", "secrets.pr.yaml", "schemaVersion: 1\nscope: pr\n", "no stack field"},
		{"moved to another stack", "secrets.pr.yaml", "schemaVersion: 1\nstack: other\nscope: pr\n", "moved file"},
	}
	for _, c := range cases {
		p := filepath.Join(dir, c.file)
		writeTestFile(t, p, c.body)
		_, err := LoadScopeFile(p)
		wantErr(t, c.name, err, c.want)
	}

	p := filepath.Join(dir, "secrets.pr.yaml")
	writeTestFile(t, p, "schemaVersion: 1\nstack: app\nscope: pr\n")
	f, err := LoadScopeFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if f.Values == nil {
		t.Error("a file with no values loads with a nil map")
	}
}

func TestScopeFileSetOpenAndSaveRefusals(t *testing.T) {
	r, _ := genEd25519Recipient(t)
	f, err := NewScopeFile("app", "pr", []string{r})
	if err != nil {
		t.Fatal(err)
	}
	wantErr(t, "bad key", f.Set("bad key!", "v"), "invalid secret key")
	f.Values = nil
	if err := f.Set("TOKEN", "v"); err != nil {
		t.Fatalf("Set on a file with a nil map: %v", err)
	}
	if got := f.String(); got != `scope "pr": 1 value(s), 1 recipient(s)` {
		t.Errorf("String() = %q", got)
	}

	empty := &ScopeFile{Stack: "app", Scope: "pr"}
	wantErr(t, "no recipients", empty.Set("TOKEN", "v"), "no recipients")
	_, _, err = f.Open("ABSENT", NewOpener(nil, false))
	wantErr(t, "missing key", err, "not found in scope")
	wantErr(t, "reseal to nobody", f.Reencrypt(nil, NewOpener(nil, false)), "empty recipient set")

	blocker := filepath.Join(t.TempDir(), "file")
	writeTestFile(t, blocker, "x")
	wantErr(t, "save under a file", f.Save(filepath.Join(blocker, "app", "secrets.pr.yaml")), "failed to write scope file")
}

func TestVerifyConsistencyRefusals(t *testing.T) {
	r, _ := genEd25519Recipient(t)
	f, err := NewScopeFile("app", "pr", []string{r})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Set("TOKEN", "v"); err != nil {
		t.Fatal(err)
	}
	id, err := recipientID(r)
	if err != nil {
		t.Fatal(err)
	}
	good := f.Values["TOKEN"]

	mutate := func(edit func(*ScopeFile)) *ScopeFile {
		c := &ScopeFile{SchemaVersion: f.SchemaVersion, Stack: f.Stack, Scope: f.Scope, Recipients: append([]string(nil), f.Recipients...), Values: map[string]EncryptedValue{}}
		ev := good
		ev.Wraps = map[string][]string{}
		for k, v := range good.Wraps {
			ev.Wraps[k] = append([]string(nil), v...)
		}
		c.Values["TOKEN"] = ev
		edit(c)
		return c
	}
	short := base64.StdEncoding.EncodeToString([]byte("short"))
	kms := testGCPRecipient()
	kmsID, err := recipientID(kms)
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		edit func(*ScopeFile)
		want string
	}{
		{"bad scope", func(c *ScopeFile) { c.Scope = "Bad Scope" }, "invalid scope name"},
		{"no stack", func(c *ScopeFile) { c.Stack = " " }, "has no stack"},
		{"no recipients", func(c *ScopeFile) { c.Recipients = nil }, "has no recipients"},
		{"unparseable recipient", func(c *ScopeFile) { c.Recipients = []string{"ssh-ed25519 not-a-key"} }, "failed to parse recipient public key"},
		{"bad key name", func(c *ScopeFile) { c.Values["bad key!"] = c.Values["TOKEN"] }, "invalid secret key"},
		{"plaintext value", func(c *ScopeFile) { ev := c.Values["TOKEN"]; ev.Ciphertext = "not base64!"; c.Values["TOKEN"] = ev }, "not base64"},
		{"short value", func(c *ScopeFile) { ev := c.Values["TOKEN"]; ev.Ciphertext = short; c.Values["TOKEN"] = ev }, "value ciphertext"},
		{"wrap count", func(c *ScopeFile) { c.Recipients = append(c.Recipients, kms) }, "expected 2"},
		{"empty wrap", func(c *ScopeFile) { c.Values["TOKEN"].Wraps[id] = nil }, "empty data-key wrap"},
		{"ssh wrap not base64", func(c *ScopeFile) { c.Values["TOKEN"].Wraps[id] = []string{"%%"} }, "is not base64"},
		{"kms wrap two blobs", func(c *ScopeFile) {
			c.Recipients = []string{kms}
			c.Values["TOKEN"].Wraps[kmsID] = []string{short, short}
			delete(c.Values["TOKEN"].Wraps, id)
		}, "exactly one blob"},
		{"kms wrap not base64", func(c *ScopeFile) {
			c.Recipients = []string{kms}
			c.Values["TOKEN"].Wraps[kmsID] = []string{"%%"}
			delete(c.Values["TOKEN"].Wraps, id)
		}, "is not base64"},
		{"kms wrap too short", func(c *ScopeFile) {
			c.Recipients = []string{kms}
			c.Values["TOKEN"].Wraps[kmsID] = []string{short}
			delete(c.Values["TOKEN"].Wraps, id)
		}, "below the"},
	}
	for _, c := range cases {
		err := mutate(c.edit).VerifyConsistency()
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v; want it to mention %q", c.name, err, c.want)
		}
	}
	if err := mutate(func(*ScopeFile) {}).VerifyConsistency(); err != nil {
		t.Errorf("the untouched copy fails: %v", err)
	}
}

func TestPathsAndScopesFileRefusals(t *testing.T) {
	if got := ScopesPath("/repo/.sc"); got != filepath.Join("/repo/.sc", ScopesFileName) {
		t.Errorf("ScopesPath = %q", got)
	}

	root := t.TempDir()
	blocker := filepath.Join(root, "file")
	writeTestFile(t, blocker, "x")
	wantErr(t, "dir is a file", writeFileAtomic(filepath.Join(blocker, "x.yaml"), []byte("x"), 0o644), "failed to create directory")
	target := filepath.Join(root, "target")
	if err := os.MkdirAll(filepath.Join(target, "occupied"), 0o755); err != nil {
		t.Fatal(err)
	}
	wantErr(t, "rename onto a non-empty dir", writeFileAtomic(target, []byte("x"), 0o644), "failed to rename")
	if left, _ := filepath.Glob(filepath.Join(root, ".target.tmp-*")); len(left) != 0 {
		t.Errorf("temp files left behind: %v", left)
	}

	sc := filepath.Join(root, ".sc")
	writeTestFile(t, filepath.Join(sc, "stacks", "README"), "not a stack")
	writeTestFile(t, filepath.Join(sc, "stacks", "app", "secrets.yaml"), "legacy")
	writeTestFile(t, filepath.Join(sc, "stacks", "app", "secrets..yaml"), "no scope")
	writeTestFile(t, filepath.Join(sc, "stacks", "app", "secrets.pr.yaml"), "schemaVersion: 1\nrecipients: []\n")
	files, err := ListScopeFiles(filepath.Join(sc, "stacks"))
	if err != nil || len(files) != 1 || filepath.Base(files[0]) != "secrets.pr.yaml" {
		t.Errorf("ListScopeFiles = %v, %v; want only the one scope file", files, err)
	}
	_, err = ListScopeFiles(blocker)
	wantErr(t, "stacks root is a file", err, "failed to list")

	scopes := filepath.Join(root, "scopes")
	if err := os.MkdirAll(scopes, 0o755); err != nil {
		t.Fatal(err)
	}
	_, err = LoadScopes(scopes)
	wantErr(t, "scopes path is a dir", err, "failed to read")
	writeTestFile(t, filepath.Join(root, "bad.yaml"), "scopes: [")
	_, err = LoadScopes(filepath.Join(root, "bad.yaml"))
	wantErr(t, "scopes not yaml", err, "failed to parse")
	writeTestFile(t, filepath.Join(root, "badname.yaml"), "schemaVersion: 1\nscopes:\n  'Bad Name': {}\n")
	_, err = LoadScopes(filepath.Join(root, "badname.yaml"))
	wantErr(t, "bad scope name", err, "invalid scope name")
	writeTestFile(t, filepath.Join(root, "noscopes.yaml"), "schemaVersion: 1\n")
	s, err := LoadScopes(filepath.Join(root, "noscopes.yaml"))
	if err != nil || s.Scopes == nil {
		t.Errorf("a file with no scopes: %v, %+v", err, s)
	}
	wantErr(t, "save under a file", s.Save(filepath.Join(blocker, "scopes.yaml")), "failed to write")

	r, _ := genEd25519Recipient(t)
	_, err = s.Recipients("absent")
	wantErr(t, "undeclared scope", err, "not declared")
	s.Scopes["empty"] = Scope{}
	_, err = s.Recipients("empty")
	wantErr(t, "scope with no recipients", err, "no recipients")
	_, err = s.Allow("Bad Name", r)
	wantErr(t, "allow bad scope", err, "invalid scope name")
	_, err = s.Allow("pr", "ssh-ed25519 not-a-key")
	wantErr(t, "allow unparseable", err, "failed to parse recipient public key")
	s.Scopes = nil
	if added, err := s.Allow("pr", r); err != nil || !added {
		t.Errorf("Allow on a nil map = %v, %v", added, err)
	}
	if added, err := s.Allow("pr", r); err != nil || added {
		t.Errorf("Allow of the same key again = %v, %v; want not added", added, err)
	}
	_, err = s.Disallow("absent", r)
	wantErr(t, "disallow undeclared", err, "not declared")
	_, err = s.Disallow("pr", "ssh-ed25519 not-a-key")
	wantErr(t, "disallow unparseable", err, "failed to parse recipient public key")
}

func TestEncryptForRecipientsRefusals(t *testing.T) {
	r, _ := genEd25519Recipient(t)
	_, err := encryptForRecipients([]string{r, r}, "app", "pr", "K", "v")
	wantErr(t, "duplicate recipient", err, "duplicate recipient")
	_, err = encryptForRecipients([]string{"gcpkms://projects/x"}, "app", "pr", "K", "v")
	wantErr(t, "malformed gcpkms recipient", err, "gcpkms")
	_, err = encryptForRecipients([]string{"ssh-ed25519 not-a-key"}, "app", "pr", "K", "v")
	wantErr(t, "unparseable ssh recipient", err, "failed to parse recipient public key")
	_, err = encryptForRecipients(nil, "app", "pr", "K", "v")
	wantErr(t, "no recipients", err, "has no recipients to encrypt")
}

func TestResolveScopedValuesInputs(t *testing.T) {
	out, err := ResolveScopedValues(filepath.Join(t.TempDir(), "absent"), nil)
	if err != nil || len(out) != 0 {
		t.Errorf("missing stack dir = %v, %v; want empty", out, err)
	}
	file := filepath.Join(t.TempDir(), "file")
	writeTestFile(t, file, "x")
	_, err = ResolveScopedValues(file, nil)
	wantErr(t, "stack dir is a file", err, "failed to list")

	dir := filepath.Join(t.TempDir(), "app")
	writeTestFile(t, filepath.Join(dir, "secrets.yaml"), "legacy")
	writeTestFile(t, filepath.Join(dir, "server.yaml"), "x")
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := ResolveScopedValues(dir, nil); err != nil || len(out) != 0 {
		t.Errorf("no scope files = %v, %v; want empty", out, err)
	}

	writeTestFile(t, filepath.Join(dir, "secrets.pr.yaml"), "schemaVersion: 1\nrecipients: []\nvalues: not-a-map\n")
	_, err = ResolveScopedValues(dir, nil)
	if !errors.Is(err, ErrScopedIntegrity) {
		t.Errorf("corrupt scope file: err = %v; want ErrScopedIntegrity", err)
	}
}

func TestResolveScopedValues_LookalikesAreNotScopeFiles(t *testing.T) {
	RegisterTestingT(t)
	auth, priv := genEd25519Recipient(t)
	dir := filepath.Join(t.TempDir(), "myapp")
	legacy := "schemaVersion: 1.0\nvalues:\n  DB_PASSWORD: changeme\n"
	writeTestFile(t, filepath.Join(dir, "secrets.yaml"), legacy)
	writeTestFile(t, filepath.Join(dir, "secrets.example.yaml"), legacy)
	writeTestFile(t, filepath.Join(dir, "secrets.backup.yaml"), "schemaVersion: 1.0\nauth:\n  aws:\n    type: aws-token\n    config:\n      account: \"1\"\nvalues:\n  K: v\n")
	writeTestFile(t, filepath.Join(dir, "secrets.broken.yaml"), "values: [")

	got, err := ResolveScopedValues(dir, []string{priv})
	Expect(err).NotTo(HaveOccurred())
	Expect(got).To(BeEmpty())

	scopeFiles, lookalikes, err := ScopeFilesIn(dir)
	Expect(err).NotTo(HaveOccurred())
	Expect(scopeFiles).To(BeEmpty())
	Expect(lookalikes).To(HaveLen(3))

	f, err := NewScopeFile("myapp", "pr", []string{auth})
	Expect(err).NotTo(HaveOccurred())
	Expect(f.Set("api-key", "v1")).To(Succeed())
	Expect(f.Save(filepath.Join(dir, ScopeFileName("pr")))).To(Succeed())

	got, err = ResolveScopedValues(dir, []string{priv})
	Expect(err).NotTo(HaveOccurred())
	Expect(got).To(Equal(map[string]string{"api-key": "v1"}))

	// A real scope file moved to the wrong stack still hard-fails: the marker is there.
	other := filepath.Join(t.TempDir(), "otherstack")
	data, rerr := os.ReadFile(filepath.Join(dir, ScopeFileName("pr")))
	Expect(rerr).NotTo(HaveOccurred())
	writeTestFile(t, filepath.Join(other, ScopeFileName("pr")), string(data))
	_, err = ResolveScopedValues(other, []string{priv})
	Expect(errors.Is(err, ErrScopedIntegrity)).To(BeTrue())
	Expect(err.Error()).To(ContainSubstring("moved file?"))
}

func TestOpenerKMSClientsAreProbedOnce(t *testing.T) {
	calls := 0
	prev := newKMSClient
	newKMSClient = func(context.Context, string) (kmsAPI, error) {
		calls++
		return &fakeKMS{}, nil
	}
	t.Cleanup(func() { newKMSClient = prev })

	o := &Opener{}
	if _, ok, err := o.kmsClientFor(context.Background(), "us-east-1"); !ok || err != nil {
		t.Fatal("no client")
	}
	if _, ok, _ := o.kmsClientFor(context.Background(), "us-east-1"); !ok || calls != 1 {
		t.Errorf("second call for one region made %d clients; want 1", calls)
	}

	newKMSClient = func(context.Context, string) (kmsAPI, error) {
		calls++
		return nil, &kmsCredentialError{Err: errors.New("no credentials")}
	}
	o = NewOpener(nil, true)
	if _, ok, err := o.kmsClientFor(context.Background(), "eu-west-1"); ok || err != nil {
		t.Errorf("no credentials: ok %v, err %v; want a silent skip", ok, err)
	}
	before := calls
	if _, ok, _ := o.kmsClientFor(context.Background(), "eu-west-2"); ok || calls != before {
		t.Error("probed again after finding no identity")
	}
}

// A credential probe that times out says nothing about the identity: it is a
// retry, and it must not trip the breaker for the rest of the run.
func TestOpenerKMSClientProbeTimeoutIsTransient(t *testing.T) {
	calls := 0
	prev := newKMSClient
	newKMSClient = func(context.Context, string) (kmsAPI, error) {
		calls++
		return nil, &kmsCredentialError{Err: context.DeadlineExceeded}
	}
	t.Cleanup(func() { newKMSClient = prev })

	o := NewOpener(nil, true)
	if _, ok, err := o.kmsClientFor(context.Background(), "us-east-1"); ok || err == nil {
		t.Fatalf("ok %v, err %v; want a transient error", ok, err)
	}
	if _, _, err := o.kmsClientFor(context.Background(), "us-east-1"); err == nil || calls != 2 {
		t.Errorf("second call: err %v after %d probes; want another probe and error", err, calls)
	}
	if o.kmsNoIdentity {
		t.Error("a timed-out probe tripped the no-identity breaker")
	}
}

func TestIsDeclaredSSHRecipientIgnoresKMSAndUnparseable(t *testing.T) {
	r, priv := genEd25519Recipient(t)
	o := NewOpener([]string{priv}, false)
	if o.IsDeclaredSSHRecipient([]string{"ssh-ed25519 not-a-key", testGCPRecipient()}) {
		t.Error("an unparseable or KMS recipient counted as a held SSH key")
	}
	if !o.IsDeclaredSSHRecipient([]string{"ssh-ed25519 not-a-key", r}) {
		t.Error("the held key was not found next to an unparseable recipient")
	}
}

func TestWrapDEKWithKMSRefusals(t *testing.T) {
	_, err := wrapDEKKMS("awskms://", []byte("dek"), "app", "pr", "K")
	wantErr(t, "malformed awskms recipient", err, "awskms")
	_, err = parseKMSRecipient("awskms://alias/x?%zz")
	wantErr(t, "broken query", err, "awskms")

	prev := newKMSClient
	t.Cleanup(func() { newKMSClient = prev })
	newKMSClient = func(context.Context, string) (kmsAPI, error) { return nil, errors.New("no credentials") }
	_, err = wrapDEKKMS(testKMSRecipient(), []byte("dek"), "app", "pr", "K")
	wantErr(t, "aws no client", err, "no credentials")

	prevGCP := newGCPKMSClient
	t.Cleanup(func() { newGCPKMSClient = prevGCP })
	newGCPKMSClient = func(context.Context) (gcpKMSAPI, error) { return nil, errors.New("no ADC") }
	r, err := parseGCPKMSRecipient(testGCPRecipient())
	if err != nil {
		t.Fatal(err)
	}
	_, err = wrapDEKGCPKMS(r, []byte("dek"), "app", "pr", "K")
	wantErr(t, "gcp no client", err, "no ADC")

	newGCPKMSClient = func(context.Context) (gcpKMSAPI, error) {
		return &failingEncryptGCPKMS{}, nil
	}
	_, err = wrapDEKGCPKMS(r, []byte("dek"), "app", "pr", "K")
	wantErr(t, "gcp encrypt denied", err, "denied")
}

type failingEncryptGCPKMS struct{ fakeGCPKMS }

func (f *failingEncryptGCPKMS) Encrypt(context.Context, *kmspb.EncryptRequest, ...gax.CallOption) (*kmspb.EncryptResponse, error) {
	return nil, status.Error(codes.PermissionDenied, "encrypt denied")
}

// A key that opens two scopes holding the same secret with different values cannot
// pick one: ${secret:} has no environment axis. Environment-qualified keys are the
// supported layout, and the error says so.
func TestResolveScopedValues_AdminAcrossEnvironmentScopes(t *testing.T) {
	admin, adminPriv := genEd25519Recipient(t)
	dir := filepath.Join(t.TempDir(), "myapp")
	seal := func(scope string, kv map[string]string) {
		t.Helper()
		f, err := NewScopeFile("myapp", scope, []string{admin})
		if err != nil {
			t.Fatal(err)
		}
		for k, v := range kv {
			if err := f.Set(k, v); err != nil {
				t.Fatal(err)
			}
		}
		if err := f.Save(filepath.Join(dir, ScopeFileName(scope))); err != nil {
			t.Fatal(err)
		}
	}

	seal("staging", map[string]string{"DB_PASSWORD": "s"})
	seal("prod", map[string]string{"DB_PASSWORD": "p"})
	_, err := ResolveScopedValues(dir, []string{adminPriv})
	if !errors.Is(err, ErrScopedIntegrity) || !strings.Contains(err.Error(), "give each value its own key") {
		t.Fatalf("err = %v; want an integrity error that names the fix", err)
	}

	seal("staging", map[string]string{"staging-db-password": "s"})
	seal("prod", map[string]string{"prod-db-password": "p"})
	got, err := ResolveScopedValues(dir, []string{adminPriv})
	if err != nil {
		t.Fatal(err)
	}
	if got["staging-db-password"] != "s" || got["prod-db-password"] != "p" || len(got) != 2 {
		t.Errorf("got %v; want both environment-qualified values", got)
	}
}
