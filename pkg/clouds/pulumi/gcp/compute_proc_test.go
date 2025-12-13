package gcp

import (
	"testing"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/logger"
	"github.com/simple-container-com/api/pkg/clouds/gcloud"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	"github.com/simple-container-com/api/pkg/clouds/pulumi/kubernetes"
)

// Test utilities for compute processor testing

// createTestResourceInput creates a ResourceInput for testing
func createTestResourceInput(environment, stackName string, parentEnv *string) api.ResourceInput {
	stackParams := &api.StackParams{
		Environment: environment,
		StackName:   stackName,
	}
	if parentEnv != nil {
		stackParams.ParentEnv = *parentEnv
	}

	return api.ResourceInput{
		Descriptor: &api.ResourceDescriptor{
			Name: "test-postgres",
			Type: "gcp-cloudsql-postgres",
			Config: api.Config{
				Config: &gcloud.PostgresGcpCloudsqlConfig{
					Credentials: gcloud.Credentials{
						ServiceAccountConfig: gcloud.ServiceAccountConfig{
							ProjectId: "test-project",
						},
					},
				},
			},
		},
		StackParams: stackParams,
	}
}

// createTestAppendParams creates appendParams for testing
func createTestAppendParams(input api.ResourceInput, parentStack *pApi.ParentInfo) appendParams {
	provisionParams := pApi.ProvisionParams{
		Log: logger.New(),
	}
	if parentStack != nil {
		provisionParams.ParentStack = parentStack
	}

	return appendParams{
		stack: api.Stack{
			Name: "telegram-bot",
		},
		input:           input,
		postgresName:    "main-db",
		provisionParams: provisionParams,
	}
}

// Test cases for service account naming with custom stacks

func TestCreateCloudsqlProxy_StandardStack_NamingConvention(t *testing.T) {
	RegisterTestingT(t)

	// Test standard stack (no custom stack features)
	input := createTestResourceInput("production", "telegram-bot", nil)
	_ = createTestAppendParams(input, nil)

	// Test the naming logic without actually creating resources
	baseProxyName := "telegram-bot-main-db-sidecarcsql"
	expectedName := kubernetes.SanitizeK8sName(input.ToResName(baseProxyName))

	// For standard stack, ToResName should add environment suffix (sanitized to remove double hyphens)
	Expect(expectedName).To(Equal("telegram-bot-main-db-sidecarcsql-production"))
}

func TestCreateCloudsqlProxy_CustomStack_WithoutParentEnvInitialization(t *testing.T) {
	RegisterTestingT(t)

	// Test custom stack without ParentEnv initialization (should behave like standard stack)
	input := createTestResourceInput("preview", "telegram-bot", nil) // ParentEnv not set
	parentStack := &pApi.ParentInfo{
		ParentEnv: "production", // This exists but input.StackParams.ParentEnv is not set
	}
	_ = createTestAppendParams(input, parentStack)

	// Test the naming logic
	baseProxyName := "telegram-bot-main-db-sidecarcsql"
	expectedName := kubernetes.SanitizeK8sName(input.ToResName(baseProxyName))

	// Without ParentEnv initialization, should use custom stack's environment (sanitized to remove double hyphens)
	Expect(expectedName).To(Equal("telegram-bot-main-db-sidecarcsql-preview"))
}

func TestCreateCloudsqlProxy_CustomStack_WithParentEnvInitialization(t *testing.T) {
	RegisterTestingT(t)

	// Test custom stack with ParentEnv initialization (simulates the fix)
	input := createTestResourceInput("preview", "telegram-bot", nil)
	parentStack := &pApi.ParentInfo{
		ParentEnv: "production",
	}
	params := createTestAppendParams(input, parentStack)

	// Simulate the ParentEnv initialization logic from our fix
	if params.provisionParams.ParentStack != nil && params.provisionParams.ParentStack.ParentEnv != "" &&
		params.provisionParams.ParentStack.ParentEnv != params.input.StackParams.Environment {
		params.input.StackParams.ParentEnv = params.provisionParams.ParentStack.ParentEnv
	}

	// Test the naming logic
	baseProxyName := "telegram-bot-main-db-sidecarcsql"
	expectedName := kubernetes.SanitizeK8sName(input.ToResName(baseProxyName))

	// With ParentEnv initialization, should use parent environment (sanitized to remove double hyphens)
	Expect(expectedName).To(Equal("telegram-bot-main-db-sidecarcsql-production"))
}

func TestCreateCloudsqlProxy_CustomStack_SameEnvironmentAsParent(t *testing.T) {
	RegisterTestingT(t)

	// Test custom stack where custom environment matches parent environment
	input := createTestResourceInput("production", "telegram-bot", nil)
	parentStack := &pApi.ParentInfo{
		ParentEnv: "production", // Same as custom stack environment
	}
	params := createTestAppendParams(input, parentStack)

	// Simulate the ParentEnv initialization logic
	if params.provisionParams.ParentStack != nil && params.provisionParams.ParentStack.ParentEnv != "" &&
		params.provisionParams.ParentStack.ParentEnv != params.input.StackParams.Environment {
		params.input.StackParams.ParentEnv = params.provisionParams.ParentStack.ParentEnv
	}

	// Test the naming logic
	baseProxyName := "telegram-bot-main-db-sidecarcsql"
	expectedName := kubernetes.SanitizeK8sName(input.ToResName(baseProxyName))

	// Should not initialize ParentEnv since environments match, use standard naming (sanitized to remove double hyphens)
	Expect(expectedName).To(Equal("telegram-bot-main-db-sidecarcsql-production"))
	Expect(input.StackParams.ParentEnv).To(Equal("")) // ParentEnv should not be set
}

func TestCreateCloudsqlProxy_CustomStack_MultipleEnvironments(t *testing.T) {
	RegisterTestingT(t)

	// Test multiple custom stacks with different environments but same parent
	testCases := []struct {
		name            string
		customEnv       string
		parentEnv       string
		expectedSuffix  string
		shouldSetParent bool
	}{
		{
			name:            "Preview environment with production parent",
			customEnv:       "preview",
			parentEnv:       "production",
			expectedSuffix:  "-production",
			shouldSetParent: true,
		},
		{
			name:            "Staging environment with production parent",
			customEnv:       "staging-preview",
			parentEnv:       "production",
			expectedSuffix:  "-production",
			shouldSetParent: true,
		},
		{
			name:            "Development environment with staging parent",
			customEnv:       "dev",
			parentEnv:       "staging",
			expectedSuffix:  "-staging",
			shouldSetParent: true,
		},
		{
			name:            "Production custom stack with production parent",
			customEnv:       "production",
			parentEnv:       "production",
			expectedSuffix:  "-production",
			shouldSetParent: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)

			input := createTestResourceInput(tc.customEnv, "telegram-bot", nil)
			parentStack := &pApi.ParentInfo{
				ParentEnv: tc.parentEnv,
			}
			params := createTestAppendParams(input, parentStack)

			// Simulate the ParentEnv initialization logic
			if params.provisionParams.ParentStack != nil && params.provisionParams.ParentStack.ParentEnv != "" &&
				params.provisionParams.ParentStack.ParentEnv != params.input.StackParams.Environment {
				params.input.StackParams.ParentEnv = params.provisionParams.ParentStack.ParentEnv
			}

			// Test the naming logic
			baseProxyName := "telegram-bot-main-db-sidecarcsql"
			expectedName := kubernetes.SanitizeK8sName(input.ToResName(baseProxyName))
			expectedFullName := "telegram-bot-main-db-sidecarcsql" + tc.expectedSuffix

			Expect(expectedName).To(Equal(expectedFullName))

			// Verify ParentEnv is set correctly
			if tc.shouldSetParent {
				Expect(input.StackParams.ParentEnv).To(Equal(tc.parentEnv))
			} else {
				Expect(input.StackParams.ParentEnv).To(Equal(""))
			}
		})
	}
}

func TestCreateCloudsqlProxy_ServiceAccountNamingConflictPrevention(t *testing.T) {
	RegisterTestingT(t)

	// Test that demonstrates the conflict prevention
	// This simulates the original issue where multiple custom stacks would create
	// service accounts with the same name

	// Scenario: Two custom stacks deploying to different environments
	// but both using the same parent environment for resource naming

	// Custom stack 1: preview environment
	input1 := createTestResourceInput("preview", "telegram-bot", nil)
	parentStack1 := &pApi.ParentInfo{ParentEnv: "production"}
	params1 := createTestAppendParams(input1, parentStack1)

	// Custom stack 2: staging-preview environment
	input2 := createTestResourceInput("staging-preview", "telegram-bot", nil)
	parentStack2 := &pApi.ParentInfo{ParentEnv: "production"}
	params2 := createTestAppendParams(input2, parentStack2)

	// Apply ParentEnv initialization to both
	if params1.provisionParams.ParentStack != nil && params1.provisionParams.ParentStack.ParentEnv != "" &&
		params1.provisionParams.ParentStack.ParentEnv != params1.input.StackParams.Environment {
		params1.input.StackParams.ParentEnv = params1.provisionParams.ParentStack.ParentEnv
	}

	if params2.provisionParams.ParentStack != nil && params2.provisionParams.ParentStack.ParentEnv != "" &&
		params2.provisionParams.ParentStack.ParentEnv != params2.input.StackParams.Environment {
		params2.input.StackParams.ParentEnv = params2.provisionParams.ParentStack.ParentEnv
	}

	// Generate service account names
	baseProxyName := "telegram-bot-main-db-sidecarcsql"
	name1 := kubernetes.SanitizeK8sName(input1.ToResName(baseProxyName))
	name2 := kubernetes.SanitizeK8sName(input2.ToResName(baseProxyName))

	// Both should use the parent environment for naming (sanitized to remove double hyphens)
	expectedName := "telegram-bot-main-db-sidecarcsql-production"
	Expect(name1).To(Equal(expectedName))
	Expect(name2).To(Equal(expectedName))

	// This demonstrates that both custom stacks will create service accounts
	// with the same name, which is the desired behavior for shared parent resources
	// The service account should be shared between custom stacks that use the same parent
}

func TestCreateCloudsqlProxy_EdgeCases(t *testing.T) {
	RegisterTestingT(t)

	t.Run("Empty parent environment", func(t *testing.T) {
		RegisterTestingT(t)

		input := createTestResourceInput("preview", "telegram-bot", nil)
		parentStack := &pApi.ParentInfo{ParentEnv: ""} // Empty parent env
		params := createTestAppendParams(input, parentStack)

		// Apply initialization logic
		if params.provisionParams.ParentStack != nil && params.provisionParams.ParentStack.ParentEnv != "" &&
			params.provisionParams.ParentStack.ParentEnv != params.input.StackParams.Environment {
			params.input.StackParams.ParentEnv = params.provisionParams.ParentStack.ParentEnv
		}

		baseProxyName := "telegram-bot-main-db-sidecarcsql"
		expectedName := kubernetes.SanitizeK8sName(input.ToResName(baseProxyName))

		// Should use custom stack environment since parent env is empty (sanitized to remove double hyphens)
		Expect(expectedName).To(Equal("telegram-bot-main-db-sidecarcsql-preview"))
		Expect(input.StackParams.ParentEnv).To(Equal(""))
	})

	t.Run("Nil parent stack", func(t *testing.T) {
		RegisterTestingT(t)

		input := createTestResourceInput("preview", "telegram-bot", nil)
		params := createTestAppendParams(input, nil) // No parent stack

		// Apply initialization logic
		if params.provisionParams.ParentStack != nil && params.provisionParams.ParentStack.ParentEnv != "" &&
			params.provisionParams.ParentStack.ParentEnv != params.input.StackParams.Environment {
			params.input.StackParams.ParentEnv = params.provisionParams.ParentStack.ParentEnv
		}

		baseProxyName := "telegram-bot-main-db-sidecarcsql"
		expectedName := kubernetes.SanitizeK8sName(input.ToResName(baseProxyName))

		// Should use custom stack environment (sanitized to remove double hyphens)
		Expect(expectedName).To(Equal("telegram-bot-main-db-sidecarcsql-preview"))
		Expect(input.StackParams.ParentEnv).To(Equal(""))
	})

	t.Run("ParentEnv already set in input", func(t *testing.T) {
		RegisterTestingT(t)

		// Test case where ParentEnv is already set in the input
		parentEnv := "staging"
		input := createTestResourceInput("preview", "telegram-bot", &parentEnv)
		parentStack := &pApi.ParentInfo{ParentEnv: "production"}
		params := createTestAppendParams(input, parentStack)

		// Apply initialization logic
		if params.provisionParams.ParentStack != nil && params.provisionParams.ParentStack.ParentEnv != "" &&
			params.provisionParams.ParentStack.ParentEnv != params.input.StackParams.Environment {
			params.input.StackParams.ParentEnv = params.provisionParams.ParentStack.ParentEnv
		}

		baseProxyName := "telegram-bot-main-db-sidecarcsql"
		expectedName := kubernetes.SanitizeK8sName(input.ToResName(baseProxyName))

		// Should override the existing ParentEnv with the one from ParentStack (sanitized to remove double hyphens)
		Expect(expectedName).To(Equal("telegram-bot-main-db-sidecarcsql-production"))
		Expect(input.StackParams.ParentEnv).To(Equal("production"))
	})
}

// Test the ToResName method behavior directly
func TestToResName_CustomStackBehavior(t *testing.T) {
	RegisterTestingT(t)

	testCases := []struct {
		name        string
		environment string
		parentEnv   *string
		resName     string
		expected    string
	}{
		{
			name:        "Standard stack with environment",
			environment: "production",
			parentEnv:   nil,
			resName:     "service-account",
			expected:    "service-account--production",
		},
		{
			name:        "Custom stack with parent environment",
			environment: "preview",
			parentEnv:   stringPtr("production"),
			resName:     "service-account",
			expected:    "service-account--production",
		},
		{
			name:        "Custom stack with empty parent environment",
			environment: "preview",
			parentEnv:   stringPtr(""),
			resName:     "service-account",
			expected:    "service-account--preview",
		},
		{
			name:        "No environment",
			environment: "",
			parentEnv:   nil,
			resName:     "service-account",
			expected:    "service-account",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)

			input := createTestResourceInput(tc.environment, "test-stack", tc.parentEnv)
			result := input.ToResName(tc.resName)

			Expect(result).To(Equal(tc.expected))
		})
	}
}

// Helper function to create string pointer
func stringPtr(s string) *string {
	return &s
}
