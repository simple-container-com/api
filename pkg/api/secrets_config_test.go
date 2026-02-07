package api

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSecretResolver_Resolve_IncludeMode(t *testing.T) {
	allSecrets := map[string]string{
		"DATABASE_URL":     "postgres://prod-db",
		"API_KEY":          "prod-key",
		"SECRET_KEY":       "prod-secret",
		"STAGING_DATABASE": "postgres://staging-db",
		"STAGING_API_KEY":  "staging-key",
	}

	tests := []struct {
		name     string
		config   *EnvironmentSecretsConfig
		expected map[string]string
		wantErr  bool
		errMsg   string
	}{
		{
			name: "include mode with direct references",
			config: &EnvironmentSecretsConfig{
				Mode: SecretsConfigModeInclude,
				Secrets: SecretsConfigMap{
					"DATABASE_URL": DirectSecretReference,
					"API_KEY":      DirectSecretReference,
				},
			},
			expected: map[string]string{
				"DATABASE_URL": "postgres://prod-db",
				"API_KEY":      "prod-key",
			},
			wantErr: false,
		},
		{
			name: "include mode with mapped references",
			config: &EnvironmentSecretsConfig{
				Mode: SecretsConfigModeInclude,
				Secrets: SecretsConfigMap{
					"DATABASE_URL": "${secret:STAGING_DATABASE}",
					"API_KEY":      "${secret:STAGING_API_KEY}",
				},
			},
			expected: map[string]string{
				"DATABASE_URL": "postgres://staging-db",
				"API_KEY":      "staging-key",
			},
			wantErr: false,
		},
		{
			name: "include mode with literal values",
			config: &EnvironmentSecretsConfig{
				Mode: SecretsConfigModeInclude,
				Secrets: SecretsConfigMap{
					"DATABASE_URL": "postgres://custom-db",
					"API_KEY":      "custom-api-key",
				},
			},
			expected: map[string]string{
				"DATABASE_URL": "postgres://custom-db",
				"API_KEY":      "custom-api-key",
			},
			wantErr: false,
		},
		{
			name: "include mode with mixed references",
			config: &EnvironmentSecretsConfig{
				Mode: SecretsConfigModeInclude,
				Secrets: SecretsConfigMap{
					"DATABASE_URL": DirectSecretReference,
					"API_KEY":      "${secret:STAGING_API_KEY}",
					"SECRET_KEY":   "literal-secret-value",
				},
			},
			expected: map[string]string{
				"DATABASE_URL": "postgres://prod-db",
				"API_KEY":      "staging-key",
				"SECRET_KEY":   "literal-secret-value",
			},
			wantErr: false,
		},
		{
			name: "include mode with non-existent direct reference",
			config: &EnvironmentSecretsConfig{
				Mode: SecretsConfigModeInclude,
				Secrets: SecretsConfigMap{
					"NON_EXISTENT": DirectSecretReference,
				},
			},
			wantErr: true,
			errMsg:  "secret \"NON_EXISTENT\" not found in secrets.yaml",
		},
		{
			name: "include mode with non-existent mapped reference",
			config: &EnvironmentSecretsConfig{
				Mode: SecretsConfigModeInclude,
				Secrets: SecretsConfigMap{
					"API_KEY": "${secret:NON_EXISTENT}",
				},
			},
			wantErr: true,
			errMsg:  "mapped secret \"NON_EXISTENT\" not found in secrets.yaml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver := NewSecretResolver(tt.config, allSecrets)
			result, err := resolver.Resolve()

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestSecretResolver_Resolve_ExcludeMode(t *testing.T) {
	allSecrets := map[string]string{
		"DATABASE_URL":     "postgres://prod-db",
		"API_KEY":          "prod-key",
		"SECRET_KEY":       "prod-secret",
		"STAGING_DATABASE": "postgres://staging-db",
	}

	tests := []struct {
		name     string
		config   *EnvironmentSecretsConfig
		expected map[string]string
		wantErr  bool
		errMsg   string
	}{
		{
			name: "exclude mode with inheritAll",
			config: &EnvironmentSecretsConfig{
				Mode:       SecretsConfigModeExclude,
				InheritAll: true,
				Secrets: SecretsConfigMap{
					"SECRET_KEY": DirectSecretReference,
				},
			},
			expected: map[string]string{
				"DATABASE_URL":     "postgres://prod-db",
				"API_KEY":          "prod-key",
				"STAGING_DATABASE": "postgres://staging-db",
			},
			wantErr: false,
		},
		{
			name: "exclude mode without inheritAll",
			config: &EnvironmentSecretsConfig{
				Mode:       SecretsConfigModeExclude,
				InheritAll: false,
				Secrets: SecretsConfigMap{
					"SECRET_KEY": DirectSecretReference,
				},
			},
			wantErr: true,
			errMsg:  "exclude mode requires inheritAll to be true",
		},
		{
			name: "exclude mode with multiple exclusions",
			config: &EnvironmentSecretsConfig{
				Mode:       SecretsConfigModeExclude,
				InheritAll: true,
				Secrets: SecretsConfigMap{
					"SECRET_KEY":       DirectSecretReference,
					"STAGING_DATABASE": DirectSecretReference,
				},
			},
			expected: map[string]string{
				"DATABASE_URL": "postgres://prod-db",
				"API_KEY":      "prod-key",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver := NewSecretResolver(tt.config, allSecrets)
			result, err := resolver.Resolve()

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestSecretResolver_Resolve_OverrideMode(t *testing.T) {
	allSecrets := map[string]string{
		"DATABASE_URL":     "postgres://prod-db",
		"API_KEY":          "prod-key",
		"SECRET_KEY":       "prod-secret",
		"STAGING_DATABASE": "postgres://staging-db",
	}

	tests := []struct {
		name     string
		config   *EnvironmentSecretsConfig
		expected map[string]string
		wantErr  bool
	}{
		{
			name: "override mode with inheritAll",
			config: &EnvironmentSecretsConfig{
				Mode:       SecretsConfigModeOverride,
				InheritAll: true,
				Secrets: SecretsConfigMap{
					"DATABASE_URL": "${secret:STAGING_DATABASE}",
					"API_KEY":      "overridden-api-key",
				},
			},
			expected: map[string]string{
				"DATABASE_URL":     "postgres://staging-db",
				"API_KEY":          "overridden-api-key",
				"SECRET_KEY":       "prod-secret",
				"STAGING_DATABASE": "postgres://staging-db",
			},
			wantErr: false,
		},
		{
			name: "override mode without inheritAll",
			config: &EnvironmentSecretsConfig{
				Mode:       SecretsConfigModeOverride,
				InheritAll: false,
				Secrets: SecretsConfigMap{
					"DATABASE_URL": "${secret:STAGING_DATABASE}",
					"API_KEY":      "overridden-api-key",
				},
			},
			expected: map[string]string{
				"DATABASE_URL": "postgres://staging-db",
				"API_KEY":      "overridden-api-key",
			},
			wantErr: false,
		},
		{
			name: "override mode adding new secret",
			config: &EnvironmentSecretsConfig{
				Mode:       SecretsConfigModeOverride,
				InheritAll: true,
				Secrets: SecretsConfigMap{
					"NEW_SECRET": "new-value",
				},
			},
			expected: map[string]string{
				"DATABASE_URL":     "postgres://prod-db",
				"API_KEY":          "prod-key",
				"SECRET_KEY":       "prod-secret",
				"STAGING_DATABASE": "postgres://staging-db",
				"NEW_SECRET":       "new-value",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver := NewSecretResolver(tt.config, allSecrets)
			result, err := resolver.Resolve()

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestSecretResolver_Resolve_InvalidMode(t *testing.T) {
	allSecrets := map[string]string{
		"DATABASE_URL": "postgres://prod-db",
	}

	config := &EnvironmentSecretsConfig{
		Mode: "invalid-mode",
		Secrets: SecretsConfigMap{
			"DATABASE_URL": DirectSecretReference,
		},
	}

	resolver := NewSecretResolver(config, allSecrets)
	_, err := resolver.Resolve()

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown secrets config mode")
}

func TestSecretResolver_Resolve_NilConfig(t *testing.T) {
	allSecrets := map[string]string{
		"DATABASE_URL": "postgres://prod-db",
		"API_KEY":      "prod-key",
	}

	resolver := NewSecretResolver(nil, allSecrets)
	result, err := resolver.Resolve()

	assert.NoError(t, err)
	assert.Equal(t, allSecrets, result)
}

func TestValidateSecretConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  *EnvironmentSecretsConfig
		wantErr bool
		errMsg  string
	}{
		{
			name:    "nil config is valid",
			config:  nil,
			wantErr: false,
		},
		{
			name: "valid include mode",
			config: &EnvironmentSecretsConfig{
				Mode: SecretsConfigModeInclude,
				Secrets: SecretsConfigMap{
					"KEY": DirectSecretReference,
				},
			},
			wantErr: false,
		},
		{
			name: "valid exclude mode with inheritAll",
			config: &EnvironmentSecretsConfig{
				Mode:       SecretsConfigModeExclude,
				InheritAll: true,
				Secrets: SecretsConfigMap{
					"KEY": DirectSecretReference,
				},
			},
			wantErr: false,
		},
		{
			name: "valid override mode",
			config: &EnvironmentSecretsConfig{
				Mode: SecretsConfigModeOverride,
				Secrets: SecretsConfigMap{
					"KEY": "value",
				},
			},
			wantErr: false,
		},
		{
			name: "invalid mode",
			config: &EnvironmentSecretsConfig{
				Mode: "invalid-mode",
			},
			wantErr: true,
			errMsg:  "invalid secrets config mode",
		},
		{
			name: "exclude mode without inheritAll",
			config: &EnvironmentSecretsConfig{
				Mode:       SecretsConfigModeExclude,
				InheritAll: false,
				Secrets: SecretsConfigMap{
					"KEY": DirectSecretReference,
				},
			},
			wantErr: true,
			errMsg:  "exclude mode requires inheritAll to be true",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSecretConfig(tt.config)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateSecretReferences(t *testing.T) {
	availableSecrets := map[string]string{
		"DATABASE_URL":     "postgres://prod-db",
		"API_KEY":          "prod-key",
		"STAGING_DATABASE": "postgres://staging-db",
	}

	tests := []struct {
		name    string
		config  *EnvironmentSecretsConfig
		wantErr bool
		errMsg  string
	}{
		{
			name:    "nil config is valid",
			config:  nil,
			wantErr: false,
		},
		{
			name: "valid direct reference",
			config: &EnvironmentSecretsConfig{
				Mode: SecretsConfigModeInclude,
				Secrets: SecretsConfigMap{
					"DATABASE_URL": DirectSecretReference,
				},
			},
			wantErr: false,
		},
		{
			name: "valid mapped reference",
			config: &EnvironmentSecretsConfig{
				Mode: SecretsConfigModeInclude,
				Secrets: SecretsConfigMap{
					"DB_URL": "${secret:STAGING_DATABASE}",
				},
			},
			wantErr: false,
		},
		{
			name: "valid literal value",
			config: &EnvironmentSecretsConfig{
				Mode: SecretsConfigModeInclude,
				Secrets: SecretsConfigMap{
					"API_KEY": "literal-value",
				},
			},
			wantErr: false,
		},
		{
			name: "invalid direct reference - secret not found",
			config: &EnvironmentSecretsConfig{
				Mode: SecretsConfigModeInclude,
				Secrets: SecretsConfigMap{
					"NON_EXISTENT": DirectSecretReference,
				},
			},
			wantErr: true,
			errMsg:  "secret \"NON_EXISTENT\" (direct reference) not found",
		},
		{
			name: "invalid mapped reference - target secret not found",
			config: &EnvironmentSecretsConfig{
				Mode: SecretsConfigModeInclude,
				Secrets: SecretsConfigMap{
					"DB_URL": "${secret:NON_EXISTENT}",
				},
			},
			wantErr: true,
			errMsg:  "mapped secret \"NON_EXISTENT\" (referenced from \"DB_URL\") not found",
		},
		{
			name: "invalid mode",
			config: &EnvironmentSecretsConfig{
				Mode: "invalid",
				Secrets: SecretsConfigMap{
					"KEY": "value",
				},
			},
			wantErr: true,
			errMsg:  "invalid secrets config mode",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSecretReferences(tt.config, availableSecrets)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateSecretAccess(t *testing.T) {
	stack := Stack{
		Name: "test-stack",
		Server: ServerDescriptor{
			Secrets: SecretsConfigDescriptor{
				SecretsConfig: &EnvironmentSecretsConfig{
					Mode: SecretsConfigModeInclude,
					Secrets: SecretsConfigMap{
						"DATABASE_URL": DirectSecretReference,
						"API_KEY":      DirectSecretReference,
					},
				},
			},
		},
		Secrets: SecretsDescriptor{
			Values: map[string]string{
				"DATABASE_URL": "postgres://prod-db",
				"API_KEY":      "prod-key",
				"SECRET_KEY":   "prod-secret",
			},
		},
	}

	params := StackParams{
		Environment: "production",
	}

	tests := []struct {
		name        string
		secretKey   string
		wantErr     bool
		errContains string
	}{
		{
			name:      "accessible secret",
			secretKey: "DATABASE_URL",
			wantErr:   false,
		},
		{
			name:        "inaccessible secret",
			secretKey:   "SECRET_KEY",
			wantErr:     true,
			errContains: "secret \"SECRET_KEY\" is not accessible",
		},
		{
			name:        "non-existent secret",
			secretKey:   "NON_EXISTENT",
			wantErr:     true,
			errContains: "secret \"NON_EXISTENT\" is not accessible",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSecretAccess(stack, tt.secretKey, params)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateSecretAccess_NoConfig(t *testing.T) {
	stack := Stack{
		Name: "test-stack",
		Server: ServerDescriptor{
			Secrets: SecretsConfigDescriptor{
				// No SecretsConfig - all secrets should be accessible
			},
		},
		Secrets: SecretsDescriptor{
			Values: map[string]string{
				"DATABASE_URL": "postgres://prod-db",
				"API_KEY":      "prod-key",
			},
		},
	}

	params := StackParams{
		Environment: "production",
	}

	// All secrets should be accessible
	err := ValidateSecretAccess(stack, "DATABASE_URL", params)
	assert.NoError(t, err)

	err = ValidateSecretAccess(stack, "API_KEY", params)
	assert.NoError(t, err)

	err = ValidateSecretAccess(stack, "NON_EXISTENT", params)
	assert.NoError(t, err) // No config means no validation
}

func TestEnvironmentSecretsConfig_BackwardsCompatibility(t *testing.T) {
	// Test that nil SecretsConfig maintains backwards compatibility
	allSecrets := map[string]string{
		"DATABASE_URL": "postgres://prod-db",
		"API_KEY":      "prod-key",
	}

	// Test with nil config
	resolver := NewSecretResolver(nil, allSecrets)
	result, err := resolver.Resolve()

	assert.NoError(t, err)
	assert.Equal(t, allSecrets, result)

	// Test validation
	err = ValidateSecretConfig(nil)
	assert.NoError(t, err)
}

// Test integration with StacksMap.ReconcileForDeploy
func TestStacksMap_ReconcileForDeploy_WithSecretsConfig(t *testing.T) {
	stacks := StacksMap{
		"parent": {
			Name: "parent",
			Server: ServerDescriptor{
				Secrets: SecretsConfigDescriptor{
					SecretsConfig: &EnvironmentSecretsConfig{
						Mode: SecretsConfigModeInclude,
						Secrets: SecretsConfigMap{
							"DATABASE_URL": DirectSecretReference,
							"API_KEY":      DirectSecretReference,
						},
					},
				},
			},
			Secrets: SecretsDescriptor{
				Values: map[string]string{
					"DATABASE_URL": "postgres://prod-db",
					"API_KEY":      "prod-api-key",
					"SECRET_KEY":   "prod-secret-key",
				},
			},
			Client: ClientDescriptor{},
		},
		"child": {
			Name: "child",
			Server: ServerDescriptor{
				Secrets: SecretsConfigDescriptor{
					SecretsConfig: &EnvironmentSecretsConfig{
						Mode: SecretsConfigModeInclude,
						Secrets: SecretsConfigMap{
							"DATABASE_URL": DirectSecretReference,
							"API_KEY":      DirectSecretReference,
						},
					},
				},
			},
			Secrets: SecretsDescriptor{
				Values: map[string]string{
					"DATABASE_URL": "postgres://prod-db",
					"API_KEY":      "prod-api-key",
					"SECRET_KEY":   "prod-secret-key",
				},
			},
			Client: ClientDescriptor{
				Stacks: map[string]StackClientDescriptor{
					"production": {
						Type:        ClientTypeSingleImage,
						ParentStack: "parent",
					},
				},
			},
		},
	}

	params := StackParams{
		StackName:   "child",
		Environment: "production",
	}

	reconciled, err := stacks.ReconcileForDeploy(params)
	assert.NoError(t, err)

	childStack := (*reconciled)["child"]
	assert.NotNil(t, childStack)

	// Only DATABASE_URL and API_KEY should be available (include mode)
	assert.Len(t, childStack.Secrets.Values, 2)
	assert.Equal(t, "postgres://prod-db", childStack.Secrets.Values["DATABASE_URL"])
	assert.Equal(t, "prod-api-key", childStack.Secrets.Values["API_KEY"])
	assert.NotContains(t, childStack.Secrets.Values, "SECRET_KEY")
}
