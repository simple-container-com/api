package modes

import (
	. "github.com/onsi/gomega"
	"context"
	"fmt"
	"strings"
	"testing"


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
		Expect(err).ToNot(HaveOccurred())
		Expect(yamlContent).ToNot(BeEmpty())

		// Validate against schema
		result := validator.ValidateClientYAML(context.Background(), yamlContent)
		Expect(result.Valid, "Generated client.yaml should be schema-compliant").To(BeTrue())

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
				Expect(err).ToNot(HaveOccurred())
				Expect(yamlContent).ToNot(BeEmpty())

				// Validate schema compliance
				result := validator.ValidateClientYAML(context.Background(), yamlContent)
				Expect(result.Valid, "Generated client.yaml should be schema-compliant").To(BeTrue())

				// Check for expected environment variables
				for envVar := range tc.expectEnv {
					Expect(yamlContent).To(ContainSubstring(envVar, "Should contain language-specific environment variable"))
				}

				// Ensure contains project name
				Expect(yamlContent).To(ContainSubstring(tc.analysis.Name, "Should contain project name"))
			})
		}
	})

	t.Run("test_schema_structure_validation", func(t *testing.T) {
		opts := &SetupOptions{
			Parent:      "myinfra",
			Environment: "staging",
		}

		yamlContent, err := devMode.generateFallbackClientYAML(opts, nil)
		Expect(err).ToNot(HaveOccurred())

		// Check required schema structure
		Expect(yamlContent).To(ContainSubstring("schemaVersion: 1.0", "Must have correct schema version"))
		Expect(yamlContent).To(ContainSubstring("stacks:", "Must have stacks section"))
		Expect(yamlContent).To(ContainSubstring("type: cloud-compose", "Must have correct stack type"))
		Expect(yamlContent).To(ContainSubstring("parent: mycompany/myinfra", "Must reference parent stack with project/stack format"))
		Expect(yamlContent).To(ContainSubstring("parentEnv: staging", "Must reference parent environment"))
		Expect(yamlContent).To(ContainSubstring("config:", "Must have config section"))
		Expect(yamlContent).To(ContainSubstring("runs: [app]", "Must have runs specification"))
		Expect(yamlContent).To(ContainSubstring("scale:", "Must have scale configuration"))
		Expect(yamlContent).To(ContainSubstring("min: 1", "Must have min scale"))
		Expect(yamlContent).To(ContainSubstring("max: 5", "Must have max scale"))
		Expect(yamlContent).To(ContainSubstring("env:", "Must have environment variables section"))
		Expect(yamlContent).To(ContainSubstring("secrets:", "Must have secrets section"))

		// Ensure no fictional properties
		Expect(yamlContent).ToNot(ContainSubstring("environments:"), "Must not use fictional environments section")
		Expect(yamlContent).ToNot(ContainSubstring("scaling:"), "Must not use fictional scaling section")
		Expect(yamlContent).ToNot(ContainSubstring("version:"), "Must not use fictional version property")
		Expect(yamlContent).ToNot(ContainSubstring("minCapacity"), "Must not use fictional minCapacity")
		Expect(yamlContent).ToNot(ContainSubstring("maxCapacity"), "Must not use fictional maxCapacity")
	})

	t.Run("test_secret_reference_format", func(t *testing.T) {
		opts := &SetupOptions{
			Parent:      "infrastructure",
			Environment: "production",
		}

		yamlContent, err := devMode.generateFallbackClientYAML(opts, nil)
		Expect(err).ToNot(HaveOccurred())

		// Check secret reference format
		Expect(yamlContent).To(ContainSubstring(`"${secret:jwt-secret}"`, "Must use correct quoted secret reference format"))
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
		Expect(hasUnquotedSecret, "Secret references should be properly quoted in YAML").To(BeFalse())
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
					Expect(exists, "Expected environment variable %s should exist", key).To(BeTrue())
					Expect(actualValue, "Environment variable %s should have correct value", key).To(Equal(expectedValue))
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
					Expect(exists, "Expected secret %s should exist", key).To(BeTrue())
					Expect(actualValue, "Secret %s should have correct value", key).To(Equal(expectedValue))
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
				Expect(err).ToNot(HaveOccurred())
				Expect(composeContent).ToNot(BeEmpty())

				// Validate Simple Container ingress labels
				Expect(composeContent).To(ContainSubstring(`"simple-container.com/ingress": "true"`, "Must contain ingress label"))
				Expect(composeContent).To(ContainSubstring(fmt.Sprintf(`"simple-container.com/ingress/port": "%s"`, tc.expectedPort)), "Must specify ingress port")
				Expect(composeContent).To(ContainSubstring(`"simple-container.com/healthcheck/path": "/health"`, "Must contain healthcheck path"))

				// Validate volume labels
				Expect(composeContent).To(ContainSubstring("volumes:", "Must have volumes section"))
				Expect(composeContent).To(ContainSubstring(`"simple-container.com/volume-size": "10Gi"`, "Must specify volume size"))
				Expect(composeContent).To(ContainSubstring(`"simple-container.com/volume-storage-class": "gp3"`, "Must specify storage class"))
				Expect(composeContent).To(ContainSubstring(`"simple-container.com/volume-access-modes": "ReadWriteOnce"`, "Must specify access modes"))

				// Validate structure requirements
				Expect(composeContent).To(ContainSubstring("version: '3.8'", "Must have proper version"))
				Expect(composeContent).To(ContainSubstring("services:", "Must have services section"))
				Expect(composeContent).To(ContainSubstring("restart: unless-stopped", "Must have restart policy"))
				Expect(composeContent).To(ContainSubstring("networks:", "Must have networks section"))
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

		Expect(devMode.validateComposeContent(validComposeWithSCLabels).To(BeTrue()), "Should validate compose with SC labels")
		Expect(devMode.validateComposeContent(invalidComposeNoIngressLabel).To(BeFalse()), "Should fail validation without ingress label")
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
		Expect(prompt).To(ContainSubstring("simple-container.com/ingress", "Prompt should mention ingress labels"))
		Expect(prompt).To(ContainSubstring("simple-container.com/volume-size", "Prompt should mention volume size labels"))
		Expect(prompt).To(ContainSubstring("simple-container.com/healthcheck", "Prompt should mention healthcheck labels"))
		Expect(prompt).To(ContainSubstring("Create separate volumes block", "Prompt should emphasize separate volumes block"))
		Expect(prompt).To(ContainSubstring("ALL required volumes", "Prompt should emphasize all volumes need labels"))
	})
}
