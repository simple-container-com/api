package api

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSecretResolver_ResolveIncludeMode(t *testing.T) {
	baseSecrets := map[string]string{
		"DATABASE_URL":           "postgres://localhost/prod",
		"DATABASE_URL_STAGING":   "postgres://localhost/staging",
		"API_KEY":                "prod-key-123",
		"API_KEY_STAGING":        "staging-key-456",
		"SECRET_KEY":             "prod-secret",
		"SECRET_KEY_STAGING":     "staging-secret",
		"STAGING_ONLY_SECRET":    "staging-only-value",
		"PRODUCTION_ONLY_SECRET": "production-only-value",
	}

	t.Run("include mode returns only specified secrets", func(t *testing.T) {
		config := &EnvironmentSecretsConfigDescriptor{
			Mode: "include",
			Secrets: SecretsConfigMap{
				"DATABASE_URL": "DATABASE_URL_STAGING",
				"API_KEY":      "API_KEY_STAGING",
				"SECRET_KEY":   "SECRET_KEY_STAGING",
			},
		}
		resolver := NewSecretResolver(baseSecrets, config)
		result, err := resolver.Resolve()
		require.NoError(t, err)
		assert.Equal(t, map[string]string{
			"DATABASE_URL": "postgres://localhost/staging",
			"API_KEY":      "staging-key-456",
			"SECRET_KEY":   "staging-secret",
		}, result)
		assert.Len(t, result, 3)
	})

	t.Run("include mode with literal values", func(t *testing.T) {
		config := &EnvironmentSecretsConfigDescriptor{
			Mode: "include",
			Secrets: SecretsConfigMap{
				"DATABASE_URL": "literal:postgres://staging-db:5432/app",
				"API_KEY":      "literal:staging-api-key-123",
			},
		}
		resolver := NewSecretResolver(baseSecrets, config)
		result, err := resolver.Resolve()
		require.NoError(t, err)
		assert.Equal(t, map[string]string{
			"DATABASE_URL": "postgres://staging-db:5432/app",
			"API_KEY":      "staging-api-key-123",
		}, result)
	})

	t.Run("include mode with secret references", func(t *testing.T) {
		config := &EnvironmentSecretsConfigDescriptor{
			Mode: "include",
			Secrets: SecretsConfigMap{
				"DB_URL":  "${secret:DATABASE_URL_STAGING}",
				"API_KEY": "${secret:API_KEY_STAGING}",
			},
		}
		resolver := NewSecretResolver(baseSecrets, config)
		result, err := resolver.Resolve()
		require.NoError(t, err)
		assert.Equal(t, map[string]string{
			"DB_URL":  "postgres://localhost/staging",
			"API_KEY": "staging-key-456",
		}, result)
	})
}

func TestSecretResolver_ResolveExcludeMode(t *testing.T) {
	baseSecrets := map[string]string{
		"DATABASE_URL":           "postgres://localhost/prod",
		"DATABASE_URL_STAGING":   "postgres://localhost/staging",
		"API_KEY":                "prod-key-123",
		"API_KEY_STAGING":        "staging-key-456",
		"SECRET_KEY":             "prod-secret",
		"SECRET_KEY_STAGING":     "staging-secret",
		"STAGING_ONLY_SECRET":    "staging-only-value",
		"PRODUCTION_ONLY_SECRET": "production-only-value",
	}

	t.Run("exclude mode returns all secrets except specified ones", func(t *testing.T) {
		config := &EnvironmentSecretsConfigDescriptor{
			Mode:       "exclude",
			InheritAll: true,
			Secrets: SecretsConfigMap{
				"PRODUCTION_ONLY_SECRET": "",
				"DATABASE_URL":           "",
			},
		}
		resolver := NewSecretResolver(baseSecrets, config)
		result, err := resolver.Resolve()
		require.NoError(t, err)
		assert.Equal(t, map[string]string{
			"DATABASE_URL_STAGING": "postgres://localhost/staging",
			"API_KEY":              "prod-key-123",
			"API_KEY_STAGING":      "staging-key-456",
			"SECRET_KEY":           "prod-secret",
			"SECRET_KEY_STAGING":   "staging-secret",
			"STAGING_ONLY_SECRET":  "staging-only-value",
		}, result)
		assert.Len(t, result, 6)
	})

	t.Run("exclude mode without inheritAll returns error", func(t *testing.T) {
		config := &EnvironmentSecretsConfigDescriptor{
			Mode: "exclude",
			Secrets: SecretsConfigMap{
				"PRODUCTION_ONLY_SECRET": "",
			},
		}
		resolver := NewSecretResolver(baseSecrets, config)
		_, err := resolver.Resolve()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "inheritAll to be set to true")
	})
}

func TestSecretResolver_ResolveOverrideMode(t *testing.T) {
	baseSecrets := map[string]string{
		"DATABASE_URL":           "postgres://localhost/prod",
		"DATABASE_URL_STAGING":   "postgres://localhost/staging",
		"API_KEY":                "prod-key-123",
		"API_KEY_STAGING":        "staging-key-456",
		"SECRET_KEY":             "prod-secret",
		"SECRET_KEY_STAGING":     "staging-secret",
		"STAGING_ONLY_SECRET":    "staging-only-value",
		"PRODUCTION_ONLY_SECRET": "production-only-value",
	}

	t.Run("override mode returns all secrets with overrides applied", func(t *testing.T) {
		config := &EnvironmentSecretsConfigDescriptor{
			Mode: "override",
			Secrets: SecretsConfigMap{
				"DATABASE_URL": "DATABASE_URL_STAGING",
				"API_KEY":      "API_KEY_STAGING",
				"SECRET_KEY":   "SECRET_KEY_STAGING",
			},
		}
		resolver := NewSecretResolver(baseSecrets, config)
		result, err := resolver.Resolve()
		require.NoError(t, err)
		assert.Equal(t, "postgres://localhost/staging", result["DATABASE_URL"])
		assert.Equal(t, "staging-key-456", result["API_KEY"])
		assert.Equal(t, "staging-secret", result["SECRET_KEY"])
		assert.Equal(t, "production-only-value", result["PRODUCTION_ONLY_SECRET"])
		assert.Len(t, result, 8)
	})

	t.Run("override mode with literal values", func(t *testing.T) {
		config := &EnvironmentSecretsConfigDescriptor{
			Mode: "override",
			Secrets: SecretsConfigMap{
				"DATABASE_URL": "literal:postgres://override-db:5432/app",
			},
		}
		resolver := NewSecretResolver(baseSecrets, config)
		result, err := resolver.Resolve()
		require.NoError(t, err)
		assert.Equal(t, "postgres://override-db:5432/app", result["DATABASE_URL"])
		assert.Equal(t, "prod-key-123", result["API_KEY"])
	})
}

func TestSecretResolver_InvalidMode(t *testing.T) {
	baseSecrets := map[string]string{"KEY": "value"}
	config := &EnvironmentSecretsConfigDescriptor{
		Mode: "invalid",
	}
	resolver := NewSecretResolver(baseSecrets, config)
	_, err := resolver.Resolve()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid secrets mode")
}

func TestSecretResolver_InvalidSecretReference(t *testing.T) {
	baseSecrets := map[string]string{
		"VALID_KEY": "value",
	}
	t.Run("non-existent secret reference", func(t *testing.T) {
		config := &EnvironmentSecretsConfigDescriptor{
			Mode: "include",
			Secrets: SecretsConfigMap{
				"MY_KEY": "NON_EXISTENT_KEY",
			},
		}
		resolver := NewSecretResolver(baseSecrets, config)
		_, err := resolver.Resolve()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found in base secrets")
	})

	t.Run("invalid secret reference syntax", func(t *testing.T) {
		config := &EnvironmentSecretsConfigDescriptor{
			Mode: "include",
			Secrets: SecretsConfigMap{
				"MY_KEY": "${secret:INVALID",
			},
		}
		resolver := NewSecretResolver(baseSecrets, config)
		_, err := resolver.Resolve()
		assert.Error(t, err)
	})

	t.Run("non-existent secret in ${secret:} reference", func(t *testing.T) {
		config := &EnvironmentSecretsConfigDescriptor{
			Mode: "include",
			Secrets: SecretsConfigMap{
				"MY_KEY": "${secret:NON_EXISTENT}",
			},
		}
		resolver := NewSecretResolver(baseSecrets, config)
		_, err := resolver.Resolve()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found in base secrets")
	})
}

func TestValidateSecretReference(t *testing.T) {
	t.Run("valid direct reference", func(t *testing.T) {
		err := ValidateSecretReference("MY_SECRET_KEY")
		assert.NoError(t, err)
	})

	t.Run("valid secret reference", func(t *testing.T) {
		err := ValidateSecretReference("${secret:MY_KEY}")
		assert.NoError(t, err)
	})

	t.Run("valid literal value", func(t *testing.T) {
		err := ValidateSecretReference("literal:my-value")
		assert.NoError(t, err)
	})

	t.Run("invalid secret reference missing closing brace", func(t *testing.T) {
		err := ValidateSecretReference("${secret:INVALID")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "missing closing brace")
	})

	t.Run("invalid secret reference empty key", func(t *testing.T) {
		err := ValidateSecretReference("${secret:}")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "empty secret key")
	})
}

func TestIsSecretReference(t *testing.T) {
	t.Run("is a secret reference", func(t *testing.T) {
		assert.True(t, IsSecretReference("${secret:MY_KEY}"))
	})

	t.Run("is not a secret reference - direct", func(t *testing.T) {
		assert.False(t, IsSecretReference("MY_KEY"))
	})

	t.Run("is not a secret reference - literal", func(t *testing.T) {
		assert.False(t, IsSecretReference("literal:value"))
	})

	t.Run("is not a secret reference - missing prefix", func(t *testing.T) {
		assert.False(t, IsSecretReference("${MY_KEY}"))
	})
}

func TestSecretsConfigDescriptor_Copy(t *testing.T) {
	config := &SecretsConfigDescriptor{
		Type: "test-type",
		Inherit: Inherit{
			Inherit: "parent",
		},
		SecretsConfig: map[string]*EnvironmentSecretsConfigDescriptor{
			"staging": {
				Mode:       "include",
				InheritAll: false,
				Secrets: SecretsConfigMap{
					"KEY1": "VALUE1",
				},
			},
			"production": {
				Mode:       "exclude",
				InheritAll: true,
				Secrets: SecretsConfigMap{
					"KEY2": "VALUE2",
				},
			},
		},
	}

	copied := config.Copy()
	assert.Equal(t, config.Type, copied.Type)
	assert.Equal(t, config.Inherit, copied.Inherit)
	assert.NotNil(t, copied.SecretsConfig)

	// Verify deep copy
	copied.SecretsConfig["staging"].Mode = "override"
	assert.Equal(t, "include", config.SecretsConfig["staging"].Mode)
	assert.Equal(t, "override", copied.SecretsConfig["staging"].Mode)
}

func TestEnvironmentSecretsConfigDescriptor_Copy(t *testing.T) {
	config := &EnvironmentSecretsConfigDescriptor{
		Mode:       "include",
		InheritAll: true,
		Secrets: SecretsConfigMap{
			"KEY1": "VALUE1",
			"KEY2": "VALUE2",
		},
	}

	copied := config.Copy()
	assert.Equal(t, config.Mode, copied.Mode)
	assert.Equal(t, config.InheritAll, copied.InheritAll)
	assert.Equal(t, config.Secrets, copied.Secrets)

	// Verify deep copy
	copied.Secrets["KEY1"] = "MODIFIED"
	assert.Equal(t, "VALUE1", config.Secrets["KEY1"])
	assert.Equal(t, "MODIFIED", copied.Secrets["KEY1"])
}

func TestEnvironmentSecretsConfigDescriptor_CopyNil(t *testing.T) {
	var config *EnvironmentSecretsConfigDescriptor
	copied := config.Copy()
	assert.Nil(t, copied)
}

func TestDetectSecretsConfigType(t *testing.T) {
	t.Run("valid config with include mode", func(t *testing.T) {
		config := &SecretsConfigDescriptor{
			SecretsConfig: map[string]*EnvironmentSecretsConfigDescriptor{
				"staging": {
					Mode: "include",
					Secrets: SecretsConfigMap{
						"KEY": "VALUE",
					},
				},
			},
		}
		result, err := DetectSecretsConfigType(config)
		assert.NoError(t, err)
		assert.NotNil(t, result)
	})

	t.Run("valid config with exclude mode and inheritAll", func(t *testing.T) {
		config := &SecretsConfigDescriptor{
			SecretsConfig: map[string]*EnvironmentSecretsConfigDescriptor{
				"production": {
					Mode:       "exclude",
					InheritAll: true,
					Secrets: SecretsConfigMap{
						"KEY": "VALUE",
					},
				},
			},
		}
		result, err := DetectSecretsConfigType(config)
		assert.NoError(t, err)
		assert.NotNil(t, result)
	})

	t.Run("invalid mode", func(t *testing.T) {
		config := &SecretsConfigDescriptor{
			SecretsConfig: map[string]*EnvironmentSecretsConfigDescriptor{
				"staging": {
					Mode: "invalid",
				},
			},
		}
		_, err := DetectSecretsConfigType(config)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid mode")
	})

	t.Run("exclude mode without inheritAll", func(t *testing.T) {
		config := &SecretsConfigDescriptor{
			SecretsConfig: map[string]*EnvironmentSecretsConfigDescriptor{
				"staging": {
					Mode: "exclude",
				},
			},
		}
		_, err := DetectSecretsConfigType(config)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "inheritAll to be set to true")
	})

	t.Run("invalid secret reference", func(t *testing.T) {
		config := &SecretsConfigDescriptor{
			SecretsConfig: map[string]*EnvironmentSecretsConfigDescriptor{
				"staging": {
					Mode: "include",
					Secrets: SecretsConfigMap{
						"KEY": "${secret:INVALID",
					},
				},
			},
		}
		_, err := DetectSecretsConfigType(config)
		assert.Error(t, err)
	})

	t.Run("nil config", func(t *testing.T) {
		var config *SecretsConfigDescriptor
		result, err := DetectSecretsConfigType(config)
		assert.NoError(t, err)
		assert.Nil(t, result)
	})
}
