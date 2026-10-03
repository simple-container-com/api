// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package actions

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRemapWorkspaceCredentialFiles(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("TMPDIR", t.TempDir())
	existing := filepath.Join(t.TempDir(), "adc.json")
	if err := os.WriteFile(existing, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "gha-creds-1.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(workspace, "gha-creds-dir.json"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GITHUB_WORKSPACE", workspace)
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "/home/runner/work/app/app/gha-creds-1.json") // runner path
	t.Setenv("CLOUDSDK_AUTH_CREDENTIAL_FILE_OVERRIDE", existing)                             // valid here
	t.Setenv("GOOGLE_GHA_CREDS_PATH", "/home/runner/work/app/app/gha-creds-dir.json")        // not a file

	changed := remapWorkspaceCredentialFiles()

	got := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")
	if filepath.Base(got) != "gha-creds-1.json" || strings.HasPrefix(got, workspace) {
		t.Errorf("GOOGLE_APPLICATION_CREDENTIALS = %q; want a copy outside the workspace", got)
	}
	// The action may replace the workspace's contents after this; the identity
	// must survive that.
	if err := os.RemoveAll(workspace); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(got); err != nil || string(data) != "{}" {
		t.Errorf("credentials lost with the workspace: %q, %v", data, err)
	}
	if st, err := os.Stat(got); err == nil && st.Mode().Perm() != 0o600 {
		t.Errorf("copy mode = %v; want 0600", st.Mode().Perm())
	}
	if got := os.Getenv("CLOUDSDK_AUTH_CREDENTIAL_FILE_OVERRIDE"); got != existing {
		t.Errorf("a path that exists must be left alone, got %q", got)
	}
	if got := os.Getenv("GOOGLE_GHA_CREDS_PATH"); got != "/home/runner/work/app/app/gha-creds-dir.json" {
		t.Errorf("a directory must not be picked, got %q", got)
	}
	if len(changed) != 1 {
		t.Errorf("changed = %v; want exactly one variable", changed)
	}
}

func TestRemapWorkspaceCredentialFiles_NoWorkspace(t *testing.T) {
	t.Setenv("GITHUB_WORKSPACE", "")
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "/nope/gha-creds-1.json")
	if changed := remapWorkspaceCredentialFiles(); len(changed) != 0 {
		t.Fatalf("changed %v without a workspace", changed)
	}
	if os.Getenv("GOOGLE_APPLICATION_CREDENTIALS") != "/nope/gha-creds-1.json" {
		t.Fatal("variable changed without a workspace")
	}
}

// The variable usually reaches the container already naming the mounted
// workspace. The file still has to leave the workspace before the clone.
func TestRemapWorkspaceCredentialFiles_TranslatedPath(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("TMPDIR", t.TempDir())
	inWorkspace := filepath.Join(workspace, "gha-creds-2.json")
	if err := os.WriteFile(inWorkspace, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GITHUB_WORKSPACE", workspace)
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", inWorkspace)

	remapWorkspaceCredentialFiles()

	got := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")
	if strings.HasPrefix(got, workspace) {
		t.Fatalf("GOOGLE_APPLICATION_CREDENTIALS = %q; still inside the workspace", got)
	}
	if err := os.RemoveAll(workspace); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(got); err != nil || string(data) != "{}" {
		t.Errorf("credentials lost with the workspace: %q, %v", data, err)
	}
}
