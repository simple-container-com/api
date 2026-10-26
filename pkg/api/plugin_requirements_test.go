// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package api

import (
	"strings"
	"testing"

	. "github.com/onsi/gomega"
)

func TestValidatePluginRequirements_MissingBlock(t *testing.T) {
	RegisterTestingT(t)

	// Simulate what sc-plugin-cloudflare/api's init() would do.
	RegisterPluginRequirement(PluginRequirement{
		PluginName: "cloudflare",
		OwnedTypes: []string{"cloudflare"},
		DefaultSource: PluginSource{
			Type:       "github-release",
			Repository: "simple-container-com/sc-plugin-cloudflare",
		},
	})
	defer resetPluginRequirementsForTest()

	// A server.yaml that uses `type: cloudflare` but never declares a
	// `plugins:` block — the classic bad-upgrade shape.
	desc := &ServerDescriptor{
		Resources: PerStackResourcesDescriptor{
			Registrar: RegistrarDescriptor{Type: "cloudflare"},
		},
	}

	err := ValidatePluginRequirements(desc)
	Expect(err).To(HaveOccurred(), "expected validation to flag the missing plugin block")
	msg := err.Error()
	Expect(msg).To(ContainSubstring("plugins:"))
	Expect(msg).To(ContainSubstring("cloudflare:"))
	Expect(msg).To(ContainSubstring("source:"))
	Expect(msg).To(ContainSubstring("simple-container-com/sc-plugin-cloudflare"))
	// Error must name the exact field that triggered the gap.
	Expect(msg).To(ContainSubstring("resources.registrar.type"))
	// Must point to the releases page.
	Expect(msg).To(ContainSubstring("github.com/simple-container-com/sc-plugin-cloudflare/releases"))
}

func TestValidatePluginRequirements_BlockPresent(t *testing.T) {
	RegisterTestingT(t)

	RegisterPluginRequirement(PluginRequirement{
		PluginName: "cloudflare",
		OwnedTypes: []string{"cloudflare"},
		DefaultSource: PluginSource{
			Type:       "github-release",
			Repository: "simple-container-com/sc-plugin-cloudflare",
		},
	})
	defer resetPluginRequirementsForTest()

	desc := &ServerDescriptor{
		Resources: PerStackResourcesDescriptor{
			Registrar: RegistrarDescriptor{Type: "cloudflare"},
		},
		Plugins: map[string]PluginSpec{
			"cloudflare": {
				Source: &PluginSource{
					Type:       "github-release",
					Repository: "simple-container-com/sc-plugin-cloudflare",
					Version:    "v2026.10.25-93a25ef",
				},
			},
		},
	}

	err := ValidatePluginRequirements(desc)
	Expect(err).ToNot(HaveOccurred(), "expected clean validation when plugin pinned")
}

func TestValidatePluginRequirements_UnrelatedTypesSilent(t *testing.T) {
	RegisterTestingT(t)

	RegisterPluginRequirement(PluginRequirement{
		PluginName: "cloudflare",
		OwnedTypes: []string{"cloudflare"},
		DefaultSource: PluginSource{
			Type:       "github-release",
			Repository: "simple-container-com/sc-plugin-cloudflare",
		},
	})
	defer resetPluginRequirementsForTest()

	// Nothing in this descriptor uses a plugin-owned type.
	desc := &ServerDescriptor{
		Resources: PerStackResourcesDescriptor{
			Registrar: RegistrarDescriptor{Type: "yc-dns"}, // in-tree
		},
	}

	err := ValidatePluginRequirements(desc)
	Expect(err).ToNot(HaveOccurred(), "unrelated types must not trip the validator")
}

func TestValidatePluginRequirements_MultipleMissingPlugins(t *testing.T) {
	RegisterTestingT(t)

	RegisterPluginRequirement(PluginRequirement{
		PluginName: "cloudflare",
		OwnedTypes: []string{"cloudflare"},
		DefaultSource: PluginSource{
			Type:       "github-release",
			Repository: "simple-container-com/sc-plugin-cloudflare",
		},
	})
	RegisterPluginRequirement(PluginRequirement{
		PluginName: "yandex",
		OwnedTypes: []string{"yc-dns", "yc-bucket", "yc-serverless-container", "yc-object-storage"},
		DefaultSource: PluginSource{
			Type:       "github-release",
			Repository: "simple-container-com/sc-plugin-yandex",
		},
	})
	defer resetPluginRequirementsForTest()

	// A stack that uses both — error should enumerate both missing blocks.
	desc := &ServerDescriptor{
		Resources: PerStackResourcesDescriptor{
			Registrars: map[string]RegistrarDescriptor{
				"cf": {Type: "cloudflare"},
				"yc": {Type: "yc-dns"},
			},
			Resources: map[string]PerEnvResourcesDescriptor{
				"staging": {
					Resources: map[string]ResourceDescriptor{
						"bucket": {Type: "yc-bucket"},
					},
				},
			},
		},
	}

	err := ValidatePluginRequirements(desc)
	Expect(err).To(HaveOccurred())
	msg := err.Error()
	// Both plugins named.
	Expect(msg).To(ContainSubstring("cloudflare:"))
	Expect(msg).To(ContainSubstring("yandex:"))
	// Alphabetical order so operators get consistent output.
	Expect(strings.Index(msg, "cloudflare:")).To(BeNumerically("<", strings.Index(msg, "yandex:")))
}

// resetPluginRequirementsForTest clears the package-level registry so
// tests don't leak state into each other.
func resetPluginRequirementsForTest() {
	pluginRequirementsMu.Lock()
	defer pluginRequirementsMu.Unlock()
	pluginRequirements = nil
	pluginByType = map[string]PluginRequirement{}
}
