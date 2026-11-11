// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package docker

import (
	"os"
	"os/exec"
	"regexp"
	"strings"
)

// resolveGitSHA returns the current commit SHA for the identity build-arg
// GITHUB_SHA, preferring $GITHUB_SHA (GitHub Actions / any CI exporting the
// same env) and falling back to `git -C <contextDir> rev-parse HEAD` so a
// local `sc deploy` from a cloned repo still carries the SHA. Returns "" if
// neither source yields a value — the caller omits the build-arg so the
// Dockerfile's `ARG GITHUB_SHA=dev` default fires honestly instead of a
// mis-labelled empty string being baked through `-X main.commitSHA=`.
//
// Deliberately silent on error: an unset-env plus not-a-git-repo is a
// legitimate state (building an image from a tarballed context outside any
// repo), not a failure mode worth surfacing at deploy time.
//
// forge-roadmap item: sc-auto-inject-github-sha-and-repository-build-args.
func resolveGitSHA(contextDir string) string {
	if v := strings.TrimSpace(os.Getenv("GITHUB_SHA")); v != "" {
		return v
	}
	if contextDir == "" {
		return ""
	}
	out, err := exec.Command("git", "-C", contextDir, "rev-parse", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// resolveGitRepository returns the repository identifier for the identity
// build-arg GITHUB_REPOSITORY, preferring $GITHUB_REPOSITORY (set by GHA
// as `<owner>/<repo>`) and falling back to parsing the local git worktree's
// `remote.origin.url` into the same `<owner>/<repo>` shape.
//
// Parses both URL dialects git uses in the wild:
//
//	git@github.com:owner/repo.git           →  owner/repo
//	https://github.com/owner/repo.git        →  owner/repo
//	https://gitlab.example/group/repo        →  group/repo
//	ssh://git@bitbucket.org/owner/repo.git   →  owner/repo
//
// The `.git` suffix is optional and stripped. Hosts other than github are
// accepted — the GITHUB_REPOSITORY arg name is kept for CI/GHA convention
// compatibility even when the identifier doesn't originate from GitHub.
//
// Returns "" when neither source yields a value or when the remote URL
// doesn't match the expected shape. Same deliberate silence as resolveGitSHA.
func resolveGitRepository(contextDir string) string {
	if v := strings.TrimSpace(os.Getenv("GITHUB_REPOSITORY")); v != "" {
		return v
	}
	if contextDir == "" {
		return ""
	}
	out, err := exec.Command("git", "-C", contextDir, "config", "--get", "remote.origin.url").Output()
	if err != nil {
		return ""
	}
	return parseRemoteOwnerRepo(strings.TrimSpace(string(out)))
}

// parseRemoteOwnerRepo extracts <owner>/<repo> from a git remote URL.
// Matches the last two `/`- or `:`-separated path segments, trimming any
// `.git` suffix on the second. Returns "" on unrecognised shapes.
var remoteOwnerRepoRe = regexp.MustCompile(`[:/]([^:/]+)/([^/]+?)(?:\.git)?/?$`)

func parseRemoteOwnerRepo(url string) string {
	m := remoteOwnerRepoRe.FindStringSubmatch(url)
	if len(m) != 3 {
		return ""
	}
	owner, repo := m[1], strings.TrimSuffix(m[2], ".git")
	if owner == "" || repo == "" || repo == ".git" {
		return ""
	}
	return owner + "/" + repo
}
