// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package cmd_secrets

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
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
		release, err := scoped.LockStore(filepath.Join(workdir, ".sc"))
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
