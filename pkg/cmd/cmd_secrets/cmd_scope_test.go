// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package cmd_secrets

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/secrets"
	"github.com/simple-container-com/api/pkg/api/secrets/ciphers"
	"github.com/simple-container-com/api/pkg/api/secrets/scoped"
	"github.com/simple-container-com/api/pkg/cmd/root_cmd"
	"github.com/simple-container-com/api/pkg/provisioner"
	"github.com/simple-container-com/api/pkg/provisioner/placeholders"
)

// execScope drives the real `secrets scope` command tree against a temp workdir.
// A fresh command tree is built per call so cobra flag state does not leak.
func execScope(t *testing.T, workdir, stdin string, args ...string) (string, error) {
	t.Helper()
	g := NewWithT(t)
	cryptor, err := secrets.NewCryptor(workdir)
	g.Expect(err).NotTo(HaveOccurred())
	p, err := provisioner.New(provisioner.WithCryptor(cryptor), provisioner.WithPlaceholders(placeholders.New()))
	g.Expect(err).NotTo(HaveOccurred())
	cmd := NewScopeCmd(&secretsCmd{Root: &root_cmd.RootCmd{Provisioner: p}})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(strings.NewReader(stdin))
	cmd.SetArgs(args)
	execErr := cmd.Execute()
	return out.String(), execErr
}

func testRecipient(t *testing.T) (authorized, privatePEM string) {
	t.Helper()
	g := NewWithT(t)
	priv, pub, err := ciphers.GenerateEd25519KeyPair()
	g.Expect(err).NotTo(HaveOccurred())
	sshPub, err := ssh.NewPublicKey(pub)
	g.Expect(err).NotTo(HaveOccurred())
	pem, err := ciphers.MarshalEd25519PrivateKey(priv)
	g.Expect(err).NotTo(HaveOccurred())
	return string(ssh.MarshalAuthorizedKey(sshPub)), pem
}

func TestScopeCmd_SetGetListLintDoctor(t *testing.T) {
	RegisterTestingT(t)
	workdir := t.TempDir()
	authorized, keyPEM := testRecipient(t)

	// allow (no key needed — no files yet)
	out, err := execScope(t, workdir, "", "allow", "--scope", "pr", authorized)
	Expect(err).NotTo(HaveOccurred(), out)

	// set (arg value + stdin value)
	_, err = execScope(t, workdir, "", "set", "--scope", "pr", "-s", "integrail", "defectdojo-api-key", "dd-123")
	Expect(err).NotTo(HaveOccurred())
	_, err = execScope(t, workdir, "stdin-secret", "set", "--scope", "pr", "-s", "integrail", "cf-token", "-")
	Expect(err).NotTo(HaveOccurred())

	// list
	out, _ = execScope(t, workdir, "", "list", "--scope", "pr", "-s", "integrail")
	Expect(out).To(ContainSubstring("defectdojo-api-key"))
	Expect(out).To(ContainSubstring("cf-token"))

	// get via SC_SCOPE_KEY env (the CI path, no ambient config)
	t.Setenv("SC_SCOPE_KEY", keyPEM)
	out, err = execScope(t, workdir, "", "get", "--scope", "pr", "-s", "integrail", "defectdojo-api-key")
	Expect(err).NotTo(HaveOccurred())
	Expect(strings.TrimSpace(out)).To(Equal("dd-123"))
	out, _ = execScope(t, workdir, "", "get", "--scope", "pr", "-s", "integrail", "cf-token")
	Expect(strings.TrimSpace(out)).To(Equal("stdin-secret"))

	// lint clean
	out, err = execScope(t, workdir, "", "lint")
	Expect(err).NotTo(HaveOccurred(), out)
	Expect(out).To(ContainSubstring("OK"))

	// doctor: openable with the scope key
	out, _ = execScope(t, workdir, "", "doctor")
	Expect(out).To(ContainSubstring("YES"))
}

func TestScopeCmd_AuthEntry(t *testing.T) {
	RegisterTestingT(t)
	workdir := t.TempDir()
	authorized, keyPEM := testRecipient(t)
	_, err := execScope(t, workdir, "", "allow", "--scope", "app-staging", authorized)
	Expect(err).NotTo(HaveOccurred())

	auth := "type: gcp-service-account\nconfig:\n  projectId: acme-staging\n  credentials: \"\"\n"
	out, err := execScope(t, workdir, auth, "set", "--scope", "app-staging", "-s", "infra", "auth:gcloud", "-")
	Expect(err).NotTo(HaveOccurred(), out)

	for _, bad := range []string{"not: [yaml", "config:\n  projectId: p\n", "type: no-such-auth\nconfig: {}\n", "inherit: common\n"} {
		_, err := execScope(t, workdir, bad, "set", "--scope", "app-staging", "-s", "infra", "auth:broken", "-")
		Expect(err).To(HaveOccurred(), "accepted auth entry %q", bad)
	}
	_, err = execScope(t, workdir, auth, "set", "--scope", "app-staging", "-s", "infra", "auth:bad name", "-")
	Expect(err).To(HaveOccurred())

	out, _ = execScope(t, workdir, "", "list", "--scope", "app-staging", "-s", "infra")
	Expect(out).To(ContainSubstring("auth:gcloud"))
	Expect(out).NotTo(ContainSubstring("auth:broken"))

	t.Setenv("SC_SCOPE_KEY", keyPEM)
	out, err = execScope(t, workdir, "", "get", "--scope", "app-staging", "-s", "infra", "auth:gcloud")
	Expect(err).NotTo(HaveOccurred())
	Expect(out).To(ContainSubstring("acme-staging"))
	out, err = execScope(t, workdir, "", "lint")
	Expect(err).NotTo(HaveOccurred(), out)
}

// A key in several scopes is how a shared secret reaches each client's scope, so
// only differing copies are ambiguous. lint compares them when it can open them
// all, and says it could not otherwise.
func TestScopeCmd_LintCatchesCrossScopeDuplicate(t *testing.T) {
	RegisterTestingT(t)
	workdir := t.TempDir()
	authorized, keyPEM := testRecipient(t)

	for _, scope := range []string{"pr", "prod"} {
		_, err := execScope(t, workdir, "", "allow", "--scope", scope, authorized)
		Expect(err).NotTo(HaveOccurred())
	}
	_, err := execScope(t, workdir, "", "set", "--scope", "pr", "-s", "integrail", "shared", "a")
	Expect(err).NotTo(HaveOccurred())
	_, err = execScope(t, workdir, "", "set", "--scope", "prod", "-s", "integrail", "shared", "b")
	Expect(err).NotTo(HaveOccurred())

	// Without a key the copies cannot be compared: a warning, not a failure.
	out, err := execScope(t, workdir, "", "lint")
	Expect(err).NotTo(HaveOccurred(), out)
	Expect(out).To(ContainSubstring("could not compare"))

	// With one, differing copies fail...
	t.Setenv("SC_SCOPE_KEY", keyPEM)
	out, err = execScope(t, workdir, "", "lint")
	Expect(err).To(HaveOccurred())
	Expect(out).To(ContainSubstring("different values"))

	// ...and equal ones pass.
	_, err = execScope(t, workdir, "", "set", "--scope", "prod", "-s", "integrail", "shared", "a")
	Expect(err).NotTo(HaveOccurred())
	out, err = execScope(t, workdir, "", "lint")
	Expect(err).NotTo(HaveOccurred(), out)
	Expect(out).NotTo(ContainSubstring("shared"))
}

func TestScopeCmd_LintLegacyDuplicates(t *testing.T) {
	RegisterTestingT(t)
	workdir := t.TempDir()
	authorized, _ := testRecipient(t)
	_, err := execScope(t, workdir, "", "allow", "--scope", "app-staging", authorized)
	Expect(err).NotTo(HaveOccurred())
	_, err = execScope(t, workdir, "", "set", "--scope", "app-staging", "-s", "infra", "DNS_TOKEN", "t")
	Expect(err).NotTo(HaveOccurred())
	legacy := filepath.Join(workdir, ".sc", "stacks", "infra", "secrets.yaml")
	Expect(os.WriteFile(legacy, []byte("values:\n  DNS_TOKEN: t\n"), 0o600)).To(Succeed())

	out, err := execScope(t, workdir, "", "lint")
	Expect(err).To(HaveOccurred())
	Expect(out).To(ContainSubstring("legacy secrets.yaml"))

	out, err = execScope(t, workdir, "", "lint", "--allow-legacy-duplicates")
	Expect(err).NotTo(HaveOccurred(), out)
	Expect(out).To(ContainSubstring("legacy secrets.yaml"))
}

func TestScopeCmd_DisallowLastRecipientRefused(t *testing.T) {
	RegisterTestingT(t)
	workdir := t.TempDir()
	authorized, _ := testRecipient(t)

	_, err := execScope(t, workdir, "", "allow", "--scope", "pr", authorized)
	Expect(err).NotTo(HaveOccurred())
	_, err = execScope(t, workdir, "", "set", "--scope", "pr", "-s", "integrail", "k", "v")
	Expect(err).NotTo(HaveOccurred())

	out, err := execScope(t, workdir, "", "disallow", "--scope", "pr", authorized)
	Expect(err).To(HaveOccurred())
	Expect(err.Error() + out).To(ContainSubstring("last recipient"))
}

func TestScopeCmd_ReconcileFailsClosedWithoutKey(t *testing.T) {
	RegisterTestingT(t)
	workdir := t.TempDir()
	authA, _ := testRecipient(t)
	authB, _ := testRecipient(t)

	_, err := execScope(t, workdir, "", "allow", "--scope", "pr", authA)
	Expect(err).NotTo(HaveOccurred())
	_, err = execScope(t, workdir, "", "set", "--scope", "pr", "-s", "integrail", "k", "v")
	Expect(err).NotTo(HaveOccurred())

	// Adding recipient B needs to reseal the populated file, which needs a key that
	// can decrypt it — none is available here, so allow must fail and persist nothing.
	_, err = execScope(t, workdir, "", "allow", "--scope", "pr", authB)
	Expect(err).To(HaveOccurred())

	// No drift: the scope file's recipients still match scopes.yaml (A only).
	out, lintErr := execScope(t, workdir, "", "lint")
	Expect(lintErr).NotTo(HaveOccurred(), out)
}

func TestScopeCmd_SetRefusesUndeclaredScope(t *testing.T) {
	RegisterTestingT(t)
	workdir := t.TempDir()
	// No allow first → scope not in scopes.yaml → set must refuse.
	out, err := execScope(t, workdir, "", "set", "--scope", "pr", "-s", "integrail", "k", "v")
	Expect(err).To(HaveOccurred())
	Expect(err.Error() + out).To(ContainSubstring("scope"))
}

// TestScopeCmd_KMSRecipientGovernance covers the v2 KMS-recipient additions at the
// CLI layer without any AWS call: allow accepts an awskms:// URL and persists it,
// a malformed KMS URL is rejected at governance time, and disallow removes it. The
// KMS crypto itself (wrap/unwrap, EncryptionContext binding) is covered by the
// scoped package's unit tests with a fake KMS client.
func TestScopeCmd_KMSRecipientGovernance(t *testing.T) {
	RegisterTestingT(t)
	workdir := t.TempDir()
	sshAuth, _ := testRecipient(t)
	kmsRec := "awskms://alias/sc-ci-pr?region=us-east-1"

	// SSH break-glass recipient + KMS recipient in the same scope.
	out, err := execScope(t, workdir, "", "allow", "--scope", "pr", sshAuth)
	Expect(err).NotTo(HaveOccurred(), out)
	out, err = execScope(t, workdir, "", "allow", "--scope", "pr", kmsRec)
	Expect(err).NotTo(HaveOccurred(), out)

	// The KMS recipient landed in scopes.yaml.
	scopesYAML, rErr := os.ReadFile(filepath.Join(workdir, ".sc", "scopes.yaml"))
	Expect(rErr).NotTo(HaveOccurred())
	Expect(string(scopesYAML)).To(ContainSubstring(kmsRec))

	// A malformed KMS URL (no region, not an ARN) is rejected at allow time.
	_, err = execScope(t, workdir, "", "allow", "--scope", "pr", "awskms://alias/no-region")
	Expect(err).To(HaveOccurred())

	// lint passes (recipients declared; no scope files with values yet).
	out, err = execScope(t, workdir, "", "lint")
	Expect(err).NotTo(HaveOccurred(), out)

	// disallow removes the KMS recipient (no scope files → no reseal / no KMS call).
	out, err = execScope(t, workdir, "", "disallow", "--scope", "pr", kmsRec)
	Expect(err).NotTo(HaveOccurred(), out)
	scopesYAML, _ = os.ReadFile(filepath.Join(workdir, ".sc", "scopes.yaml"))
	Expect(string(scopesYAML)).NotTo(ContainSubstring(kmsRec))
}

func writeStacksDirConfig(t *testing.T, workdir, stacksDir string) *api.ConfigFile {
	t.Helper()
	cfg := &api.ConfigFile{ProjectName: "myapp", StacksDir: stacksDir}
	Expect(os.MkdirAll(filepath.Join(workdir, ".sc"), 0o755)).To(Succeed())
	Expect(cfg.WriteConfigFile(workdir, "default")).To(Succeed())
	return cfg
}

// deployRead reads the stack the way a deploy does: through the provisioner's
// ReadStacks with the configured (or --dir) stacks dir.
func deployRead(t *testing.T, workdir string, cfg *api.ConfigFile, flagDir, stack string) (map[string]string, error) {
	t.Helper()
	cryptor, err := secrets.NewCryptor(workdir)
	Expect(err).NotTo(HaveOccurred())
	p, err := provisioner.New(provisioner.WithCryptor(cryptor), provisioner.WithPlaceholders(placeholders.New()))
	Expect(err).NotTo(HaveOccurred())
	Expect(p.Init(context.Background(), api.InitParams{ProjectName: "myapp", RootDir: workdir, SkipScDirCreation: true, SkipProfileCreation: true, IgnoreWorkdirErrors: true})).To(Succeed())
	err = p.ReadStacks(context.Background(), cfg, api.ProvisionParams{StacksDir: flagDir, Stacks: []string{stack}}, api.ReadIgnoreNoAnyCfg)
	if err != nil {
		return nil, err
	}
	return p.Stacks()[stack].Secrets.Values, nil
}

func TestScopeCmd_ConfiguredStacksDir(t *testing.T) {
	RegisterTestingT(t)
	workdir := t.TempDir()
	authorized, keyPEM := testRecipient(t)
	cfg := writeStacksDirConfig(t, workdir, "deploy/stacks")
	t.Setenv("SC_SCOPE_KEY", keyPEM)

	out, err := execScope(t, workdir, "", "allow", "--scope", "pr", authorized)
	Expect(err).NotTo(HaveOccurred(), out)
	out, err = execScope(t, workdir, "", "set", "--scope", "pr", "-s", "myapp", "api-key", "v1")
	Expect(err).NotTo(HaveOccurred(), out)

	custom := filepath.Join(workdir, "deploy", "stacks", "myapp", "secrets.pr.yaml")
	Expect(custom).To(BeAnExistingFile())
	Expect(filepath.Join(workdir, ".sc", "stacks")).NotTo(BeADirectory())
	Expect(filepath.Join(workdir, ".sc", "scopes.yaml")).To(BeAnExistingFile())

	vals, err := deployRead(t, workdir, cfg, "", "myapp")
	Expect(err).NotTo(HaveOccurred())
	Expect(vals).To(HaveKeyWithValue("api-key", "v1"))

	out, err = execScope(t, workdir, "", "get", "--scope", "pr", "-s", "myapp", "api-key")
	Expect(err).NotTo(HaveOccurred(), out)
	Expect(strings.TrimSpace(out)).To(Equal("v1"))
	out, _ = execScope(t, workdir, "", "list", "--scope", "pr", "-s", "myapp")
	Expect(out).To(ContainSubstring("api-key"))

	// lint, doctor and reseal-on-allow walk the configured dir
	out, err = execScope(t, workdir, "", "lint")
	Expect(err).NotTo(HaveOccurred(), out)
	Expect(out).To(ContainSubstring("1 scope file(s) OK"))
	out, _ = execScope(t, workdir, "", "doctor")
	Expect(out).To(ContainSubstring("YES"))
	second, _ := testRecipient(t)
	out, err = execScope(t, workdir, "", "allow", "--scope", "pr", second)
	Expect(err).NotTo(HaveOccurred(), out)
	f, err := scoped.LoadScopeFile(custom)
	Expect(err).NotTo(HaveOccurred())
	Expect(f.Recipients).To(HaveLen(2))
	out, err = execScope(t, workdir, "", "lint")
	Expect(err).NotTo(HaveOccurred(), out)

	out, err = execScope(t, workdir, "", "delete", "--scope", "pr", "-s", "myapp", "api-key")
	Expect(err).NotTo(HaveOccurred(), out)
	vals, err = deployRead(t, workdir, cfg, "", "myapp")
	Expect(err).NotTo(HaveOccurred())
	Expect(vals).NotTo(HaveKey("api-key"))
}

func TestScopeCmd_DefaultStacksDir(t *testing.T) {
	RegisterTestingT(t)
	workdir := t.TempDir()
	authorized, keyPEM := testRecipient(t)
	cfg := writeStacksDirConfig(t, workdir, "")
	t.Setenv("SC_SCOPE_KEY", keyPEM)

	_, err := execScope(t, workdir, "", "allow", "--scope", "pr", authorized)
	Expect(err).NotTo(HaveOccurred())
	_, err = execScope(t, workdir, "", "set", "--scope", "pr", "-s", "myapp", "api-key", "v1")
	Expect(err).NotTo(HaveOccurred())
	Expect(filepath.Join(workdir, ".sc", "stacks", "myapp", "secrets.pr.yaml")).To(BeAnExistingFile())
	vals, err := deployRead(t, workdir, cfg, "", "myapp")
	Expect(err).NotTo(HaveOccurred())
	Expect(vals).To(HaveKeyWithValue("api-key", "v1"))

	// no config file at all behaves the same (CI with only a scope key)
	bare := t.TempDir()
	_, err = execScope(t, bare, "", "allow", "--scope", "pr", authorized)
	Expect(err).NotTo(HaveOccurred())
	_, err = execScope(t, bare, "", "set", "--scope", "pr", "-s", "myapp", "k", "v")
	Expect(err).NotTo(HaveOccurred())
	Expect(filepath.Join(bare, ".sc", "stacks", "myapp", "secrets.pr.yaml")).To(BeAnExistingFile())
}

func TestScopeCmd_DirFlagOverridesConfig(t *testing.T) {
	RegisterTestingT(t)
	workdir := t.TempDir()
	authorized, keyPEM := testRecipient(t)
	cfg := writeStacksDirConfig(t, workdir, "deploy/stacks")
	t.Setenv("SC_SCOPE_KEY", keyPEM)

	_, err := execScope(t, workdir, "", "allow", "--scope", "pr", authorized)
	Expect(err).NotTo(HaveOccurred())
	out, err := execScope(t, workdir, "", "set", "--scope", "pr", "-s", "myapp", "-d", "flag/stacks", "k", "v")
	Expect(err).NotTo(HaveOccurred(), out)
	Expect(filepath.Join(workdir, "flag", "stacks", "myapp", "secrets.pr.yaml")).To(BeAnExistingFile())
	Expect(filepath.Join(workdir, "deploy")).NotTo(BeADirectory())

	vals, err := deployRead(t, workdir, cfg, "flag/stacks", "myapp")
	Expect(err).NotTo(HaveOccurred())
	Expect(vals).To(HaveKeyWithValue("k", "v"))

	out, err = execScope(t, workdir, "", "lint", "--dir", "flag/stacks")
	Expect(err).NotTo(HaveOccurred(), out)
	Expect(out).To(ContainSubstring("1 scope file(s) OK"))
	// without the flag the config dir is used and holds nothing
	out, err = execScope(t, workdir, "", "lint")
	Expect(err).NotTo(HaveOccurred(), out)
	Expect(out).To(ContainSubstring("0 scope file(s) OK"))
}

func TestScopeCmd_LintWarnsOnFilesInDefaultDir(t *testing.T) {
	RegisterTestingT(t)
	workdir := t.TempDir()
	authorized, _ := testRecipient(t)

	// written while the default dir was in effect
	_, err := execScope(t, workdir, "", "allow", "--scope", "pr", authorized)
	Expect(err).NotTo(HaveOccurred())
	_, err = execScope(t, workdir, "", "set", "--scope", "pr", "-s", "myapp", "k", "v")
	Expect(err).NotTo(HaveOccurred())

	writeStacksDirConfig(t, workdir, "deploy/stacks")
	out, err := execScope(t, workdir, "", "lint")
	Expect(err).NotTo(HaveOccurred(), out)
	Expect(out).To(ContainSubstring("deploys will not read it"))
	Expect(out).To(ContainSubstring(filepath.Join(".sc", "stacks", "myapp", "secrets.pr.yaml")))
}

func TestScopeCmd_BrokenConfigIsNotSilentlyIgnored(t *testing.T) {
	RegisterTestingT(t)
	workdir := t.TempDir()
	authorized, _ := testRecipient(t)
	_, err := execScope(t, workdir, "", "allow", "--scope", "pr", authorized)
	Expect(err).NotTo(HaveOccurred())
	Expect(os.WriteFile(filepath.Join(workdir, ".sc", "cfg.default.yaml"), []byte("stacksDir: [unclosed"), 0o644)).To(Succeed())
	out, err := execScope(t, workdir, "", "set", "--scope", "pr", "-s", "myapp", "k", "v")
	Expect(err).To(HaveOccurred(), out)
	Expect(err.Error()).To(ContainSubstring("failed to resolve the stacks directory"))
	Expect(filepath.Join(workdir, ".sc", "stacks")).NotTo(BeADirectory())
}

func TestScopeCmd_LintWarnsOnLookalike(t *testing.T) {
	RegisterTestingT(t)
	workdir := t.TempDir()
	authorized, _ := testRecipient(t)

	_, err := execScope(t, workdir, "", "allow", "--scope", "pr", authorized)
	Expect(err).NotTo(HaveOccurred())
	_, err = execScope(t, workdir, "", "set", "--scope", "pr", "-s", "myapp", "api-key", "v")
	Expect(err).NotTo(HaveOccurred())

	example := filepath.Join(workdir, ".sc", "stacks", "myapp", "secrets.example.yaml")
	Expect(os.WriteFile(example, []byte("schemaVersion: 1.0\nvalues:\n  DB_PASSWORD: changeme\n"), 0o644)).To(Succeed())

	out, err := execScope(t, workdir, "", "lint")
	Expect(err).NotTo(HaveOccurred(), out)
	Expect(out).To(ContainSubstring("secrets.example.yaml is not a scope file"))
	Expect(out).To(ContainSubstring("1 scope file(s) OK"))
}
