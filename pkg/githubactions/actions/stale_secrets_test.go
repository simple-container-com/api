// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package actions

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// A job that deploys twice, first with the whole-file store and then with a key
// that cannot open it, must not resolve the second deploy from the first one's
// plaintext.
func TestStaleParentSecretsDoNotOutliveACopyWithoutThem(t *testing.T) {
	root := t.TempDir()
	parent := filepath.Join(root, ".devops", ".sc", "stacks")
	workspace := filepath.Join(root, ".sc", "stacks")

	writeFile(t, filepath.Join(parent, "infra", "server.yaml"), "server")
	writeFile(t, filepath.Join(parent, "infra", "secrets.team.yaml"), "scope")
	writeFile(t, filepath.Join(workspace, "infra", "secrets.yaml"), "revealed by an earlier step")
	writeFile(t, filepath.Join(workspace, "app", "client.yaml"), "client")
	writeFile(t, filepath.Join(workspace, "app", "secrets.yaml"), "the client's own")

	removed, err := removeStaleParentSecrets(parent, workspace)
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 1 {
		t.Errorf("removed %v; want only the parent stack's revealed store", removed)
	}

	e := &Executor{}
	if err := e.copyDirectory(parent, workspace); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(workspace, "infra", "secrets.yaml")); !os.IsNotExist(err) {
		t.Errorf("the earlier step's plaintext store survived the copy (stat err: %v)", err)
	}
	for _, keep := range []string{
		filepath.Join(workspace, "infra", "server.yaml"),
		filepath.Join(workspace, "infra", "secrets.team.yaml"),
		filepath.Join(workspace, "app", "client.yaml"),
		filepath.Join(workspace, "app", "secrets.yaml"),
	} {
		if _, err := os.Stat(keep); err != nil {
			t.Errorf("%s: %v", keep, err)
		}
	}
}

// When this run did reveal the store, the copy brings it back.
func TestRevealedParentSecretsAreReplacedNotLost(t *testing.T) {
	root := t.TempDir()
	parent := filepath.Join(root, ".devops", ".sc", "stacks")
	workspace := filepath.Join(root, ".sc", "stacks")

	writeFile(t, filepath.Join(parent, "infra", "secrets.yaml"), "revealed now")
	writeFile(t, filepath.Join(workspace, "infra", "secrets.yaml"), "revealed before")

	if _, err := removeStaleParentSecrets(parent, workspace); err != nil {
		t.Fatal(err)
	}
	if err := (&Executor{}).copyDirectory(parent, workspace); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(workspace, "infra", "secrets.yaml"))
	if err != nil || string(got) != "revealed now" {
		t.Errorf("got %q, %v; want this run's store", got, err)
	}
}

func TestStaleParentSecretsMissingParentDir(t *testing.T) {
	if _, err := removeStaleParentSecrets(filepath.Join(t.TempDir(), "absent"), t.TempDir()); err == nil {
		t.Error("a missing parent stacks directory must be reported")
	}
}
