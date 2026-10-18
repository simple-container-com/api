// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

// Package scopedstore holds end-to-end tests for the scoped secrets store. They
// drive the same cobra command tree that cmd/sc builds (secrets scope ..., stack
// secret-get) against a temp git repo with generated SSH keys, and the public
// provisioner Deploy up to the Pulumi boundary, so a bug that only shows across
// components (CLI writes -> deploy-time read -> placeholder guard) is caught.
// No cloud access is needed.
package scopedstore

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/mock"
	"go.uber.org/atomic"
	"golang.org/x/crypto/ssh"
	"gopkg.in/yaml.v3"

	"github.com/simple-container-com/api/pkg/api"
	git_mocks "github.com/simple-container-com/api/pkg/api/git/mocks"
	"github.com/simple-container-com/api/pkg/api/secrets/ciphers"
	"github.com/simple-container-com/api/pkg/api/secrets/scoped"
	pulumi_mocks "github.com/simple-container-com/api/pkg/clouds/pulumi/mocks"
	"github.com/simple-container-com/api/pkg/cmd/cmd_secrets"
	"github.com/simple-container-com/api/pkg/cmd/cmd_stack"
	"github.com/simple-container-com/api/pkg/cmd/root_cmd"
	"github.com/simple-container-com/api/pkg/provisioner"
	"github.com/simple-container-com/api/pkg/provisioner/placeholders"
)

type key struct {
	pub string // authorized_keys line, no trailing newline
	pem string
}

func newEd25519(t *testing.T) key {
	t.Helper()
	priv, pub, err := ciphers.GenerateEd25519KeyPair()
	if err != nil {
		t.Fatal(err)
	}
	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	pem, err := ciphers.MarshalEd25519PrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	return key{pub: strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sshPub))), pem: pem}
}

func newRSA(t *testing.T) key {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	sshPub, err := ssh.NewPublicKey(&priv.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	return key{pub: strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sshPub))), pem: ciphers.MarshalRSAPrivateKey(priv)}
}

// adminEnv is a workstation / full-access identity: the key lives in the inline
// SIMPLE_CONTAINER_CONFIG, exactly as a CI job carries it.
func adminEnv(k key, extra map[string]string) map[string]string {
	cfg := api.ConfigFile{ProjectName: "myapp", PrivateKey: k.pem, PublicKey: k.pub}
	return withConfig(cfg, extra)
}

func withConfig(cfg api.ConfigFile, extra map[string]string) map[string]string {
	out, err := yaml.Marshal(cfg)
	if err != nil {
		panic(err)
	}
	env := map[string]string{api.ScConfigEnvVariable: string(out)}
	for k, v := range extra {
		env[k] = v
	}
	return env
}

// jobEnv is a narrow CI job: no key in the config, only a per-scope key variable.
func jobEnv(scopeVar string, k key) map[string]string {
	return map[string]string{
		api.ScConfigEnvVariable: "projectName: myapp\n",
		scopeVar:                k.pem,
	}
}

type repo struct {
	dir string
}

// newRepo makes a git repo with .sc/stacks/infra/server.yaml.
func newRepo(t *testing.T) *repo {
	t.Helper()
	dir := t.TempDir()
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		dir = resolved
	}
	if out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	r := &repo{dir: dir}
	r.write(t, ".sc/stacks/infra/server.yaml", serverYAML(""))
	return r
}

func (r *repo) path(rel string) string { return filepath.Join(r.dir, rel) }

func (r *repo) write(t *testing.T, rel, content string) {
	t.Helper()
	p := r.path(rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func (r *repo) stackFile(stack, name string) string {
	return r.path(filepath.Join(".sc", "stacks", stack, name))
}

// resetEnv drops everything that could leak a key from the developer's shell or an
// earlier call into this one.
func resetEnv(t *testing.T) {
	t.Helper()
	for _, e := range os.Environ() {
		name, _, _ := strings.Cut(e, "=")
		if name == api.ScConfigEnvVariable || name == "SC_SCOPE_KEY" || strings.HasPrefix(name, "SC_KEY_") {
			t.Setenv(name, "")
			_ = os.Unsetenv(name)
		}
	}
}

func setEnv(t *testing.T, env map[string]string) {
	t.Helper()
	resetEnv(t)
	for k, v := range env {
		t.Setenv(k, v)
	}
}

var stdoutMu sync.Mutex

// capture runs fn with os.Stdout redirected, since the logger and stack secret-get
// print there directly.
func capture(fn func()) string {
	stdoutMu.Lock()
	defer stdoutMu.Unlock()
	orig := os.Stdout
	rd, wr, err := os.Pipe()
	if err != nil {
		panic(err)
	}
	os.Stdout = wr
	done := make(chan string)
	go func() {
		b, _ := io.ReadAll(rd)
		done <- string(b)
	}()
	fn()
	_ = wr.Close()
	os.Stdout = orig
	return <-done
}

// buildRoot mirrors cmd/sc/main.go for the commands under test.
func buildRoot() *cobra.Command {
	params := &root_cmd.Params{IsCanceled: atomic.NewBool(false), CancelFunc: func() {}}
	rc := &root_cmd.RootCmd{Params: params}
	root := &cobra.Command{
		Use: "sc",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if cmd.Name() != "init" {
				return rc.Init(root_cmd.IgnoreAllErrors)
			}
			return nil
		},
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(cmd_secrets.NewSecretsCmd(rc), cmd_stack.NewStackCmd(rc))
	root.PersistentFlags().BoolVarP(&params.Verbose, "verbose", "v", false, "Verbose mode")
	root.PersistentFlags().StringVarP(&params.Profile, "profile", "p", "", "Use profile")
	return root
}

// sc runs one `sc ...` invocation in the repo with the given environment and returns
// everything it printed (cobra output and stdout) plus the command error.
func (r *repo) sc(t *testing.T, env map[string]string, stdin string, args ...string) (string, error) {
	t.Helper()
	t.Chdir(r.dir)
	setEnv(t, env)
	root := buildRoot()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetIn(strings.NewReader(stdin))
	root.SetArgs(args)
	var err error
	printed := capture(func() { err = root.ExecuteContext(context.Background()) })
	return buf.String() + printed, err
}

func (r *repo) mustSC(t *testing.T, env map[string]string, stdin string, args ...string) string {
	t.Helper()
	out, err := r.sc(t, env, stdin, args...)
	if err != nil {
		t.Fatalf("sc %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return out
}

func lastLine(out string) string {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

func (r *repo) allow(t *testing.T, admin map[string]string, scope string, k key) {
	t.Helper()
	r.mustSC(t, admin, "", "secrets", "scope", "allow", "--scope", scope, k.pub)
}

func (r *repo) set(t *testing.T, admin map[string]string, scope, name, value string) {
	t.Helper()
	r.mustSC(t, admin, "", "secrets", "scope", "set", "--scope", scope, "-s", "infra", name, value)
}

// secretGet is the deploy-time read path: stack secret-get goes through
// provisioner.GetStack -> ReadStacks -> readSecretsDescriptorFromFile.
func (r *repo) secretGet(t *testing.T, env map[string]string, name string) (string, error) {
	t.Helper()
	out, err := r.sc(t, env, "", "stack", "secret-get", "-s", "infra", name)
	return lastLine(out), err
}

func (r *repo) mustSecretGet(t *testing.T, env map[string]string, name string) string {
	t.Helper()
	v, err := r.secretGet(t, env, name)
	if err != nil {
		t.Fatalf("secret-get %s: %v", name, err)
	}
	return v
}

// scopes.yaml is a reviewed file: allow and disallow change the recipient lines
// they are asked to, and nothing else in it.
func TestScopesYAMLEditsKeepTheReviewedFile(t *testing.T) {
	admin, pr := newEd25519(t), newRSA(t)
	r := newRepo(t)
	ae := adminEnv(admin, nil)
	r.allow(t, ae, "pr", admin)
	r.set(t, ae, "pr", "DD_TOKEN", "dd-123")

	reviewed := "# Which keys open each scope. Reviewed by the platform team.\n" +
		"schemaVersion: 1\n" +
		"scopes:\n" +
		"  # pull request scans\n" +
		"  pr:\n" +
		"    description: scans\n" +
		"    recipients:\n" +
		"      - " + admin.pub + " # break-glass\n"
	r.write(t, ".sc/scopes.yaml", reviewed)

	r.allow(t, ae, "pr", pr)
	data, err := os.ReadFile(r.path(".sc/scopes.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if want := reviewed + "      - " + pr.pub + "\n"; string(data) != want {
		t.Fatalf("allow rewrote scopes.yaml:\n--- got\n%s--- want\n%s", data, want)
	}
	if got := lastLine(r.mustSC(t, jobEnv("SC_KEY_PR", pr), "", "secrets", "scope", "get", "--scope", "pr", "-s", "infra", "DD_TOKEN")); got != "dd-123" {
		t.Errorf("added recipient reads %q", got)
	}

	r.mustSC(t, ae, "", "secrets", "scope", "disallow", "--scope", "pr", pr.pub)
	data, err = os.ReadFile(r.path(".sc/scopes.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != reviewed {
		t.Fatalf("disallow did not restore the reviewed file:\n--- got\n%s--- want\n%s", data, reviewed)
	}
	if out := r.mustSC(t, ae, "", "secrets", "scope", "lint"); !strings.Contains(out, "scope file(s) OK") {
		t.Errorf("lint after the edits: %s", out)
	}
}

func TestScopedStoreE2E(t *testing.T) {
	t.Run("1 admin creates a scope and both keys open it", func(t *testing.T) {
		admin, pr := newEd25519(t), newRSA(t)
		r := newRepo(t)
		ae := adminEnv(admin, nil)
		r.allow(t, ae, "pr", admin)
		r.allow(t, ae, "pr", pr)
		r.allow(t, ae, "prod", admin)
		r.set(t, ae, "pr", "DD_TOKEN", "dd-123")
		r.set(t, ae, "prod", "PROD_TOKEN", "prod-456")

		get := []string{"secrets", "scope", "get", "--scope", "pr", "-s", "infra", "DD_TOKEN"}
		if got := lastLine(r.mustSC(t, ae, "", get...)); got != "dd-123" {
			t.Errorf("admin get = %q", got)
		}
		prEnv := jobEnv("SC_KEY_PR", pr)
		if got := lastLine(r.mustSC(t, prEnv, "", get...)); got != "dd-123" {
			t.Errorf("pr key get via SC_KEY_PR = %q", got)
		}
		keyFile := filepath.Join(t.TempDir(), "pr.pem")
		if err := os.WriteFile(keyFile, []byte(pr.pem), 0o600); err != nil {
			t.Fatal(err)
		}
		fileEnv := map[string]string{api.ScConfigEnvVariable: "projectName: myapp\n"}
		if got := lastLine(r.mustSC(t, fileEnv, "", append(get, "--key-file", keyFile)...)); got != "dd-123" {
			t.Errorf("pr key get via --key-file = %q", got)
		}

		if out := r.mustSC(t, ae, "", "secrets", "scope", "lint"); !strings.Contains(out, "2 scope file(s) OK") {
			t.Errorf("lint output = %q", out)
		}

		doctor := func(env map[string]string, args ...string) map[string]string {
			out := r.mustSC(t, env, "", append([]string{"secrets", "scope", "doctor"}, args...)...)
			status := map[string]string{}
			for _, m := range regexp.MustCompile(`(?m)^(\S+)\s+scope=(\S+)`).FindAllStringSubmatch(out, -1) {
				status[m[2]] = m[1]
			}
			return status
		}
		got := doctor(fileEnv, "--key-file", keyFile)
		if got["pr"] != "YES" || got["prod"] != "no" || len(got) != 2 {
			t.Errorf("doctor with the pr key = %v, want pr=YES prod=no", got)
		}
		got = doctor(map[string]string{api.ScConfigEnvVariable: "projectName: myapp\n", "SC_SCOPE_KEY": pr.pem})
		if got["pr"] != "YES" || got["prod"] != "no" {
			t.Errorf("doctor with SC_SCOPE_KEY = %v, want pr=YES prod=no", got)
		}
		got = doctor(map[string]string{api.ScConfigEnvVariable: "projectName: myapp\n", "SC_KEY_PR": pr.pem})
		if got["pr"] != "YES" || got["prod"] != "no" {
			t.Errorf("doctor with SC_KEY_PR = %v, want pr=YES prod=no", got)
		}
		got = doctor(ae)
		if got["pr"] != "YES" || got["prod"] != "YES" {
			t.Errorf("doctor with the admin key = %v, want both YES", got)
		}
		if _, err := r.sc(t, map[string]string{api.ScConfigEnvVariable: "projectName: myapp\n", "SC_SCOPE_KEY": pr.pem}, "", "secrets", "scope", "get", "--scope", "prod", "-s", "infra", "PROD_TOKEN"); err == nil ||
			!strings.Contains(err.Error(), "not a recipient") {
			t.Errorf("pr key reading prod: err = %v, want not a recipient", err)
		}
	})

	t.Run("2 least privilege: the pr job resolves pr values and not prod", func(t *testing.T) {
		admin, pr := newRSA(t), newEd25519(t)
		r := newRepo(t)
		ae := adminEnv(admin, nil)
		r.allow(t, ae, "pr", admin)
		r.allow(t, ae, "pr", pr)
		r.allow(t, ae, "prod", admin)
		r.set(t, ae, "pr", "PR_TOKEN", "pr-token")
		r.set(t, ae, "prod", "PROD_TOKEN", "prod-token")

		prEnv := jobEnv("SC_KEY_PR", pr)
		if got := r.mustSecretGet(t, prEnv, "PR_TOKEN"); got != "pr-token" {
			t.Errorf("PR_TOKEN = %q", got)
		}
		_, err := r.secretGet(t, prEnv, "PROD_TOKEN")
		if err == nil || !strings.Contains(err.Error(), `secret "PROD_TOKEN" not found in stack`) {
			t.Errorf("PROD_TOKEN for the pr job: err = %v, want a plain not-found", err)
		}
		if errors.Is(err, scoped.ErrScopedIntegrity) || errors.Is(err, scoped.ErrScopedUnavailable) {
			t.Errorf("reading past the prod scope must not be an integrity or availability error: %v", err)
		}

		for name, want := range map[string]string{"PR_TOKEN": "pr-token", "PROD_TOKEN": "prod-token"} {
			if got := r.mustSecretGet(t, ae, name); got != want {
				t.Errorf("admin %s = %q, want %q", name, got, want)
			}
		}
	})

	t.Run("3 legacy secrets.yaml wins over a scope and the shadowing is reported", func(t *testing.T) {
		admin := newEd25519(t)
		r := newRepo(t)
		ae := adminEnv(admin, nil)
		r.allow(t, ae, "pr", admin)
		r.set(t, ae, "pr", "KEY", "b")
		r.write(t, ".sc/stacks/infra/secrets.yaml", "schemaVersion: 1.0\nvalues:\n  KEY: a\n")

		out, err := r.sc(t, ae, "", "stack", "secret-get", "-s", "infra", "KEY")
		if err != nil {
			t.Fatal(err, out)
		}
		if got := lastLine(out); got != "a" {
			t.Errorf("deploy-time value = %q, want the legacy %q", got, "a")
		}
		if !strings.Contains(out, "WARN") || !strings.Contains(out, `scoped secret "KEY" is shadowed by the legacy secrets.yaml`) {
			t.Errorf("no shadowing warning in output:\n%s", out)
		}

		out, err = r.sc(t, ae, "", "secrets", "scope", "lint")
		if err == nil || !strings.Contains(err.Error(), "scoped secrets lint failed") {
			t.Errorf("lint without the flag: err = %v", err)
		}
		if !strings.Contains(out, `key "KEY" is in both scope "pr" and the legacy secrets.yaml`) {
			t.Errorf("lint output = %q", out)
		}
		out, err = r.sc(t, ae, "", "secrets", "scope", "lint", "--allow-legacy-duplicates")
		if err != nil {
			t.Errorf("lint with --allow-legacy-duplicates: %v\n%s", err, out)
		}
		if !strings.Contains(out, "legacy secrets.yaml") {
			t.Errorf("duplicate should still be reported as a warning: %q", out)
		}
	})

	t.Run("4 tamper: a recipient fails closed, a non-recipient skips, a renamed file is rejected", func(t *testing.T) {
		admin, pr, other := newEd25519(t), newRSA(t), newEd25519(t)
		r := newRepo(t)
		ae := adminEnv(admin, nil)
		for _, k := range []key{admin, pr} {
			r.allow(t, ae, "pr", k)
		}
		r.allow(t, ae, "other", admin)
		r.allow(t, ae, "other", other)
		r.set(t, ae, "pr", "TOK", "pr-value")
		r.set(t, ae, "other", "OTHER_TOK", "other-value")

		path := r.stackFile("infra", scoped.ScopeFileName("pr"))
		f, err := scoped.LoadScopeFile(path)
		if err != nil {
			t.Fatal(err)
		}
		ev := f.Values["TOK"]
		blob, err := base64.StdEncoding.DecodeString(ev.Ciphertext)
		if err != nil {
			t.Fatal(err)
		}
		blob[len(blob)-1] ^= 0x01
		ev.Ciphertext = base64.StdEncoding.EncodeToString(blob)
		f.Values["TOK"] = ev
		if err := f.Save(path); err != nil {
			t.Fatal(err)
		}

		_, err = r.secretGet(t, jobEnv("SC_KEY_PR", pr), "TOK")
		if !errors.Is(err, scoped.ErrScopedIntegrity) {
			t.Errorf("recipient of a tampered value: err = %v, want ErrScopedIntegrity", err)
		}
		_, err = r.secretGet(t, ae, "OTHER_TOK")
		if !errors.Is(err, scoped.ErrScopedIntegrity) {
			t.Errorf("admin (recipient of both scopes): err = %v, want ErrScopedIntegrity", err)
		}
		got, err := r.secretGet(t, jobEnv("SC_KEY_OTHER", other), "OTHER_TOK")
		if err != nil || got != "other-value" {
			t.Errorf("non-recipient of the tampered scope: got %q, err %v; want its own scope, no error", got, err)
		}

		r2 := newRepo(t)
		r2.allow(t, ae, "pr", admin)
		r2.allow(t, ae, "pr", pr)
		r2.set(t, ae, "pr", "TOK", "pr-value")
		if err := os.Rename(r2.stackFile("infra", "secrets.pr.yaml"), r2.stackFile("infra", "secrets.prod.yaml")); err != nil {
			t.Fatal(err)
		}
		_, err = r2.secretGet(t, jobEnv("SC_KEY_PR", pr), "TOK")
		if !errors.Is(err, scoped.ErrScopedIntegrity) {
			t.Errorf("renamed scope file: err = %v, want ErrScopedIntegrity", err)
		}
		if err == nil || !strings.Contains(err.Error(), "renamed file") {
			t.Errorf("renamed scope file: error should say so: %v", err)
		}
		if out, lerr := r2.sc(t, ae, "", "secrets", "scope", "lint"); lerr == nil {
			t.Errorf("lint accepted a renamed scope file:\n%s", out)
		}
	})

	t.Run("5 disallow cuts the removed key off and keeps the admin", func(t *testing.T) {
		admin, pr := newEd25519(t), newRSA(t)
		r := newRepo(t)
		ae := adminEnv(admin, nil)
		prEnv := jobEnv("SC_KEY_PR", pr)
		r.allow(t, ae, "pr", admin)
		r.allow(t, ae, "pr", pr)
		r.set(t, ae, "pr", "K", "v")
		if got := r.mustSecretGet(t, prEnv, "K"); got != "v" {
			t.Fatalf("pr key before disallow = %q", got)
		}

		out := r.mustSC(t, ae, "", "secrets", "scope", "disallow", "--scope", "pr", pr.pub)
		if !strings.Contains(out, "Rotate every value") {
			t.Errorf("disallow gave no rotate warning: %q", out)
		}
		_, err := r.secretGet(t, prEnv, "K")
		if !errors.Is(err, os.ErrNotExist) || errors.Is(err, scoped.ErrScopedIntegrity) {
			t.Errorf("removed key: err = %v, want nothing resolved (not-exist), not an integrity error", err)
		}
		if _, err := r.sc(t, prEnv, "", "secrets", "scope", "get", "--scope", "pr", "-s", "infra", "K"); err == nil ||
			!strings.Contains(err.Error(), "not a recipient") {
			t.Errorf("scope get with the removed key: err = %v", err)
		}
		if got := r.mustSecretGet(t, ae, "K"); got != "v" {
			t.Errorf("admin after disallow = %q", got)
		}
		if out := r.mustSC(t, ae, "", "secrets", "scope", "lint"); !strings.Contains(out, "OK") {
			t.Errorf("lint after disallow: %q", out)
		}
	})

	t.Run("6 an unresolved placeholder fails the deploy before Pulumi", func(t *testing.T) {
		admin, deployer := newEd25519(t), newRSA(t)
		r := newDeployRepo(t, map[string]string{"app": clientYAML("app", "staging", "infra", "${secret:APP_KEY}", "${secret:MISSING}")})
		ae := adminEnv(admin, nil)
		r.allow(t, ae, "app-staging", admin)
		r.allow(t, ae, "app-staging", deployer)
		r.mustSC(t, ae, ambientGcloudAuth, "secrets", "scope", "set", "--scope", "app-staging", "-s", "infra", "auth:gcloud", "-")
		r.set(t, ae, "app-staging", "APP_KEY", "s3cr3t")

		de := jobEnv("SC_KEY_APP_STAGING", deployer)
		stack, log, err := r.deploy(t, de, "app", "staging")
		if !errors.Is(err, provisioner.ErrUnresolvedPlaceholders) {
			t.Fatalf("err = %v, want ErrUnresolvedPlaceholders", err)
		}
		if !strings.Contains(err.Error(), "${secret:MISSING}") || strings.Contains(err.Error(), "APP_KEY") {
			t.Errorf("error should name only the missing placeholder: %v", err)
		}
		if stack != nil {
			t.Error("the deploy reached Pulumi with an unresolved placeholder")
		}
		_ = log

		r.set(t, ae, "app-staging", "MISSING", "now-present")
		stack, _, err = r.deploy(t, de, "app", "staging")
		if err != nil {
			t.Fatalf("after the secret was added: %v", err)
		}
		if env := composeEnv(t, stack, "staging"); env["APP_KEY"] != "s3cr3t" || env["OTHER"] != "now-present" {
			t.Errorf("resolved env = %v", env)
		}
	})

	t.Run("6b without scopes an unresolved placeholder only warns", func(t *testing.T) {
		r := newDeployRepo(t, map[string]string{"app": clientYAML("app", "staging", "infra", "${secret:APP_KEY}", "${secret:MISSING}")})
		r.write(t, ".sc/stacks/infra/secrets.yaml", "schemaVersion: 1.0\nauth:\n  gcloud:\n    type: gcp-service-account\n    config:\n      projectId: p\n      credentials: '{\"type\":\"service_account\"}'\nvalues:\n  APP_KEY: v\n")
		stack, log, err := r.deploy(t, map[string]string{api.ScConfigEnvVariable: "projectName: myapp\n"}, "app", "staging")
		if err != nil || stack == nil {
			t.Fatalf("legacy-only deploy: stack %v, err %v", stack != nil, err)
		}
		if !strings.Contains(strings.Join(log.warns, "\n"), "${secret:MISSING}") {
			t.Errorf("warnings = %v", log.warns)
		}
	})

	t.Run("8 a plaintext secrets.example.yaml is ignored everywhere", func(t *testing.T) {
		admin := newEd25519(t)
		r := newRepo(t)
		ae := adminEnv(admin, nil)
		r.write(t, ".sc/stacks/infra/secrets.yaml", "schemaVersion: 1.0\nvalues:\n  REAL: real-value\n")
		r.write(t, ".sc/stacks/infra/secrets.example.yaml", "schemaVersion: 1.0\nvalues:\n  REAL: changeme\n  EXAMPLE_ONLY: changeme\n")

		if got := r.mustSecretGet(t, ae, "REAL"); got != "real-value" {
			t.Errorf("REAL = %q, want the legacy value", got)
		}
		if _, err := r.secretGet(t, ae, "EXAMPLE_ONLY"); err == nil || !strings.Contains(err.Error(), `secret "EXAMPLE_ONLY" not found`) {
			t.Errorf("a value from secrets.example.yaml must not resolve: err = %v", err)
		}
		out, err := r.sc(t, ae, "", "secrets", "scope", "lint")
		if err != nil {
			t.Errorf("lint must not fail on an example file: %v\n%s", err, out)
		}
		if strings.Contains(out, "not valid") || strings.Contains(out, "example") && strings.Contains(out, "✗") {
			t.Errorf("lint treated the example file as a scope: %q", out)
		}
	})

	t.Run("9 a custom stacksDir is honoured by scope set and the deploy read", func(t *testing.T) {
		admin := newEd25519(t)
		r := newRepo(t)
		r.write(t, "deploy/stacks/infra/server.yaml", serverYAML(""))
		cfg := api.ConfigFile{ProjectName: "myapp", PrivateKey: admin.pem, PublicKey: admin.pub, StacksDir: "deploy/stacks"}
		ce := withConfig(cfg, nil)
		r.allow(t, ce, "pr", admin)
		r.set(t, ce, "pr", "K", "v")

		if _, err := os.Stat(r.path("deploy/stacks/infra/secrets.pr.yaml")); err != nil {
			t.Errorf("scope file not written under the configured stacksDir: %v", err)
		}
		if _, err := os.Stat(r.path(".sc/stacks/infra/secrets.pr.yaml")); err == nil {
			t.Error("scope file was also written under the default .sc/stacks")
		}
		if got := r.mustSecretGet(t, ce, "K"); got != "v" {
			t.Errorf("deploy read = %q", got)
		}
		if out := r.mustSC(t, ce, "", "secrets", "scope", "lint"); !strings.Contains(out, "1 scope file(s) OK") {
			t.Errorf("lint = %q", out)
		}
	})

	t.Run("10 a KMS-only scope without AWS credentials is skipped", func(t *testing.T) {
		for _, e := range os.Environ() {
			if name, _, _ := strings.Cut(e, "="); strings.HasPrefix(name, "AWS_") {
				t.Setenv(name, "")
				_ = os.Unsetenv(name)
			}
		}
		t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
		t.Setenv("AWS_CONFIG_FILE", filepath.Join(t.TempDir(), "none"))
		t.Setenv("AWS_SHARED_CREDENTIALS_FILE", filepath.Join(t.TempDir(), "none"))

		admin, pr := newEd25519(t), newRSA(t)
		r := newRepo(t)
		ae := adminEnv(admin, nil)
		r.allow(t, ae, "pr", admin)
		r.allow(t, ae, "pr", pr)
		r.set(t, ae, "pr", "PR_TOKEN", "pr-token")

		const kmsRecipient = "awskms://alias/x?region=us-east-1"
		blob := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x42}, 96))
		kmsFile := scoped.ScopeFile{
			SchemaVersion: scoped.CurrentScopesSchemaVersion,
			Stack:         "infra",
			Scope:         "kms",
			Recipients:    []string{kmsRecipient},
			Values: map[string]scoped.EncryptedValue{
				"KMS_TOKEN": {Ciphertext: blob, Wraps: map[string][]string{kmsRecipient: {blob}}},
			},
		}
		if err := kmsFile.Save(r.stackFile("infra", scoped.ScopeFileName("kms"))); err != nil {
			t.Fatal(err)
		}

		prEnv := jobEnv("SC_KEY_PR", pr)
		prEnv["AWS_EC2_METADATA_DISABLED"] = "true"
		got, err := r.secretGet(t, prEnv, "PR_TOKEN")
		if err != nil || got != "pr-token" {
			t.Errorf("own scope next to an unreachable KMS scope: got %q, err %v", got, err)
		}
		if _, err := r.secretGet(t, prEnv, "KMS_TOKEN"); err == nil || !strings.Contains(err.Error(), `secret "KMS_TOKEN" not found`) {
			t.Errorf("KMS value must be skipped, not an error about KMS: %v", err)
		}
	})

	t.Run("11 each client deploy needs only its own secrets", func(t *testing.T) {
		admin, web, batch := newEd25519(t), newRSA(t), newEd25519(t)
		r := newDeployRepo(t, map[string]string{
			"web":   clientYAML("web", "staging", "infra", "${secret:WEB_KEY}", "literal"),
			"batch": clientYAML("batch", "staging", "infra", "literal", "literal") + "      uses: [batch-bucket]\n",
		})
		r.write(t, ".sc/stacks/infra/server.yaml", serverYAML(`
        batch-bucket:
          type: gcp-bucket
          config:
            projectId: "${auth:gcloud.projectId}"
            credentials: "${auth:gcloud}"
            name: "${secret:BATCH_TOKEN}"`))
		ae := adminEnv(admin, nil)
		for scope, k := range map[string]key{"web": web, "batch": batch} {
			r.allow(t, ae, scope, admin)
			r.allow(t, ae, scope, k)
			r.mustSC(t, ae, ambientGcloudAuth, "secrets", "scope", "set", "--scope", scope, "-s", "infra", "auth:gcloud", "-")
		}
		r.set(t, ae, "web", "WEB_KEY", "web-secret")
		r.set(t, ae, "batch", "BATCH_TOKEN", "batch-secret")

		if _, _, err := r.deploy(t, jobEnv("SC_KEY_WEB", web), "web", "staging"); err != nil {
			t.Errorf("web deployer: %v, want the placeholder check to pass", err)
		}
		if _, _, err := r.deploy(t, jobEnv("SC_KEY_BATCH", batch), "batch", "staging"); err != nil {
			t.Errorf("batch deployer: %v", err)
		}
		_, _, err := r.deploy(t, jobEnv("SC_KEY_WEB", web), "batch", "staging")
		if !errors.Is(err, provisioner.ErrUnresolvedPlaceholders) || !strings.Contains(err.Error(), "${secret:BATCH_TOKEN}") {
			t.Errorf("batch deployed with the web key: err = %v, want ErrUnresolvedPlaceholders naming BATCH_TOKEN", err)
		}
	})
}

const ambientGcloudAuth = "type: gcp-service-account\nconfig:\n  projectId: acme-staging\n  credentials: \"\"\n"

func serverYAML(stagingResources string) string {
	if strings.TrimSpace(stagingResources) == "" {
		stagingResources = " {}"
	}
	return `schemaVersion: 1.0
provisioner:
  type: pulumi
  config:
    state-storage:
      type: gcp-bucket
      config:
        credentials: "${auth:gcloud}"
        projectId: "${auth:gcloud.projectId}"
        bucketName: state
    secrets-provider:
      type: gcp-kms
      config:
        projectId: "${auth:gcloud.projectId}"
        keyName: sc-state
        keyLocation: global
        credentials: "${auth:gcloud}"
templates:
  stack-per-app:
    type: cloudrun
    config:
      projectId: "${auth:gcloud.projectId}"
      credentials: "${auth:gcloud}"
resources:
  resources:
    staging:
      template: stack-per-app
      resources:` + stagingResources + "\n"
}

func clientYAML(name, env, parent, appKey, other string) string {
	return fmt.Sprintf(`schemaVersion: 1.0
stacks:
  %s:
    type: cloud-compose
    parent: acme/%s
    config:
      dockerComposeFile: docker-compose.yaml
      runs: [api]
      env:
        APP_KEY: %q
        OTHER: %q
`, env, parent, appKey, other)
}

type captureLogger struct {
	mu    sync.Mutex
	warns []string
}

func (l *captureLogger) Error(context.Context, string, ...any) {}
func (l *captureLogger) Warn(_ context.Context, f string, a ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.warns = append(l.warns, fmt.Sprintf(f, a...))
}
func (l *captureLogger) Info(context.Context, string, ...any)                   {}
func (l *captureLogger) Debug(context.Context, string, ...any)                  {}
func (l *captureLogger) SetLogLevel(ctx context.Context, _ int) context.Context { return ctx }
func (l *captureLogger) Silent(ctx context.Context) context.Context             { return ctx }

// newDeployRepo is newRepo plus a parent stack "infra" with a deployable server.yaml
// and the given client stacks (name -> client.yaml).
func newDeployRepo(t *testing.T, clients map[string]string) *repo {
	t.Helper()
	r := newRepo(t)
	r.write(t, ".sc/cfg.default.yaml", "projectName: myapp\n")
	r.write(t, ".sc/stacks/infra/server.yaml", serverYAML(""))
	for name, yml := range clients {
		r.write(t, ".sc/stacks/"+name+"/client.yaml", yml)
		r.write(t, ".sc/stacks/"+name+"/docker-compose.yaml", "services:\n  api:\n    image: nginx\n")
	}
	return r
}

// deploy runs the public provisioner Deploy up to the Pulumi boundary and returns the
// stack that would have been handed to Pulumi (nil if the deploy never got there).
func (r *repo) deploy(t *testing.T, env map[string]string, stackName, environment string) (*api.Stack, *captureLogger, error) {
	t.Helper()
	t.Chdir(r.dir)
	setEnv(t, env)
	git := git_mocks.NewGitRepoMock(t)
	git.On("Workdir").Return(r.dir).Maybe()
	git.On("Hash").Return("abc123", nil).Maybe()
	git.On("Branch").Return("main", nil).Maybe()
	pm := pulumi_mocks.NewPulumiMock(t)
	var got *api.Stack
	pm.On("SetPublicKey", mock.Anything).Return().Maybe()
	pm.On("DeployStack", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			s := args.Get(2).(api.Stack)
			got = &s
		}).Return(nil).Maybe()
	log := &captureLogger{}
	p, err := provisioner.New(
		provisioner.WithPlaceholders(placeholders.New(placeholders.WithGitRepo(git))),
		provisioner.WithOverrideProvisioner(pm),
		provisioner.WithGitRepo(git),
		provisioner.WithLogger(log),
	)
	if err != nil {
		t.Fatal(err)
	}
	err = p.Deploy(context.Background(), api.DeployParams{StackParams: api.StackParams{
		StacksDir: ".sc/stacks", StackName: stackName, Environment: environment,
	}})
	return got, log, err
}

func composeEnv(t *testing.T, s *api.Stack, env string) map[string]string {
	t.Helper()
	if s == nil {
		t.Fatal("the deploy never reached Pulumi")
	}
	cfg, ok := s.Client.Stacks[env].Config.Config.(*api.StackConfigCompose)
	if !ok {
		t.Fatalf("client config is %T", s.Client.Stacks[env].Config.Config)
	}
	return cfg.Env
}
