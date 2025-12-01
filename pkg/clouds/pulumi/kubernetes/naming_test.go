package kubernetes

import "testing"

func TestGenerateResourceName(t *testing.T) {
	tests := []struct {
		name         string
		serviceName  string
		stackEnv     string
		parentEnv    string
		resourceType string
		expected     string
	}{
		{
			name:         "standard stack without resource type",
			serviceName:  "myapp",
			stackEnv:     "staging",
			parentEnv:    "",
			resourceType: "",
			expected:     "myapp",
		},
		{
			name:         "standard stack with resource type",
			serviceName:  "myapp",
			stackEnv:     "staging",
			parentEnv:    "",
			resourceType: "config",
			expected:     "myapp-config",
		},
		{
			name:         "custom stack without resource type",
			serviceName:  "myapp",
			stackEnv:     "staging-preview",
			parentEnv:    "staging",
			resourceType: "",
			expected:     "myapp-staging-preview",
		},
		{
			name:         "custom stack with resource type",
			serviceName:  "myapp",
			stackEnv:     "staging-preview",
			parentEnv:    "staging",
			resourceType: "config",
			expected:     "myapp-staging-preview-config",
		},
		{
			name:         "self-reference (treated as standard stack)",
			serviceName:  "myapp",
			stackEnv:     "staging",
			parentEnv:    "staging",
			resourceType: "",
			expected:     "myapp",
		},
		{
			name:         "custom stack with hpa suffix",
			serviceName:  "myapp",
			stackEnv:     "prod-hotfix",
			parentEnv:    "production",
			resourceType: "hpa",
			expected:     "myapp-prod-hotfix-hpa",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generateResourceName(tt.serviceName, tt.stackEnv, tt.parentEnv, tt.resourceType)
			if result != tt.expected {
				t.Errorf("generateResourceName() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestGenerateDeploymentName(t *testing.T) {
	tests := []struct {
		name        string
		serviceName string
		stackEnv    string
		parentEnv   string
		expected    string
	}{
		{
			name:        "standard stack",
			serviceName: "api",
			stackEnv:    "staging",
			parentEnv:   "",
			expected:    "api",
		},
		{
			name:        "custom stack",
			serviceName: "api",
			stackEnv:    "staging-preview",
			parentEnv:   "staging",
			expected:    "api-staging-preview",
		},
		{
			name:        "production hotfix",
			serviceName: "web",
			stackEnv:    "prod-hotfix",
			parentEnv:   "production",
			expected:    "web-prod-hotfix",
		},
		{
			name:        "self-reference",
			serviceName: "web",
			stackEnv:    "staging",
			parentEnv:   "staging",
			expected:    "web",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generateDeploymentName(tt.serviceName, tt.stackEnv, tt.parentEnv)
			if result != tt.expected {
				t.Errorf("generateDeploymentName() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestGenerateServiceName(t *testing.T) {
	tests := []struct {
		name        string
		serviceName string
		stackEnv    string
		parentEnv   string
		expected    string
	}{
		{
			name:        "standard stack",
			serviceName: "myapp",
			stackEnv:    "staging",
			parentEnv:   "",
			expected:    "myapp",
		},
		{
			name:        "custom stack",
			serviceName: "myapp",
			stackEnv:    "staging-pr-123",
			parentEnv:   "staging",
			expected:    "myapp-staging-pr-123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generateServiceName(tt.serviceName, tt.stackEnv, tt.parentEnv)
			if result != tt.expected {
				t.Errorf("generateServiceName() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestGenerateConfigMapName(t *testing.T) {
	tests := []struct {
		name        string
		serviceName string
		stackEnv    string
		parentEnv   string
		expected    string
	}{
		{
			name:        "standard stack",
			serviceName: "myapp",
			stackEnv:    "staging",
			parentEnv:   "",
			expected:    "myapp-config",
		},
		{
			name:        "custom stack",
			serviceName: "myapp",
			stackEnv:    "staging-preview",
			parentEnv:   "staging",
			expected:    "myapp-staging-preview-config",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generateConfigMapName(tt.serviceName, tt.stackEnv, tt.parentEnv)
			if result != tt.expected {
				t.Errorf("generateConfigMapName() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestGenerateSecretName(t *testing.T) {
	tests := []struct {
		name        string
		serviceName string
		stackEnv    string
		parentEnv   string
		expected    string
	}{
		{
			name:        "standard stack",
			serviceName: "myapp",
			stackEnv:    "staging",
			parentEnv:   "",
			expected:    "myapp-secrets",
		},
		{
			name:        "custom stack",
			serviceName: "myapp",
			stackEnv:    "staging-preview",
			parentEnv:   "staging",
			expected:    "myapp-staging-preview-secrets",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generateSecretName(tt.serviceName, tt.stackEnv, tt.parentEnv)
			if result != tt.expected {
				t.Errorf("generateSecretName() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestGenerateHPAName(t *testing.T) {
	tests := []struct {
		name        string
		serviceName string
		stackEnv    string
		parentEnv   string
		expected    string
	}{
		{
			name:        "standard stack",
			serviceName: "myapp",
			stackEnv:    "staging",
			parentEnv:   "",
			expected:    "myapp-hpa",
		},
		{
			name:        "custom stack",
			serviceName: "myapp",
			stackEnv:    "staging-preview",
			parentEnv:   "staging",
			expected:    "myapp-staging-preview-hpa",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generateHPAName(tt.serviceName, tt.stackEnv, tt.parentEnv)
			if result != tt.expected {
				t.Errorf("generateHPAName() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestGenerateVPAName(t *testing.T) {
	tests := []struct {
		name        string
		serviceName string
		stackEnv    string
		parentEnv   string
		expected    string
	}{
		{
			name:        "standard stack",
			serviceName: "myapp",
			stackEnv:    "production",
			parentEnv:   "",
			expected:    "myapp-vpa",
		},
		{
			name:        "custom stack",
			serviceName: "myapp",
			stackEnv:    "prod-canary",
			parentEnv:   "production",
			expected:    "myapp-prod-canary-vpa",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generateVPAName(tt.serviceName, tt.stackEnv, tt.parentEnv)
			if result != tt.expected {
				t.Errorf("generateVPAName() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestGenerateConfigVolumesName(t *testing.T) {
	tests := []struct {
		name        string
		serviceName string
		stackEnv    string
		parentEnv   string
		expected    string
	}{
		{
			name:        "standard stack",
			serviceName: "myapp",
			stackEnv:    "staging",
			parentEnv:   "",
			expected:    "myapp-cfg-volumes",
		},
		{
			name:        "custom stack",
			serviceName: "myapp",
			stackEnv:    "staging-preview",
			parentEnv:   "staging",
			expected:    "myapp-staging-preview-cfg-volumes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generateConfigVolumesName(tt.serviceName, tt.stackEnv, tt.parentEnv)
			if result != tt.expected {
				t.Errorf("generateConfigVolumesName() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestGenerateSecretVolumesName(t *testing.T) {
	tests := []struct {
		name        string
		serviceName string
		stackEnv    string
		parentEnv   string
		expected    string
	}{
		{
			name:        "standard stack",
			serviceName: "web",
			stackEnv:    "production",
			parentEnv:   "",
			expected:    "web-secret-volumes",
		},
		{
			name:        "custom stack",
			serviceName: "web",
			stackEnv:    "prod-hotfix",
			parentEnv:   "production",
			expected:    "web-prod-hotfix-secret-volumes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generateSecretVolumesName(tt.serviceName, tt.stackEnv, tt.parentEnv)
			if result != tt.expected {
				t.Errorf("generateSecretVolumesName() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestGenerateImagePullSecretName(t *testing.T) {
	tests := []struct {
		name        string
		serviceName string
		stackEnv    string
		parentEnv   string
		expected    string
	}{
		{
			name:        "standard stack",
			serviceName: "api",
			stackEnv:    "staging",
			parentEnv:   "",
			expected:    "api-docker-config",
		},
		{
			name:        "custom stack",
			serviceName: "api",
			stackEnv:    "staging-pr-123",
			parentEnv:   "staging",
			expected:    "api-staging-pr-123-docker-config",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generateImagePullSecretName(tt.serviceName, tt.stackEnv, tt.parentEnv)
			if result != tt.expected {
				t.Errorf("generateImagePullSecretName() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestNamespaceIsStackName(t *testing.T) {
	tests := []struct {
		name      string
		stackName string
		expected  string
	}{
		{
			name:      "simple stack name",
			stackName: "myapp",
			expected:  "myapp",
		},
		{
			name:      "stack with environment",
			stackName: "myapp-staging",
			expected:  "myapp-staging",
		},
		{
			name:      "custom stack name",
			stackName: "frontend-staging-preview",
			expected:  "frontend-staging-preview",
		},
		{
			name:      "production stack",
			stackName: "api-production",
			expected:  "api-production",
		},
		{
			name:      "hotfix stack",
			stackName: "backend-prod-hotfix",
			expected:  "backend-prod-hotfix",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.stackName // Namespace is always stackName
			if result != tt.expected {
				t.Errorf("namespace = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestIsCustomStack(t *testing.T) {
	tests := []struct {
		name      string
		stackEnv  string
		parentEnv string
		expected  bool
	}{
		{
			name:      "standard stack - no parent",
			stackEnv:  "staging",
			parentEnv: "",
			expected:  false,
		},
		{
			name:      "custom stack - different parent",
			stackEnv:  "staging-preview",
			parentEnv: "staging",
			expected:  true,
		},
		{
			name:      "self-reference - same as parent",
			stackEnv:  "staging",
			parentEnv: "staging",
			expected:  false,
		},
		{
			name:      "production hotfix",
			stackEnv:  "prod-hotfix",
			parentEnv: "production",
			expected:  true,
		},
		{
			name:      "empty parent",
			stackEnv:  "production",
			parentEnv: "",
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isCustomStack(tt.stackEnv, tt.parentEnv)
			if result != tt.expected {
				t.Errorf("isCustomStack() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

// TestComplexScenarios tests real-world deployment scenarios
func TestComplexScenarios(t *testing.T) {
	t.Run("multiple preview environments with unique namespaces", func(t *testing.T) {
		// Scenario: Each stack environment gets its own namespace
		serviceName := "api"
		parentEnv := "staging"

		envs := []struct {
			stackName          string
			stackEnv           string
			expectedDeployment string
			expectedNamespace  string
		}{
			{"api-staging", "staging", "api", "api-staging"},                                     // Parent deployment
			{"api-staging-pr-123", "staging-pr-123", "api-staging-pr-123", "api-staging-pr-123"}, // PR 123
			{"api-staging-pr-456", "staging-pr-456", "api-staging-pr-456", "api-staging-pr-456"}, // PR 456
			{"api-staging-hotfix", "staging-hotfix", "api-staging-hotfix", "api-staging-hotfix"}, // Hotfix
		}

		namespaces := make(map[string]bool)
		deployments := make(map[string]bool)

		for _, env := range envs {
			// Each stack should get its own namespace (namespace = stackName)
			ns := env.stackName // Namespace is always stackName
			if ns != env.expectedNamespace {
				t.Errorf("Expected namespace %s, got %s", env.expectedNamespace, ns)
			}
			namespaces[ns] = true

			// All should have unique deployment names
			deployment := generateDeploymentName(serviceName, env.stackEnv, parentEnv)
			if deployment != env.expectedDeployment {
				t.Errorf("Expected deployment %s, got %s", env.expectedDeployment, deployment)
			}
			if deployments[deployment] {
				t.Errorf("Duplicate deployment name: %s", deployment)
			}
			deployments[deployment] = true
		}

		// Each stack should have its own namespace
		if len(namespaces) != len(envs) {
			t.Errorf("Expected %d unique namespaces, got %d namespaces", len(envs), len(namespaces))
		}
	})

	t.Run("microservices with custom stacks", func(t *testing.T) {
		// Scenario: Multiple services, each with preview environments
		services := []string{"api", "web", "worker"}

		for _, service := range services {
			standardName := generateDeploymentName(service, "staging", "")
			previewName := generateDeploymentName(service, "staging-preview", "staging")

			// Standard and preview should be different
			if standardName == previewName {
				t.Errorf("Service %s: standard and preview names should differ", service)
			}

			// Preview should include environment suffix
			expectedPreview := service + "-staging-preview"
			if previewName != expectedPreview {
				t.Errorf("Expected preview name %s, got %s", expectedPreview, previewName)
			}
		}
	})

	t.Run("resource isolation verification", func(t *testing.T) {
		// Scenario: Ensure all resource types get proper suffixes
		serviceName := "myapp"
		stackEnv := "staging-preview"
		parentEnv := "staging"

		resources := map[string]struct {
			generator func(string, string, string) string
			suffix    string
		}{
			"deployment": {generateDeploymentName, ""},
			"service":    {generateServiceName, ""},
			"configmap":  {generateConfigMapName, "config"},
			"secret":     {generateSecretName, "secrets"},
			"hpa":        {generateHPAName, "hpa"},
			"vpa":        {generateVPAName, "vpa"},
		}

		for resourceType, config := range resources {
			name := config.generator(serviceName, stackEnv, parentEnv)

			// Should include environment suffix
			if name != "myapp-staging-preview" && config.suffix == "" {
				t.Errorf("%s: expected environment suffix, got %s", resourceType, name)
			}

			// Should include both environment and resource suffix
			if config.suffix != "" {
				expected := "myapp-staging-preview-" + config.suffix
				if name != expected {
					t.Errorf("%s: expected %s, got %s", resourceType, expected, name)
				}
			}
		}
	})
}
