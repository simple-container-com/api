// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package provisioner

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	"golang.org/x/crypto/ssh"

	"github.com/simple-container-com/api/pkg/api/secrets/ciphers"
	"github.com/simple-container-com/api/pkg/api/secrets/scoped"
)

// Test_readSecretsDescriptor_Scoped exercises the deploy-time read path for scoped
// secrets: a stack with ONLY secrets.<scope>.yaml (no legacy secrets.yaml), resolved
// via a CI scope key supplied through the environment (no SIMPLE_CONTAINER_CONFIG),
// and the fail-closed integrity propagation.
func Test_readSecretsDescriptor_Scoped(t *testing.T) {
	RegisterTestingT(t)

	// An "admin/CI" ed25519 recipient of the pr scope.
	priv, pub, err := ciphers.GenerateEd25519KeyPair()
	Expect(err).NotTo(HaveOccurred())
	sshPub, err := ssh.NewPublicKey(pub)
	Expect(err).NotTo(HaveOccurred())
	authorized := string(ssh.MarshalAuthorizedKey(sshPub))
	pem, err := ciphers.MarshalEd25519PrivateKey(priv)
	Expect(err).NotTo(HaveOccurred())

	stacksDir := t.TempDir()
	stackDir := filepath.Join(stacksDir, "teststack")
	Expect(os.MkdirAll(stackDir, 0o755)).To(Succeed())

	f, err := scoped.NewScopeFile("teststack", "pr", []string{authorized})
	Expect(err).NotTo(HaveOccurred())
	Expect(f.Set("defectdojo-api-key", "dd-secret")).To(Succeed())
	scopePath := filepath.Join(stackDir, scoped.ScopeFileName("pr"))
	Expect(f.Save(scopePath)).To(Succeed())
	// NB: no secrets.yaml written — this is a scoped-only stack.

	// Supply the scope key the way a pull_request job would (env, no full config).
	t.Setenv("SC_SCOPE_KEY", pem)
	p := &provisioner{} // no cryptor: key must come from the env

	// P1a + P1b: scoped-only stack resolves via the env scope key.
	desc, err := p.readSecretsDescriptor(stacksDir, "teststack")
	Expect(err).NotTo(HaveOccurred())
	Expect(desc).NotTo(BeNil())
	Expect(desc.Values["defectdojo-api-key"]).To(Equal("dd-secret"))

	// P2: a corrupt scope file is a hard error (ErrScopedIntegrity), never a silent
	// skip — even though nothing references the value.
	Expect(os.WriteFile(scopePath, []byte("schemaVersion: 999\nscope: pr\n"), 0o644)).To(Succeed())
	_, err = p.readSecretsDescriptor(stacksDir, "teststack")
	Expect(err).To(HaveOccurred())
	Expect(errors.Is(err, scoped.ErrScopedIntegrity)).To(BeTrue())
}

// Test_readSecretsDescriptor_NoSecretsStillNotFound confirms a stack with neither a
// legacy secrets.yaml nor any openable scoped value still reports not-found (the
// pre-scoped behavior that IgnoreSecretsMissing relies on).
func Test_readSecretsDescriptor_NoSecretsStillNotFound(t *testing.T) {
	RegisterTestingT(t)
	stacksDir := t.TempDir()
	Expect(os.MkdirAll(filepath.Join(stacksDir, "empty"), 0o755)).To(Succeed())

	p := &provisioner{}
	_, err := p.readSecretsDescriptor(stacksDir, "empty")
	Expect(err).To(HaveOccurred())
	Expect(errors.Is(err, os.ErrNotExist)).To(BeTrue())
	Expect(errors.Is(err, scoped.ErrScopedIntegrity)).To(BeFalse())
}
