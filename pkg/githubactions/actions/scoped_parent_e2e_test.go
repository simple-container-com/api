// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package actions

import (
	"context"
	"crypto/ed25519"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
	"gopkg.in/yaml.v3"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/logger"
	"github.com/simple-container-com/api/pkg/api/secrets/ciphers"
	"github.com/simple-container-com/api/pkg/api/secrets/scoped"
)

// newParentRepo creates a committed git repo standing in for the parent stack
// repository: .sc/stacks/infra/server.yaml plus whatever scope files are asked for.
func newParentRepo(t *testing.T, scopes []string, extra map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, ".sc", "stacks", "infra", "server.yaml"), "schemaVersion: 1.0\n")
	for rel, content := range extra {
		mustWrite(t, filepath.Join(dir, ".sc", "stacks", "infra", rel), content)
	}
	if len(scopes) > 0 {
		priv, pub, err := ciphers.GenerateEd25519KeyPair()
		if err != nil || len(priv) == 0 {
			t.Fatal(err)
		}
		recipient := sshAuthorizedKey(t, pub)
		for _, scope := range scopes {
			f, err := scoped.NewScopeFile("infra", scope, []string{recipient})
			if err != nil {
				t.Fatal(err)
			}
			if err := f.Set("TOKEN", "v"); err != nil {
				t.Fatal(err)
			}
			if err := f.Save(filepath.Join(dir, ".sc", "stacks", "infra", scoped.ScopeFileName(scope))); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, args := range [][]string{
		{"init", "-q"},
		{"add", "-A"},
		{"-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "commit.gpgsign=false", "commit", "-q", "-m", "parent"},
	} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	return dir
}

// runClientParentSetup runs cloneParentRepository, the step a client-deploy job
// executes, in a workspace that already holds a revealed secrets.yaml of the parent
// stack left by an earlier step.
func runClientParentSetup(t *testing.T, parent string, staleSecrets bool) string {
	t.Helper()
	workspace := t.TempDir()
	if resolved, err := filepath.EvalSymlinks(workspace); err == nil {
		workspace = resolved
	}
	if staleSecrets {
		mustWrite(t, filepath.Join(workspace, ".sc", "stacks", "infra", "secrets.yaml"), "schemaVersion: 1.0\nvalues:\n  STALE: revealed-by-an-earlier-step\n")
	}
	mustWrite(t, filepath.Join(workspace, ".sc", "stacks", "app", "client.yaml"), "schemaVersion: 1.0\n")
	cfg, err := yaml.Marshal(api.ConfigFile{ProjectName: "myapp", ParentRepository: parent})
	if err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", workspace, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	t.Chdir(workspace)
	t.Setenv(api.ScConfigEnvVariable, string(cfg))
	t.Setenv("GITHUB_ACTIONS", "")
	t.Setenv("GITHUB_ACTION_TYPE", "")
	t.Setenv("ENVIRONMENT", "")

	e := NewExecutor(nil, logger.New(), nil)
	if err := e.cloneParentRepository(context.Background()); err != nil {
		t.Fatalf("cloneParentRepository: %v", err)
	}
	return workspace
}

func sshAuthorizedKey(t *testing.T, pub ed25519.PublicKey) string {
	t.Helper()
	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sshPub)))
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func TestClientDeployParentSetup_E2E(t *testing.T) {
	t.Run("7a a parent with scope files replaces the stale revealed store", func(t *testing.T) {
		workspace := runClientParentSetup(t, newParentRepo(t, []string{"app-staging"}, nil), true)
		stacks := filepath.Join(workspace, ".sc", "stacks")
		if exists(filepath.Join(stacks, "infra", "secrets.yaml")) {
			t.Error("the stale revealed secrets.yaml survived the parent setup")
		}
		if !exists(filepath.Join(stacks, "infra", scoped.ScopeFileName("app-staging"))) {
			t.Error("the parent's scope file was not copied into the workspace")
		}
		if !exists(filepath.Join(stacks, "infra", "server.yaml")) {
			t.Error("the parent's server.yaml was not copied into the workspace")
		}
		if !exists(filepath.Join(stacks, "app", "client.yaml")) {
			t.Error("the client's own files must stay")
		}
	})

	t.Run("7b a parent without scope files keeps the workspace secrets.yaml", func(t *testing.T) {
		workspace := runClientParentSetup(t, newParentRepo(t, nil, nil), true)
		stale := filepath.Join(workspace, ".sc", "stacks", "infra", "secrets.yaml")
		body, err := os.ReadFile(stale)
		if err != nil {
			t.Fatalf("the workspace secrets.yaml of a stack without scopes was removed: %v", err)
		}
		if !strings.Contains(string(body), "revealed-by-an-earlier-step") {
			t.Errorf("workspace secrets.yaml changed: %q", body)
		}
	})

	t.Run("8 a plaintext secrets.example.yaml does not count as a scope file", func(t *testing.T) {
		parent := newParentRepo(t, nil, map[string]string{"secrets.example.yaml": "schemaVersion: 1.0\nvalues:\n  REAL: changeme\n"})
		workspace := runClientParentSetup(t, parent, true)
		if !exists(filepath.Join(workspace, ".sc", "stacks", "infra", "secrets.yaml")) {
			t.Error("an example file next to a legacy secrets.yaml must behave as if absent; the workspace secrets.yaml was removed")
		}
	})
}
