package github

import (
	"github.com/simple-container-com/api/pkg/api"
)

// GitHubActionsCiCdConfig represents the GitHub Actions CI/CD configuration
type GitHubActionsCiCdConfig struct {
	// Organization settings
	Organization string `json:"organization" yaml:"organization"`

	// Environment-specific configurations
	Environments map[string]GitHubEnvironmentConfig `json:"environments" yaml:"environments"`

	// Notification settings
	Notifications GitHubNotificationConfig `json:"notifications" yaml:"notifications"`

	// Workflow generation settings
	WorkflowGeneration GitHubWorkflowConfig `json:"workflow-generation" yaml:"workflow-generation"`
}

// GitHubEnvironmentConfig defines environment-specific settings
type GitHubEnvironmentConfig struct {
	Type        string            `json:"type" yaml:"type"`
	Runner      string            `json:"runner" yaml:"runner"`
	Protection  bool              `json:"protection" yaml:"protection"`
	Reviewers   []string          `json:"reviewers" yaml:"reviewers"`
	Secrets     []string          `json:"secrets" yaml:"secrets"`
	Variables   map[string]string `json:"variables" yaml:"variables"`
	DeployFlags []string          `json:"deploy-flags" yaml:"deploy-flags"`
	AutoDeploy  bool              `json:"auto-deploy" yaml:"auto-deploy"`
}

// GitHubNotificationConfig defines notification settings
type GitHubNotificationConfig struct {
	Slack    SlackNotificationConfig    `json:"slack" yaml:"slack"`
	Discord  DiscordNotificationConfig  `json:"discord" yaml:"discord"`
	Telegram TelegramNotificationConfig `json:"telegram" yaml:"telegram"`
}

// SlackNotificationConfig defines Slack notification settings
type SlackNotificationConfig struct {
	WebhookURL string `json:"webhook-url" yaml:"webhook-url"`
	Enabled    bool   `json:"enabled" yaml:"enabled"`
}

// DiscordNotificationConfig defines Discord notification settings
type DiscordNotificationConfig struct {
	WebhookURL string `json:"webhook-url" yaml:"webhook-url"`
	Enabled    bool   `json:"enabled" yaml:"enabled"`
}

// TelegramNotificationConfig defines Telegram notification settings
type TelegramNotificationConfig struct {
	BotToken string `json:"bot-token" yaml:"bot-token"`
	ChatID   string `json:"chat-id" yaml:"chat-id"`
	Enabled  bool   `json:"enabled" yaml:"enabled"`
}

// GitHubWorkflowConfig defines workflow generation settings
type GitHubWorkflowConfig struct {
	Enabled       bool              `json:"enabled" yaml:"enabled"`
	OutputPath    string            `json:"output-path" yaml:"output-path"`
	Templates     []string          `json:"templates" yaml:"templates"`
	AutoUpdate    bool              `json:"auto-update" yaml:"auto-update"`
	CustomActions map[string]string `json:"custom-actions" yaml:"custom-actions"`
	SCVersion     string            `json:"sc-version" yaml:"sc-version"`
}

// ConvertToGitHubActionsCiCdConfig converts a generic config to GitHub Actions specific config
// Following the same pattern as other SC resources using api.ConvertConfig
func ConvertToGitHubActionsCiCdConfig(config *api.Config) (*GitHubActionsCiCdConfig, error) {
	if config == nil || config.Config == nil {
		// Return default configuration
		return &GitHubActionsCiCdConfig{
			Organization: "simple-container-org",
			Environments: map[string]GitHubEnvironmentConfig{
				"staging":    {Type: "staging"},
				"production": {Type: "production"},
			},
			Notifications: GitHubNotificationConfig{},
			WorkflowGeneration: GitHubWorkflowConfig{
				Enabled:       true,
				Templates:     []string{"deploy", "destroy"},
				CustomActions: map[string]string{},
			},
		}, nil
	}

	// Use SC's standard conversion pattern - let the YAML/JSON unmarshaler handle the type conversion
	result := &GitHubActionsCiCdConfig{}
	convertedConfig, err := api.ConvertConfig(config, result)
	if err != nil {
		// If conversion fails, return default configuration
		return &GitHubActionsCiCdConfig{
			Organization: "simple-container-org",
			Environments: map[string]GitHubEnvironmentConfig{
				"staging":    {Type: "staging"},
				"production": {Type: "production"},
			},
			Notifications: GitHubNotificationConfig{},
			WorkflowGeneration: GitHubWorkflowConfig{
				Enabled:       true,
				Templates:     []string{"deploy", "destroy"},
				CustomActions: map[string]string{},
			},
		}, nil
	}

	// Extract the converted configuration from the returned Config
	if gitHubConfig, ok := convertedConfig.Config.(*GitHubActionsCiCdConfig); ok {
		// Set defaults for any missing required fields
		if gitHubConfig.Organization == "" {
			gitHubConfig.Organization = "simple-container-org"
		}
		if len(gitHubConfig.Environments) == 0 {
			gitHubConfig.Environments = map[string]GitHubEnvironmentConfig{
				"staging":    {Type: "staging"},
				"production": {Type: "production"},
			}
		}
		if len(gitHubConfig.WorkflowGeneration.Templates) == 0 {
			gitHubConfig.WorkflowGeneration.Templates = []string{"deploy", "destroy"}
		}
		if gitHubConfig.WorkflowGeneration.CustomActions == nil {
			gitHubConfig.WorkflowGeneration.CustomActions = map[string]string{}
		}
		return gitHubConfig, nil
	}

	// Fallback if type assertion fails
	return result, nil
}
