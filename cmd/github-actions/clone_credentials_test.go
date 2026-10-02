// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/simple-container-com/api/pkg/githubactions/actions"
)

// With no checkout in the job, the action clones the repository and replaces the
// workspace's contents. A Workload Identity Federation config that an earlier
// step wrote into the workspace must still be what the run authenticates with.
func TestCloneKeepsFederatedCredentials(t *testing.T) {
	workspace := t.TempDir()
	clone := t.TempDir()
	t.Setenv("TMPDIR", t.TempDir())
	if err := os.WriteFile(filepath.Join(workspace, "gha-creds-1.json"), []byte(`{"type":"external_account"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(clone, "README.md"), []byte("repo"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GITHUB_WORKSPACE", workspace)
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "/home/runner/work/app/app/gha-creds-1.json")

	actions.PreserveWorkspaceCredentialFiles()
	if err := copyRepositoryContents(clone, workspace); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(workspace, "gha-creds-1.json")); !os.IsNotExist(err) {
		t.Fatalf("expected the clone to replace the workspace (stat err: %v)", err)
	}
	data, err := os.ReadFile(os.Getenv("GOOGLE_APPLICATION_CREDENTIALS"))
	if err != nil || string(data) != `{"type":"external_account"}` {
		t.Errorf("credentials did not survive the clone: %q, %v", data, err)
	}
}
