// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package cmd_secrets

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/api/secrets"
	"github.com/simple-container-com/api/pkg/api/secrets/scoped"
	"github.com/simple-container-com/api/pkg/cmd/root_cmd"
	"github.com/simple-container-com/api/pkg/provisioner"
	"github.com/simple-container-com/api/pkg/provisioner/placeholders"
)

// newScopeRepo is a repository with stack "app" and a scope "pr" sealed to admin.
func newScopeRepo(t *testing.T) (workdir, adminPub, adminPEM string) {
	t.Helper()
	workdir = t.TempDir()
	mkStackDirs(t, workdir, ".sc/stacks", "app")
	adminPub, adminPEM = testRecipient(t)
	if out, err := execScope(t, workdir, "", "allow", "--scope", "pr", adminPub); err != nil {
		t.Fatalf("allow: %v\n%s", err, out)
	}
	return workdir, adminPub, adminPEM
}

func scopeFile(workdir, scope string) string {
	return filepath.Join(workdir, ".sc", "stacks", "app", "secrets."+scope+".yaml")
}

func TestScopeCmd_SetRefusesStackWithoutDirectory(t *testing.T) {
	RegisterTestingT(t)
	workdir, _, _ := newScopeRepo(t)
	out, err := execScope(t, workdir, "", "set", "--scope", "pr", "-s", "typo", "K", "v")
	Expect(err).To(HaveOccurred(), out)
	Expect(err.Error()).To(ContainSubstring("has no directory"))
	Expect(filepath.Join(workdir, ".sc", "stacks", "typo")).NotTo(BeAnExistingFile())

	Expect(os.WriteFile(filepath.Join(workdir, ".sc", "stacks", "afile"), []byte("x"), 0o644)).To(Succeed())
	_, err = execScope(t, workdir, "", "set", "--scope", "pr", "-s", "afile", "K", "v")
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("has no directory"))
}

func TestScopeCmd_EveryVerbValidatesNames(t *testing.T) {
	RegisterTestingT(t)
	workdir, _, adminPEM := newScopeRepo(t)
	t.Setenv("SC_SCOPE_KEY", adminPEM)
	for _, stack := range []string{"a/b", "..", "."} {
		for _, args := range [][]string{
			{"set", "--scope", "pr", "-s", stack, "K", "v"},
			{"get", "--scope", "pr", "-s", stack, "K"},
			{"list", "--scope", "pr", "-s", stack},
			{"delete", "--scope", "pr", "-s", stack, "K"},
		} {
			_, err := execScope(t, workdir, "", args...)
			Expect(err).To(HaveOccurred(), "%v", args)
			Expect(err.Error()).To(ContainSubstring("invalid stack name"), "%v", args)
		}
	}
	for _, args := range [][]string{
		{"list", "--scope", "../x", "-s", "app"},
		{"delete", "--scope", "../x", "-s", "app", "K"},
	} {
		_, err := execScope(t, workdir, "", args...)
		Expect(err).To(HaveOccurred(), "%v", args)
		Expect(err.Error()).To(ContainSubstring("invalid scope name"), "%v", args)
	}
}

func TestScopeCmd_SetWithoutValueIsRefused(t *testing.T) {
	RegisterTestingT(t)
	workdir, _, _ := newScopeRepo(t)
	for _, args := range [][]string{
		{"set", "--scope", "pr", "-s", "app", "K"},
		{"set", "--scope", "pr", "-s", "app"},
	} {
		_, err := execScope(t, workdir, "piped", args...)
		Expect(err).To(HaveOccurred(), "%v", args)
	}
	_, err := execScope(t, workdir, "piped", "set", "--scope", "pr", "-s", "app", "K")
	Expect(err.Error()).To(ContainSubstring("VALUE is required"))
	Expect(scopeFile(workdir, "pr")).NotTo(BeAnExistingFile())
}

// disallow of a recipient that scopes.yaml no longer lists, while a file still
// holds it, reseals the file without it and warns to rotate.
func TestScopeCmd_DisallowResealsDriftWithoutChangingScopesYAML(t *testing.T) {
	RegisterTestingT(t)
	workdir, adminPub, adminPEM := newScopeRepo(t)
	bobPub, bobPEM := testRecipient(t)
	t.Setenv("SC_SCOPE_KEY", adminPEM)
	_, err := execScope(t, workdir, "", "allow", "--scope", "pr", bobPub)
	Expect(err).NotTo(HaveOccurred())
	_, err = execScope(t, workdir, "", "set", "--scope", "pr", "-s", "app", "K", "v")
	Expect(err).NotTo(HaveOccurred())
	// scopes.yaml drops bob by hand; the file still holds him.
	scopesPath := filepath.Join(workdir, ".sc", "scopes.yaml")
	sc, err := scoped.LoadScopes(scopesPath)
	Expect(err).NotTo(HaveOccurred())
	sc.Scopes["pr"] = scoped.Scope{Recipients: []string{adminPub}}
	Expect(sc.Save(scopesPath)).To(Succeed())
	before, err := os.ReadFile(scopesPath)
	Expect(err).NotTo(HaveOccurred())

	out, err := execScope(t, workdir, "", "disallow", "--scope", "pr", bobPub)
	Expect(err).NotTo(HaveOccurred(), out)
	Expect(out).To(ContainSubstring("resealed 1 file(s)"))
	Expect(out).To(ContainSubstring("drifted"))
	Expect(out).To(ContainSubstring("Rotate every value"))
	after, _ := os.ReadFile(scopesPath)
	Expect(string(after)).To(Equal(string(before)))
	f, err := scoped.LoadScopeFile(scopeFile(workdir, "pr"))
	Expect(err).NotTo(HaveOccurred())
	Expect(scoped.SameRecipients(f.Recipients, []string{adminPub})).To(Succeed())
	t.Setenv("SC_SCOPE_KEY", bobPEM)
	_, err = execScope(t, workdir, "", "get", "--scope", "pr", "-s", "app", "K")
	Expect(err).To(HaveOccurred())
}

// allow of a recipient scopes.yaml already lists reseals a file that lacks it,
// and adding someone is no reason to rotate.
func TestScopeCmd_AllowResealsDriftWithoutChangingScopesYAML(t *testing.T) {
	RegisterTestingT(t)
	workdir, adminPub, adminPEM := newScopeRepo(t)
	bobPub, bobPEM := testRecipient(t)
	t.Setenv("SC_SCOPE_KEY", adminPEM)
	_, err := execScope(t, workdir, "", "set", "--scope", "pr", "-s", "app", "K", "v")
	Expect(err).NotTo(HaveOccurred())
	scopesPath := filepath.Join(workdir, ".sc", "scopes.yaml")
	sc, _ := scoped.LoadScopes(scopesPath)
	sc.Scopes["pr"] = scoped.Scope{Recipients: []string{adminPub, bobPub}}
	Expect(sc.Save(scopesPath)).To(Succeed())

	out, err := execScope(t, workdir, "", "allow", "--scope", "pr", bobPub)
	Expect(err).NotTo(HaveOccurred(), out)
	Expect(out).To(ContainSubstring("resealed 1 file(s)"))
	Expect(out).NotTo(ContainSubstring("Rotate"))
	t.Setenv("SC_SCOPE_KEY", bobPEM)
	out, err = execScope(t, workdir, "", "get", "--scope", "pr", "-s", "app", "K")
	Expect(err).NotTo(HaveOccurred(), out)
	Expect(strings.TrimSpace(out)).To(Equal("v"))

	out, err = execScope(t, workdir, "", "allow", "--scope", "pr", bobPub)
	Expect(err).NotTo(HaveOccurred())
	Expect(out).To(ContainSubstring("nothing to do"))
}

// lint opens what the job's SC_KEY_* keys open, and reports a broken auth entry
// without printing its decrypted content.
func TestScopeCmd_LintChecksAuthEntriesWithEnvScopeKeysOnly(t *testing.T) {
	RegisterTestingT(t)
	workdir, _, adminPEM := newScopeRepo(t)
	_, err := execScope(t, workdir, "", "set", "--scope", "pr", "-s", "app", "K", "v")
	Expect(err).NotTo(HaveOccurred())
	f, err := scoped.LoadScopeFile(scopeFile(workdir, "pr"))
	Expect(err).NotTo(HaveOccurred())
	const secretish = "s3cr3t-value-that-must-not-be-printed"
	Expect(f.Set("auth:broken", "type: gcp-service-account\nconfig: "+secretish)).To(Succeed())
	Expect(f.Save(scopeFile(workdir, "pr"))).To(Succeed())

	out, err := execScope(t, workdir, "", "lint")
	Expect(err).NotTo(HaveOccurred(), "without a key lint cannot open the entry: %s", out)

	t.Setenv("SC_KEY_PR", adminPEM)
	out, err = execScope(t, workdir, "", "lint")
	Expect(err).To(HaveOccurred())
	Expect(out).To(ContainSubstring("auth:broken does not parse"))
	Expect(out).NotTo(ContainSubstring(secretish))

	// A valid auth entry stays quiet.
	Expect(f.Delete("auth:broken")).To(BeTrue())
	Expect(f.Set("auth:gcloud", "type: gcp-service-account\nconfig:\n  projectId: p\n  credentials: \"\"\n")).To(Succeed())
	Expect(f.Save(scopeFile(workdir, "pr"))).To(Succeed())
	out, err = execScope(t, workdir, "", "lint")
	Expect(err).NotTo(HaveOccurred(), out)
}

// A key that was given and cannot be parsed stops every command that would use
// it, naming where it came from.
func TestScopeCmd_UnparseableKeyNamesItsSource(t *testing.T) {
	RegisterTestingT(t)
	workdir, _, _ := newScopeRepo(t)
	_, err := execScope(t, workdir, "", "set", "--scope", "pr", "-s", "app", "K", "v")
	Expect(err).NotTo(HaveOccurred())
	junk := filepath.Join(t.TempDir(), "junk.pem")
	Expect(os.WriteFile(junk, []byte("not a key"), 0o600)).To(Succeed())

	for source, setup := range map[string]func() []string{
		"--key-file": func() []string { return []string{"--key-file", junk} },
		"SC_KEY_PR":  func() []string { t.Setenv("SC_KEY_PR", "not a key"); return nil },
		"SC_SCOPE_KEY": func() []string {
			t.Setenv("SC_KEY_PR", "")
			t.Setenv("SC_SCOPE_KEY", "not a key")
			return nil
		},
	} {
		extra := setup()
		args := append([]string{"get", "--scope", "pr", "-s", "app", "K"}, extra...)
		_, err := execScope(t, workdir, "", args...)
		Expect(err).To(HaveOccurred(), source)
		Expect(err.Error()).To(ContainSubstring(source), source)
		Expect(err.Error()).To(ContainSubstring("cannot be parsed"), source)
		// lint and doctor have no --scope, so SC_KEY_PR is not theirs to name; lint
		// has no --key-file.
		verbs := map[string][]string{"--key-file": {"doctor"}, "SC_SCOPE_KEY": {"lint", "doctor"}}[source]
		{
			for _, verb := range verbs {
				vargs := append([]string{verb}, extra...)
				_, err := execScope(t, workdir, "", vargs...)
				Expect(err).To(HaveOccurred(), "%s with %s", verb, source)
				Expect(err.Error()).To(ContainSubstring("cannot be parsed"), "%s with %s", verb, source)
			}
		}
	}
}

func TestScopeCmd_DoctorStatuses(t *testing.T) {
	RegisterTestingT(t)
	workdir, adminPub, adminPEM := newScopeRepo(t)
	_, err := execScope(t, workdir, "", "allow", "--scope", "qa", adminPub)
	Expect(err).NotTo(HaveOccurred())
	_, err = execScope(t, workdir, "", "allow", "--scope", "ops", adminPub)
	Expect(err).NotTo(HaveOccurred())
	_, err = execScope(t, workdir, "", "set", "--scope", "pr", "-s", "app", "K", "v")
	Expect(err).NotTo(HaveOccurred())
	_, err = execScope(t, workdir, "", "set", "--scope", "qa", "-s", "app", "K", "v")
	Expect(err).NotTo(HaveOccurred())
	_, err = execScope(t, workdir, "", "set", "--scope", "ops", "-s", "app", "K", "v")
	Expect(err).NotTo(HaveOccurred())
	// ops: emptied; qa: its value's ciphertext corrupted.
	ops, _ := scoped.LoadScopeFile(scopeFile(workdir, "ops"))
	ops.Delete("K")
	Expect(ops.Save(scopeFile(workdir, "ops"))).To(Succeed())
	qa, _ := scoped.LoadScopeFile(scopeFile(workdir, "qa"))
	v := qa.Values["K"]
	v.Ciphertext = base64.StdEncoding.EncodeToString([]byte(strings.Repeat("x", 64)))
	qa.Values["K"] = v
	Expect(qa.Save(scopeFile(workdir, "qa"))).To(Succeed())

	out, err := execScope(t, workdir, "", "doctor")
	Expect(err).NotTo(HaveOccurred())
	Expect(out).To(ContainSubstring("no private key found"))
	Expect(out).To(MatchRegexp(`(?m)^no\s+scope=pr`))

	t.Setenv("SC_SCOPE_KEY", adminPEM)
	out, err = execScope(t, workdir, "", "doctor")
	Expect(err).NotTo(HaveOccurred())
	Expect(out).NotTo(ContainSubstring("no private key found"))
	Expect(out).To(MatchRegexp(`(?m)^YES\s+scope=pr`))
	Expect(out).To(MatchRegexp(`(?m)^ERR\s+scope=qa`))
	Expect(out).To(MatchRegexp(`(?m)^empty\s+scope=ops`))
}

// Parallel set calls in one repository keep every value, and every command
// releases the lock, including when it fails.
func TestScopeCmd_ConcurrentSetsKeepEveryValue(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the store is not locked on Windows (releases are Linux and macOS only)")
	}
	workdir, _, adminPEM := newScopeRepo(t)
	t.Setenv("SC_SCOPE_KEY", adminPEM)
	const n = 12
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if out, err := execScopeQuiet(workdir, "set", "--scope", "pr", "-s", "app", fmt.Sprintf("K%d", i), "v"); err != nil {
				errs <- fmt.Errorf("set K%d: %v: %s", i, err, out)
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	f, err := scoped.LoadScopeFile(scopeFile(workdir, "pr"))
	if err != nil {
		t.Fatal(err)
	}
	if got := len(f.Keys()); got != n {
		t.Fatalf("%d set calls succeeded but %d keys survive", n, got)
	}

	// A failing command still releases the lock.
	if _, err := execScopeQuiet(workdir, "set", "--scope", "undeclared", "-s", "app", "K", "v"); err == nil {
		t.Fatal("set into an undeclared scope succeeded")
	}
	done := make(chan error, 1)
	go func() {
		release, err := scoped.LockStore(filepath.Join(workdir, ".sc"), nil)
		if err == nil {
			release()
		}
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the lock was not released after a failed command")
	}
}

// execScopeQuiet is execScope for goroutines: it reports failures instead of
// failing the test from another goroutine.
func execScopeQuiet(workdir string, args ...string) (string, error) {
	cryptor, err := secrets.NewCryptor(workdir)
	if err != nil {
		return "", err
	}
	p, err := provisioner.New(provisioner.WithCryptor(cryptor), provisioner.WithPlaceholders(placeholders.New()))
	if err != nil {
		return "", err
	}
	cmd := NewScopeCmd(&secretsCmd{Root: &root_cmd.RootCmd{Provisioner: p}})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(strings.NewReader(""))
	cmd.SetArgs(args)
	err = cmd.Execute()
	return out.String(), err
}

func TestScopeCmd_SetAndLintRefuseADriftedFile(t *testing.T) {
	RegisterTestingT(t)
	workdir, adminPub, _ := newScopeRepo(t)
	bobPub, _ := testRecipient(t)
	_, err := execScope(t, workdir, "", "set", "--scope", "pr", "-s", "app", "K", "v")
	Expect(err).NotTo(HaveOccurred())
	scopesPath := filepath.Join(workdir, ".sc", "scopes.yaml")
	sc, _ := scoped.LoadScopes(scopesPath)
	sc.Scopes["pr"] = scoped.Scope{Recipients: []string{adminPub, bobPub}}
	Expect(sc.Save(scopesPath)).To(Succeed())
	before, _ := os.ReadFile(scopeFile(workdir, "pr"))

	_, err = execScope(t, workdir, "", "set", "--scope", "pr", "-s", "app", "K2", "v")
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("drifted"))
	after, _ := os.ReadFile(scopeFile(workdir, "pr"))
	Expect(string(after)).To(Equal(string(before)))

	out, err := execScope(t, workdir, "", "lint")
	Expect(err).To(HaveOccurred())
	Expect(out).To(ContainSubstring("recipients drift"))
}

// delete, allow and disallow wait for the store lock like set does.
func TestScopeCmd_EveryWritingVerbTakesTheLock(t *testing.T) {
	RegisterTestingT(t)
	workdir, _, adminPEM := newScopeRepo(t)
	bobPub, _ := testRecipient(t)
	t.Setenv("SC_SCOPE_KEY", adminPEM)
	_, err := execScope(t, workdir, "", "set", "--scope", "pr", "-s", "app", "K", "v")
	Expect(err).NotTo(HaveOccurred())
	release, err := scoped.LockStore(filepath.Join(workdir, ".sc"), nil)
	Expect(err).NotTo(HaveOccurred())
	defer release()
	t.Setenv("SC_SCOPE_LOCK_TIMEOUT", "300ms")
	for _, args := range [][]string{
		{"set", "--scope", "pr", "-s", "app", "K2", "v"},
		{"delete", "--scope", "pr", "-s", "app", "K"},
		{"allow", "--scope", "pr", bobPub},
		{"disallow", "--scope", "pr", bobPub},
	} {
		_, err := execScope(t, workdir, "", args...)
		Expect(err).To(HaveOccurred(), "%v ran while the store was locked", args)
		Expect(err.Error()).To(ContainSubstring("another sc process"), "%v", args)
	}
}

// A scopes.yaml that allow cannot edit stops it before any scope file is
// resealed, so files and governance never disagree.
func TestScopeCmd_AllowTouchesNothingWhenScopesYAMLCannotBeEdited(t *testing.T) {
	RegisterTestingT(t)
	workdir, adminPub, adminPEM := newScopeRepo(t)
	bobPub, _ := testRecipient(t)
	t.Setenv("SC_SCOPE_KEY", adminPEM)
	_, err := execScope(t, workdir, "", "set", "--scope", "pr", "-s", "app", "K", "v")
	Expect(err).NotTo(HaveOccurred())
	scopesPath := filepath.Join(workdir, ".sc", "scopes.yaml")
	anchored := "schemaVersion: 1\nkeys: &admin\n  - " + strings.TrimSpace(adminPub) + "\nscopes:\n  pr:\n    recipients: *admin\n"
	Expect(os.WriteFile(scopesPath, []byte(anchored), 0o644)).To(Succeed())
	beforeFile, _ := os.ReadFile(scopeFile(workdir, "pr"))

	_, err = execScope(t, workdir, "", "allow", "--scope", "pr", bobPub)
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("anchors"))
	afterFile, _ := os.ReadFile(scopeFile(workdir, "pr"))
	Expect(string(afterFile)).To(Equal(string(beforeFile)))
	afterScopes, _ := os.ReadFile(scopesPath)
	Expect(string(afterScopes)).To(Equal(anchored))
}

func TestScopeCmd_ScopeKeyEnvironment(t *testing.T) {
	RegisterTestingT(t)
	workdir, adminPub, adminPEM := newScopeRepo(t)
	_, err := execScope(t, workdir, "", "allow", "--scope", "my-scope", adminPub)
	Expect(err).NotTo(HaveOccurred())
	_, err = execScope(t, workdir, "", "set", "--scope", "my-scope", "-s", "app", "K", "v")
	Expect(err).NotTo(HaveOccurred())

	// my-scope is read with SC_KEY_MY_SCOPE.
	t.Setenv("SC_KEY_MY_SCOPE", adminPEM)
	out, err := execScope(t, workdir, "", "get", "--scope", "my-scope", "-s", "app", "K")
	Expect(err).NotTo(HaveOccurred(), out)
	Expect(strings.TrimSpace(out)).To(Equal("v"))

	// A blank SC_KEY_<SCOPE> is not a key: the next source is used.
	t.Setenv("SC_KEY_MY_SCOPE", "   ")
	t.Setenv("SC_SCOPE_KEY", adminPEM)
	out, err = execScope(t, workdir, "", "get", "--scope", "my-scope", "-s", "app", "K")
	Expect(err).NotTo(HaveOccurred(), out)

	// A junk SC_KEY_<SCOPE> stops lint and doctor, naming it.
	t.Setenv("SC_KEY_PR", "not a key")
	for _, verb := range []string{"lint", "doctor"} {
		_, err := execScope(t, workdir, "", verb)
		Expect(err).To(HaveOccurred(), verb)
		Expect(err.Error()).To(ContainSubstring("SC_KEY_PR"), verb)
	}
}

// A file whose first value fails to open is ERR even if a later one opens.
func TestScopeCmd_DoctorErrIsSticky(t *testing.T) {
	RegisterTestingT(t)
	workdir, _, adminPEM := newScopeRepo(t)
	_, err := execScope(t, workdir, "", "set", "--scope", "pr", "-s", "app", "A", "a")
	Expect(err).NotTo(HaveOccurred())
	_, err = execScope(t, workdir, "", "set", "--scope", "pr", "-s", "app", "B", "b")
	Expect(err).NotTo(HaveOccurred())
	f, _ := scoped.LoadScopeFile(scopeFile(workdir, "pr"))
	v := f.Values["A"]
	v.Ciphertext = base64.StdEncoding.EncodeToString([]byte(strings.Repeat("x", 64)))
	f.Values["A"] = v
	Expect(f.Save(scopeFile(workdir, "pr"))).To(Succeed())
	t.Setenv("SC_SCOPE_KEY", adminPEM)
	out, err := execScope(t, workdir, "", "doctor")
	Expect(err).NotTo(HaveOccurred())
	Expect(out).To(MatchRegexp(`(?m)^ERR\s+scope=pr`))
}

func TestScopeCmd_StdinLineEndings(t *testing.T) {
	RegisterTestingT(t)
	workdir, _, adminPEM := newScopeRepo(t)
	t.Setenv("SC_SCOPE_KEY", adminPEM)
	for in, want := range map[string]string{
		"v\n":    "v",
		"v\r\n":  "v",
		"v\n\n":  "v\n",
		"v":      "v",
		"a\nb\n": "a\nb",
	} {
		_, err := execScope(t, workdir, in, "set", "--scope", "pr", "-s", "app", "K", "-")
		Expect(err).NotTo(HaveOccurred())
		f, _ := scoped.LoadScopeFile(scopeFile(workdir, "pr"))
		got, owned, err := f.Open("K", scoped.NewOpener([]string{adminPEM}, false))
		Expect(err).NotTo(HaveOccurred())
		Expect(owned).To(BeTrue())
		Expect(got).To(Equal(want), "stdin %q", in)
	}
}

// A mistyped call fails without creating anything, .sc included.
func TestScopeCmd_InvalidNamesCreateNothing(t *testing.T) {
	RegisterTestingT(t)
	workdir := t.TempDir()
	for _, args := range [][]string{
		{"set", "--scope", "pr", "-s", "a/b", "K", "v"},
		{"delete", "--scope", "pr", "-s", "..", "K"},
		{"set", "--scope", "Bad Scope", "-s", "app", "K", "v"},
	} {
		_, err := execScope(t, workdir, "", args...)
		Expect(err).To(HaveOccurred(), "%v", args)
	}
	Expect(filepath.Join(workdir, ".sc")).NotTo(BeADirectory())
}

// set refuses a broken auth entry without echoing the start of the value.
func TestScopeCmd_SetBrokenAuthDoesNotEchoIt(t *testing.T) {
	RegisterTestingT(t)
	workdir, _, _ := newScopeRepo(t)
	out, err := execScope(t, workdir, "ghp_SUPERSECRETTOKEN\n", "set", "--scope", "pr", "-s", "app", "auth:gcloud", "-")
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("must be the YAML of one auth entry"))
	Expect(err.Error() + out).NotTo(ContainSubstring("ghp_SUP"))
}
