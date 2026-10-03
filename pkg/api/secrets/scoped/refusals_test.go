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
	wantErr(t, "bad scope", err, "scope")
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
		{"bad scope name", "secrets.pr.yaml", "schemaVersion: 1\nstack: app\nscope: '../x'\n", "scope"},
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
	if err := f.Set("bad key!", "v"); err == nil {
		t.Error("an invalid key name was accepted")
	}
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
		{"bad scope", func(c *ScopeFile) { c.Scope = "Bad Scope" }, "scope"},
		{"no stack", func(c *ScopeFile) { c.Stack = " " }, "has no stack"},
		{"no recipients", func(c *ScopeFile) { c.Recipients = nil }, "has no recipients"},
		{"unparseable recipient", func(c *ScopeFile) { c.Recipients = []string{"ssh-ed25519 not-a-key"} }, ""},
		{"bad key name", func(c *ScopeFile) { c.Values["bad key!"] = c.Values["TOKEN"] }, ""},
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
	writeTestFile(t, filepath.Join(sc, "stacks", "app", "secrets.pr.yaml"), "scope")
	files, err := ListScopeFiles(sc)
	if err != nil || len(files) != 1 || filepath.Base(files[0]) != "secrets.pr.yaml" {
		t.Errorf("ListScopeFiles = %v, %v; want only the one scope file", files, err)
	}
	if _, err := ListScopeFiles(blocker); err == nil {
		t.Error("listed scope files under a path that is a file")
	}

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
	wantErr(t, "bad scope name", err, "in ")
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
	wantErr(t, "allow bad scope", err, "scope")
	_, err = s.Allow("pr", "ssh-ed25519 not-a-key")
	if err == nil {
		t.Error("allowed an unparseable recipient")
	}
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
	if err == nil {
		t.Error("disallowed an unparseable recipient")
	}
}

func TestEncryptForRecipientsRefusals(t *testing.T) {
	r, _ := genEd25519Recipient(t)
	_, err := encryptForRecipients([]string{r, r}, "app", "pr", "K", "v")
	if err == nil {
		t.Error("sealed twice to the same recipient")
	}
	_, err = encryptForRecipients([]string{"gcpkms://projects/x"}, "app", "pr", "K", "v")
	if err == nil {
		t.Error("sealed to a malformed gcpkms recipient")
	}
	_, err = encryptForRecipients([]string{"ssh-ed25519 not-a-key"}, "app", "pr", "K", "v")
	if err == nil {
		t.Error("sealed to an unparseable ssh recipient")
	}
	_, err = encryptForRecipients(nil, "app", "pr", "K", "v")
	if err == nil {
		t.Error("sealed to nobody")
	}
}

func TestResolveScopedValuesInputs(t *testing.T) {
	out, err := ResolveScopedValues(filepath.Join(t.TempDir(), "absent"), nil)
	if err != nil || len(out) != 0 {
		t.Errorf("missing stack dir = %v, %v; want empty", out, err)
	}
	file := filepath.Join(t.TempDir(), "file")
	writeTestFile(t, file, "x")
	if _, err := ResolveScopedValues(file, nil); err == nil {
		t.Error("a stack dir that is a file was read")
	}

	dir := filepath.Join(t.TempDir(), "app")
	writeTestFile(t, filepath.Join(dir, "secrets.yaml"), "legacy")
	writeTestFile(t, filepath.Join(dir, "server.yaml"), "x")
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := ResolveScopedValues(dir, nil); err != nil || len(out) != 0 {
		t.Errorf("no scope files = %v, %v; want empty", out, err)
	}

	writeTestFile(t, filepath.Join(dir, "secrets.pr.yaml"), "values: [")
	_, err = ResolveScopedValues(dir, nil)
	if !errors.Is(err, ErrScopedIntegrity) {
		t.Errorf("corrupt scope file: err = %v; want ErrScopedIntegrity", err)
	}
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
	if _, ok := o.kmsClientFor(context.Background(), "us-east-1"); !ok {
		t.Fatal("no client")
	}
	if _, ok := o.kmsClientFor(context.Background(), "us-east-1"); !ok || calls != 1 {
		t.Errorf("second call for one region made %d clients; want 1", calls)
	}

	newKMSClient = func(context.Context, string) (kmsAPI, error) { calls++; return nil, errors.New("no credentials") }
	o = NewOpener(nil, true)
	if _, ok := o.kmsClientFor(context.Background(), "eu-west-1"); ok {
		t.Error("a client without credentials")
	}
	before := calls
	if _, ok := o.kmsClientFor(context.Background(), "eu-west-2"); ok || calls != before {
		t.Error("probed again after finding no identity")
	}

	if o.IsDeclaredSSHRecipient([]string{"ssh-ed25519 not-a-key", testGCPRecipient()}) {
		t.Error("an unparseable or KMS recipient counted as a held SSH key")
	}
}

func TestWrapDEKWithKMSRefusals(t *testing.T) {
	_, err := wrapDEKKMS("awskms://", []byte("dek"), "app", "pr", "K")
	if err == nil {
		t.Error("wrapped for a malformed awskms recipient")
	}
	_, err = parseKMSRecipient("awskms://alias/x?%zz")
	if err == nil {
		t.Error("parsed a recipient with a broken query")
	}

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
