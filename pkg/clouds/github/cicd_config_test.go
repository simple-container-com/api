// SPDX-License-Identifier: MIT
// Copyright (c) 2025 Simple Container

package github

import (
	"testing"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/api"
)

func TestConvertToGitHubActionsCiCdConfig(t *testing.T) {
	t.Run("nil config returns defaults", func(t *testing.T) {
		RegisterTestingT(t)
		got, err := ConvertToGitHubActionsCiCdConfig(nil)
		Expect(err).ToNot(HaveOccurred())
		Expect(got.Organization).To(Equal("simple-container-org"))
		Expect(got.Environments).To(HaveKey("staging"))
		Expect(got.Environments).To(HaveKey("production"))
		Expect(got.WorkflowGeneration.Enabled).To(BeTrue())
		Expect(got.WorkflowGeneration.Templates).To(ContainElement("deploy"))
	})

	t.Run("empty inner config returns defaults", func(t *testing.T) {
		RegisterTestingT(t)
		got, err := ConvertToGitHubActionsCiCdConfig(&api.Config{})
		Expect(err).ToNot(HaveOccurred())
		Expect(got.Organization).To(Equal("simple-container-org"))
	})

	t.Run("populated config is converted", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := &api.Config{Config: map[string]any{
			"organization": "myorg",
			"environments": map[string]any{
				"prod": map[string]any{"type": "production", "runner": "ubuntu-latest"},
			},
			"workflow-generation": map[string]any{
				"enabled":   true,
				"templates": []any{"deploy"},
			},
		}}
		got, err := ConvertToGitHubActionsCiCdConfig(cfg)
		Expect(err).ToNot(HaveOccurred())
		Expect(got.Organization).To(Equal("myorg"))
		Expect(got.Environments).To(HaveKey("prod"))
		Expect(got.Environments["prod"].Type).To(Equal("production"))
		Expect(got.WorkflowGeneration.Templates).To(Equal([]string{"deploy"}))
		// CustomActions defaulted to non-nil empty map.
		Expect(got.WorkflowGeneration.CustomActions).ToNot(BeNil())
	})

	t.Run("partial config gets field defaults", func(t *testing.T) {
		RegisterTestingT(t)
		// organization omitted -> defaulted; environments omitted -> defaulted.
		cfg := &api.Config{Config: map[string]any{
			"notifications": map[string]any{},
		}}
		got, err := ConvertToGitHubActionsCiCdConfig(cfg)
		Expect(err).ToNot(HaveOccurred())
		Expect(got.Organization).To(Equal("simple-container-org"))
		Expect(got.Environments).To(HaveKey("staging"))
		Expect(got.WorkflowGeneration.Templates).To(ContainElements("deploy", "destroy"))
	})
}

func TestReadCiCdConfig(t *testing.T) {
	RegisterTestingT(t)

	cfg := &api.Config{Config: map[string]any{
		"organization": "readorg",
	}}
	out, err := ReadCiCdConfig(cfg)
	Expect(err).ToNot(HaveOccurred())

	gh, ok := out.Config.(*GitHubActionsCiCdConfig)
	Expect(ok).To(BeTrue())
	Expect(gh.Organization).To(Equal("readorg"))
	// defaults backfilled
	Expect(gh.Environments).To(HaveKey("staging"))
	Expect(gh.WorkflowGeneration.Templates).To(ContainElement("deploy"))
	Expect(gh.WorkflowGeneration.CustomActions).ToNot(BeNil())
}

func TestReadEnhancedCiCdConfig(t *testing.T) {
	RegisterTestingT(t)

	cfg := &api.Config{Config: map[string]any{
		"auth-token":   "tok",
		"organization": map[string]any{"name": "enh"},
	}}
	out, err := ReadEnhancedCiCdConfig(cfg)
	Expect(err).ToNot(HaveOccurred())

	enh, ok := out.Config.(*EnhancedActionsCiCdConfig)
	Expect(ok).To(BeTrue())
	Expect(enh.AuthToken).To(Equal("tok"))
	Expect(enh.Organization.Name).To(Equal("enh"))
	// SetDefaults applied
	Expect(enh.Organization.DefaultBranch).To(Equal("main"))
	Expect(enh.Execution.DefaultTimeout).To(Equal("30m"))
}

func TestGetRequiredSecrets(t *testing.T) {
	RegisterTestingT(t)

	c := &EnhancedActionsCiCdConfig{
		Organization: OrganizationConfig{RequiredSecrets: []string{"ORG_SECRET", "SHARED"}},
		Environments: map[string]EnvironmentConfig{
			"a": {Secrets: []string{"A_SECRET", "SHARED"}},
			"b": {Secrets: []string{"B_SECRET"}},
		},
	}
	got := c.GetRequiredSecrets()
	Expect(got).To(ConsistOf("ORG_SECRET", "SHARED", "A_SECRET", "B_SECRET"))
}

func TestGetRequiredSecrets_Empty(t *testing.T) {
	RegisterTestingT(t)
	Expect((&EnhancedActionsCiCdConfig{}).GetRequiredSecrets()).To(BeEmpty())
}

func TestGetEnvironmentsByType(t *testing.T) {
	RegisterTestingT(t)

	c := &EnhancedActionsCiCdConfig{
		Environments: map[string]EnvironmentConfig{
			"s1":   {Type: "staging"},
			"s2":   {Type: "staging"},
			"p1":   {Type: "production"},
			"prev": {Type: "preview"},
		},
	}
	Expect(c.GetStagingEnvironments()).To(HaveLen(2))
	Expect(c.GetProductionEnvironments()).To(HaveKey("p1"))
	Expect(c.GetPreviewEnvironments()).To(HaveKey("prev"))
	Expect(c.GetEnvironmentsByType("nope")).To(BeEmpty())
}

func TestIsWorkflowGenerationEnabled(t *testing.T) {
	RegisterTestingT(t)
	Expect((&EnhancedActionsCiCdConfig{WorkflowGeneration: WorkflowGenerationConfig{Enabled: true}}).IsWorkflowGenerationEnabled()).To(BeTrue())
	Expect((&EnhancedActionsCiCdConfig{}).IsWorkflowGenerationEnabled()).To(BeFalse())
}

func TestActionsCiCdConfig_LegacyGetters(t *testing.T) {
	RegisterTestingT(t)
	// Enhanced config getters (ProjectId from org name).
	enh := &EnhancedActionsCiCdConfig{AuthToken: "z", Organization: OrganizationConfig{Name: "org"}}
	Expect(enh.CredentialsValue()).To(Equal("z"))
	Expect(enh.ProjectIdValue()).To(Equal("org"))
	Expect(enh.ProviderType()).To(Equal(ProviderType))
}
