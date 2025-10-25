package actions

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/pkg/errors"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/discord"
	"github.com/simple-container-com/api/pkg/clouds/github"
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

// prepareProvisionerForSecretResolution prepares the provisioner for resolving placeholders (same pattern as 'sc provision')
func (e *Executor) prepareProvisionerForSecretResolution(ctx context.Context) error {
	// Read config file for current profile (same as provisioner.prepareForParentStack)
	profile := os.Getenv("SC_PROFILE")
	if profile == "" {
		profile = "default"
	}

	cfg, err := api.ReadConfigFile(".", profile)
	if err != nil {
		return errors.Wrapf(err, "failed to read config file for profile %q", profile)
	}

	// Create minimal provision params for reading stacks
	params := api.ProvisionParams{
		Profile: profile,
	}

	// Read stacks into provisioner (this loads secrets and prepares for placeholder resolution)
	if err := e.provisioner.ReadStacks(ctx, cfg, params, api.ReadIgnoreNoSecretsAndClientCfg); err != nil {
		return errors.Wrapf(err, "failed to read stacks for secret resolution")
	}

	e.logger.Info(ctx, "✅ Stacks loaded - placeholder resolution should now work")
	return nil
}

// getRelevantParentStackName determines which parent stack to use for CI/CD configuration
func (e *Executor) getRelevantParentStackName(ctx context.Context) string {
	stackName := os.Getenv("STACK_NAME")
	if stackName == "" {
		e.logger.Info(ctx, "No STACK_NAME environment variable")
		return ""
	}

	e.logger.Info(ctx, "STACK_NAME environment variable: %s", stackName)

	stacks := e.provisioner.Stacks()
	e.logger.Info(ctx, "Available stacks in provisioner: %v", func() []string {
		keys := make([]string, 0, len(stacks))
		for k := range stacks {
			keys = append(keys, k)
		}
		return keys
	}())

	// Try exact match first
	if stack, exists := stacks[stackName]; exists {
		e.logger.Info(ctx, "Found exact stack match: %s", stackName)
		return e.processStackForParentName(ctx, stackName, stack)
	}

	// If STACK_NAME is in format "org/project/stack", try just the stack part
	if parts := strings.Split(stackName, "/"); len(parts) > 1 {
		shortStackName := parts[len(parts)-1] // Get "pay-space" from "organization/infrastructure/pay-space"
		if stack, exists := stacks[shortStackName]; exists {
			e.logger.Info(ctx, "Found stack using short name: %s (from %s)", shortStackName, stackName)
			return e.processStackForParentName(ctx, shortStackName, stack)
		}
	}

	// Try to find any stack that ends with the stack name
	for loadedStackName, stack := range stacks {
		if strings.HasSuffix(stackName, loadedStackName) || strings.HasSuffix(loadedStackName, stackName) {
			e.logger.Info(ctx, "Found stack by suffix match: %s (matches %s)", loadedStackName, stackName)
			return e.processStackForParentName(ctx, loadedStackName, stack)
		}
	}

	e.logger.Warn(ctx, "Stack %s not found in loaded stacks (tried exact match, short name, and suffix matching)", stackName)
	return ""
}

// processStackForParentName processes a found stack to determine the parent stack name
func (e *Executor) processStackForParentName(ctx context.Context, stackName string, stack api.Stack) string {
	// Check if this is a client stack with a parent reference
	if len(stack.Client.Stacks) > 0 {
		// This is a client stack, find its parent
		for _, clientStack := range stack.Client.Stacks {
			if clientStack.ParentStack != "" {
				// Parent stack reference might be in format "<project>/<stack>", we only need "<stack>"
				parentStackRef := clientStack.ParentStack
				parentStackName := parentStackRef
				if parts := strings.Split(parentStackRef, "/"); len(parts) > 1 {
					parentStackName = parts[len(parts)-1] // Get the last part (stack name)
				}
				e.logger.Info(ctx, "Client stack %s references parent stack %s (resolved to: %s)",
					stackName, parentStackRef, parentStackName)
				return parentStackName
			}
		}
	}

	// This is a parent stack or no parent reference found, use the stack itself
	e.logger.Info(ctx, "Using stack %s as parent stack", stackName)
	return stackName
}

// getNotificationConfigFromLoadedStack extracts notification config from the loaded parent stack
func (e *Executor) getNotificationConfigFromLoadedStack(ctx context.Context) *CICDNotificationConfig {
	parentStackName := e.getRelevantParentStackName(ctx)
	if parentStackName == "" {
		e.logger.Info(ctx, "Could not determine parent stack name")
		return nil
	}

	stacks := e.provisioner.Stacks()
	parentStack, exists := stacks[parentStackName]
	if !exists {
		e.logger.Info(ctx, "Parent stack %s not found in loaded stacks", parentStackName)
		return nil
	}

	// Check if this parent stack has GitHub Actions CI/CD configuration
	if parentStack.Server.CiCd.Type != "github-actions" {
		e.logger.Info(ctx, "Parent stack %s does not have GitHub Actions CI/CD configuration (type: %s)",
			parentStackName, parentStack.Server.CiCd.Type)
		return nil
	}

	e.logger.Info(ctx, "✅ Found GitHub Actions CI/CD configuration in parent stack %s (secrets already resolved)", parentStackName)

	// Extract notification configuration from the loaded config
	if parentStack.Server.CiCd.Config.Config == nil {
		e.logger.Info(ctx, "CI/CD config is nil - no notification settings available")
		return nil
	}

	// Debug: Log the actual config structure
	e.logger.Info(ctx, "🔍 CI/CD config type: %T", parentStack.Server.CiCd.Config.Config)
	e.logger.Info(ctx, "🔍 CI/CD config content: %+v", parentStack.Server.CiCd.Config.Config)

	// Handle GitHub Actions CI/CD configuration struct directly
	if githubConfig, ok := parentStack.Server.CiCd.Config.Config.(*github.GitHubActionsCiCdConfig); ok {
		e.logger.Info(ctx, "🔍 Successfully cast CI/CD config to GitHubActionsCiCdConfig")
		e.logger.Info(ctx, "🔍 GitHub notification config: %+v", githubConfig.Notifications)

		config := &CICDNotificationConfig{}

		// Extract Slack config
		if githubConfig.Notifications.SlackWebhook != "" {
			config.Slack.WebhookURL = githubConfig.Notifications.SlackWebhook
			config.Slack.Enabled = true
			e.logger.Info(ctx, "✅ Found Slack webhook configuration")
		}

		// Extract Discord config
		if githubConfig.Notifications.DiscordWebhook != "" {
			config.Discord.WebhookURL = githubConfig.Notifications.DiscordWebhook
			config.Discord.Enabled = true
			e.logger.Info(ctx, "✅ Found Discord webhook configuration")
		}

		// Extract Telegram config
		if githubConfig.Notifications.TelegramToken != "" && githubConfig.Notifications.TelegramChatID != "" {
			config.Telegram.BotToken = githubConfig.Notifications.TelegramToken
			config.Telegram.ChatID = githubConfig.Notifications.TelegramChatID
			config.Telegram.Enabled = true
			e.logger.Info(ctx, "✅ Found Telegram configuration")
		}

		// Check if any notifications were configured
		if config.Slack.Enabled || config.Discord.Enabled || config.Telegram.Enabled {
			e.logger.Info(ctx, "✅ Found notification configuration in GitHub Actions CI/CD config")
			return config
		} else {
			e.logger.Info(ctx, "📝 GitHub Actions CI/CD config found but no notification webhooks/tokens configured")
			e.logger.Info(ctx, "💡 To enable notifications, add webhook URLs and tokens to your server.yaml:")
			e.logger.Info(ctx, "   cicd.config.notifications.slack: 'your-slack-webhook-url'")
			e.logger.Info(ctx, "   cicd.config.notifications.telegram-token: '${secret:TELEGRAM_BOT_TOKEN}'")
			e.logger.Info(ctx, "   cicd.config.notifications.telegram-chat-id: 'your-chat-id'")
			return nil
		}
	} else {
		e.logger.Info(ctx, "🔍 CI/CD config is not GitHubActionsCiCdConfig, type: %T", parentStack.Server.CiCd.Config.Config)
		e.logger.Info(ctx, "💡 Currently only GitHub Actions CI/CD configurations are supported for notifications")
	}

	e.logger.Info(ctx, "Could not extract notification configuration from loaded config")
	return nil
}

// initializeNotifications initializes notification senders from loaded stack configuration or environment variables (fallback)
func (e *Executor) initializeNotifications(ctx context.Context) {
	e.logger.Info(ctx, "🚀 Starting notification initialization...")

	// Step 1: Prepare provisioner for secret resolution (same pattern as 'sc provision')
	e.logger.Info(ctx, "🔐 Preparing provisioner for secret resolution...")
	if err := e.prepareProvisionerForSecretResolution(ctx); err != nil {
		e.logger.Warn(ctx, "Failed to prepare provisioner for secret resolution: %v", err)
		e.logger.Info(ctx, "🔍 Skipping loaded stack configuration, going to environment variables")
	} else {
		e.logger.Info(ctx, "✅ Provisioner prepared successfully - secrets should be resolved")

		// Step 2: Try to get notification config from loaded stack data (secrets already resolved!)
		e.logger.Info(ctx, "🔍 Looking for notification config in loaded stacks...")
		notificationConfig := e.getNotificationConfigFromLoadedStack(ctx)
		if notificationConfig != nil {
			e.logger.Info(ctx, "✅ Found notification config in loaded stack")
			e.initializeFromConfig(ctx, notificationConfig, "loaded parent stack")
			return
		}

		e.logger.Info(ctx, "❌ No notification config found in loaded stacks")
	}

	// Step 3: Fallback to environment variables
	e.logger.Info(ctx, "🔍 Falling back to environment variables...")
	e.initializeFromEnvironmentVariables(ctx)

	// Final safety check - log what we initialized
	notificationChannels := []string{}
	if e.slackSender != nil {
		notificationChannels = append(notificationChannels, "Slack")
	}
	if e.discordSender != nil {
		notificationChannels = append(notificationChannels, "Discord")
	}
	if e.telegramSender != nil {
		notificationChannels = append(notificationChannels, "Telegram")
	}

	if len(notificationChannels) > 0 {
		e.logger.Info(ctx, "✅ Notification initialization completed - active channels: %v", notificationChannels)
	} else {
		e.logger.Warn(ctx, "❌ Notification initialization completed - NO active channels found")
		e.logger.Info(ctx, "🔧 Check your CI/CD configuration or environment variables:")
		e.logger.Info(ctx, "   - SLACK_WEBHOOK_URL, DISCORD_WEBHOOK_URL, TELEGRAM_BOT_TOKEN/TELEGRAM_CHAT_ID")
		e.logger.Info(ctx, "   - Or ensure parent stack has GitHub Actions CI/CD config with notification settings")
	}
}

// initializeFromConfig initializes notifications from a config object
func (e *Executor) initializeFromConfig(ctx context.Context, config *CICDNotificationConfig, source string) {
	e.logger.Info(ctx, "🔧 Initializing notifications from %s", source)

	// Initialize Slack
	if config.Slack.Enabled && config.Slack.WebhookURL != "" {
		if slackSender, err := slack.New(config.Slack.WebhookURL); err == nil {
			e.slackSender = slackSender
			e.logger.Info(ctx, "✅ Slack notifications enabled (from %s)", source)
		} else {
			e.logger.Warn(ctx, "Failed to initialize Slack notifications from %s: %v", source, err)
		}
	}

	// Initialize Discord
	if config.Discord.Enabled && config.Discord.WebhookURL != "" {
		if discordSender, err := discord.New(config.Discord.WebhookURL); err == nil {
			e.discordSender = discordSender
			e.logger.Info(ctx, "✅ Discord notifications enabled (from %s)", source)
		} else {
			e.logger.Warn(ctx, "Failed to initialize Discord notifications from %s: %v", source, err)
		}
	}

	// Initialize Telegram
	if config.Telegram.Enabled && config.Telegram.BotToken != "" && config.Telegram.ChatID != "" {
		// Fixed parameter order: New(chatId, token)
		telegramSender := telegram.New(config.Telegram.ChatID, config.Telegram.BotToken)
		e.telegramSender = telegramSender
		e.logger.Info(ctx, "✅ Telegram notifications enabled (from %s)", source)
	}
}

// initializeFromEnvironmentVariables initializes notifications from environment variables
func (e *Executor) initializeFromEnvironmentVariables(ctx context.Context) {
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
			// Fixed parameter order: New(chatId, token)
			telegramSender := telegram.New(telegramChatID, telegramBotToken)
			e.telegramSender = telegramSender
			e.logger.Info(ctx, "✅ Telegram notifications enabled (from environment variables)")
		}
	}

	// Log notification status
	if e.slackSender == nil && e.discordSender == nil && e.telegramSender == nil {
		e.logger.Info(ctx, "No notification channels configured")
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
