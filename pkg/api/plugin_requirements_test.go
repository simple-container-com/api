// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package api

import (
	"fmt"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
)

func TestValidatePluginRequirements_AutoInjectsLatest(t *testing.T) {
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

	stubResolver(func(repo string) (string, error) {
		Expect(repo).To(Equal("simple-container-com/sc-plugin-cloudflare"))
		return "v2026.10.26-5eddde3", nil
	})
	defer restoreResolver()

	desc := &ServerDescriptor{
		Resources: PerStackResourcesDescriptor{
			Registrar: RegistrarDescriptor{Type: "cloudflare"},
		},
	}

	err := ValidatePluginRequirements(desc)
	Expect(err).ToNot(HaveOccurred(), "resolver succeeded so validator must auto-inject, not error")
	Expect(desc.Plugins).To(HaveKey("cloudflare"), "missing plugin block must be auto-injected")
	Expect(desc.Plugins["cloudflare"].Source).ToNot(BeNil())
	Expect(desc.Plugins["cloudflare"].Source.Version).To(Equal("v2026.10.26-5eddde3"))
	Expect(desc.Plugins["cloudflare"].Source.Repository).To(Equal("simple-container-com/sc-plugin-cloudflare"))
	Expect(desc.Plugins["cloudflare"].Source.Type).To(Equal("github-release"))
}

func TestValidatePluginRequirements_ResolverFailureFallsBackToError(t *testing.T) {
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
	Expect(err).To(HaveOccurred(), "resolver failure must surface — operator needs to pin by hand")
	msg := err.Error()
	Expect(msg).To(ContainSubstring("github api unreachable"))
	Expect(msg).To(ContainSubstring("plugins:"))
	Expect(msg).To(ContainSubstring("cloudflare:"))
	Expect(msg).To(ContainSubstring("source:"))
	Expect(msg).To(ContainSubstring("simple-container-com/sc-plugin-cloudflare"))
	Expect(msg).To(ContainSubstring("resources.registrar.type"))
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

	stubResolver(func(repo string) (string, error) {
		return "", fmt.Errorf("resolver must not be called when operator declared the plugin")
	})
	defer restoreResolver()

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
	Expect(desc.Plugins["cloudflare"].Source.Version).To(Equal("v2026.10.25-93a25ef"),
		"operator's pin must survive untouched")
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

	stubResolver(func(repo string) (string, error) {
		return "", fmt.Errorf("resolver must not be called — no plugin-owned types used")
	})
	defer restoreResolver()

	desc := &ServerDescriptor{
		Resources: PerStackResourcesDescriptor{
			Registrar: RegistrarDescriptor{Type: "yc-dns"},
		},
	}

	err := ValidatePluginRequirements(desc)
	Expect(err).ToNot(HaveOccurred(), "unrelated types must not trip the validator")
}

func TestValidatePluginRequirements_MultipleMissingAutoInjected(t *testing.T) {
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

	stubResolver(func(repo string) (string, error) {
		switch repo {
		case "simple-container-com/sc-plugin-cloudflare":
			return "v2026.10.26-5eddde3", nil
		case "simple-container-com/sc-plugin-yandex":
			return "v2026.10.26-abcdef0", nil
		}
		return "", fmt.Errorf("unexpected repo %s", repo)
	})
	defer restoreResolver()

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
	Expect(err).ToNot(HaveOccurred())
	Expect(desc.Plugins).To(HaveKey("cloudflare"))
	Expect(desc.Plugins).To(HaveKey("yandex"))
	Expect(desc.Plugins["cloudflare"].Source.Version).To(Equal("v2026.10.26-5eddde3"))
	Expect(desc.Plugins["yandex"].Source.Version).To(Equal("v2026.10.26-abcdef0"))
}

func TestValidatePluginRequirements_PartialResolverFailure(t *testing.T) {
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
		OwnedTypes: []string{"yc-dns"},
		DefaultSource: PluginSource{
			Type:       "github-release",
			Repository: "simple-container-com/sc-plugin-yandex",
		},
	})
	defer resetPluginRequirementsForTest()

	stubResolver(func(repo string) (string, error) {
		if repo == "simple-container-com/sc-plugin-cloudflare" {
			return "v2026.10.26-5eddde3", nil
		}
		return "", fmt.Errorf("yandex release endpoint 503")
	})
	defer restoreResolver()

	desc := &ServerDescriptor{
		Resources: PerStackResourcesDescriptor{
			Registrars: map[string]RegistrarDescriptor{
				"cf": {Type: "cloudflare"},
				"yc": {Type: "yc-dns"},
			},
		},
	}

	err := ValidatePluginRequirements(desc)
	Expect(err).To(HaveOccurred(), "any single resolver failure must surface")
	msg := err.Error()
	// Error names the FAILED plugin only, so operators know which block to paste.
	Expect(msg).To(ContainSubstring("yandex"))
	Expect(msg).To(ContainSubstring("yandex release endpoint 503"))
	// The CF side still got auto-injected.
	Expect(desc.Plugins).To(HaveKey("cloudflare"))
	Expect(desc.Plugins["cloudflare"].Source.Version).To(Equal("v2026.10.26-5eddde3"))
	// The YC side did not.
	_, present := desc.Plugins["yandex"]
	Expect(present).To(BeFalse())
}

// Keep alphabetical ordering of the paste-block in the error message —
// operators get the same output on every run.
func TestValidatePluginRequirements_AlphabeticalErrorOrder(t *testing.T) {
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
		OwnedTypes: []string{"yc-dns"},
		DefaultSource: PluginSource{
			Type:       "github-release",
			Repository: "simple-container-com/sc-plugin-yandex",
		},
	})
	defer resetPluginRequirementsForTest()

	stubResolver(func(repo string) (string, error) {
		return "", fmt.Errorf("offline")
	})
	defer restoreResolver()

	desc := &ServerDescriptor{
		Resources: PerStackResourcesDescriptor{
			Registrars: map[string]RegistrarDescriptor{
				"yc": {Type: "yc-dns"},
				"cf": {Type: "cloudflare"},
			},
		},
	}

	err := ValidatePluginRequirements(desc)
	Expect(err).To(HaveOccurred())
	msg := err.Error()
	Expect(strings.Index(msg, "cloudflare:")).To(BeNumerically("<", strings.Index(msg, "yandex:")))
}

// stubResolver / restoreResolver swap out the package-level
// resolveLatestVersion so unit tests never hit the network.
var realResolver = resolveLatestVersion

func stubResolver(fn func(repo string) (string, error)) { resolveLatestVersion = fn }
func restoreResolver()                                  { resolveLatestVersion = realResolver }

// resetPluginRequirementsForTest clears the package-level registry so
// tests don't leak state into each other.
func resetPluginRequirementsForTest() {
	pluginRequirementsMu.Lock()
	defer pluginRequirementsMu.Unlock()
	pluginRequirements = nil
	pluginByType = map[string]PluginRequirement{}
}
