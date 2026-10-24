// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package docker

import (
	"regexp"
	"strings"
)

// legacySeparatorRunRegexp matches a run of two or more consecutive separators
// in a repository name.
var legacySeparatorRunRegexp = regexp.MustCompile(`[._-]{2,}`)

// legacyGrammarRegistrySuffixes lists the registry hosts known to enforce the
// *legacy* Docker repository grammar, `[a-z0-9]+(?:[._-][a-z0-9]+)*` — at most
// ONE separator between components.
//
// Modern Docker clients accept `__` and `--`, so the client forms the request
// happily and the registry refuses it; neither refusal names the repository
// name as the problem in a way a caller can act on:
//
//   - cr.yandex answers a bare `400 Bad Request` to the blob HEAD. Live-caught
//     on the first YC container deploy, 2026-09-26.
//   - Cloud.ru Evolution Artifact Registry answers `invalid repository name`
//     plus a docs URL. Live-caught 2026-10-23 on the first k3s deploy against
//     Cloud.ru: the pushed name was `ruk3sapp--staging--api` (BuildAndPushImages,
//     kubernetes/images_build.go, composes `<stack.Name>--<container>` onto a
//     stack name that is itself `<stack>--<env>`). Its registry-create endpoint
//     enforces the same grammar a step earlier, with
//     `400 invalid CreateRegistryRequest.Name`.
//
// Matched as a SUFFIX so a per-project host like `myreg.cr.cloud.ru` is covered.
//
// This is a host allowlist rather than a blanket collapse on purpose. The legacy
// grammar is a subset of what ECR/GCR/GHCR/Docker Hub accept, so collapsing
// everywhere would be harmless at the registry — but it would rename the
// repository of every already-deployed stack on those registries (SC's image
// name is `<stack>--<env>[--<service>]`, so nearly all of them), orphaning the
// pushed history behind a new path on the fleet's next deploy. The registries
// that cannot accept the name are the ones that get the rewrite.
var legacyGrammarRegistrySuffixes = []string{
	"cr.yandex",
	"cr.cloud.ru",
}

// RegistryRequiresSingleSeparators reports whether a push to this registry has
// to go to a repository name with no doubled separators.
//
// `server` is the registry URL as configured — it may carry a scheme and a path
// (`dockerRegistryURL: rusmokereg.cr.cloud.ru/foo`), so only the host is
// compared.
func RegistryRequiresSingleSeparators(server string) bool {
	host := strings.ToLower(strings.TrimSpace(server))
	if i := strings.Index(host, "://"); i >= 0 {
		host = host[i+3:]
	}
	host = strings.TrimSuffix(host, "/")
	if i := strings.Index(host, "/"); i >= 0 {
		host = host[:i]
	}
	if i := strings.LastIndex(host, ":"); i >= 0 && !strings.Contains(host[i+1:], ".") {
		// A port, not part of the host.
		host = host[:i]
	}
	for _, suffix := range legacyGrammarRegistrySuffixes {
		if host == suffix || strings.HasSuffix(host, "."+suffix) {
			return true
		}
	}
	return false
}

// ToLegacyRepositoryName rewrites a repository name to satisfy the legacy
// grammar described on legacyGrammarRegistrySuffixes.
//
// Runs collapse to their first character (`a--b` -> `a-b`, `a__b` -> `a_b`)
// rather than being hashed or stripped, to keep the registry readable. Two
// stacks whose names differ only in the position of a doubled separator would
// collide, which is accepted: it takes deliberately adversarial naming to hit.
//
// The Yandex driver carries its own copy of this (toRepositoryName,
// pulumi/yandex/docker.go) applied one layer up, where it also feeds the name it
// logs. The two encode the same registry grammar; this one is the general path
// and runs for every template, including the Yandex one — where it is then a
// no-op, because the name arrives already collapsed.
func ToLegacyRepositoryName(name string) string {
	collapsed := legacySeparatorRunRegexp.ReplaceAllStringFunc(strings.ToLower(name), func(run string) string {
		return run[:1]
	})
	// A leading or trailing separator is rejected by the same grammar. Each path
	// segment is validated separately, so trim around the separators too.
	segments := strings.Split(collapsed, "/")
	for i, segment := range segments {
		segments[i] = strings.Trim(segment, "._-")
	}
	return strings.Join(segments, "/")
}

// repositoryPathFor returns the repository path to push `name` to on `server`,
// rewriting it only when that registry cannot accept it as given.
func repositoryPathFor(server, name string) string {
	if !RegistryRequiresSingleSeparators(server) {
		return name
	}
	return ToLegacyRepositoryName(name)
}
