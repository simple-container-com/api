// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package api

import (
	"fmt"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
)

// Empty schemaVersion is accepted as implicit 1.0 for backwards
// compatibility with every server.yaml written before the field became
// a strictness switch.
func TestValidateSchemaVersion_EmptyAcceptedAsImplicitOneZero(t *testing.T) {
	RegisterTestingT(t)
	desc := &ServerDescriptor{}
	Expect(desc.ValidateSchemaVersion()).To(Succeed())
	Expect(desc.IsStrictSchemaVersion()).To(BeFalse())
}

func TestValidateSchemaVersion_KnownVersionsAccepted(t *testing.T) {
	RegisterTestingT(t)
	for _, v := range []string{"1.0", "1.1"} {
		desc := &ServerDescriptor{SchemaVersion: v}
		Expect(desc.ValidateSchemaVersion()).To(Succeed(), "schemaVersion %q", v)
	}
}

func TestValidateSchemaVersion_UnknownRejectedWithDocsLink(t *testing.T) {
	RegisterTestingT(t)
	desc := &ServerDescriptor{SchemaVersion: "99.0"}
	err := desc.ValidateSchemaVersion()
	Expect(err).To(HaveOccurred())
	msg := err.Error()
	Expect(msg).To(ContainSubstring("unsupported schemaVersion"))
	Expect(msg).To(ContainSubstring("99.0"))
	Expect(msg).To(ContainSubstring(UpgradingToPluginsDocsURL))
}

func TestIsStrictSchemaVersion(t *testing.T) {
	RegisterTestingT(t)
	Expect((&ServerDescriptor{SchemaVersion: "1.1"}).IsStrictSchemaVersion()).To(BeTrue())
	Expect((&ServerDescriptor{SchemaVersion: "1.0"}).IsStrictSchemaVersion()).To(BeFalse())
	Expect((&ServerDescriptor{SchemaVersion: ""}).IsStrictSchemaVersion()).To(BeFalse())
}

// In strict mode (schemaVersion 1.1), the validator must NOT consult the
// GitHub resolver — the error should name the paste-this-block fix and
// the upgrade-guide link, without any network call.
func TestValidatePluginRequirements_StrictMode_NoResolverCall(t *testing.T) {
	RegisterTestingT(t)
	resetPluginRequirementsAndRebootstrap()
	defer resetPluginRequirementsForTest()

	stubResolver(func(repo string) (string, error) {
		return "", fmt.Errorf("resolver must not be called in strict mode (repo=%s)", repo)
	})
	defer restoreResolver()

	desc := &ServerDescriptor{
		SchemaVersion: ServerSchemaVersion11,
		Resources: PerStackResourcesDescriptor{
			Registrar: RegistrarDescriptor{Type: "cloudflare"},
		},
	}

	err := ValidatePluginRequirements(desc)
	Expect(err).To(HaveOccurred(), "strict mode must reject undeclared plugins without a network call")
	msg := err.Error()
	Expect(msg).To(ContainSubstring("schemaVersion \"1.1\""))
	Expect(msg).To(ContainSubstring("explicit `plugins:` declaration"))
	Expect(msg).To(ContainSubstring("plugins:"))
	Expect(msg).To(ContainSubstring("cloudflare:"))
	Expect(msg).To(ContainSubstring("simple-container-com/sc-plugin-cloudflare"))
	Expect(msg).To(ContainSubstring(UpgradingToPluginsDocsURL))
}

// The lenient (1.0 / implicit) auto-inject error must ALSO include the
// upgrade-guide link, since the fallback is one release away from being
// removed.
func TestValidatePluginRequirements_LenientErrorIncludesDocsLink(t *testing.T) {
	RegisterTestingT(t)
	resetPluginRequirementsAndRebootstrap()
	defer resetPluginRequirementsForTest()

	stubResolver(func(repo string) (string, error) {
		return "", fmt.Errorf("github api unreachable")
	})
	defer restoreResolver()

	desc := &ServerDescriptor{
		Resources: PerStackResourcesDescriptor{
			Registrar: RegistrarDescriptor{Type: "cloudflare"},
		},
	}

	err := ValidatePluginRequirements(desc)
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring(UpgradingToPluginsDocsURL))
}

// unknownTypeHint should recognise a known plugin-owned token and emit
// a paste-this-block hint plus the docs link.
func TestUnknownTypeHint_KnownPluginToken(t *testing.T) {
	RegisterTestingT(t)
	resetPluginRequirementsAndRebootstrap()
	defer resetPluginRequirementsForTest()

	hint := unknownTypeHint("aws-lambda")
	Expect(hint).To(ContainSubstring("aws"))
	Expect(hint).To(ContainSubstring("plugins:"))
	Expect(hint).To(ContainSubstring("source:"))
	Expect(hint).To(ContainSubstring("simple-container-com/sc-plugin-aws"))
	Expect(hint).To(ContainSubstring(UpgradingToPluginsDocsURL))
}

// For an unknown token (neither in-tree nor bootstrap-registered), the
// hint still points at the upgrade guide so an operator sees a path
// forward instead of only "unknown type".
func TestUnknownTypeHint_UnknownTokenStillPointsAtDocs(t *testing.T) {
	RegisterTestingT(t)
	resetPluginRequirementsAndRebootstrap()
	defer resetPluginRequirementsForTest()

	hint := unknownTypeHint("totally-unknown-type")
	Expect(hint).To(ContainSubstring(UpgradingToPluginsDocsURL))
	Expect(hint).ToNot(ContainSubstring("owned by"), "unknown token must not claim ownership")
}

func TestUnknownTypeHint_EmptyToken(t *testing.T) {
	RegisterTestingT(t)
	Expect(unknownTypeHint("")).To(BeEmpty())
}

// Belt-and-braces: the strict-mode error builder lists ALL unresolved
// plugins (not just one) so a server.yaml using several out-of-tree
// types gets one error with every paste-this-block hint.
func TestValidatePluginRequirements_StrictMode_ListsAllMissing(t *testing.T) {
	RegisterTestingT(t)
	resetPluginRequirementsAndRebootstrap()
	defer resetPluginRequirementsForTest()

	desc := &ServerDescriptor{
		SchemaVersion: ServerSchemaVersion11,
		Resources: PerStackResourcesDescriptor{
			Registrars: map[string]RegistrarDescriptor{
				"cf": {Type: "cloudflare"},
				"yc": {Type: "yc-dns"},
			},
		},
	}

	err := ValidatePluginRequirements(desc)
	Expect(err).To(HaveOccurred())
	msg := err.Error()
	// Alphabetical ordering — cloudflare before yandex.
	Expect(strings.Index(msg, "cloudflare:")).To(BeNumerically("<", strings.Index(msg, "yandex:")))
	// Both repos named in the paste blocks.
	Expect(msg).To(ContainSubstring("simple-container-com/sc-plugin-cloudflare"))
	Expect(msg).To(ContainSubstring("simple-container-com/sc-plugin-yandex"))
}
