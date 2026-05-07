package placeholders

import (
	"testing"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTemplateSecrets_WithEnvironmentContext(t *testing.T) {
	p := &placeholders{}

	tests := []struct {
		name           string
		stackName      string
		stack          api.Stack
		stacks         api.StacksMap
		path           string
		expectedSecret string
		expectError    bool
	}{
		{
			name:      "shared secret without environment",
			stackName: "test-stack",
			stack: api.Stack{
				Secrets: api.SecretsDescriptor{
					SchemaVersion: "2.0",
					Values: map[string]string{
						"API_KEY": "shared-api-key",
					},
				},
			},
			stacks: api.StacksMap{
				"test-stack": {},
			},
			path:           "API_KEY",
			expectedSecret: "shared-api-key",
			expectError:    false,
		},
		{
			name:      "environment-specific secret from server environment",
			stackName: "test-stack",
			stack: api.Stack{
				Secrets: api.SecretsDescriptor{
					SchemaVersion: "2.0",
					Values: map[string]string{
						"API_KEY": "shared-api-key",
					},
					Environments: map[string]api.EnvironmentSecrets{
						"production": {
							Values: map[string]string{
								"API_KEY": "prod-api-key",
							},
						},
					},
				},
				Server: api.ServerDescriptor{
					Environment: "production",
				},
			},
			stacks: api.StacksMap{
				"test-stack": {},
			},
			path:           "API_KEY",
			expectedSecret: "prod-api-key",
			expectError:    false,
		},
		{
			name:      "explicit environment override in placeholder",
			stackName: "test-stack",
			stack: api.Stack{
				Secrets: api.SecretsDescriptor{
					SchemaVersion: "2.0",
					Values: map[string]string{
						"API_KEY": "shared-api-key",
					},
					Environments: map[string]api.EnvironmentSecrets{
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
			},
			stacks: api.StacksMap{
				"test-stack": {},
			},
			path:           "API_KEY:staging",
			expectedSecret: "staging-api-key",
			expectError:    false,
		},
		{
			name:      "fallback to shared when environment-specific not found",
			stackName: "test-stack",
			stack: api.Stack{
				Secrets: api.SecretsDescriptor{
					SchemaVersion: "2.0",
					Values: map[string]string{
						"SHARED_KEY": "shared-value",
					},
					Environments: map[string]api.EnvironmentSecrets{
						"production": {
							Values: map[string]string{
								"API_KEY": "prod-api-key",
							},
						},
					},
				},
				Server: api.ServerDescriptor{
					Environment: "production",
				},
			},
			stacks: api.StacksMap{
				"test-stack": {},
			},
			path:           "SHARED_KEY",
			expectedSecret: "shared-value",
			expectError:    false,
		},
		{
			name:      "secret not found in environment",
			stackName: "test-stack",
			stack: api.Stack{
				Secrets: api.SecretsDescriptor{
					SchemaVersion: "2.0",
					Values: map[string]string{
						"API_KEY": "shared-api-key",
					},
					Environments: map[string]api.EnvironmentSecrets{
						"production": {
							Values: map[string]string{
								"API_KEY": "prod-api-key",
							},
						},
					},
				},
				Server: api.ServerDescriptor{
					Environment: "production",
				},
			},
			stacks: api.StacksMap{
				"test-stack": {},
			},
			path:           "NONEXISTENT",
			expectedSecret: "",
			expectError:    true,
		},
		{
			name:      "inherited secret from parent stack with environment",
			stackName: "child-stack",
			stack: api.Stack{
				Server: api.ServerDescriptor{
					Secrets: api.SecretsConfigDescriptor{
						Inherit: api.Inherit{
							Inherit: "parent-stack",
						},
					},
					Environment: "production",
				},
			},
			stacks: api.StacksMap{
				"child-stack": {},
				"parent-stack": {
					Secrets: api.SecretsDescriptor{
						SchemaVersion: "2.0",
						Values: map[string]string{
							"SHARED_KEY": "shared-from-parent",
						},
						Environments: map[string]api.EnvironmentSecrets{
							"production": {
								Values: map[string]string{
									"API_KEY": "prod-api-from-parent",
								},
							},
							"staging": {
								Values: map[string]string{
									"API_KEY": "staging-api-from-parent",
								},
							},
						},
					},
				},
			},
			path:           "API_KEY",
			expectedSecret: "prod-api-from-parent",
			expectError:    false,
		},
		{
			name:      "inherited secret from parent stack with explicit environment",
			stackName: "child-stack",
			stack: api.Stack{
				Server: api.ServerDescriptor{
					Secrets: api.SecretsConfigDescriptor{
						Inherit: api.Inherit{
							Inherit: "parent-stack",
						},
					},
				},
			},
			stacks: api.StacksMap{
				"child-stack": {},
				"parent-stack": {
					Secrets: api.SecretsDescriptor{
						SchemaVersion: "2.0",
						Environments: map[string]api.EnvironmentSecrets{
							"production": {
								Values: map[string]string{
									"API_KEY": "prod-api-from-parent",
								},
							},
							"staging": {
								Values: map[string]string{
									"API_KEY": "staging-api-from-parent",
								},
							},
						},
					},
				},
			},
			path:           "API_KEY:staging",
			expectedSecret: "staging-api-from-parent",
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tplSecrets := p.tplSecrets(tt.stackName, tt.stack, tt.stacks)
			result, err := tplSecrets("${secret:" + tt.path + "}", tt.path, nil)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedSecret, result)
			}
		})
	}
}

func TestTemplateSecrets_BackwardCompatibility(t *testing.T) {
	p := &placeholders{}

	// Test v1.0 schema (no environments) still works
	v1Stack := api.Stack{
		Secrets: api.SecretsDescriptor{
			SchemaVersion: "1.0",
			Values: map[string]string{
				"API_KEY": "shared-api-key",
				"DB_PASS": "shared-db-pass",
			},
		},
	}

	stacks := api.StacksMap{
		"test-stack": v1Stack,
	}

	tplSecrets := p.tplSecrets("test-stack", v1Stack, stacks)

	// Should be able to get shared secrets
	result, err := tplSecrets("${secret:API_KEY}", "API_KEY", nil)
	require.NoError(t, err)
	assert.Equal(t, "shared-api-key", result)

	result, err = tplSecrets("${secret:DB_PASS}", "DB_PASS", nil)
	require.NoError(t, err)
	assert.Equal(t, "shared-db-pass", result)

	// Should error for non-existent secret
	_, err = tplSecrets("${secret:NONEXISTENT}", "NONEXISTENT", nil)
	assert.Error(t, err)
}
