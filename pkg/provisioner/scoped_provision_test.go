// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package provisioner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	"golang.org/x/crypto/ssh"

	"github.com/simple-container-com/api/pkg/api/secrets/ciphers"
	"github.com/simple-container-com/api/pkg/api/secrets/scoped"
)

// captureLogger records Warn lines so a test can assert the deploy-time
// legacy-shadows-scoped warning fires. It satisfies api/logger.Logger structurally.
type captureLogger struct{ warns []string }

func (l *captureLogger) Error(_ context.Context, f string, a ...any) {}
func (l *captureLogger) Warn(_ context.Context, f string, a ...any) {
	l.warns = append(l.warns, fmt.Sprintf(f, a...))
}
func (l *captureLogger) Info(_ context.Context, f string, a ...any)             {}
func (l *captureLogger) Debug(_ context.Context, f string, a ...any)            {}
func (l *captureLogger) SetLogLevel(ctx context.Context, _ int) context.Context { return ctx }
func (l *captureLogger) Silent(ctx context.Context) context.Context             { return ctx }

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
	desc, err := p.readSecretsDescriptor(context.Background(), stacksDir, "teststack")
	Expect(err).NotTo(HaveOccurred())
	Expect(desc).NotTo(BeNil())
	Expect(desc.Values["defectdojo-api-key"]).To(Equal("dd-secret"))

	// P2: a corrupt scope file is a hard error (ErrScopedIntegrity), never a silent
	// skip — even though nothing references the value.
	Expect(os.WriteFile(scopePath, []byte("schemaVersion: 999\nscope: pr\n"), 0o644)).To(Succeed())
	_, err = p.readSecretsDescriptor(context.Background(), stacksDir, "teststack")
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
	_, err := p.readSecretsDescriptor(context.Background(), stacksDir, "empty")
	Expect(err).To(HaveOccurred())
	Expect(errors.Is(err, os.ErrNotExist)).To(BeTrue())
	Expect(errors.Is(err, scoped.ErrScopedIntegrity)).To(BeFalse())
}

// Test_readSecretsDescriptor_ScopedShadowedByLegacy verifies the phase-2 deploy-time
// behavior: when a key exists in BOTH the legacy secrets.yaml and an openable scope,
// the legacy value wins (fail-safe direction) AND a warning is emitted so the operator
// is not silently surprised.
func Test_readSecretsDescriptor_ScopedShadowedByLegacy(t *testing.T) {
	RegisterTestingT(t)

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

	// Legacy secrets.yaml defines shared-key.
	Expect(os.WriteFile(filepath.Join(stackDir, "secrets.yaml"),
		[]byte("values:\n  shared-key: legacy-value\n"), 0o644)).To(Succeed())

	// A scope file ALSO defines shared-key (with a different value).
	f, err := scoped.NewScopeFile("teststack", "pr", []string{authorized})
	Expect(err).NotTo(HaveOccurred())
	Expect(f.Set("shared-key", "scoped-value")).To(Succeed())
	Expect(f.Save(filepath.Join(stackDir, scoped.ScopeFileName("pr")))).To(Succeed())

	t.Setenv("SC_SCOPE_KEY", pem)
	log := &captureLogger{}
	p := &provisioner{log: log}

	desc, err := p.readSecretsDescriptor(context.Background(), stacksDir, "teststack")
	Expect(err).NotTo(HaveOccurred())
	// Legacy wins on conflict.
	Expect(desc.Values["shared-key"]).To(Equal("legacy-value"))
	// And a shadow warning was emitted, naming the key (never the value).
	joined := strings.Join(log.warns, "\n")
	Expect(joined).To(ContainSubstring("shared-key"))
	Expect(joined).To(ContainSubstring("shadowed"))
	Expect(joined).NotTo(ContainSubstring("legacy-value"))
	Expect(joined).NotTo(ContainSubstring("scoped-value"))
}
