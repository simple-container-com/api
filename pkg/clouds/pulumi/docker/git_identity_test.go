// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package docker

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestParseRemoteOwnerRepo(t *testing.T) {
	cases := []struct {
		url  string
		want string
	}{
		// github shapes — the two most common
		{"git@github.com:simple-container-com/api.git", "simple-container-com/api"},
		{"https://github.com/simple-container-com/api.git", "simple-container-com/api"},
		// without .git suffix
		{"https://github.com/simple-container-com/api", "simple-container-com/api"},
		// trailing slash
		{"https://github.com/simple-container-com/api/", "simple-container-com/api"},
		// gitlab-style nested groups: we only keep the last two segments, by design
		// (GITHUB_REPOSITORY is <owner>/<repo>; nested-group preservation is a future knob).
		{"https://gitlab.example/group/subgroup/repo.git", "subgroup/repo"},
		// ssh://  scheme
		{"ssh://git@bitbucket.org/owner/repo.git", "owner/repo"},
		// self-hosted with a port
		{"ssh://git@git.example.com:2222/team/service.git", "team/service"},
		// unrecognised: not a URL at all
		{"", ""},
		{"not-a-url", ""},
		// edge: trailing .git with no preceding path
		{"https://github.com/.git", ""},
	}
	for _, c := range cases {
		t.Run(c.url, func(t *testing.T) {
			got := parseRemoteOwnerRepo(c.url)
			if got != c.want {
				t.Fatalf("parseRemoteOwnerRepo(%q) = %q, want %q", c.url, got, c.want)
			}
		})
	}
}

func TestResolveGitSHA_EnvOverride(t *testing.T) {
	t.Setenv("GITHUB_SHA", "deadbeefcafebabe")
	if got := resolveGitSHA("/does/not/exist"); got != "deadbeefcafebabe" {
		t.Fatalf("env override ignored: got %q", got)
	}
}

func TestResolveGitSHA_WorktreeFallback(t *testing.T) {
	t.Setenv("GITHUB_SHA", "")
	dir := gitFixture(t)
	got := resolveGitSHA(dir)
	if len(got) != 40 {
		t.Fatalf("expected a 40-char SHA from the fixture worktree; got %q (len %d)", got, len(got))
	}
}

func TestResolveGitSHA_NotARepo(t *testing.T) {
	t.Setenv("GITHUB_SHA", "")
	dir := t.TempDir()
	if got := resolveGitSHA(dir); got != "" {
		t.Fatalf("expected empty for a non-repo; got %q", got)
	}
}

func TestResolveGitSHA_EmptyContext(t *testing.T) {
	t.Setenv("GITHUB_SHA", "")
	if got := resolveGitSHA(""); got != "" {
		t.Fatalf("expected empty for empty contextDir; got %q", got)
	}
}

func TestResolveGitRepository_EnvOverride(t *testing.T) {
	t.Setenv("GITHUB_REPOSITORY", "my-org/my-repo")
	if got := resolveGitRepository("/does/not/exist"); got != "my-org/my-repo" {
		t.Fatalf("env override ignored: got %q", got)
	}
}

func TestResolveGitRepository_WorktreeFallback(t *testing.T) {
	t.Setenv("GITHUB_REPOSITORY", "")
	dir := gitFixture(t)
	// Set a remote.origin.url so the fallback has something to parse.
	runGit(t, dir, "remote", "add", "origin", "https://github.com/test-org/test-repo.git")
	if got := resolveGitRepository(dir); got != "test-org/test-repo" {
		t.Fatalf("expected test-org/test-repo; got %q", got)
	}
}

func TestResolveGitRepository_NoRemote(t *testing.T) {
	t.Setenv("GITHUB_REPOSITORY", "")
	dir := gitFixture(t)
	if got := resolveGitRepository(dir); got != "" {
		t.Fatalf("expected empty when no remote; got %q", got)
	}
}

// gitFixture makes a minimal git worktree in a temp dir and returns its path.
// One commit, no remote. Caller can `git remote add origin ...` if testing
// the remote-parsing path.
func gitFixture(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	runGit(t, dir, "init", "-q", "-b", "main")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test")
	runGit(t, dir, "config", "commit.gpgsign", "false")
	runGit(t, dir, "commit", "--allow-empty", "-q", "-m", "fixture")
	return dir
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL="+filepath.Join(dir, "no-such-global-config"))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}
