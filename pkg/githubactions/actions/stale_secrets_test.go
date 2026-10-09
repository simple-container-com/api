// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package actions

import (
	"os"
	"path/filepath"
	"testing"
)

// A job that deploys twice, first with the whole-file store and then with a key
// that cannot open it, must not resolve the second deploy from the first one's
// plaintext.
func TestStaleParentSecretsDoNotOutliveACopyWithoutThem(t *testing.T) {
	root := t.TempDir()
	parent := filepath.Join(root, ".devops", ".sc", "stacks")
	workspace := filepath.Join(root, ".sc", "stacks")

	mustWrite(t, filepath.Join(parent, "infra", "server.yaml"), "server")
	mustWrite(t, filepath.Join(parent, "infra", "secrets.team.yaml"), "schemaVersion: 1\nrecipients: []\n")
	mustWrite(t, filepath.Join(workspace, "infra", "secrets.yaml"), "revealed by an earlier step")
	mustWrite(t, filepath.Join(workspace, "app", "client.yaml"), "client")
	mustWrite(t, filepath.Join(workspace, "app", "secrets.yaml"), "the client's own")

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

	mustWrite(t, filepath.Join(parent, "infra", "secrets.yaml"), "revealed now")
	mustWrite(t, filepath.Join(workspace, "infra", "secrets.yaml"), "revealed before")

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

// Without scopes the revealed file is the parent's only store: an earlier step
// may have revealed it on purpose, and it stays.
func TestStaleParentSecretsKeptWithoutScopes(t *testing.T) {
	root := t.TempDir()
	parent := filepath.Join(root, ".devops", ".sc", "stacks")
	workspace := filepath.Join(root, ".sc", "stacks")
	mustWrite(t, filepath.Join(parent, "infra", "server.yaml"), "server")
	mustWrite(t, filepath.Join(workspace, "infra", "secrets.yaml"), "revealed by an earlier step")

	removed, err := removeStaleParentSecrets(parent, workspace)
	if err != nil || len(removed) != 0 {
		t.Fatalf("removed %v, %v; want nothing removed", removed, err)
	}
	if _, err := os.Stat(filepath.Join(workspace, "infra", "secrets.yaml")); err != nil {
		t.Errorf("a store without scopes was removed: %v", err)
	}
}

// A workspace stack directory that is a symlink points somewhere this run does
// not own; nothing is removed through it.
func TestStaleParentSecretsSkipsSymlinkedStackDir(t *testing.T) {
	root := t.TempDir()
	parent := filepath.Join(root, ".devops", ".sc", "stacks")
	workspace := filepath.Join(root, ".sc", "stacks")
	elsewhere := filepath.Join(root, "elsewhere")
	mustWrite(t, filepath.Join(parent, "infra", "secrets.team.yaml"), "schemaVersion: 1\nrecipients: []\n")
	mustWrite(t, filepath.Join(elsewhere, "secrets.yaml"), "not ours")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(elsewhere, filepath.Join(workspace, "infra")); err != nil {
		t.Fatal(err)
	}

	if removed, err := removeStaleParentSecrets(parent, workspace); err != nil || len(removed) != 0 {
		t.Fatalf("removed %v, %v", removed, err)
	}
	if _, err := os.Stat(filepath.Join(elsewhere, "secrets.yaml")); err != nil {
		t.Errorf("a file behind a symlinked stack dir was removed: %v", err)
	}
}

func TestStaleParentSecretsErrors(t *testing.T) {
	root := t.TempDir()
	if removed, err := removeStaleParentSecrets(filepath.Join(root, "absent"), root); err != nil || removed != nil {
		t.Errorf("a parent without stacks: %v, %v; want nothing and no error (the copy reports it)", removed, err)
	}

	parent := filepath.Join(root, ".devops", ".sc", "stacks")
	workspace := filepath.Join(root, ".sc", "stacks")
	mustWrite(t, filepath.Join(parent, "infra", "secrets.team.yaml"), "schemaVersion: 1\nrecipients: []\n")
	mustWrite(t, filepath.Join(workspace, "infra", "secrets.yaml", "inside"), "a directory, not a file")
	if _, err := removeStaleParentSecrets(parent, workspace); err == nil {
		t.Error("a secrets.yaml that cannot be removed was not reported")
	}

	notDir := filepath.Join(root, "file")
	mustWrite(t, notDir, "x")
	if _, err := removeStaleParentSecrets(notDir, workspace); err == nil {
		t.Error("a parent stacks path that is a file was not reported")
	}
}

// A secrets.<x>.yaml that the scoped store did not write is not a scope file; the
// revealed secrets.yaml may be the only store and must stay.
func TestStaleParentSecretsKeptWithLookalikeFile(t *testing.T) {
	root := t.TempDir()
	parent := filepath.Join(root, ".devops", ".sc", "stacks")
	workspace := filepath.Join(root, ".sc", "stacks")
	mustWrite(t, filepath.Join(parent, "infra", "secrets.example.yaml"), "schemaVersion: 1.0\nvalues:\n  DB_PASSWORD: changeme\n")
	mustWrite(t, filepath.Join(workspace, "infra", "secrets.yaml"), "revealed by an earlier step")

	removed, err := removeStaleParentSecrets(parent, workspace)
	if err != nil || len(removed) != 0 {
		t.Fatalf("removed %v, %v; want nothing removed", removed, err)
	}
	if _, err := os.Stat(filepath.Join(workspace, "infra", "secrets.yaml")); err != nil {
		t.Errorf("secrets.yaml was removed because of a look-alike file: %v", err)
	}
}

// A damaged scope file in the parent still marks the stack as scoped: the stale
// store goes, and the deploy then fails on the damaged file rather than running
// on a store this run did not reveal. A zero-byte file is a truncated scope file;
// a whitespace or comments-only placeholder holds nothing and changes nothing.
func TestStaleParentSecretsWithDamagedOrEmptyScopeFile(t *testing.T) {
	for name, tc := range map[string]struct {
		content string
		removed bool
	}{
		"merge conflict": {"<<<<<<< HEAD\nscope: team\n=======\n", true},
		"zero bytes":     {"", true},
		"whitespace":     {"\n\n", false},
		"comments only":  {"# filled in later\n", false},
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			parent := filepath.Join(root, ".devops", ".sc", "stacks")
			workspace := filepath.Join(root, ".sc", "stacks")
			mustWrite(t, filepath.Join(parent, "infra", "secrets.team.yaml"), tc.content)
			mustWrite(t, filepath.Join(workspace, "infra", "secrets.yaml"), "revealed by an earlier step")

			removed, err := removeStaleParentSecrets(parent, workspace)
			if err != nil {
				t.Fatal(err)
			}
			if got := len(removed) == 1; got != tc.removed {
				t.Errorf("removed %v; want removal %v", removed, tc.removed)
			}
		})
	}
}
