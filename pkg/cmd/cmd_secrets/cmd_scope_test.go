// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package cmd_secrets

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
	"golang.org/x/crypto/ssh"

	"github.com/simple-container-com/api/pkg/api/secrets"
	"github.com/simple-container-com/api/pkg/api/secrets/ciphers"
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

func TestScopeCmd_LintCatchesCrossScopeDuplicate(t *testing.T) {
	RegisterTestingT(t)
	workdir := t.TempDir()
	authorized, _ := testRecipient(t)

	_, err := execScope(t, workdir, "", "allow", "--scope", "pr", authorized)
	Expect(err).NotTo(HaveOccurred())
	_, err = execScope(t, workdir, "", "allow", "--scope", "prod", authorized)
	Expect(err).NotTo(HaveOccurred())
	_, err = execScope(t, workdir, "", "set", "--scope", "pr", "-s", "integrail", "shared", "a")
	Expect(err).NotTo(HaveOccurred())
	_, err = execScope(t, workdir, "", "set", "--scope", "prod", "-s", "integrail", "shared", "b")
	Expect(err).NotTo(HaveOccurred())

	out, err := execScope(t, workdir, "", "lint")
	Expect(err).To(HaveOccurred())
	Expect(out).To(ContainSubstring("multiple scopes"))
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
