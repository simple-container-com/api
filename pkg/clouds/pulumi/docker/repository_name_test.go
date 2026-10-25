// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package docker

import (
	"testing"

	. "github.com/onsi/gomega"
)

func TestRegistryRequiresSingleSeparators(t *testing.T) {
	RegisterTestingT(t)

	// Registries that enforce the legacy grammar, in the shapes a client.yaml or
	// a driver actually produces.
	for _, server := range []string{
		"cr.yandex",
		"cr.yandex/crp1234567890abcdefg",
		"https://cr.yandex/crp1234567890abcdefg",
		"rusmokereg.cr.cloud.ru",
		"RUSMOKEREG.CR.CLOUD.RU",
		"rusmokereg.cr.cloud.ru/",
		"https://rusmokereg.cr.cloud.ru/28650be3-6c51-4cd1-8632-e21308743f74",
	} {
		Expect(RegistryRequiresSingleSeparators(server)).To(BeTrue(), "expected %q to require single separators", server)
	}

	// Registries that accept `--`. Rewriting these would move the repository of
	// every already-deployed stack, so they must be left alone.
	for _, server := range []string{
		"",
		"docker.io",
		"index.docker.io/simplecontainer",
		"123456789012.dkr.ecr.us-east-1.amazonaws.com",
		"ghcr.io/simple-container-com",
		"europe-west1-docker.pkg.dev/my-project/my-repo",
		"localhost:5000",
		// Near-misses: the suffix match is on a dot boundary, so a host that
		// merely ENDS with the same letters is not swept in.
		"evilcr.yandex.com",
		"notcr.cloud.ru.example.com",
	} {
		Expect(RegistryRequiresSingleSeparators(server)).To(BeFalse(), "expected %q NOT to require single separators", server)
	}
}

func TestToLegacyRepositoryName(t *testing.T) {
	RegisterTestingT(t)

	// The names SC actually generates. `<stack>--<env>` comes from the single-image
	// path; `<stack>--<env>--<service>` from kubernetes/images_build.go, which is
	// the one that failed live against Cloud.ru AR on 2026-10-23.
	Expect(ToLegacyRepositoryName("atrium--staging")).To(Equal("atrium-staging"))
	Expect(ToLegacyRepositoryName("ruk3sapp--staging--api")).To(Equal("ruk3sapp-staging-api"))

	// Already legal: unchanged, so the rewrite is a no-op on the Yandex path
	// (where toRepositoryName has collapsed the name one layer up) and on any
	// hand-written name.
	Expect(ToLegacyRepositoryName("atrium-staging")).To(Equal("atrium-staging"))
	Expect(ToLegacyRepositoryName("atrium_staging")).To(Equal("atrium_staging"))
	Expect(ToLegacyRepositoryName("atrium.staging")).To(Equal("atrium.staging"))

	// Longer runs and mixed separators collapse to the first character of the run.
	Expect(ToLegacyRepositoryName("atrium----staging")).To(Equal("atrium-staging"))
	Expect(ToLegacyRepositoryName("atrium__staging")).To(Equal("atrium_staging"))
	Expect(ToLegacyRepositoryName("atrium-_-staging")).To(Equal("atrium-staging"))

	// The grammar is anchored, so a leading or trailing separator is rejected too.
	Expect(ToLegacyRepositoryName("-atrium-staging-")).To(Equal("atrium-staging"))
	Expect(ToLegacyRepositoryName("__atrium__")).To(Equal("atrium"))

	// Uppercase is rejected by the same grammar (the name is a path segment, not
	// a tag).
	Expect(ToLegacyRepositoryName("Atrium--Staging")).To(Equal("atrium-staging"))

	// A registry path prefix is validated segment by segment, so trimming is per
	// segment and the separators between them survive.
	Expect(ToLegacyRepositoryName("my--project/atrium--staging")).To(Equal("my-project/atrium-staging"))
	Expect(ToLegacyRepositoryName("proj/-atrium-/api")).To(Equal("proj/atrium/api"))
}

func TestRepositoryPathFor(t *testing.T) {
	RegisterTestingT(t)

	// The live failure and its fix: this exact name, on this exact registry, is
	// what `sc deploy -s ruk3sapp -e staging` pushed and Cloud.ru AR refused
	// with `invalid repository name`.
	Expect(repositoryPathFor("rusmokereg.cr.cloud.ru", "ruk3sapp--staging--api")).
		To(Equal("ruk3sapp-staging-api"))
	Expect(repositoryPathFor("cr.yandex/crp123", "atrium--staging")).
		To(Equal("atrium-staging"))

	// Everywhere else the name is passed through byte for byte — including the
	// case the rewrite WOULD have changed. This is the regression that matters:
	// the fleet's images live at `<stack>--<env>` on ECR and GHCR today.
	Expect(repositoryPathFor("123456789012.dkr.ecr.us-east-1.amazonaws.com", "atrium--staging")).
		To(Equal("atrium--staging"))
	Expect(repositoryPathFor("ghcr.io/simple-container-com", "forge--staging--api")).
		To(Equal("forge--staging--api"))
	Expect(repositoryPathFor("", "atrium--staging")).To(Equal("atrium--staging"))
}
