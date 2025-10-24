package actions

import (
	"context"
	"fmt"
	"os"

	"gopkg.in/yaml.v2"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/discord"
	"github.com/simple-container-com/api/pkg/clouds/slack"
	"github.com/simple-container-com/api/pkg/clouds/telegram"
)

// CICDNotificationConfig represents notification configuration in server.yaml
type CICDNotificationConfig struct {
	Slack struct {
		WebhookURL string `yaml:"webhook-url"`
		Enabled    bool   `yaml:"enabled"`
	} `yaml:"slack"`
	Discord struct {
		WebhookURL string `yaml:"webhook-url"`
		Enabled    bool   `yaml:"enabled"`
	} `yaml:"discord"`
	Telegram struct {
		BotToken string `yaml:"bot-token"`
		ChatID   string `yaml:"chat-id"`
		Enabled  bool   `yaml:"enabled"`
	} `yaml:"telegram"`
}

// CICDConfig represents the CI/CD section in server.yaml
type CICDConfig struct {
	Type   string `yaml:"type"`
	Config struct {
		Organization  string                 `yaml:"organization"`
		AuthToken     string                 `yaml:"auth-token"`
		Notifications CICDNotificationConfig `yaml:"notifications"`
	} `yaml:"config"`
}

// ServerConfig represents the relevant parts of server.yaml
type ServerConfig struct {
	CICD CICDConfig `yaml:"cicd"`
}

// loadNotificationConfigFromServerYaml reads notification configuration from server.yaml in parent stack
func (e *Executor) loadNotificationConfigFromServerYaml(ctx context.Context) *CICDNotificationConfig {
	// Find parent stack directories in .sc/stacks/
	stacksDir := ".sc/stacks"
	if _, err := os.Stat(stacksDir); os.IsNotExist(err) {
		e.logger.Info(ctx, "No .sc/stacks directory found - using environment variables as fallback")
		return nil
	}

	// Read all stack directories to find server.yaml
	entries, err := os.ReadDir(stacksDir)
	if err != nil {
		e.logger.Warn(ctx, "Failed to read .sc/stacks directory: %v", err)
		return nil
	}

	for _, entry := range entries {
		if entry.IsDir() {
			stackName := entry.Name()
			serverYamlPath := fmt.Sprintf(".sc/stacks/%s/server.yaml", stackName)

			if _, err := os.Stat(serverYamlPath); err == nil {
				e.logger.Info(ctx, "Found server.yaml at %s", serverYamlPath)

				data, err := os.ReadFile(serverYamlPath)
				if err != nil {
					e.logger.Warn(ctx, "Failed to read server.yaml from %s: %v", serverYamlPath, err)
					continue
				}

				var config ServerConfig
				if err := yaml.Unmarshal(data, &config); err != nil {
					e.logger.Warn(ctx, "Failed to parse server.yaml from %s: %v", serverYamlPath, err)
					continue
				}

				if config.CICD.Type == "github-actions" {
					e.logger.Info(ctx, "✅ Found GitHub Actions CI/CD configuration in %s", serverYamlPath)
					return &config.CICD.Config.Notifications
				}
			}
		}
	}

	e.logger.Info(ctx, "No server.yaml with GitHub Actions CI/CD config found in stack directories - using environment variables as fallback")
	return nil
}

// initializeNotifications initializes notification senders from server.yaml or environment variables (fallback)
func (e *Executor) initializeNotifications(ctx context.Context) {
	// Try to load configuration from server.yaml first
	notificationConfig := e.loadNotificationConfigFromServerYaml(ctx)

	if notificationConfig != nil {
		e.logger.Info(ctx, "🔧 Initializing notifications from server.yaml configuration")

		// Initialize Slack from server.yaml
		if notificationConfig.Slack.Enabled && notificationConfig.Slack.WebhookURL != "" {
			if slackSender, err := slack.New(notificationConfig.Slack.WebhookURL); err == nil {
				e.slackSender = slackSender
				e.logger.Info(ctx, "✅ Slack notifications enabled (from server.yaml)")
			} else {
				e.logger.Warn(ctx, "Failed to initialize Slack notifications from server.yaml: %v", err)
			}
		}

		// Initialize Discord from server.yaml
		if notificationConfig.Discord.Enabled && notificationConfig.Discord.WebhookURL != "" {
			if discordSender, err := discord.New(notificationConfig.Discord.WebhookURL); err == nil {
				e.discordSender = discordSender
				e.logger.Info(ctx, "✅ Discord notifications enabled (from server.yaml)")
			} else {
				e.logger.Warn(ctx, "Failed to initialize Discord notifications from server.yaml: %v", err)
			}
		}

		// Initialize Telegram from server.yaml
		if notificationConfig.Telegram.Enabled && notificationConfig.Telegram.BotToken != "" && notificationConfig.Telegram.ChatID != "" {
			telegramSender := telegram.New(notificationConfig.Telegram.BotToken, notificationConfig.Telegram.ChatID)
			e.telegramSender = telegramSender
			e.logger.Info(ctx, "✅ Telegram notifications enabled (from server.yaml)")
		}
	} else {
		// Fallback to environment variables (legacy support)
		e.logger.Info(ctx, "🔧 Initializing notifications from environment variables (fallback mode)")

		// Slack notifications from environment
		if slackWebhookURL := os.Getenv("SLACK_WEBHOOK_URL"); slackWebhookURL != "" {
			if slackSender, err := slack.New(slackWebhookURL); err == nil {
				e.slackSender = slackSender
				e.logger.Info(ctx, "✅ Slack notifications enabled (from environment variables)")
			} else {
				e.logger.Warn(ctx, "Failed to initialize Slack notifications: %v", err)
			}
		}

		// Discord notifications from environment
		if discordWebhookURL := os.Getenv("DISCORD_WEBHOOK_URL"); discordWebhookURL != "" {
			if discordSender, err := discord.New(discordWebhookURL); err == nil {
				e.discordSender = discordSender
				e.logger.Info(ctx, "✅ Discord notifications enabled (from environment variables)")
			} else {
				e.logger.Warn(ctx, "Failed to initialize Discord notifications: %v", err)
			}
		}

		// Telegram notifications from environment
		if telegramBotToken := os.Getenv("TELEGRAM_BOT_TOKEN"); telegramBotToken != "" {
			if telegramChatID := os.Getenv("TELEGRAM_CHAT_ID"); telegramChatID != "" {
				telegramSender := telegram.New(telegramBotToken, telegramChatID)
				e.telegramSender = telegramSender
				e.logger.Info(ctx, "✅ Telegram notifications enabled (from environment variables)")
			}
		}
	}

	// Log notification status
	if e.slackSender == nil && e.discordSender == nil && e.telegramSender == nil {
		e.logger.Info(ctx, "No notification webhooks configured, skipping notifications")
	}
}

// sendAlert sends notifications using SC's internal alert system
func (e *Executor) sendAlert(ctx context.Context, alertType api.AlertType, title, description, stackName, stackEnv string) {
	// Create alert payload using SC's alert structure
	alert := api.Alert{
		Name:        fmt.Sprintf("%s-%s", stackName, stackEnv),
		Title:       title,
		Reason:      fmt.Sprintf("GitHub Action: %s", os.Getenv("GITHUB_WORKFLOW")),
		Description: description,
		StackName:   stackName,
		StackEnv:    stackEnv,
		DetailsUrl:  fmt.Sprintf("https://github.com/%s/actions/runs/%s", os.Getenv("GITHUB_REPOSITORY"), os.Getenv("GITHUB_RUN_ID")),
		AlertType:   alertType,
	}

	// Send to all configured notification channels using SC's alert senders
	if e.slackSender != nil {
		if err := e.slackSender.Send(alert); err != nil {
			e.logger.Warn(ctx, "Failed to send Slack notification: %v", err)
		}
	}

	if e.discordSender != nil {
		if err := e.discordSender.Send(alert); err != nil {
			e.logger.Warn(ctx, "Failed to send Discord notification: %v", err)
		}
	}

	if e.telegramSender != nil {
		if err := e.telegramSender.Send(alert); err != nil {
			e.logger.Warn(ctx, "Failed to send Telegram notification: %v", err)
		}
	}
}
