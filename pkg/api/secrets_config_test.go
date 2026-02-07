package api

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSecretResolver_IncludeMode(t *testing.T) {
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
	require.NoError(t, err)

	assert.Equal(t, map[string]string{
		"API_KEY_STAGING":   "staging-key",
		"DATABASE_PASSWORD": "db-password",
	}, result)
}

func TestSecretResolver_ExcludeMode(t *testing.T) {
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
	require.NoError(t, err)

	assert.Equal(t, map[string]string{
		"API_KEY_STAGING":   "staging-key",
		"DATABASE_PASSWORD": "db-password",
		"SMTP_PASSWORD":     "smtp-password",
	}, result)

	// Verify API_KEY_PROD is excluded
	_, exists := result["API_KEY_PROD"]
	assert.False(t, exists)
}

func TestSecretResolver_OverrideMode(t *testing.T) {
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
	require.NoError(t, err)

	assert.Equal(t, map[string]string{
		"API_KEY":           "staging-key",
		"DATABASE_PASSWORD": "staging-db-password",
		"APP_NAME":          "my-app",
	}, result)
}

func TestSecretResolver_MappedReference(t *testing.T) {
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
	require.NoError(t, err)

	assert.Equal(t, map[string]string{
		"API_KEY": "staging-key",
	}, result)
}

func TestSecretResolver_LiteralValue(t *testing.T) {
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
	require.NoError(t, err)

	assert.Equal(t, map[string]string{
		"API_KEY": "literal-api-key",
	}, result)
}

func TestSecretResolver_NoConfig(t *testing.T) {
	allSecrets := map[string]string{
		"API_KEY": "api-key",
	}

	// Nil config = no filtering
	resolver := NewSecretResolver(nil)

	result, err := resolver.ResolveSecrets(allSecrets, "staging")
	require.NoError(t, err)

	// All secrets should be returned
	assert.Equal(t, allSecrets, result)
}

func TestSecretResolver_NoEnvironmentConfig(t *testing.T) {
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
	require.NoError(t, err)

	assert.Equal(t, allSecrets, result)
}

func TestSecretResolver_InvalidMode(t *testing.T) {
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
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown secrets config mode")
}

func TestSecretResolver_NonExistentSecretReference(t *testing.T) {
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
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "secret")
	assert.Contains(t, err.Error(), "not found")
}

func TestSecretResolver_GetAvailableSecrets(t *testing.T) {
	allSecrets := map[string]string{
		"API_KEY":           "api-key",
		"DATABASE_PASSWORD": "db-password",
		"SMTP_PASSWORD":     "smtp-password",
	}

	t.Run("Include mode", func(t *testing.T) {
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
		require.NoError(t, err)

		assert.ElementsMatch(t, []string{"API_KEY", "DATABASE_PASSWORD"}, available)
	})

	t.Run("Exclude mode", func(t *testing.T) {
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
		require.NoError(t, err)

		assert.ElementsMatch(t, []string{"API_KEY", "DATABASE_PASSWORD"}, available)
	})

	t.Run("Override mode", func(t *testing.T) {
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
		require.NoError(t, err)

		assert.ElementsMatch(t, []string{"API_KEY"}, available)
	})

	t.Run("No config", func(t *testing.T) {
		resolver := NewSecretResolver(nil)
		available, err := resolver.GetAvailableSecrets(allSecrets, "staging")
		require.NoError(t, err)

		assert.ElementsMatch(t, []string{"API_KEY", "DATABASE_PASSWORD", "SMTP_PASSWORD"}, available)
	})
}

func TestSecretsConfigDescriptor_Copy(t *testing.T) {
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
	assert.Equal(t, original.Type, copied.Type)
	require.NotNil(t, copied.SecretsConfig)
	assert.Equal(t, original.SecretsConfig.Mode, copied.SecretsConfig.Mode)

	// Verify deep copy
	assert.Equal(t, original.SecretsConfig.Secrets["staging"].Include, copied.SecretsConfig.Secrets["staging"].Include)
	assert.Equal(t, original.SecretsConfig.Secrets["staging"].Override, copied.SecretsConfig.Secrets["staging"].Override)

	// Modify original and verify copy is unaffected
	original.SecretsConfig.Secrets["staging"].Include[0] = "~MODIFIED"
	assert.Equal(t, "~API_KEY", copied.SecretsConfig.Secrets["staging"].Include[0])

	original.SecretsConfig.Secrets["staging"].Override["KEY"] = "modified"
	assert.Equal(t, "value", copied.SecretsConfig.Secrets["staging"].Override["KEY"])
}

func TestValidateSecretAccess_IncludeMode(t *testing.T) {
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
	assert.Empty(t, errs)

	// Test with secret not in include list
	clientConfigInvalid := &StackConfigCompose{
		Secrets: map[string]string{
			"SMTP_PASSWORD": "placeholder",
		},
	}

	errs = ValidateSecretAccess(descriptor, clientConfigInvalid, "staging")
	assert.NotEmpty(t, errs)
	assert.Contains(t, errs[0].Error(), "SMTP_PASSWORD")
	assert.Contains(t, errs[0].Error(), "not in the include list")
}

func TestValidateSecretAccess_ExcludeMode(t *testing.T) {
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
	assert.Empty(t, errs)

	// Test with excluded secret
	clientConfigInvalid := &StackConfigCompose{
		Secrets: map[string]string{
			"PROD_SECRET": "placeholder",
		},
	}

	errs = ValidateSecretAccess(descriptor, clientConfigInvalid, "staging")
	assert.NotEmpty(t, errs)
	assert.Contains(t, errs[0].Error(), "PROD_SECRET")
	assert.Contains(t, errs[0].Error(), "excluded")
}

func TestValidateSecretAccess_OverrideMode(t *testing.T) {
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
	assert.Empty(t, errs)

	// Test with secret not in override map
	clientConfigInvalid := &StackConfigCompose{
		Secrets: map[string]string{
			"OTHER_SECRET": "placeholder",
		},
	}

	errs = ValidateSecretAccess(descriptor, clientConfigInvalid, "staging")
	assert.NotEmpty(t, errs)
	assert.Contains(t, errs[0].Error(), "OTHER_SECRET")
	assert.Contains(t, errs[0].Error(), "not in the override list")
}

func TestValidateSecretAccess_NoConfig(t *testing.T) {
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
	assert.Empty(t, errs)
}

func TestReconcileForDeploy_SecretFiltering(t *testing.T) {
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
	require.NoError(t, err)

	childStack := (*result)["child"]

	// Verify only included secrets are available
	assert.Contains(t, childStack.Secrets.Values, "API_KEY_STAGING")
	assert.Contains(t, childStack.Secrets.Values, "DATABASE_PASSWORD")
	assert.NotContains(t, childStack.Secrets.Values, "PROD_SECRET")
}

func TestExtractKeyFromRef(t *testing.T) {
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
			result := extractKeyFromRef(tt.ref)
			assert.Equal(t, tt.expected, result)
		})
	}
}
