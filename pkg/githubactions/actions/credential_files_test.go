// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package actions

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRemapWorkspaceCredentialFiles(t *testing.T) {
	workspace := t.TempDir()
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

	if got := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS"); got != filepath.Join(workspace, "gha-creds-1.json") {
		t.Errorf("GOOGLE_APPLICATION_CREDENTIALS = %q; want the workspace copy", got)
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
