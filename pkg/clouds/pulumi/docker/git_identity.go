// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package docker

import "github.com/simple-container-com/api/pkg/util/gitid"

// The git identity resolvers moved to pkg/util/gitid — the plugin
// sidecars need the same values to put on the bus (RequestHeader's
// deploy_commit / deploy_repository), and resolving a commit SHA was
// never a docker concern.
//
// These remain as the names this package's call sites already use.
//
// forge-roadmap item: sc-auto-inject-github-sha-and-repository-build-args.

// resolveGitSHA returns the current commit SHA for the identity
// build-arg GITHUB_SHA. See gitid.SHA.
func resolveGitSHA(contextDir string) string { return gitid.SHA(contextDir) }

// resolveGitRepository returns the `<owner>/<repo>` identifier for the
// identity build-arg GITHUB_REPOSITORY. See gitid.Repository.
func resolveGitRepository(contextDir string) string { return gitid.Repository(contextDir) }

// parseRemoteOwnerRepo is kept for this package's tests, which cover the
// remote-URL dialects. See gitid.ParseRemoteOwnerRepo.
func parseRemoteOwnerRepo(url string) string { return gitid.ParseRemoteOwnerRepo(url) }
