package api

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSecretResolver_IncludeMode tests include mode - only specified secrets are available
func TestSecretResolver_IncludeMode(t *testing.T) {
	baseSecrets := &SecretsDescriptor{
		Values: map[string]string{
			"DATABASE_URL_PROD":     "postgres://prod-db",
			"DATABASE_URL_STAGING":  "postgres://staging-db",
			"API_KEY_PROD":          "prod-key-123",
			"API_KEY_STAGING":       "staging-key-456",
			"SHARED_SECRET":         "shared-value",
			"SHARED_SECRET_STAGING": "shared-staging-value",
		},
	}

	t.Run("direct mapping - secret name maps to same key", func(t *testing.T) {
		config := &EnvironmentSecretsConfigDescriptor{
			Mode: "include",
			Secrets: SecretsConfigMap{
				"DATABASE_URL": "DATABASE_URL_STAGING",
				"API_KEY":      "API_KEY_STAGING",
			},
		}

		resolver, err := NewSecretResolver(baseSecrets, config)
		require.NoError(t, err)

		result, err := resolver.Resolve()
		require.NoError(t, err)

		assert.Len(t, result, 2)
		assert.Equal(t, "postgres://staging-db", result["DATABASE_URL"])
		assert.Equal(t, "staging-key-456", result["API_KEY"])
	})

	t.Run("literal reference - using ${secret:} syntax", func(t *testing.T) {
		config := &EnvironmentSecretsConfigDescriptor{
			Mode: "include",
			Secrets: SecretsConfigMap{
				"DATABASE_URL": "${secret:DATABASE_URL_STAGING}",
				"API_KEY":      "${secret:API_KEY_STAGING}",
			},
		}

		resolver, err := NewSecretResolver(baseSecrets, config)
		require.NoError(t, err)

		result, err := resolver.Resolve()
		require.NoError(t, err)

		assert.Len(t, result, 2)
		assert.Equal(t, "postgres://staging-db", result["DATABASE_URL"])
		assert.Equal(t, "staging-key-456", result["API_KEY"])
	})

	t.Run("error when secret not found", func(t *testing.T) {
		config := &EnvironmentSecretsConfigDescriptor{
			Mode: "include",
			Secrets: SecretsConfigMap{
				"DATABASE_URL": "NONEXISTENT_SECRET",
			},
		}

		resolver, err := NewSecretResolver(baseSecrets, config)
		require.NoError(t, err)

		_, err = resolver.Resolve()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "secret")
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("no config returns all secrets", func(t *testing.T) {
		resolver, err := NewSecretResolver(baseSecrets, nil)
		require.NoError(t, err)

		result, err := resolver.Resolve()
		require.NoError(t, err)

		assert.Len(t, result, 6)
		assert.Equal(t, "postgres://prod-db", result["DATABASE_URL_PROD"])
	})
}

// TestSecretResolver_ExcludeMode tests exclude mode - all secrets except excluded ones
func TestSecretResolver_ExcludeMode(t *testing.T) {
	baseSecrets := &SecretsDescriptor{
		Values: map[string]string{
			"DATABASE_URL":     "postgres://db",
			"API_KEY":          "api-key-123",
			"PROD_SECRET":      "prod-only",
			"SHARED_SECRET":    "shared",
			"ANOTHER_SECRET":   "another",
		},
	}

	t.Run("exclude mode with inheritAll", func(t *testing.T) {
		config := &EnvironmentSecretsConfigDescriptor{
			Mode:       "exclude",
			InheritAll: true,
			Secrets: SecretsConfigMap{
				"PROD_SECRET": "", // value doesn't matter for exclude
			},
		}

		resolver, err := NewSecretResolver(baseSecrets, config)
		require.NoError(t, err)

		result, err := resolver.Resolve()
		require.NoError(t, err)

		assert.Len(t, result, 4)
		assert.Equal(t, "postgres://db", result["DATABASE_URL"])
		assert.Equal(t, "api-key-123", result["API_KEY"])
		assert.NotContains(t, result, "PROD_SECRET")
	})

	t.Run("exclude mode without inheritAll returns error", func(t *testing.T) {
		config := &EnvironmentSecretsConfigDescriptor{
			Mode:       "exclude",
			InheritAll: false,
			Secrets: SecretsConfigMap{
				"PROD_SECRET": "",
			},
		}

		resolver, err := NewSecretResolver(baseSecrets, config)
		require.NoError(t, err)

		_, err = resolver.Resolve()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "inheritAll")
	})

	t.Run("exclude multiple secrets", func(t *testing.T) {
		config := &EnvironmentSecretsConfigDescriptor{
			Mode:       "exclude",
			InheritAll: true,
			Secrets: SecretsConfigMap{
				"PROD_SECRET":    "",
				"ANOTHER_SECRET": "",
			},
		}

		resolver, err := NewSecretResolver(baseSecrets, config)
		require.NoError(t, err)

		result, err := resolver.Resolve()
		require.NoError(t, err)

		assert.Len(t, result, 3)
		assert.NotContains(t, result, "PROD_SECRET")
		assert.NotContains(t, result, "ANOTHER_SECRET")
		assert.Contains(t, result, "DATABASE_URL")
	})
}

// TestSecretResolver_OverrideMode tests override mode - all secrets with overrides
func TestSecretResolver_OverrideMode(t *testing.T) {
	t.Run("override with staging values", func(t *testing.T) {
		// Add staging-specific secrets to base for this test
		baseWithStaging := &SecretsDescriptor{
			Values: map[string]string{
				"DATABASE_URL":          "postgres://prod-db",
				"DATABASE_URL_STAGING":  "postgres://staging-db",
				"API_KEY":               "prod-key-123",
				"API_KEY_STAGING":       "staging-key-456",
				"SHARED_SECRET":         "shared-value",
				"SHARED_SECRET_STAGING": "shared-staging-value",
				"ANOTHER_SECRET":        "another-value",
			},
		}

		config := &EnvironmentSecretsConfigDescriptor{
			Mode: "override",
			Secrets: SecretsConfigMap{
				"DATABASE_URL":  "DATABASE_URL_STAGING",
				"API_KEY":       "API_KEY_STAGING",
				"SHARED_SECRET": "SHARED_SECRET_STAGING",
			},
		}

		resolver, err := NewSecretResolver(baseWithStaging, config)
		require.NoError(t, err)

		result, err := resolver.Resolve()
		require.NoError(t, err)

		// Override mode includes all base secrets (including staging-specific ones)
		// and applies the overrides for the specified keys
		assert.Len(t, result, 7)
		assert.Equal(t, "postgres://staging-db", result["DATABASE_URL"], "should override with staging value")
		assert.Equal(t, "staging-key-456", result["API_KEY"], "should override with staging value")
		assert.Equal(t, "shared-staging-value", result["SHARED_SECRET"], "should override with staging value")
		assert.Equal(t, "another-value", result["ANOTHER_SECRET"], "should keep non-overridden value")
		// Staging-specific secrets are also included
		assert.Equal(t, "postgres://staging-db", result["DATABASE_URL_STAGING"])
		assert.Equal(t, "staging-key-456", result["API_KEY_STAGING"])
		assert.Equal(t, "shared-staging-value", result["SHARED_SECRET_STAGING"])
	})

	t.Run("override with literal references", func(t *testing.T) {
		baseWithStaging := &SecretsDescriptor{
			Values: map[string]string{
				"DATABASE_URL":         "postgres://prod-db",
				"DATABASE_URL_STAGING": "postgres://staging-db",
				"API_KEY":              "prod-key-123",
				"API_KEY_STAGING":      "staging-key-456",
			},
		}

		config := &EnvironmentSecretsConfigDescriptor{
			Mode: "override",
			Secrets: SecretsConfigMap{
				"DATABASE_URL": "${secret:DATABASE_URL_STAGING}",
				"API_KEY":      "${secret:API_KEY_STAGING}",
			},
		}

		resolver, err := NewSecretResolver(baseWithStaging, config)
		require.NoError(t, err)

		result, err := resolver.Resolve()
		require.NoError(t, err)

		assert.Equal(t, "postgres://staging-db", result["DATABASE_URL"])
		assert.Equal(t, "staging-key-456", result["API_KEY"])
	})
}

// TestSecretResolver_Errors tests error conditions
func TestSecretResolver_Errors(t *testing.T) {
	t.Run("nil base secrets returns error", func(t *testing.T) {
		config := &EnvironmentSecretsConfigDescriptor{
			Mode: "include",
		}

		_, err := NewSecretResolver(nil, config)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "nil")
	})

	t.Run("invalid mode returns error", func(t *testing.T) {
		baseSecrets := &SecretsDescriptor{
			Values: map[string]string{
				"SECRET": "value",
			},
		}

		config := &EnvironmentSecretsConfigDescriptor{
			Mode: "invalid-mode",
		}

		resolver, err := NewSecretResolver(baseSecrets, config)
		require.NoError(t, err)

		_, err = resolver.Resolve()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unknown secretsConfig mode")
	})

	t.Run("invalid secret reference format returns error", func(t *testing.T) {
		err := ValidateSecretReference("${invalid}")
		assert.Error(t, err)
	})

	t.Run("valid secret reference format", func(t *testing.T) {
		err := ValidateSecretReference("${secret:MY_SECRET_KEY}")
		assert.NoError(t, err)

		err = ValidateSecretReference("${secret:my-secret-123}")
		assert.NoError(t, err)
	})
}

// TestIsSecretReference tests the IsSecretReference helper function
func TestIsSecretReference(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{
			name:  "valid secret reference",
			value: "${secret:MY_KEY}",
			want:  true,
		},
		{
			name:  "valid secret reference with dash",
			value: "${secret:my-key-123}",
			want:  true,
		},
		{
			name:  "missing opening brace",
			value: "secret:MY_KEY}",
			want:  false,
		},
		{
			name:  "missing closing brace",
			value: "${secret:MY_KEY",
			want:  false,
		},
		{
			name:  "wrong prefix",
			value: "${var:MY_KEY}",
			want:  false,
		},
		{
			name:  "plain string",
			value: "MY_SECRET_KEY",
			want:  false,
		},
		{
			name:  "empty string",
			value: "",
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsSecretReference(tt.value)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestEnvironmentSecretsConfigDescriptor_Copy tests copying the secrets config
func TestEnvironmentSecretsConfigDescriptor_Copy(t *testing.T) {
	original := &EnvironmentSecretsConfigDescriptor{
		Mode:       "include",
		InheritAll: true,
		Secrets: SecretsConfigMap{
			"DATABASE_URL": "DATABASE_URL_STAGING",
			"API_KEY":      "${secret:API_KEY_STAGING}",
		},
	}

	copied := original.Copy()

	assert.Equal(t, original.Mode, copied.Mode)
	assert.Equal(t, original.InheritAll, copied.InheritAll)
	assert.Equal(t, original.Secrets, copied.Secrets)

	// Verify it's a deep copy
	copied.Mode = "override"
	copied.Secrets["NEW_SECRET"] = "new_value"

	assert.Equal(t, "include", original.Mode, "original should not be modified")
	assert.NotContains(t, original.Secrets, "NEW_SECRET", "original should not have new key")
}

// TestSecretsConfigDescriptor_Copy tests copying the secrets config descriptor
func TestSecretsConfigDescriptor_Copy(t *testing.T) {
	original := &SecretsConfigDescriptor{
		Type: "file",
		SecretsConfig: &EnvironmentSecretsConfigDescriptor{
			Mode:       "include",
			InheritAll: false,
			Secrets: SecretsConfigMap{
				"KEY": "VALUE",
			},
		},
	}

	copied := original.Copy()

	assert.Equal(t, original.Type, copied.Type)
	assert.NotNil(t, copied.SecretsConfig)
	assert.Equal(t, original.SecretsConfig.Mode, copied.SecretsConfig.Mode)
}

// TestDetectSecretsConfigType tests validation of secrets config
func TestDetectSecretsConfigType(t *testing.T) {
	t.Run("valid include mode", func(t *testing.T) {
		config := &EnvironmentSecretsConfigDescriptor{
			Mode: "include",
			Secrets: SecretsConfigMap{
				"KEY": "VALUE",
			},
		}

		err := DetectSecretsConfigType(config)
		assert.NoError(t, err)
	})

	t.Run("valid exclude mode with inheritAll", func(t *testing.T) {
		config := &EnvironmentSecretsConfigDescriptor{
			Mode:       "exclude",
			InheritAll: true,
			Secrets: SecretsConfigMap{
				"KEY": "",
			},
		}

		err := DetectSecretsConfigType(config)
		assert.NoError(t, err)
	})

	t.Run("exclude mode without inheritAll fails", func(t *testing.T) {
		config := &EnvironmentSecretsConfigDescriptor{
			Mode:       "exclude",
			InheritAll: false,
		}

		err := DetectSecretsConfigType(config)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "inheritAll")
	})

	t.Run("invalid mode fails", func(t *testing.T) {
		config := &EnvironmentSecretsConfigDescriptor{
			Mode: "invalid",
		}

		err := DetectSecretsConfigType(config)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid secretsConfig mode")
	})

	t.Run("nil config passes", func(t *testing.T) {
		err := DetectSecretsConfigType(nil)
		assert.NoError(t, err)
	})

	t.Run("valid override mode", func(t *testing.T) {
		config := &EnvironmentSecretsConfigDescriptor{
			Mode: "override",
			Secrets: SecretsConfigMap{
				"KEY": "${secret:OTHER_KEY}",
			},
		}

		err := DetectSecretsConfigType(config)
		assert.NoError(t, err)
	})
}

// TestValidateSecretsConfigInStacks tests validation of secrets in stacks
func TestValidateSecretsConfigInStacks(t *testing.T) {
	t.Run("nil stacks map returns error", func(t *testing.T) {
		err := ValidateSecretsConfigInStacks(nil, StackParams{})
		assert.Error(t, err)
	})

	t.Run("stack without secretsConfig passes", func(t *testing.T) {
		stacks := &StacksMap{
			"parent": {
				Name: "parent",
				Server: ServerDescriptor{
					Secrets: SecretsConfigDescriptor{
						Type: "file",
					},
				},
				Client: ClientDescriptor{},
			},
		}

		err := ValidateSecretsConfigInStacks(stacks, StackParams{Environment: "staging"})
		assert.NoError(t, err)
	})
}

// TestGetAvailableSecrets tests getting available secrets
func TestGetAvailableSecrets(t *testing.T) {
	baseSecrets := &SecretsDescriptor{
		Values: map[string]string{
			"KEY1": "value1",
			"KEY2": "value2",
			"KEY3": "value3",
		},
	}

	t.Run("no config returns all secrets", func(t *testing.T) {
		secrets, err := GetAvailableSecrets(baseSecrets, nil)
		require.NoError(t, err)
		assert.Len(t, secrets, 3)
		assert.Contains(t, secrets, "KEY1")
		assert.Contains(t, secrets, "KEY2")
		assert.Contains(t, secrets, "KEY3")
	})

	t.Run("include mode returns filtered secrets", func(t *testing.T) {
		config := &EnvironmentSecretsConfigDescriptor{
			Mode: "include",
			Secrets: SecretsConfigMap{
				"KEY1": "KEY1",
				"KEY2": "KEY2",
			},
		}

		secrets, err := GetAvailableSecrets(baseSecrets, config)
		require.NoError(t, err)
		assert.Len(t, secrets, 2)
		assert.Contains(t, secrets, "KEY1")
		assert.Contains(t, secrets, "KEY2")
		assert.NotContains(t, secrets, "KEY3")
	})

	t.Run("nil secrets returns error", func(t *testing.T) {
		_, err := GetAvailableSecrets(nil, nil)
		assert.Error(t, err)
	})
}

// TestSecretResolver_ReferenceResolution tests different reference resolution patterns
func TestSecretResolver_ReferenceResolution(t *testing.T) {
	baseSecrets := &SecretsDescriptor{
		Values: map[string]string{
			"PROD_DB":     "postgres://prod",
			"STAGING_DB":  "postgres://staging",
			"COMMON_KEY":  "common-value",
		},
	}

	t.Run("pattern 1: direct reference - same key name", func(t *testing.T) {
		config := &EnvironmentSecretsConfigDescriptor{
			Mode: "include",
			Secrets: SecretsConfigMap{
				"DATABASE_URL": "STAGING_DB",
			},
		}

		resolver, err := NewSecretResolver(baseSecrets, config)
		require.NoError(t, err)

		result, err := resolver.Resolve()
		require.NoError(t, err)

		assert.Equal(t, "postgres://staging", result["DATABASE_URL"])
	})

	t.Run("pattern 2: mapped reference - different key name", func(t *testing.T) {
		config := &EnvironmentSecretsConfigDescriptor{
			Mode: "include",
			Secrets: SecretsConfigMap{
				"DB": "STAGING_DB",
			},
		}

		resolver, err := NewSecretResolver(baseSecrets, config)
		require.NoError(t, err)

		result, err := resolver.Resolve()
		require.NoError(t, err)

		assert.Equal(t, "postgres://staging", result["DB"])
	})

	t.Run("pattern 3: literal reference using ${secret:}", func(t *testing.T) {
		config := &EnvironmentSecretsConfigDescriptor{
			Mode: "include",
			Secrets: SecretsConfigMap{
				"DATABASE_URL": "${secret:STAGING_DB}",
			},
		}

		resolver, err := NewSecretResolver(baseSecrets, config)
		require.NoError(t, err)

		result, err := resolver.Resolve()
		require.NoError(t, err)

		assert.Equal(t, "postgres://staging", result["DATABASE_URL"])
	})
}

// BenchmarkSecretResolver_Resolve benchmarks the secret resolution
func BenchmarkSecretResolver_Resolve(b *testing.B) {
	baseSecrets := &SecretsDescriptor{
		Values: make(map[string]string),
	}
	for i := 0; i < 100; i++ {
		baseSecrets.Values[fmt.Sprintf("SECRET_%d", i)] = fmt.Sprintf("value-%d", i)
	}

	config := &EnvironmentSecretsConfigDescriptor{
		Mode: "include",
		Secrets: make(SecretsConfigMap),
	}
	for i := 0; i < 10; i++ {
		config.Secrets[fmt.Sprintf("SECRET_%d", i)] = fmt.Sprintf("SECRET_%d", i)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resolver, _ := NewSecretResolver(baseSecrets, config)
		_, _ = resolver.Resolve()
	}
}
