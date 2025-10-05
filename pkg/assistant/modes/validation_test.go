package modes

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/simple-container-com/api/pkg/assistant/analysis"
	"github.com/simple-container-com/api/pkg/assistant/validation"
)

func TestDeveloperModeValidation(t *testing.T) {
	devMode := NewDeveloperMode()
	validator := validation.NewValidator()

	t.Run("test_fallback_client_yaml_validation", func(t *testing.T) {
		// Test basic fallback generation
		opts := &SetupOptions{
			Parent:      "infrastructure",
			Environment: "production",
		}

		yamlContent, err := devMode.generateFallbackClientYAML(opts, nil)
		require.NoError(t, err)
		require.NotEmpty(t, yamlContent)

		// Validate against schema
		result := validator.ValidateClientYAML(context.Background(), yamlContent)
		assert.True(t, result.Valid, "Generated client.yaml should be schema-compliant")

		if !result.Valid {
			t.Logf("Validation errors: %v", result.Errors)
			t.Logf("Generated YAML:\n%s", yamlContent)
		}
	})

	t.Run("test_language_specific_fallbacks", func(t *testing.T) {
		testCases := []struct {
			name      string
			analysis  *analysis.ProjectAnalysis
			expectEnv map[string]bool // Expected environment variables to be present
		}{
			{
				name: "nodejs_express_project",
				analysis: &analysis.ProjectAnalysis{
					Name: "express-app",
					PrimaryStack: &analysis.TechStackInfo{
						Language:  "nodejs",
						Framework: "express",
					},
				},
				expectEnv: map[string]bool{
					"NODE_ENV": true,
					"PORT":     true,
				},
			},
			{
				name: "python_django_project",
				analysis: &analysis.ProjectAnalysis{
					Name: "django-app",
					PrimaryStack: &analysis.TechStackInfo{
						Language:  "python",
						Framework: "django",
					},
				},
				expectEnv: map[string]bool{
					"PYTHON_ENV":             true,
					"DJANGO_SETTINGS_MODULE": true,
					"PORT":                   true,
				},
			},
			{
				name: "go_gin_project",
				analysis: &analysis.ProjectAnalysis{
					Name: "gin-api",
					PrimaryStack: &analysis.TechStackInfo{
						Language:  "go",
						Framework: "gin",
					},
				},
				expectEnv: map[string]bool{
					"GO_ENV":   true,
					"GIN_MODE": true,
					"PORT":     true,
				},
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				opts := &SetupOptions{
					Parent:      "infrastructure",
					Environment: "production",
				}

				yamlContent, err := devMode.generateFallbackClientYAML(opts, tc.analysis)
				require.NoError(t, err)
				require.NotEmpty(t, yamlContent)

				// Validate schema compliance
				result := validator.ValidateClientYAML(context.Background(), yamlContent)
				assert.True(t, result.Valid, "Generated client.yaml should be schema-compliant")

				// Check for expected environment variables
				for envVar := range tc.expectEnv {
					assert.Contains(t, yamlContent, envVar, "Should contain language-specific environment variable")
				}

				// Ensure contains project name
				assert.Contains(t, yamlContent, tc.analysis.Name, "Should contain project name")
			})
		}
	})

	t.Run("test_schema_structure_validation", func(t *testing.T) {
		opts := &SetupOptions{
			Parent:      "myinfra",
			Environment: "staging",
		}

		yamlContent, err := devMode.generateFallbackClientYAML(opts, nil)
		require.NoError(t, err)

		// Check required schema structure
		assert.Contains(t, yamlContent, "schemaVersion: 1.0", "Must have correct schema version")
		assert.Contains(t, yamlContent, "stacks:", "Must have stacks section")
		assert.Contains(t, yamlContent, "type: cloud-compose", "Must have correct stack type")
		assert.Contains(t, yamlContent, "parent: myinfra", "Must reference parent stack")
		assert.Contains(t, yamlContent, "parentEnv: staging", "Must reference parent environment")
		assert.Contains(t, yamlContent, "config:", "Must have config section")
		assert.Contains(t, yamlContent, "runs: [app]", "Must have runs specification")
		assert.Contains(t, yamlContent, "scale:", "Must have scale configuration")
		assert.Contains(t, yamlContent, "min: 1", "Must have min scale")
		assert.Contains(t, yamlContent, "max: 5", "Must have max scale")
		assert.Contains(t, yamlContent, "env:", "Must have environment variables section")
		assert.Contains(t, yamlContent, "secrets:", "Must have secrets section")

		// Ensure no fictional properties
		assert.NotContains(t, yamlContent, "environments:", "Must not use fictional environments section")
		assert.NotContains(t, yamlContent, "scaling:", "Must not use fictional scaling section")
		assert.NotContains(t, yamlContent, "version:", "Must not use fictional version property")
		assert.NotContains(t, yamlContent, "minCapacity", "Must not use fictional minCapacity")
		assert.NotContains(t, yamlContent, "maxCapacity", "Must not use fictional maxCapacity")
	})

	t.Run("test_secret_reference_format", func(t *testing.T) {
		opts := &SetupOptions{
			Parent:      "infrastructure",
			Environment: "production",
		}

		yamlContent, err := devMode.generateFallbackClientYAML(opts, nil)
		require.NoError(t, err)

		// Check secret reference format
		assert.Contains(t, yamlContent, `"${secret:jwt-secret}"`, "Must use correct quoted secret reference format")
		// Check that unquoted versions are not present (except inside the quoted strings)
		lines := strings.Split(yamlContent, "\n")
		hasUnquotedSecret := false
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			// Check for unquoted secret references (not part of quoted strings)
			if strings.Contains(trimmed, "${secret:") && !strings.Contains(trimmed, `"${secret:`) {
				hasUnquotedSecret = true
				break
			}
		}
		assert.False(t, hasUnquotedSecret, "Secret references should be properly quoted in YAML")
	})
}

func TestLanguageSpecificGeneration(t *testing.T) {
	devMode := NewDeveloperMode()

	t.Run("test_build_language_specific_env_vars", func(t *testing.T) {
		testCases := []struct {
			name     string
			analysis *analysis.ProjectAnalysis
			expected map[string]string
		}{
			{
				name:     "nil_analysis",
				analysis: nil,
				expected: map[string]string{
					"PORT": "3000",
				},
			},
			{
				name: "nodejs_express",
				analysis: &analysis.ProjectAnalysis{
					PrimaryStack: &analysis.TechStackInfo{
						Language:  "nodejs",
						Framework: "express",
					},
				},
				expected: map[string]string{
					"NODE_ENV": "production",
					"PORT":     "3000",
				},
			},
			{
				name: "python_django",
				analysis: &analysis.ProjectAnalysis{
					PrimaryStack: &analysis.TechStackInfo{
						Language:  "python",
						Framework: "django",
					},
				},
				expected: map[string]string{
					"PYTHON_ENV":             "production",
					"PORT":                   "8000",
					"DJANGO_SETTINGS_MODULE": "myapp.settings.production",
				},
			},
			{
				name: "go_gin",
				analysis: &analysis.ProjectAnalysis{
					PrimaryStack: &analysis.TechStackInfo{
						Language:  "go",
						Framework: "gin",
					},
				},
				expected: map[string]string{
					"GO_ENV":   "production",
					"PORT":     "8080",
					"GIN_MODE": "release",
				},
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				result := devMode.buildLanguageSpecificEnvVars(tc.analysis)

				for key, expectedValue := range tc.expected {
					actualValue, exists := result[key]
					assert.True(t, exists, "Expected environment variable %s should exist", key)
					assert.Equal(t, expectedValue, actualValue, "Environment variable %s should have correct value", key)
				}
			})
		}
	})

	t.Run("test_build_language_specific_secrets", func(t *testing.T) {
		testCases := []struct {
			name     string
			analysis *analysis.ProjectAnalysis
			expected map[string]string
		}{
			{
				name:     "nil_analysis",
				analysis: nil,
				expected: map[string]string{
					"JWT_SECRET": "${secret:jwt-secret}",
				},
			},
			{
				name: "nodejs_nextjs",
				analysis: &analysis.ProjectAnalysis{
					PrimaryStack: &analysis.TechStackInfo{
						Language:  "nodejs",
						Framework: "nextjs",
					},
				},
				expected: map[string]string{
					"JWT_SECRET":      "${secret:jwt-secret}",
					"NEXTAUTH_SECRET": "${secret:nextauth-secret}",
					"SESSION_SECRET":  "${secret:session-secret}",
				},
			},
			{
				name: "python_flask",
				analysis: &analysis.ProjectAnalysis{
					PrimaryStack: &analysis.TechStackInfo{
						Language:  "python",
						Framework: "flask",
					},
				},
				expected: map[string]string{
					"JWT_SECRET":       "${secret:jwt-secret}",
					"FLASK_SECRET_KEY": "${secret:flask-secret}",
				},
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				result := devMode.buildLanguageSpecificSecrets(tc.analysis)

				for key, expectedValue := range tc.expected {
					actualValue, exists := result[key]
					assert.True(t, exists, "Expected secret %s should exist", key)
					assert.Equal(t, expectedValue, actualValue, "Secret %s should have correct value", key)
				}
			})
		}
	})
}

func TestDockerComposeGeneration(t *testing.T) {
	devMode := NewDeveloperMode()

	t.Run("test_fallback_compose_includes_sc_labels", func(t *testing.T) {
		testCases := []struct {
			name         string
			analysis     *analysis.ProjectAnalysis
			expectedPort string
		}{
			{
				name:         "default_nodejs",
				analysis:     nil,
				expectedPort: "3000",
			},
			{
				name: "python_project",
				analysis: &analysis.ProjectAnalysis{
					PrimaryStack: &analysis.TechStackInfo{
						Language: "python",
					},
				},
				expectedPort: "8000",
			},
			{
				name: "go_project",
				analysis: &analysis.ProjectAnalysis{
					PrimaryStack: &analysis.TechStackInfo{
						Language: "go",
					},
				},
				expectedPort: "8080",
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				composeContent, err := devMode.generateFallbackComposeYAML(tc.analysis)
				require.NoError(t, err)
				require.NotEmpty(t, composeContent)

				// Validate Simple Container ingress labels
				assert.Contains(t, composeContent, `"simple-container.com/ingress": "true"`, "Must contain ingress label")
				assert.Contains(t, composeContent, fmt.Sprintf(`"simple-container.com/ingress/port": "%s"`, tc.expectedPort), "Must specify ingress port")
				assert.Contains(t, composeContent, `"simple-container.com/healthcheck/path": "/health"`, "Must contain healthcheck path")

				// Validate volume labels
				assert.Contains(t, composeContent, "volumes:", "Must have volumes section")
				assert.Contains(t, composeContent, `"simple-container.com/volume-size": "10Gi"`, "Must specify volume size")
				assert.Contains(t, composeContent, `"simple-container.com/volume-storage-class": "gp3"`, "Must specify storage class")
				assert.Contains(t, composeContent, `"simple-container.com/volume-access-modes": "ReadWriteOnce"`, "Must specify access modes")

				// Validate structure requirements
				assert.Contains(t, composeContent, "version: '3.8'", "Must have proper version")
				assert.Contains(t, composeContent, "services:", "Must have services section")
				assert.Contains(t, composeContent, "restart: unless-stopped", "Must have restart policy")
				assert.Contains(t, composeContent, "networks:", "Must have networks section")
			})
		}
	})

	t.Run("test_compose_validation_function", func(t *testing.T) {
		validComposeWithSCLabels := `version: '3.8'
services:
  app:
    build: .
    labels:
      "simple-container.com/ingress": "true"
    ports:
      - "3000:3000"
    environment:
      - NODE_ENV=development
    restart: unless-stopped

volumes:
  app_data:
    labels:
      "simple-container.com/volume-size": "10Gi"`

		invalidComposeNoIngressLabel := `version: '3.8'
services:
  app:
    build: .
    ports:
      - "3000:3000"
    environment:
      - NODE_ENV=development`

		assert.True(t, devMode.validateComposeContent(validComposeWithSCLabels), "Should validate compose with SC labels")
		assert.False(t, devMode.validateComposeContent(invalidComposeNoIngressLabel), "Should fail validation without ingress label")
	})

	t.Run("test_compose_prompt_includes_sc_instructions", func(t *testing.T) {
		analysis := &analysis.ProjectAnalysis{
			PrimaryStack: &analysis.TechStackInfo{
				Language:  "nodejs",
				Framework: "express",
			},
		}

		prompt := devMode.buildComposeYAMLPrompt(analysis)

		// Check that prompt includes Simple Container label instructions
		assert.Contains(t, prompt, "simple-container.com/ingress", "Prompt should mention ingress labels")
		assert.Contains(t, prompt, "simple-container.com/volume-size", "Prompt should mention volume size labels")
		assert.Contains(t, prompt, "simple-container.com/healthcheck", "Prompt should mention healthcheck labels")
		assert.Contains(t, prompt, "Create separate volumes block", "Prompt should emphasize separate volumes block")
		assert.Contains(t, prompt, "ALL required volumes", "Prompt should emphasize all volumes need labels")
	})
}
