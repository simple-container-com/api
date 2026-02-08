package api

import (
	"testing"

	. "github.com/onsi/gomega"
)

func TestSecretResolver_IncludeMode(t *testing.T) {
	RegisterTestingT(t)

	allSecrets := map[string]string{
		"API_KEY_PROD":      "prod-key",
		"API_KEY_STAGING":   "staging-key",
		"DATABASE_PASSWORD": "db-password",
		"SMTP_PASSWORD":     "smtp-password",
	}

	config := &EnvironmentSecretsConfig{
		Mode: "include",
		Secrets: map[string]SecretsConfigMap{
			"staging": {
				Include: []string{"~API_KEY_STAGING", "~DATABASE_PASSWORD"},
			},
		},
	}

	resolver := NewSecretResolver(config)

	result, err := resolver.ResolveSecrets(allSecrets, "staging")
	Expect(err).ToNot(HaveOccurred())
	Expect(result).To(Equal(map[string]string{
		"API_KEY_STAGING":   "staging-key",
		"DATABASE_PASSWORD": "db-password",
	}))
}

func TestSecretResolver_ExcludeMode(t *testing.T) {
	RegisterTestingT(t)

	allSecrets := map[string]string{
		"API_KEY_PROD":      "prod-key",
		"API_KEY_STAGING":   "staging-key",
		"DATABASE_PASSWORD": "db-password",
		"SMTP_PASSWORD":     "smtp-password",
	}

	config := &EnvironmentSecretsConfig{
		Mode: "exclude",
		Secrets: map[string]SecretsConfigMap{
			"staging": {
				InheritAll: true,
				Exclude:    []string{"~API_KEY_PROD"},
			},
		},
	}

	resolver := NewSecretResolver(config)

	result, err := resolver.ResolveSecrets(allSecrets, "staging")
	Expect(err).ToNot(HaveOccurred())

	Expect(result).To(Equal(map[string]string{
		"API_KEY_STAGING":   "staging-key",
		"DATABASE_PASSWORD": "db-password",
		"SMTP_PASSWORD":     "smtp-password",
	}))

	// Verify API_KEY_PROD is excluded
	_, exists := result["API_KEY_PROD"]
	Expect(exists).To(BeFalse())
}

func TestSecretResolver_OverrideMode(t *testing.T) {
	RegisterTestingT(t)

	allSecrets := map[string]string{
		"API_KEY_STAGING":           "staging-key",
		"DATABASE_PASSWORD_STAGING": "staging-db-password",
	}

	config := &EnvironmentSecretsConfig{
		Mode: "override",
		Secrets: map[string]SecretsConfigMap{
			"staging": {
				Override: map[string]string{
					"API_KEY":           "${secret:API_KEY_STAGING}",
					"DATABASE_PASSWORD": "${secret:DATABASE_PASSWORD_STAGING}",
					"APP_NAME":          "my-app",
				},
			},
		},
	}

	resolver := NewSecretResolver(config)

	result, err := resolver.ResolveSecrets(allSecrets, "staging")
	Expect(err).ToNot(HaveOccurred())

	Expect(result).To(Equal(map[string]string{
		"API_KEY":           "staging-key",
		"DATABASE_PASSWORD": "staging-db-password",
		"APP_NAME":          "my-app",
	}))
}

func TestSecretResolver_MappedReference(t *testing.T) {
	RegisterTestingT(t)

	allSecrets := map[string]string{
		"API_KEY_STAGING": "staging-key",
	}

	config := &EnvironmentSecretsConfig{
		Mode: "override",
		Secrets: map[string]SecretsConfigMap{
			"staging": {
				Override: map[string]string{
					"API_KEY": "${secret:API_KEY_STAGING}",
				},
			},
		},
	}

	resolver := NewSecretResolver(config)

	result, err := resolver.ResolveSecrets(allSecrets, "staging")
	Expect(err).ToNot(HaveOccurred())

	Expect(result).To(Equal(map[string]string{
		"API_KEY": "staging-key",
	}))
}

func TestSecretResolver_LiteralValue(t *testing.T) {
	RegisterTestingT(t)

	allSecrets := map[string]string{}

	config := &EnvironmentSecretsConfig{
		Mode: "override",
		Secrets: map[string]SecretsConfigMap{
			"staging": {
				Override: map[string]string{
					"API_KEY": "literal-api-key",
				},
			},
		},
	}

	resolver := NewSecretResolver(config)

	result, err := resolver.ResolveSecrets(allSecrets, "staging")
	Expect(err).ToNot(HaveOccurred())

	Expect(result).To(Equal(map[string]string{
		"API_KEY": "literal-api-key",
	}))
}

func TestSecretResolver_NoConfig(t *testing.T) {
	RegisterTestingT(t)

	allSecrets := map[string]string{
		"API_KEY": "api-key",
	}

	// Nil config = no filtering
	resolver := NewSecretResolver(nil)

	result, err := resolver.ResolveSecrets(allSecrets, "staging")
	Expect(err).ToNot(HaveOccurred())

	// All secrets should be returned
	Expect(result).To(Equal(allSecrets))
}

func TestSecretResolver_NoEnvironmentConfig(t *testing.T) {
	RegisterTestingT(t)

	allSecrets := map[string]string{
		"API_KEY": "api-key",
	}

	config := &EnvironmentSecretsConfig{
		Mode: "include",
		Secrets: map[string]SecretsConfigMap{
			"production": {
				Include: []string{"~API_KEY"},
			},
		},
	}

	resolver := NewSecretResolver(config)

	// Request environment not in config - should return all secrets (backwards compatibility)
	result, err := resolver.ResolveSecrets(allSecrets, "staging")
	Expect(err).ToNot(HaveOccurred())

	Expect(result).To(Equal(allSecrets))
}

func TestSecretResolver_InvalidMode(t *testing.T) {
	RegisterTestingT(t)

	allSecrets := map[string]string{
		"API_KEY": "api-key",
	}

	config := &EnvironmentSecretsConfig{
		Mode: "invalid",
		Secrets: map[string]SecretsConfigMap{
			"staging": {},
		},
	}

	resolver := NewSecretResolver(config)

	_, err := resolver.ResolveSecrets(allSecrets, "staging")
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("unknown secrets config mode"))
}

func TestSecretResolver_NonExistentSecretReference(t *testing.T) {
	RegisterTestingT(t)

	allSecrets := map[string]string{
		"API_KEY": "api-key",
	}

	config := &EnvironmentSecretsConfig{
		Mode: "include",
		Secrets: map[string]SecretsConfigMap{
			"staging": {
				Include: []string{"~NON_EXISTENT_SECRET"},
			},
		},
	}

	resolver := NewSecretResolver(config)

	_, err := resolver.ResolveSecrets(allSecrets, "staging")
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("secret"))
	Expect(err.Error()).To(ContainSubstring("not found"))
}

func TestSecretResolver_GetAvailableSecrets(t *testing.T) {
	RegisterTestingT(t)

	allSecrets := map[string]string{
		"API_KEY":           "api-key",
		"DATABASE_PASSWORD": "db-password",
		"SMTP_PASSWORD":     "smtp-password",
	}

	t.Run("Include mode", func(t *testing.T) {
		RegisterTestingT(t)

		config := &EnvironmentSecretsConfig{
			Mode: "include",
			Secrets: map[string]SecretsConfigMap{
				"staging": {
					Include: []string{"~API_KEY", "~DATABASE_PASSWORD"},
				},
			},
		}

		resolver := NewSecretResolver(config)
		available, err := resolver.GetAvailableSecrets(allSecrets, "staging")
		Expect(err).ToNot(HaveOccurred())

		Expect(available).To(ConsistOf("API_KEY", "DATABASE_PASSWORD"))
	})

	t.Run("Exclude mode", func(t *testing.T) {
		RegisterTestingT(t)

		config := &EnvironmentSecretsConfig{
			Mode: "exclude",
			Secrets: map[string]SecretsConfigMap{
				"staging": {
					InheritAll: true,
					Exclude:    []string{"~SMTP_PASSWORD"},
				},
			},
		}

		resolver := NewSecretResolver(config)
		available, err := resolver.GetAvailableSecrets(allSecrets, "staging")
		Expect(err).ToNot(HaveOccurred())

		Expect(available).To(ConsistOf("API_KEY", "DATABASE_PASSWORD"))
	})

	t.Run("Override mode", func(t *testing.T) {
		RegisterTestingT(t)

		config := &EnvironmentSecretsConfig{
			Mode: "override",
			Secrets: map[string]SecretsConfigMap{
				"staging": {
					Override: map[string]string{
						"API_KEY": "new-key",
					},
				},
			},
		}

		resolver := NewSecretResolver(config)
		available, err := resolver.GetAvailableSecrets(allSecrets, "staging")
		Expect(err).ToNot(HaveOccurred())

		Expect(available).To(ConsistOf("API_KEY"))
	})

	t.Run("No config", func(t *testing.T) {
		RegisterTestingT(t)

		resolver := NewSecretResolver(nil)
		available, err := resolver.GetAvailableSecrets(allSecrets, "staging")
		Expect(err).ToNot(HaveOccurred())

		Expect(available).To(ConsistOf("API_KEY", "DATABASE_PASSWORD", "SMTP_PASSWORD"))
	})
}

func TestSecretsConfigDescriptor_Copy(t *testing.T) {
	RegisterTestingT(t)

	original := &SecretsConfigDescriptor{
		Type: "test-type",
		SecretsConfig: &EnvironmentSecretsConfig{
			Mode: "include",
			Secrets: map[string]SecretsConfigMap{
				"staging": {
					Include: []string{"~API_KEY"},
					Override: map[string]string{
						"KEY": "value",
					},
				},
			},
		},
	}

	copied := original.Copy()

	// Verify values match
	Expect(copied.Type).To(Equal(original.Type))
	Expect(copied.SecretsConfig).ToNot(BeNil())
	Expect(copied.SecretsConfig.Mode).To(Equal(original.SecretsConfig.Mode))

	// Verify deep copy
	Expect(copied.SecretsConfig.Secrets["staging"].Include).To(Equal(original.SecretsConfig.Secrets["staging"].Include))
	Expect(copied.SecretsConfig.Secrets["staging"].Override).To(Equal(original.SecretsConfig.Secrets["staging"].Override))

	// Modify original and verify copy is unaffected
	original.SecretsConfig.Secrets["staging"].Include[0] = "~MODIFIED"
	Expect(copied.SecretsConfig.Secrets["staging"].Include[0]).To(Equal("~API_KEY"))

	original.SecretsConfig.Secrets["staging"].Override["KEY"] = "modified"
	Expect(copied.SecretsConfig.Secrets["staging"].Override["KEY"]).To(Equal("value"))
}

func TestValidateSecretAccess_IncludeMode(t *testing.T) {
	RegisterTestingT(t)

	descriptor := &ServerDescriptor{
		Secrets: SecretsConfigDescriptor{
			SecretsConfig: &EnvironmentSecretsConfig{
				Mode: "include",
				Secrets: map[string]SecretsConfigMap{
					"staging": {
						Include: []string{"~API_KEY", "~DATABASE_PASSWORD"},
					},
				},
			},
		},
	}

	clientConfig := &StackConfigCompose{
		Secrets: map[string]string{
			"API_KEY": "placeholder",
		},
	}

	errs := ValidateSecretAccess(descriptor, clientConfig, "staging")
	Expect(errs).To(BeEmpty())

	// Test with secret not in include list
	clientConfigInvalid := &StackConfigCompose{
		Secrets: map[string]string{
			"SMTP_PASSWORD": "placeholder",
		},
	}

	errs = ValidateSecretAccess(descriptor, clientConfigInvalid, "staging")
	Expect(errs).ToNot(BeEmpty())
	Expect(errs[0].Error()).To(ContainSubstring("SMTP_PASSWORD"))
	Expect(errs[0].Error()).To(ContainSubstring("not in the include list"))
}

func TestValidateSecretAccess_ExcludeMode(t *testing.T) {
	RegisterTestingT(t)

	descriptor := &ServerDescriptor{
		Secrets: SecretsConfigDescriptor{
			SecretsConfig: &EnvironmentSecretsConfig{
				Mode: "exclude",
				Secrets: map[string]SecretsConfigMap{
					"staging": {
						InheritAll: true,
						Exclude:    []string{"~PROD_SECRET"},
					},
				},
			},
		},
	}

	clientConfig := &StackConfigCompose{
		Secrets: map[string]string{
			"API_KEY": "placeholder",
		},
	}

	errs := ValidateSecretAccess(descriptor, clientConfig, "staging")
	Expect(errs).To(BeEmpty())

	// Test with excluded secret
	clientConfigInvalid := &StackConfigCompose{
		Secrets: map[string]string{
			"PROD_SECRET": "placeholder",
		},
	}

	errs = ValidateSecretAccess(descriptor, clientConfigInvalid, "staging")
	Expect(errs).ToNot(BeEmpty())
	Expect(errs[0].Error()).To(ContainSubstring("PROD_SECRET"))
	Expect(errs[0].Error()).To(ContainSubstring("excluded"))
}

func TestValidateSecretAccess_OverrideMode(t *testing.T) {
	RegisterTestingT(t)

	descriptor := &ServerDescriptor{
		Secrets: SecretsConfigDescriptor{
			SecretsConfig: &EnvironmentSecretsConfig{
				Mode: "override",
				Secrets: map[string]SecretsConfigMap{
					"staging": {
						Override: map[string]string{
							"API_KEY": "new-key",
						},
					},
				},
			},
		},
	}

	clientConfig := &StackConfigCompose{
		Secrets: map[string]string{
			"API_KEY": "placeholder",
		},
	}

	errs := ValidateSecretAccess(descriptor, clientConfig, "staging")
	Expect(errs).To(BeEmpty())

	// Test with secret not in override map
	clientConfigInvalid := &StackConfigCompose{
		Secrets: map[string]string{
			"OTHER_SECRET": "placeholder",
		},
	}

	errs = ValidateSecretAccess(descriptor, clientConfigInvalid, "staging")
	Expect(errs).ToNot(BeEmpty())
	Expect(errs[0].Error()).To(ContainSubstring("OTHER_SECRET"))
	Expect(errs[0].Error()).To(ContainSubstring("not in the override list"))
}

func TestValidateSecretAccess_NoConfig(t *testing.T) {
	RegisterTestingT(t)

	descriptor := &ServerDescriptor{
		Secrets: SecretsConfigDescriptor{
			// No SecretsConfig set
		},
	}

	clientConfig := &StackConfigCompose{
		Secrets: map[string]string{
			"API_KEY": "placeholder",
		},
	}

	errs := ValidateSecretAccess(descriptor, clientConfig, "staging")
	Expect(errs).To(BeEmpty())
}

func TestReconcileForDeploy_SecretFiltering(t *testing.T) {
	RegisterTestingT(t)

	stacks := &StacksMap{
		"parent": {
			Name: "parent",
			Secrets: SecretsDescriptor{
				Values: map[string]string{
					"API_KEY_STAGING":   "staging-key",
					"DATABASE_PASSWORD": "db-password",
					"PROD_SECRET":       "prod-secret",
				},
			},
			Server: ServerDescriptor{
				Secrets: SecretsConfigDescriptor{
					SecretsConfig: &EnvironmentSecretsConfig{
						Mode: "include",
						Secrets: map[string]SecretsConfigMap{
							"staging": {
								Include: []string{"~API_KEY_STAGING", "~DATABASE_PASSWORD"},
							},
						},
					},
				},
			},
		},
		"child": {
			Name: "child",
			Client: ClientDescriptor{
				Stacks: map[string]StackClientDescriptor{
					"staging": {
						ParentStack: "parent",
					},
				},
			},
		},
	}

	params := StackParams{
		StackName:   "child",
		Environment: "staging",
	}

	result, err := stacks.ReconcileForDeploy(params)
	Expect(err).ToNot(HaveOccurred())

	childStack := (*result)["child"]

	// Verify only included secrets are available
	Expect(childStack.Secrets.Values).To(HaveKey("API_KEY_STAGING"))
	Expect(childStack.Secrets.Values).To(HaveKey("DATABASE_PASSWORD"))
	Expect(childStack.Secrets.Values).ToNot(HaveKey("PROD_SECRET"))
}

func TestExtractKeyFromRef(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name     string
		ref      string
		expected string
	}{
		{
			name:     "direct reference",
			ref:      "SECRET_NAME",
			expected: "SECRET_NAME",
		},
		{
			name:     "tilde prefix",
			ref:      "~SECRET_NAME",
			expected: "SECRET_NAME",
		},
		{
			name:     "secret reference pattern",
			ref:      "${secret:OTHER_SECRET}",
			expected: "OTHER_SECRET",
		},
		{
			name:     "literal value",
			ref:      "literal-value",
			expected: "literal-value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			result := extractKeyFromRef(tt.ref)
			Expect(result).To(Equal(tt.expected))
		})
	}
}
