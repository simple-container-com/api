// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

// Package gitid resolves the identity of the code being deployed — the
// commit SHA and the `<owner>/<repo>` identifier.
//
// Lives here rather than in the docker package it started in because it
// is not a docker concern. Two callers need it now: the image builder,
// which passes GITHUB_SHA / GITHUB_REPOSITORY as build args, and the
// plugin sidecars, which put the same values on the bus so a plugin can
// report what it deployed.
//
// Both resolvers prefer the CI environment and fall back to the local
// git worktree, so a `sc deploy` from a clone behaves like one from a
// GitHub Actions runner. Both are deliberately SILENT on failure: an
// unset env plus a non-git directory is a legitimate state — building an
// image from a tarballed context outside any repo — and not a failure
// worth surfacing at deploy time. The caller omits the value instead, so
// a Dockerfile's own `ARG GITHUB_SHA=dev` default fires honestly rather
// than an empty string being baked through `-X main.commitSHA=`.
package gitid

import (
	"os"
	"os/exec"
	"regexp"
	"strings"
)

// SHA returns the current commit SHA, preferring $GITHUB_SHA (GitHub
// Actions, or any CI exporting the same name) and falling back to
// `git -C <dir> rev-parse HEAD`.
//
// Returns the FULL sha. The short form is derivable; the long form is
// not, which matters because a CalVer deploy version carries only the
// short one.
func SHA(dir string) string {
	if v := strings.TrimSpace(os.Getenv("GITHUB_SHA")); v != "" {
		return v
	}
	if dir == "" {
		return ""
	}
	out, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// Repository returns the repository identifier, preferring
// $GITHUB_REPOSITORY (set by GHA as `<owner>/<repo>`) and falling back to
// parsing the local worktree's `remote.origin.url` into the same shape.
//
// Parses both URL dialects git uses in the wild:
//
//	git@github.com:owner/repo.git           →  owner/repo
//	https://github.com/owner/repo.git       →  owner/repo
//	https://gitlab.example/group/repo       →  group/repo
//	ssh://git@bitbucket.org/owner/repo.git  →  owner/repo
//
// Hosts other than GitHub are accepted — the GITHUB_* naming is kept for
// CI convention compatibility even when the identifier does not come
// from GitHub.
func Repository(dir string) string {
	if v := strings.TrimSpace(os.Getenv("GITHUB_REPOSITORY")); v != "" {
		return v
	}
	if dir == "" {
		return ""
	}
	out, err := exec.Command("git", "-C", dir, "config", "--get", "remote.origin.url").Output()
	if err != nil {
		return ""
	}
	return ParseRemoteOwnerRepo(strings.TrimSpace(string(out)))
}

// remoteOwnerRepoRe matches the last two `/`- or `:`-separated path
// segments, trimming an optional `.git` suffix on the second.
var remoteOwnerRepoRe = regexp.MustCompile(`[:/]([^:/]+)/([^/]+?)(?:\.git)?/?$`)

// ParseRemoteOwnerRepo extracts `<owner>/<repo>` from a git remote URL,
// returning "" on an unrecognised shape.
func ParseRemoteOwnerRepo(url string) string {
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
