// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package provisioner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/mock"
	"golang.org/x/crypto/ssh"

	"github.com/simple-container-com/api/pkg/api"
	git_mocks "github.com/simple-container-com/api/pkg/api/git/mocks"
	"github.com/simple-container-com/api/pkg/api/secrets/ciphers"
	"github.com/simple-container-com/api/pkg/api/secrets/scoped"
	"github.com/simple-container-com/api/pkg/clouds/gcloud"
	pulumi_mocks "github.com/simple-container-com/api/pkg/clouds/pulumi/mocks"
	"github.com/simple-container-com/api/pkg/provisioner/placeholders"
)

// The shape of a client deploy that holds no key to the parent's whole-file store:
// the parent repo is cloned (so its server.yaml and scope files are present) but its
// secrets.yaml could not be revealed, and the job holds only its own scope key.

const scopedParentServer = `schemaVersion: 1.0
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
cicd:
  type: github-actions
  config:
    organization: acme
    notifications:
      telegram:
        bot-token: "${secret:NOTIFY_TOKEN}"
        chat-id: "-100"
        enabled: true
resources:
  resources:
    staging:
      template: stack-per-app
      resources: {}
    production:
      template: stack-per-app
      resources:
        prod-db-password:
          type: gcp-bucket
          config:
            projectId: "${auth:gcloud.projectId}"
            credentials: "${auth:gcloud}"
            name: "${secret:production-only}"
`

const scopedClient = `schemaVersion: 1.0
stacks:
  staging:
    type: cloud-compose
    parent: acme/infra
    config:
      dockerComposeFile: docker-compose.yaml
      runs: [api]
      env:
        APP_KEY: "${secret:staging-app-key}"
  production:
    type: cloud-compose
    parent: acme/infra
    config:
      dockerComposeFile: docker-compose.yaml
      runs: [api]
      env:
        APP_KEY: "${secret:production-app-key}"
`

const ambientGcloudAuth = `type: gcp-service-account
config:
  projectId: acme-staging
  credentials: ""
`

func scopeKey(t *testing.T) (authorized, pemKey string) {
	t.Helper()
	priv, pub, err := ciphers.GenerateEd25519KeyPair()
	Expect(err).NotTo(HaveOccurred())
	sshPub, err := ssh.NewPublicKey(pub)
	Expect(err).NotTo(HaveOccurred())
	pem, err := ciphers.MarshalEd25519PrivateKey(priv)
	Expect(err).NotTo(HaveOccurred())
	return string(ssh.MarshalAuthorizedKey(sshPub)), pem
}

type scopedWorkspace struct {
	root, parentDir string
	recipient       string
}

// newScopedWorkspace lays out a client repo after the parent was cloned into it.
// entries are sealed into the parent's app-staging scope.
func newScopedWorkspace(t *testing.T, recipient string, entries map[string]string) scopedWorkspace {
	t.Helper()
	root := t.TempDir()
	sc := filepath.Join(root, ".sc")
	parent := filepath.Join(sc, "stacks", "infra")
	client := filepath.Join(sc, "stacks", "app")
	for _, d := range []string{parent, client} {
		Expect(os.MkdirAll(d, 0o755)).To(Succeed())
	}
	Expect(os.WriteFile(filepath.Join(sc, "cfg.default.yaml"), []byte("projectName: app\n"), 0o644)).To(Succeed())
	Expect(os.WriteFile(filepath.Join(parent, "server.yaml"), []byte(scopedParentServer), 0o644)).To(Succeed())
	Expect(os.WriteFile(filepath.Join(client, "client.yaml"), []byte(scopedClient), 0o644)).To(Succeed())
	Expect(os.WriteFile(filepath.Join(client, "docker-compose.yaml"), []byte("services:\n  api:\n    image: nginx\n"), 0o644)).To(Succeed())

	f, err := scoped.NewScopeFile("infra", "app-staging", []string{recipient})
	Expect(err).NotTo(HaveOccurred())
	for k, v := range entries {
		Expect(f.Set(k, v)).To(Succeed())
	}
	Expect(f.Save(filepath.Join(parent, scoped.ScopeFileName("app-staging")))).To(Succeed())
	return scopedWorkspace{root: root, parentDir: parent, recipient: recipient}
}

// deploy runs a real Deploy up to the Pulumi boundary and returns the stack that
// would have been handed to Pulumi (nil if it never got there).
func (w scopedWorkspace) deploy(t *testing.T, log *captureLogger, env string) (*api.Stack, error) {
	t.Helper()
	t.Setenv("SIMPLE_CONTAINER_CONFIG", "")
	git := git_mocks.NewGitRepoMock(t)
	git.On("Workdir").Return(w.root).Maybe()
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
	opts := []Option{
		WithPlaceholders(placeholders.New(placeholders.WithGitRepo(git))),
		WithOverrideProvisioner(pm),
		WithGitRepo(git),
	}
	if log != nil {
		opts = append(opts, WithLogger(log))
	}
	p, err := New(opts...)
	Expect(err).NotTo(HaveOccurred())
	err = p.Deploy(context.Background(), api.DeployParams{StackParams: api.StackParams{
		StacksDir: ".sc/stacks", StackName: "app", Environment: env,
	}})
	return got, err
}

func appEnv(s *api.Stack, env string) map[string]string {
	cfg, ok := s.Client.Stacks[env].Config.Config.(*api.StackConfigCompose)
	Expect(ok).To(BeTrue(), "client config is %T", s.Client.Stacks[env].Config.Config)
	return cfg.Env
}

func Test_Deploy_ScopedOnly_AmbientAuth(t *testing.T) {
	RegisterTestingT(t)
	rec, key := scopeKey(t)
	w := newScopedWorkspace(t, rec, map[string]string{
		"auth:gcloud":     ambientGcloudAuth,
		"staging-app-key": "s3cr3t",
		"NOTIFY_TOKEN":    "notify",
	})
	t.Setenv("SC_KEY_APP_STAGING", key)

	got, err := w.deploy(t, nil, "staging")
	Expect(err).NotTo(HaveOccurred())
	Expect(got).NotTo(BeNil())

	Expect(appEnv(got, "staging")).To(HaveKeyWithValue("APP_KEY", "s3cr3t"))
	auth, ok := got.Secrets.Auth["gcloud"]
	Expect(ok).To(BeTrue())
	creds, ok := auth.Config.Config.(*gcloud.Credentials)
	Expect(ok).To(BeTrue(), "auth config is %T", auth.Config.Config)
	Expect(creds.UsesAmbientCredentials()).To(BeTrue())
	Expect(creds.ProjectId).To(Equal("acme-staging"))

	out, _ := api.MarshalDescriptor(&got.Server)
	Expect(string(out)).NotTo(ContainSubstring("${auth:"), "parent config still has an unresolved ${auth:}")
	Expect(string(out)).To(ContainSubstring("acme-staging"))
	Expect(string(out)).To(ContainSubstring("notify"))
}

func Test_Deploy_ScopedOnly_MissingSecretFailsBeforePulumi(t *testing.T) {
	RegisterTestingT(t)
	rec, key := scopeKey(t)
	w := newScopedWorkspace(t, rec, map[string]string{
		"auth:gcloud":     ambientGcloudAuth,
		"staging-app-key": "s3cr3t",
		// NOTIFY_TOKEN is missing from the scope
	})
	t.Setenv("SC_KEY_APP_STAGING", key)

	got, err := w.deploy(t, nil, "staging")
	Expect(errors.Is(err, ErrUnresolvedPlaceholders)).To(BeTrue(), "err = %v", err)
	Expect(err.Error()).To(ContainSubstring("${secret:NOTIFY_TOKEN}"))
	Expect(got).To(BeNil(), "the deploy reached Pulumi with an unresolved secret")
}

func Test_Deploy_ScopedOnly_MissingAuthFailsBeforePulumi(t *testing.T) {
	RegisterTestingT(t)
	rec, key := scopeKey(t)
	w := newScopedWorkspace(t, rec, map[string]string{
		"staging-app-key": "s3cr3t",
		"NOTIFY_TOKEN":    "notify",
	})
	t.Setenv("SC_KEY_APP_STAGING", key)

	got, err := w.deploy(t, nil, "staging")
	Expect(errors.Is(err, ErrUnresolvedPlaceholders)).To(BeTrue(), "err = %v", err)
	Expect(err.Error()).To(ContainSubstring("${auth:gcloud"))
	Expect(got).To(BeNil())
}

func Test_Deploy_ScopedOnly_OtherEnvironmentsSecretsAreNotRequired(t *testing.T) {
	RegisterTestingT(t)
	// production-app-key is not in the staging scope. That is least privilege, not
	// a missing secret, and must not block a staging deploy.
	rec, key := scopeKey(t)
	w := newScopedWorkspace(t, rec, map[string]string{
		"auth:gcloud":     ambientGcloudAuth,
		"staging-app-key": "s3cr3t",
		"NOTIFY_TOKEN":    "notify",
	})
	t.Setenv("SC_KEY_APP_STAGING", key)
	_, err := w.deploy(t, nil, "staging")
	Expect(err).NotTo(HaveOccurred())
}

func Test_Deploy_ScopedOnly_KeyOfAnotherScopeOpensNothing(t *testing.T) {
	RegisterTestingT(t)
	rec, _ := scopeKey(t)
	_, otherKey := scopeKey(t)
	w := newScopedWorkspace(t, rec, map[string]string{
		"auth:gcloud":     ambientGcloudAuth,
		"staging-app-key": "s3cr3t",
		"NOTIFY_TOKEN":    "notify",
	})
	t.Setenv("SC_KEY_APP_STAGING", otherKey)
	// The key opens no scope, so the parent has no secrets at all. That must fail
	// the deploy, not ship the placeholders as text.
	got, err := w.deploy(t, nil, "staging")
	Expect(errors.Is(err, ErrUnresolvedPlaceholders)).To(BeTrue(), "err = %v", err)
	Expect(err.Error()).To(ContainSubstring("${secret:staging-app-key}"))
	Expect(got).To(BeNil())
}

func Test_Deploy_ScopedAuthCannotOverrideLegacy(t *testing.T) {
	RegisterTestingT(t)
	rec, key := scopeKey(t)
	w := newScopedWorkspace(t, rec, map[string]string{"auth:gcloud": ambientGcloudAuth})
	legacy := "auth:\n  gcloud:\n    type: gcp-service-account\n    config:\n      projectId: legacy-project\n      credentials: '{\"type\":\"service_account\"}'\n" +
		"values:\n  staging-app-key: legacy\n  NOTIFY_TOKEN: legacy\n"
	Expect(os.WriteFile(filepath.Join(w.parentDir, "secrets.yaml"), []byte(legacy), 0o644)).To(Succeed())
	t.Setenv("SC_KEY_APP_STAGING", key)

	log := &captureLogger{}
	got, err := w.deploy(t, log, "staging")
	Expect(err).NotTo(HaveOccurred())
	creds := got.Secrets.Auth["gcloud"].Config.Config.(*gcloud.Credentials)
	Expect(creds.ProjectId).To(Equal("legacy-project"))
	Expect(strings.Join(log.warns, "\n")).To(ContainSubstring(`scoped auth "gcloud" is shadowed`))
}

// Backward compatibility: with the whole-file store readable and no scopes, an
// unresolved placeholder keeps deploying as before and is only reported by name.
func Test_Deploy_LegacyUnresolvedPlaceholderOnlyWarns(t *testing.T) {
	RegisterTestingT(t)
	root := t.TempDir()
	w := scopedWorkspace{root: root, parentDir: filepath.Join(root, ".sc", "stacks", "infra")}
	client := filepath.Join(root, ".sc", "stacks", "app")
	for _, d := range []string{w.parentDir, client} {
		Expect(os.MkdirAll(d, 0o755)).To(Succeed())
	}
	Expect(os.WriteFile(filepath.Join(root, ".sc", "cfg.default.yaml"), []byte("projectName: app\n"), 0o644)).To(Succeed())
	Expect(os.WriteFile(filepath.Join(w.parentDir, "server.yaml"), []byte(scopedParentServer), 0o644)).To(Succeed())
	Expect(os.WriteFile(filepath.Join(client, "client.yaml"), []byte(scopedClient), 0o644)).To(Succeed())
	Expect(os.WriteFile(filepath.Join(client, "docker-compose.yaml"), []byte("services:\n  api:\n    image: nginx\n"), 0o644)).To(Succeed())
	legacy := "auth:\n  gcloud:\n    type: gcp-service-account\n    config:\n      projectId: p\n      credentials: '{\"type\":\"service_account\"}'\n" +
		"values:\n  staging-app-key: v\n"
	Expect(os.WriteFile(filepath.Join(w.parentDir, "secrets.yaml"), []byte(legacy), 0o644)).To(Succeed())

	log := &captureLogger{}
	got, err := w.deploy(t, log, "staging")
	Expect(err).NotTo(HaveOccurred())
	Expect(got).NotTo(BeNil())
	joined := strings.Join(log.warns, "\n")
	Expect(joined).To(ContainSubstring("${secret:NOTIFY_TOKEN}"))
	Expect(joined).NotTo(ContainSubstring(`{"type":"service_account"}`))
}

// A client picks its parent template per run with ${env:NAME:default}, so one
// client.yaml serves a keyless run (which sets the variable) and a run with the
// master key (which gets the default, i.e. today's template).
func Test_Deploy_TemplateFromEnvironment(t *testing.T) {
	RegisterTestingT(t)
	rec, key := scopeKey(t)
	w := newScopedWorkspace(t, rec, map[string]string{
		"auth:gcloud":     ambientGcloudAuth,
		"staging-app-key": "s3cr3t",
		"NOTIFY_TOKEN":    "notify",
	})
	client := filepath.Join(w.root, ".sc", "stacks", "app", "client.yaml")
	body, err := os.ReadFile(client)
	Expect(err).NotTo(HaveOccurred())
	Expect(os.WriteFile(client, []byte(strings.Replace(string(body),
		"    parent: acme/infra\n    config:\n      dockerComposeFile: docker-compose.yaml\n      runs: [api]\n      env:\n        APP_KEY: \"${secret:staging-app-key}\"",
		"    parent: acme/infra\n    template: ${env:SC_TEST_TEMPLATE:stack-per-app}\n    config:\n      dockerComposeFile: docker-compose.yaml\n      runs: [api]\n      env:\n        APP_KEY: \"${secret:staging-app-key}\"", 1)), 0o644)).To(Succeed())
	t.Setenv("SC_KEY_APP_STAGING", key)

	got, err := w.deploy(t, nil, "staging")
	Expect(err).NotTo(HaveOccurred())
	Expect(got.Client.Stacks["staging"].Template).To(Equal("stack-per-app"))

	t.Setenv("SC_TEST_TEMPLATE", "stack-per-app-ambient")
	got, err = w.deploy(t, nil, "staging")
	Expect(err).NotTo(HaveOccurred())
	Expect(got.Client.Stacks["staging"].Template).To(Equal("stack-per-app-ambient"))
}
