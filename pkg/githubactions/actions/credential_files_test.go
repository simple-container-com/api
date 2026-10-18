// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package actions

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestPreserveWorkspaceCredentialFiles(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("TMPDIR", t.TempDir())
	existing := filepath.Join(t.TempDir(), "adc.json")
	mustWrite(t, existing, `{"outside":true}`)
	mustWrite(t, filepath.Join(workspace, "gha-creds-1.json"), `{"runner":true}`)
	if err := os.Mkdir(filepath.Join(workspace, "gha-creds-dir.json"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GITHUB_WORKSPACE", workspace)
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "/home/runner/work/app/app/gha-creds-1.json") // runner path
	t.Setenv("CLOUDSDK_AUTH_CREDENTIAL_FILE_OVERRIDE", existing)                             // outside the workspace
	t.Setenv("GOOGLE_GHA_CREDS_PATH", "/home/runner/work/app/app/gha-creds-dir.json")        // not a file

	changed, failed := PreserveWorkspaceCredentialFiles()

	if len(failed) != 0 {
		t.Fatalf("failed = %v", failed)
	}
	got := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")
	if insideDir(got, workspace) || !strings.HasSuffix(got, "gha-creds-1.json") || changed["GOOGLE_APPLICATION_CREDENTIALS"] != got {
		t.Errorf("GOOGLE_APPLICATION_CREDENTIALS = %q (changed %v); want a copy outside the workspace", got, changed)
	}
	if st, err := os.Stat(got); err != nil || st.Mode().Perm() != 0o600 {
		t.Errorf("copy: %v, mode %v; want 0600", err, st.Mode().Perm())
	}
	if st, err := os.Stat(filepath.Dir(got)); err != nil || st.Mode().Perm() != 0o700 {
		t.Errorf("copy directory: %v, mode %v; want 0700", err, st.Mode().Perm())
	}
	if filepath.Dir(got) == filepath.Join(os.TempDir(), "sc-credentials") {
		t.Error("the copy sits in a predictable directory another user could pre-create")
	}
	// The action may replace the workspace's contents after this; the identity
	// must survive that.
	if err := os.RemoveAll(workspace); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(got); err != nil || string(data) != `{"runner":true}` {
		t.Errorf("credentials lost with the workspace: %q, %v", data, err)
	}
	if got := os.Getenv("CLOUDSDK_AUTH_CREDENTIAL_FILE_OVERRIDE"); got != existing {
		t.Errorf("a file outside the workspace must be left alone, got %q", got)
	}
	if got := os.Getenv("GOOGLE_GHA_CREDS_PATH"); got != "/home/runner/work/app/app/gha-creds-dir.json" {
		t.Errorf("a directory must not be picked, got %q", got)
	}
	if len(changed) != 1 {
		t.Errorf("changed = %v; want exactly one variable", changed)
	}
}

func TestPreserveWorkspaceCredentialFiles_NoWorkspace(t *testing.T) {
	t.Setenv("GITHUB_WORKSPACE", "")
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "/nope/gha-creds-1.json")
	if changed, failed := PreserveWorkspaceCredentialFiles(); len(changed)+len(failed) != 0 {
		t.Fatalf("changed %v, failed %v without a workspace", changed, failed)
	}
	if os.Getenv("GOOGLE_APPLICATION_CREDENTIALS") != "/nope/gha-creds-1.json" {
		t.Fatal("variable changed without a workspace")
	}
}

// The variable usually reaches the container already naming the mounted
// workspace. The file still has to leave the workspace before the clone, and a
// second call (the action makes one before the clone and one after) changes
// nothing.
func TestPreserveWorkspaceCredentialFiles_TranslatedPathAndSecondCall(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("TMPDIR", t.TempDir())
	inWorkspace := filepath.Join(workspace, "gha-creds-2.json")
	mustWrite(t, inWorkspace, "{}")
	t.Setenv("GITHUB_WORKSPACE", workspace)
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", inWorkspace)

	PreserveWorkspaceCredentialFiles()
	got := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")
	if insideDir(got, workspace) {
		t.Fatalf("GOOGLE_APPLICATION_CREDENTIALS = %q; still inside the workspace", got)
	}
	if changed, failed := PreserveWorkspaceCredentialFiles(); len(changed)+len(failed) != 0 || os.Getenv("GOOGLE_APPLICATION_CREDENTIALS") != got {
		t.Errorf("second call changed %v, failed %v", changed, failed)
	}
	if err := os.RemoveAll(workspace); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(got); err != nil || string(data) != "{}" {
		t.Errorf("credentials lost with the workspace: %q, %v", data, err)
	}
}

// Two variables naming different files of the same name each keep their own.
func TestPreserveWorkspaceCredentialFiles_SameBaseName(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("TMPDIR", t.TempDir())
	mustWrite(t, filepath.Join(workspace, "a", "creds.json"), "A")
	mustWrite(t, filepath.Join(workspace, "b", "creds.json"), "B")
	t.Setenv("GITHUB_WORKSPACE", workspace)
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", filepath.Join(workspace, "a", "creds.json"))
	t.Setenv("CLOUDSDK_AUTH_CREDENTIAL_FILE_OVERRIDE", filepath.Join(workspace, "b", "creds.json"))
	t.Setenv("GOOGLE_GHA_CREDS_PATH", "")

	PreserveWorkspaceCredentialFiles()

	for name, want := range map[string]string{"GOOGLE_APPLICATION_CREDENTIALS": "A", "CLOUDSDK_AUTH_CREDENTIAL_FILE_OVERRIDE": "B"} {
		if data, err := os.ReadFile(os.Getenv(name)); err != nil || string(data) != want {
			t.Errorf("%s reads %q, %v; want %q", name, data, err, want)
		}
	}
}

// A copy that cannot be made is reported, and the variable is left as it was.
func TestPreserveWorkspaceCredentialFiles_ReportsFailure(t *testing.T) {
	workspace := t.TempDir()
	blocker := filepath.Join(t.TempDir(), "not-a-dir")
	mustWrite(t, blocker, "x")
	t.Setenv("TMPDIR", blocker)
	inWorkspace := filepath.Join(workspace, "gha-creds-3.json")
	mustWrite(t, inWorkspace, "{}")
	t.Setenv("GITHUB_WORKSPACE", workspace)
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", inWorkspace)
	t.Setenv("CLOUDSDK_AUTH_CREDENTIAL_FILE_OVERRIDE", "")
	t.Setenv("GOOGLE_GHA_CREDS_PATH", "")

	changed, failed := PreserveWorkspaceCredentialFiles()

	if len(changed) != 0 || failed["GOOGLE_APPLICATION_CREDENTIALS"] == nil {
		t.Errorf("changed %v, failed %v; want the failure reported", changed, failed)
	}
	if os.Getenv("GOOGLE_APPLICATION_CREDENTIALS") != inWorkspace {
		t.Error("a variable whose copy failed was changed")
	}
}

func TestInsideDir(t *testing.T) {
	ws := t.TempDir()
	mustWrite(t, filepath.Join(ws, "..creds.json"), "x")
	link := filepath.Join(t.TempDir(), "ws-link")
	if err := os.Symlink(ws, link); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		path string
		want bool
	}{
		{filepath.Join(ws, "creds.json"), true},
		{filepath.Join(ws, "..creds.json"), true},
		{filepath.Join(ws, "sub", "creds.json"), true},
		{filepath.Join(link, "..creds.json"), true},
		{ws, false},
		{filepath.Dir(ws), false},
		{filepath.Join(ws, "..", "other.json"), false},
		{"/etc/passwd", false},
	} {
		if got := insideDir(c.path, ws); got != c.want {
			t.Errorf("insideDir(%q) = %v; want %v", c.path, got, c.want)
		}
	}
}

// A credentials file the run cannot read is reported too, not skipped.
func TestPreserveWorkspaceCredentialFiles_ReportsUnreadableSource(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads files regardless of their mode")
	}
	workspace := t.TempDir()
	t.Setenv("TMPDIR", t.TempDir())
	src := filepath.Join(workspace, "gha-creds-4.json")
	mustWrite(t, src, "{}")
	if err := os.Chmod(src, 0); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GITHUB_WORKSPACE", workspace)
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", src)
	t.Setenv("CLOUDSDK_AUTH_CREDENTIAL_FILE_OVERRIDE", "")
	t.Setenv("GOOGLE_GHA_CREDS_PATH", "")

	changed, failed := PreserveWorkspaceCredentialFiles()

	if len(changed) != 0 || failed["GOOGLE_APPLICATION_CREDENTIALS"] == nil || !strings.Contains(failed["GOOGLE_APPLICATION_CREDENTIALS"].Error(), "failed to read") {
		t.Errorf("changed %v, failed %v; want the unreadable file reported", changed, failed)
	}
	if os.Getenv("GOOGLE_APPLICATION_CREDENTIALS") != src {
		t.Error("a variable whose copy failed was changed")
	}
}
