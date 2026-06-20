// SPDX-License-Identifier: MIT
// Copyright (c) 2025 Simple Container

package github

import (
	"testing"
	"time"

	. "github.com/onsi/gomega"
)

func TestActionsCiCdConfig_Getters(t *testing.T) {
	RegisterTestingT(t)

	c := &ActionsCiCdConfig{AuthToken: "ghp_secret_token"}

	Expect(c.CredentialsValue()).To(Equal("ghp_secret_token"))
	Expect(c.ProjectIdValue()).To(Equal(""))
	Expect(c.ProviderType()).To(Equal(ProviderType))
	Expect(c.ProviderType()).To(Equal("github"))
}

func TestActionsCiCdConfig_ZeroValue(t *testing.T) {
	RegisterTestingT(t)

	c := &ActionsCiCdConfig{}
	Expect(c.CredentialsValue()).To(Equal(""))
	Expect(c.ProjectIdValue()).To(Equal(""))
	Expect(c.ProviderType()).To(Equal("github"))
}

func TestEnhancedActionsCiCdConfig_Getters(t *testing.T) {
	RegisterTestingT(t)

	c := &EnhancedActionsCiCdConfig{
		AuthToken:    "enhanced-token",
		Organization: OrganizationConfig{Name: "acme"},
	}

	Expect(c.CredentialsValue()).To(Equal("enhanced-token"))
	Expect(c.ProjectIdValue()).To(Equal("acme"))
	Expect(c.ProviderType()).To(Equal(ProviderType))
}

func TestEnhancedActionsCiCdConfig_SetDefaults(t *testing.T) {
	RegisterTestingT(t)

	c := &EnhancedActionsCiCdConfig{}
	c.SetDefaults()

	Expect(c.Organization.DefaultBranch).To(Equal("main"))
	Expect(c.Organization.DefaultRunner).To(Equal("ubuntu-latest"))
	Expect(c.WorkflowGeneration.OutputPath).To(Equal(".github/workflows/"))
	Expect(c.WorkflowGeneration.Templates).To(ContainElement("deploy"))
	Expect(c.WorkflowGeneration.Templates).To(ContainElement("destroy"))
	Expect(c.WorkflowGeneration.Templates).To(ContainElement("pr-preview"))
	Expect(c.WorkflowGeneration.SCVersion).To(Equal("latest"))
	Expect(c.WorkflowGeneration.CustomActions).To(HaveKey("deploy"))
	Expect(c.WorkflowGeneration.CustomActions["deploy"]).To(ContainSubstring("@main"))
	Expect(c.Execution.DefaultTimeout).To(Equal("30m"))
	Expect(c.Execution.Concurrency.Group).To(ContainSubstring("github.workflow"))
	Expect(c.Execution.Permissions).To(HaveKey("default"))
	Expect(c.Execution.RetryPolicy.MaxAttempts).To(Equal(3))
	Expect(c.Execution.RetryPolicy.BackoffDelay).To(Equal(30 * time.Second))
	Expect(c.Execution.RetryPolicy.RetryOn).To(ContainElement("network-error"))
}

func TestEnhancedActionsCiCdConfig_SetDefaults_PreservesProvidedValues(t *testing.T) {
	RegisterTestingT(t)

	c := &EnhancedActionsCiCdConfig{
		Organization: OrganizationConfig{
			DefaultBranch: "develop",
			DefaultRunner: "self-hosted",
		},
		WorkflowGeneration: WorkflowGenerationConfig{
			OutputPath: "ci/",
			Templates:  []string{"custom"},
			SCVersion:  "2026.5.0",
			CustomActions: map[string]string{
				"deploy": "myorg/actions/deploy@v1",
			},
		},
		Execution: ExecutionConfig{
			DefaultTimeout: "60m",
		},
	}
	c.SetDefaults()

	// Explicitly-set values must survive the defaulting pass.
	Expect(c.Organization.DefaultBranch).To(Equal("develop"))
	Expect(c.Organization.DefaultRunner).To(Equal("self-hosted"))
	Expect(c.WorkflowGeneration.OutputPath).To(Equal("ci/"))
	Expect(c.WorkflowGeneration.Templates).To(Equal([]string{"custom"}))
	Expect(c.WorkflowGeneration.SCVersion).To(Equal("2026.5.0"))
	Expect(c.WorkflowGeneration.CustomActions["deploy"]).To(Equal("myorg/actions/deploy@v1"))
	Expect(c.Execution.DefaultTimeout).To(Equal("60m"))
}

func TestEnhancedActionsCiCdConfig_SetDefaults_UsesSCVersionInActions(t *testing.T) {
	RegisterTestingT(t)

	// When SCVersion is a CalVer tag (not "latest"), the auto-generated
	// CustomActions should reference that tag rather than @main.
	c := &EnhancedActionsCiCdConfig{
		WorkflowGeneration: WorkflowGenerationConfig{
			SCVersion: "v2026.5.0",
			// CustomActions left nil → defaulter constructs them from SCVersion
		},
	}
	c.SetDefaults()

	Expect(c.WorkflowGeneration.CustomActions["deploy"]).To(ContainSubstring("@v2026.5.0"))
	Expect(c.WorkflowGeneration.CustomActions["destroy-client"]).To(ContainSubstring("@v2026.5.0"))
}

func TestEnhancedActionsCiCdConfig_Validate(t *testing.T) {
	cases := []struct {
		name      string
		setup     func(c *EnhancedActionsCiCdConfig)
		wantError string
	}{
		{
			name: "missing auth-token",
			setup: func(c *EnhancedActionsCiCdConfig) {
				c.AuthToken = ""
			},
			wantError: "auth-token is required",
		},
		{
			name: "missing organization name",
			setup: func(c *EnhancedActionsCiCdConfig) {
				c.AuthToken = "t"
				c.Organization.Name = ""
			},
			wantError: "organization.name is required",
		},
		{
			name: "environment missing type",
			setup: func(c *EnhancedActionsCiCdConfig) {
				c.AuthToken = "t"
				c.Organization.Name = "acme"
				c.Environments = map[string]EnvironmentConfig{
					"prod": {Runner: "ubuntu-latest"},
				}
			},
			wantError: "type is required",
		},
		{
			name: "environment missing runner",
			setup: func(c *EnhancedActionsCiCdConfig) {
				c.AuthToken = "t"
				c.Organization.Name = "acme"
				c.Environments = map[string]EnvironmentConfig{
					"prod": {Type: "production"},
				}
			},
			wantError: "runner is required",
		},
		{
			name: "protected environment without reviewers",
			setup: func(c *EnhancedActionsCiCdConfig) {
				c.AuthToken = "t"
				c.Organization.Name = "acme"
				c.Environments = map[string]EnvironmentConfig{
					"prod": {
						Type:       "production",
						Runner:     "ubuntu-latest",
						Protection: true,
					},
				}
			},
			wantError: "protected environments require reviewers",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			c := &EnhancedActionsCiCdConfig{}
			tc.setup(c)

			err := c.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(tc.wantError))
		})
	}
}

func TestEnhancedActionsCiCdConfig_Validate_HappyPath(t *testing.T) {
	RegisterTestingT(t)

	c := &EnhancedActionsCiCdConfig{
		AuthToken:    "t",
		Organization: OrganizationConfig{Name: "acme"},
		Environments: map[string]EnvironmentConfig{
			"prod": {
				Type:       "production",
				Runner:     "ubuntu-latest",
				Protection: true,
				Reviewers:  []string{"alice", "bob"},
			},
			"staging": {Type: "staging", Runner: "ubuntu-latest"},
		},
	}

	Expect(c.Validate()).To(Succeed())
}

func TestProviderTypeConstants(t *testing.T) {
	RegisterTestingT(t)

	Expect(ProviderType).To(Equal("github"))
	Expect(CiCdTypeGithubActions).To(Equal("github-actions"))
}
