package api

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSecretsDescriptor_GetSecretValue(t *testing.T) {
	tests := []struct {
		name        string
		descriptor  SecretsDescriptor
		secretName  string
		environment string
		expected    string
		found       bool
	}{
		{
			name: "shared secret (backward compatibility)",
			descriptor: SecretsDescriptor{
				SchemaVersion: "2.0",
				Values: map[string]string{
					"API_KEY": "shared-api-key",
				},
			},
			secretName:  "API_KEY",
			environment: "",
			expected:    "shared-api-key",
			found:       true,
		},
		{
			name: "environment-specific secret",
			descriptor: SecretsDescriptor{
				SchemaVersion: "2.0",
				Values: map[string]string{
					"API_KEY": "shared-api-key",
				},
				Environments: map[string]EnvironmentSecrets{
					"production": {
						Values: map[string]string{
							"API_KEY": "prod-api-key",
							"DB_PASS": "prod-db-pass",
						},
					},
				},
			},
			secretName:  "API_KEY",
			environment: "production",
			expected:    "prod-api-key",
			found:       true,
		},
		{
			name: "fallback to shared when environment-specific not found",
			descriptor: SecretsDescriptor{
				SchemaVersion: "2.0",
				Values: map[string]string{
					"API_KEY": "shared-api-key",
				},
				Environments: map[string]EnvironmentSecrets{
					"production": {
						Values: map[string]string{
							"DB_PASS": "prod-db-pass",
						},
					},
				},
			},
			secretName:  "API_KEY",
			environment: "production",
			expected:    "shared-api-key",
			found:       true,
		},
		{
			name: "secret not found",
			descriptor: SecretsDescriptor{
				SchemaVersion: "2.0",
				Values: map[string]string{
					"API_KEY": "shared-api-key",
				},
			},
			secretName:  "NONEXISTENT",
			environment: "",
			expected:    "",
			found:       false,
		},
		{
			name: "environment-specific secret not found, shared also not found",
			descriptor: SecretsDescriptor{
				SchemaVersion: "2.0",
				Environments: map[string]EnvironmentSecrets{
					"production": {
						Values: map[string]string{
							"API_KEY": "prod-api-key",
						},
					},
				},
			},
			secretName:  "DB_PASS",
			environment: "production",
			expected:    "",
			found:       false,
		},
		{
			name: "multiple environments",
			descriptor: SecretsDescriptor{
				SchemaVersion: "2.0",
				Values: map[string]string{
					"SHARED": "shared-value",
				},
				Environments: map[string]EnvironmentSecrets{
					"production": {
						Values: map[string]string{
							"API_KEY": "prod-api-key",
						},
					},
					"staging": {
						Values: map[string]string{
							"API_KEY": "staging-api-key",
						},
					},
				},
			},
			secretName:  "API_KEY",
			environment: "staging",
			expected:    "staging-api-key",
			found:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value, found := tt.descriptor.GetSecretValue(tt.secretName, tt.environment)
			assert.Equal(t, tt.found, found)
			if found {
				assert.Equal(t, tt.expected, value)
			}
		})
	}
}

func TestSecretsDescriptor_HasEnvironment(t *testing.T) {
	descriptor := SecretsDescriptor{
		SchemaVersion: "2.0",
		Environments: map[string]EnvironmentSecrets{
			"production": {
				Values: map[string]string{
					"API_KEY": "prod-api-key",
				},
			},
		},
	}

	assert.True(t, descriptor.HasEnvironment("production"))
	assert.False(t, descriptor.HasEnvironment("staging"))
	assert.False(t, descriptor.HasEnvironment(""))
}

func TestSecretsDescriptor_GetEnvironments(t *testing.T) {
	descriptor := SecretsDescriptor{
		SchemaVersion: "2.0",
		Environments: map[string]EnvironmentSecrets{
			"production": {
				Values: map[string]string{
					"API_KEY": "prod-api-key",
				},
			},
			"staging": {
				Values: map[string]string{
					"API_KEY": "staging-api-key",
				},
			},
		},
	}

	environments := descriptor.GetEnvironments()
	assert.Len(t, environments, 2)
	assert.Contains(t, environments, "production")
	assert.Contains(t, environments, "staging")
}

func TestSecretsDescriptor_IsV2Schema(t *testing.T) {
	tests := []struct {
		name     string
		isV2     bool
		descriptor SecretsDescriptor
	}{
		{
			name: "v1.0 schema (no environments)",
			isV2: false,
			descriptor: SecretsDescriptor{
				SchemaVersion: "1.0",
				Values: map[string]string{
					"API_KEY": "shared-api-key",
				},
			},
		},
		{
			name: "v2.0 schema with environments",
			isV2: true,
			descriptor: SecretsDescriptor{
				SchemaVersion: "2.0",
				Environments: map[string]EnvironmentSecrets{
					"production": {
						Values: map[string]string{
							"API_KEY": "prod-api-key",
						},
					},
				},
			},
		},
		{
			name: "v2.0 schema version but no environments",
			isV2: false,
			descriptor: SecretsDescriptor{
				SchemaVersion: "2.0",
				Values: map[string]string{
					"API_KEY": "shared-api-key",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.isV2, tt.descriptor.IsV2Schema())
		})
	}
}

func TestSecretsDescriptor_Copy(t *testing.T) {
	original := SecretsDescriptor{
		SchemaVersion: "2.0",
		Values: map[string]string{
			"SHARED": "shared-value",
		},
		Environments: map[string]EnvironmentSecrets{
			"production": {
				Values: map[string]string{
					"API_KEY": "prod-api-key",
				},
			},
		},
		Auth: map[string]AuthDescriptor{
			"test": {
				Type: "test-type",
			},
		},
	}

	copy := original.Copy()

	// Verify values are copied
	assert.Equal(t, original.SchemaVersion, copy.SchemaVersion)
	assert.Equal(t, original.Values, copy.Values)
	assert.Equal(t, original.Environments, copy.Environments)

	// Verify it's a deep copy (modifying copy doesn't affect original)
	copy.Values["SHARED"] = "modified"
	assert.Equal(t, "shared-value", original.Values["SHARED"])
	assert.Equal(t, "modified", copy.Values["SHARED"])

	copy.Environments["production"].Values["API_KEY"] = "modified"
	assert.Equal(t, "prod-api-key", original.Environments["production"].Values["API_KEY"])
	assert.Equal(t, "modified", copy.Environments["production"].Values["API_KEY"])
}

func TestEnvironmentSecrets_Copy(t *testing.T) {
	original := EnvironmentSecrets{
		Values: map[string]string{
			"API_KEY": "prod-api-key",
			"DB_PASS": "prod-db-pass",
		},
	}

	copy := original.Copy()

	// Verify values are copied
	assert.Equal(t, original.Values, copy.Values)

	// Verify it's a deep copy
	copy.Values["API_KEY"] = "modified"
	assert.Equal(t, "prod-api-key", original.Values["API_KEY"])
	assert.Equal(t, "modified", copy.Values["API_KEY"])
}

func TestSecretsDescriptor_WithParentStackInheritance(t *testing.T) {
	// This test simulates the parent stack inheritance scenario
	// where a child stack needs to get environment-specific secrets from parent

	parentStack := Stack{
		Name: "parent",
		Secrets: SecretsDescriptor{
			SchemaVersion: "2.0",
			Values: map[string]string{
				"SHARED_KEY": "shared-from-parent",
			},
			Environments: map[string]EnvironmentSecrets{
				"production": {
					Values: map[string]string{
						"API_KEY": "prod-api-from-parent",
						"DB_PASS": "prod-db-from-parent",
					},
				},
				"staging": {
					Values: map[string]string{
						"API_KEY": "staging-api-from-parent",
					},
				},
			},
		},
	}

	tests := []struct {
		name        string
		environment string
		secretName  string
		expected    string
		found       bool
	}{
		{
			name:        "child stack gets prod secret from parent",
			environment: "production",
			secretName:  "API_KEY",
			expected:    "prod-api-from-parent",
			found:       true,
		},
		{
			name:        "child stack gets staging secret from parent",
			environment: "staging",
			secretName:  "API_KEY",
			expected:    "staging-api-from-parent",
			found:       true,
		},
		{
			name:        "child stack falls back to shared from parent",
			environment: "production",
			secretName:  "SHARED_KEY",
			expected:    "shared-from-parent",
			found:       true,
		},
		{
			name:        "child stack secret not found in parent environment",
			environment: "production",
			secretName:  "NONEXISTENT",
			expected:    "",
			found:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value, found := parentStack.Secrets.GetSecretValue(tt.secretName, tt.environment)
			assert.Equal(t, tt.found, found)
			if found {
				assert.Equal(t, tt.expected, value)
			}
		})
	}
}

func TestSecretsDescriptor_BackwardCompatibility(t *testing.T) {
	// Test that v1.0 secrets still work with the new schema
	v1Descriptor := SecretsDescriptor{
		SchemaVersion: "1.0",
		Values: map[string]string{
			"API_KEY": "shared-api-key",
			"DB_PASS": "shared-db-pass",
		},
	}

	// Should be able to get shared secrets without environment
	value, found := v1Descriptor.GetSecretValue("API_KEY", "")
	require.True(t, found)
	assert.Equal(t, "shared-api-key", value)

	// Should return false for environment lookup in v1.0
	value, found = v1Descriptor.GetSecretValue("API_KEY", "production")
	// Falls back to shared for backward compatibility
	require.True(t, found)
	assert.Equal(t, "shared-api-key", value)

	// Should not be considered v2 schema
	assert.False(t, v1Descriptor.IsV2Schema())
}
