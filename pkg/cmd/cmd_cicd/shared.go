package cmd_cicd

import (
	"fmt"
	"os"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/logger/color"
	"github.com/simple-container-com/api/pkg/clouds/github"
)

func createEnhancedConfig(serverDesc *api.ServerDescriptor, stackName string) *github.EnhancedActionsCiCdConfig {
	// Use SC's standard conversion pattern to get strongly typed GitHub Actions configuration
	convertedConfig, err := api.ConvertConfig(&serverDesc.CiCd.Config, &github.GitHubActionsCiCdConfig{})
	if err != nil {
		// Fallback to default configuration
		return &github.EnhancedActionsCiCdConfig{
			Organization: github.OrganizationConfig{
				Name:          "simple-container-org",
				DefaultBranch: "main",
			},
			WorkflowGeneration: github.WorkflowGenerationConfig{
				Enabled:   true,
				Templates: []string{"deploy", "destroy"},
				CustomActions: map[string]string{
					"deploy":         "simple-container-com/api/.github/actions/deploy@v1",
					"destroy-client": "simple-container-com/api/.github/actions/destroy@v1",
					"provision":      "simple-container-com/api/.github/actions/provision@v1",
				},
				SCVersion: "latest",
			},
			Execution: github.ExecutionConfig{
				DefaultTimeout: "30",
			},
			Environments: map[string]github.EnvironmentConfig{
				"staging":    {Type: "staging", Runners: []string{"ubuntu-latest"}},
				"production": {Type: "production", Runners: []string{"ubuntu-latest"}},
			},
			Notifications: github.NotificationConfig{CCOnStart: false},
		}
	}

	// Extract the strongly typed configuration
	gitHubConfig, ok := convertedConfig.Config.(*github.GitHubActionsCiCdConfig)
	if !ok {
		// Fallback to default if type assertion fails
		return &github.EnhancedActionsCiCdConfig{
			Organization: github.OrganizationConfig{
				Name:          "simple-container-org",
				DefaultBranch: "main",
			},
			WorkflowGeneration: github.WorkflowGenerationConfig{
				Enabled:   true,
				Templates: []string{"deploy", "destroy"},
				CustomActions: map[string]string{
					"deploy":         "simple-container-com/api/.github/actions/deploy@v1",
					"destroy-client": "simple-container-com/api/.github/actions/destroy@v1",
					"provision":      "simple-container-com/api/.github/actions/provision@v1",
				},
				SCVersion: "latest",
			},
			Execution: github.ExecutionConfig{
				DefaultTimeout: "30",
			},
			Environments: map[string]github.EnvironmentConfig{
				"staging":    {Type: "staging", Runners: []string{"ubuntu-latest"}},
				"production": {Type: "production", Runners: []string{"ubuntu-latest"}},
			},
			Notifications: github.NotificationConfig{CCOnStart: false},
		}
	}

	// Create enhanced configuration from strongly typed config with proper defaults
	config := &github.EnhancedActionsCiCdConfig{
		Organization: github.OrganizationConfig{
			Name:          gitHubConfig.Organization,
			DefaultBranch: "main", // Default to main branch
		},
		WorkflowGeneration: github.WorkflowGenerationConfig{
			Enabled:    true,
			OutputPath: ".github/workflows/",
			Templates:  []string{"deploy", "destroy"},
			CustomActions: map[string]string{
				"deploy":         "simple-container-com/api/.github/actions/deploy@v1",
				"destroy-client": "simple-container-com/api/.github/actions/destroy@v1",
				"provision":      "simple-container-com/api/.github/actions/provision@v1",
			},
			SCVersion: "latest",
		},
		Execution: github.ExecutionConfig{
			DefaultTimeout: "30", // 30 minutes
			Concurrency: github.ConcurrencyConfig{
				Group:            "${{ github.workflow }}-${{ github.ref }}",
				CancelInProgress: false,
			},
		},
		Environments: make(map[string]github.EnvironmentConfig),
		Notifications: github.NotificationConfig{
			SlackWebhook:   gitHubConfig.Notifications.SlackWebhook,
			DiscordWebhook: gitHubConfig.Notifications.DiscordWebhook,
			TelegramChatID: gitHubConfig.Notifications.TelegramChatID,
			TelegramToken:  gitHubConfig.Notifications.TelegramToken,
			CCOnStart:      false, // Don't CC on start by default
		},
		Validation: github.ValidationConfig{
			Required: false, // No validation by default
		},
	}

	// Convert environments to enhanced format with proper defaults
	for envName, envConfig := range gitHubConfig.Environments {
		// Set default runners if none specified
		runners := envConfig.Runners
		if len(runners) == 0 {
			runners = []string{"ubuntu-latest"}
		}

		config.Environments[envName] = github.EnvironmentConfig{
			Type:        envConfig.Type,
			Runners:     runners,
			Protection:  envConfig.Protection,
			Reviewers:   envConfig.Reviewers,
			Secrets:     envConfig.Secrets,
			Variables:   envConfig.Variables,
			DeployFlags: envConfig.DeployFlags,
			AutoDeploy:  envConfig.AutoDeploy,
		}
	}

	// Override with user-provided config if available
	if gitHubConfig.WorkflowGeneration.Enabled {
		config.WorkflowGeneration.Enabled = gitHubConfig.WorkflowGeneration.Enabled
	}
	if gitHubConfig.WorkflowGeneration.OutputPath != "" {
		config.WorkflowGeneration.OutputPath = gitHubConfig.WorkflowGeneration.OutputPath
	}
	if len(gitHubConfig.WorkflowGeneration.Templates) > 0 {
		config.WorkflowGeneration.Templates = gitHubConfig.WorkflowGeneration.Templates
	}
	if len(gitHubConfig.WorkflowGeneration.CustomActions) > 0 {
		for key, value := range gitHubConfig.WorkflowGeneration.CustomActions {
			config.WorkflowGeneration.CustomActions[key] = value
		}
	}

	return config
}

func getEnvironmentNames(environments map[string]github.EnvironmentConfig) []string {
	var names []string
	for name := range environments {
		names = append(names, name)
	}
	return names
}

func getRequiredSecrets(config *github.EnhancedActionsCiCdConfig) []string {
	// Simple Container uses unified secrets management:
	// - Only SC_CONFIG is required as a GitHub repository secret
	// - All other secrets (notifications, webhooks, tokens) are managed in .sc/stacks/<stack>/secrets.yaml
	// - SC automatically decrypts and provides these secrets via SC_CONFIG
	requiredSecrets := []string{
		"SC_CONFIG", // Contains SSH key for decrypting all Simple Container secrets
	}

	return requiredSecrets
}

// processStackName handles stack name validation and defaulting
func processStackName(stackName string) string {
	if stackName == "" {
		return "default-stack"
	}
	return stackName
}

// autoDetectConfigFile detects server.yaml file location based on stack name
func autoDetectConfigFile(configFile, stackName string) (string, error) {
	if configFile != "" && configFile != "server.yaml" {
		return configFile, nil
	}

	// Auto-detect server.yaml file based on stack name
	possiblePaths := []string{
		".sc/stacks/" + stackName + "/server.yaml",
		"server.yaml",
		".sc/stacks/common/server.yaml",
	}

	for _, path := range possiblePaths {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	return "", fmt.Errorf("could not find server.yaml file. Tried: %v\nUse --config to specify path", possiblePaths)
}

// validateAndLoadServerConfig loads and validates server configuration
func validateAndLoadServerConfig(configFile string) (*api.ServerDescriptor, error) {
	serverDesc, err := readServerConfig(configFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read server configuration: %w", err)
	}

	// Validate CI/CD configuration
	if serverDesc.CiCd.Type == "" {
		return nil, fmt.Errorf(`no CI/CD configuration found in %s

To add GitHub Actions CI/CD support, add the following to your server.yaml:

cicd:
  type: github-actions
  config:
    organization: "your-org-name"
    environments:
      staging:
        type: staging
        auto-deploy: true
        runners: ["ubuntu-latest"]
      production:
        type: production
        protection: true
        auto-deploy: false
        runners: ["ubuntu-latest"]
    notifications:
      slack: "\${secret:slack-webhook-url}"
    workflow-generation:
      enabled: true`, configFile)
	}

	if serverDesc.CiCd.Type != github.CiCdTypeGithubActions {
		return nil, fmt.Errorf("unsupported CI/CD type: %s (only 'github-actions' is supported)", serverDesc.CiCd.Type)
	}

	return serverDesc, nil
}

// setupEnhancedConfigWithLogging creates enhanced config and logs the details
func setupEnhancedConfigWithLogging(serverDesc *api.ServerDescriptor, stackName, configFile string) *github.EnhancedActionsCiCdConfig {
	enhancedConfig := createEnhancedConfig(serverDesc, stackName)

	fmt.Printf("🔧 CI/CD Type: %s\n", color.GreenString(serverDesc.CiCd.Type))
	fmt.Printf("🏢 Organization: %s\n", color.GreenString(enhancedConfig.Organization.Name))
	fmt.Printf("📄 Templates: %v\n", enhancedConfig.WorkflowGeneration.Templates)
	fmt.Printf("🌍 Environments: %v\n", getEnvironmentNames(enhancedConfig.Environments))

	return enhancedConfig
}

func readServerConfig(configFile string) (*api.ServerDescriptor, error) {
	// Use SC's internal API to read server configuration
	serverDesc, err := api.ReadServerDescriptor(configFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read server configuration from %s: %w", configFile, err)
	}

	// Don't default to any CI/CD type - let the caller handle empty configuration
	// This ensures proper error handling when no CI/CD config exists

	return serverDesc, nil
}
