package cmd_cicd

import (
	"fmt"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/github"
)

func createEnhancedConfig(serverDesc *api.ServerDescriptor, stackName string) *github.EnhancedActionsCiCdConfig {
	// Use SC's standard conversion pattern to get strongly typed GitHub Actions configuration
	convertedConfig, err := api.ConvertConfig(&serverDesc.CiCd.Config, &github.GitHubActionsCiCdConfig{})
	if err != nil {
		// Fallback to default configuration
		return &github.EnhancedActionsCiCdConfig{
			Organization: github.OrganizationConfig{Name: "simple-container-org"},
			WorkflowGeneration: github.WorkflowGenerationConfig{
				Templates:     []string{"deploy", "destroy"},
				CustomActions: map[string]string{},
			},
			Environments: map[string]github.EnvironmentConfig{
				"staging":    {Type: "staging"},
				"production": {Type: "production"},
			},
			Notifications: github.NotificationConfig{},
		}
	}

	// Extract the strongly typed configuration
	gitHubConfig, ok := convertedConfig.Config.(*github.GitHubActionsCiCdConfig)
	if !ok {
		// Fallback to default if type assertion fails
		return &github.EnhancedActionsCiCdConfig{
			Organization: github.OrganizationConfig{Name: "simple-container-org"},
			WorkflowGeneration: github.WorkflowGenerationConfig{
				Templates:     []string{"deploy", "destroy"},
				CustomActions: map[string]string{},
			},
			Environments: map[string]github.EnvironmentConfig{
				"staging":    {Type: "staging"},
				"production": {Type: "production"},
			},
			Notifications: github.NotificationConfig{},
		}
	}

	// Create enhanced configuration from strongly typed config
	config := &github.EnhancedActionsCiCdConfig{
		Organization: github.OrganizationConfig{
			Name: gitHubConfig.Organization,
		},
		WorkflowGeneration: github.WorkflowGenerationConfig{
			Enabled:       gitHubConfig.WorkflowGeneration.Enabled,
			OutputPath:    gitHubConfig.WorkflowGeneration.OutputPath,
			Templates:     gitHubConfig.WorkflowGeneration.Templates,
			AutoUpdate:    gitHubConfig.WorkflowGeneration.AutoUpdate,
			CustomActions: gitHubConfig.WorkflowGeneration.CustomActions,
			SCVersion:     gitHubConfig.WorkflowGeneration.SCVersion,
		},
		Environments: make(map[string]github.EnvironmentConfig),
		Notifications: github.NotificationConfig{
			SlackWebhook:   gitHubConfig.Notifications.SlackWebhook,
			DiscordWebhook: gitHubConfig.Notifications.DiscordWebhook,
			TelegramChatID: gitHubConfig.Notifications.TelegramChatID,
			TelegramToken:  gitHubConfig.Notifications.TelegramToken,
		},
	}

	// Convert environments to enhanced format
	for envName, envConfig := range gitHubConfig.Environments {
		config.Environments[envName] = github.EnvironmentConfig{
			Type:        envConfig.Type,
			Runners:     envConfig.Runners,
			Protection:  envConfig.Protection,
			Reviewers:   envConfig.Reviewers,
			Secrets:     envConfig.Secrets,
			Variables:   envConfig.Variables,
			DeployFlags: envConfig.DeployFlags,
			AutoDeploy:  envConfig.AutoDeploy,
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
	requiredSecrets := []string{
		"SC_CONFIG", // Always required for Simple Container operations
	}

	// Add notification secrets if configured
	if config.Notifications.SlackWebhook != "" {
		requiredSecrets = append(requiredSecrets, "SLACK_WEBHOOK_URL")
	}
	if config.Notifications.DiscordWebhook != "" {
		requiredSecrets = append(requiredSecrets, "DISCORD_WEBHOOK_URL")
	}

	// Add Telegram secrets as optional
	requiredSecrets = append(requiredSecrets,
		"TELEGRAM_CHAT_ID", // Optional
		"TELEGRAM_TOKEN",   // Optional
	)

	return requiredSecrets
}

func readServerConfig(configFile string) (*api.ServerDescriptor, error) {
	// Use SC's internal API to read server configuration
	serverDesc, err := api.ReadServerDescriptor(configFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read server configuration from %s: %w", configFile, err)
	}

	// If no CI/CD configuration is found, default to GitHub Actions
	if serverDesc.CiCd.Type == "" {
		serverDesc.CiCd.Type = github.CiCdTypeGithubActions
		serverDesc.CiCd.Config = api.Config{}
	}

	return serverDesc, nil
}
