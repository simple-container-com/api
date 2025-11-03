package discord

import (
	"fmt"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/webhook"
	"github.com/pkg/errors"

	"github.com/simple-container-com/api/pkg/api"
)

type alertSender struct {
	client     webhook.Client
	webhookUrl string
}

func (a *alertSender) Send(alert api.Alert) error {
	icon := getIconForAlertType(alert.AlertType)

	// Build message with commit information if available
	message := fmt.Sprintf(" **%s** [%s](%s) for **%s** in *%s*",
		alert.AlertType, alert.Title, alert.DetailsUrl, alert.StackName, alert.StackEnv)

	if alert.CommitAuthor != "" || alert.CommitMessage != "" {
		message += "\n"
		if alert.CommitAuthor != "" {
			message += fmt.Sprintf("👤 Author: %s", alert.CommitAuthor)
		}
		if alert.CommitMessage != "" {
			// Truncate long commit messages
			commitMsg := alert.CommitMessage
			if len(commitMsg) > 100 {
				commitMsg = commitMsg[:97] + "..."
			}
			if alert.CommitAuthor != "" {
				message += " • "
			}
			message += fmt.Sprintf("💬 %s", commitMsg)
		}
	}

	if alert.Description != "" {
		message += fmt.Sprintf("\n%s", alert.Description)
	}

	_, err := a.client.CreateMessage(discord.WebhookMessageCreate{
		Content: icon + message,
	})
	return err
}

func getIconForAlertType(alertType api.AlertType) string {
	switch alertType {
	// Monitoring Alert Types
	case api.AlertTriggered:
		return "⚠️"
	case api.AlertResolved:
		return "✅"
	// Build/Deployment Notification Types
	case api.BuildStarted:
		return "🚀"
	case api.BuildSucceeded:
		return "✅"
	case api.BuildFailed:
		return "❌"
	case api.BuildCancelled:
		return "⏹️"
	default:
		return "ℹ️"
	}
}

func New(webhookUrl string) (api.AlertSender, error) {
	client, err := webhook.NewWithURL(webhookUrl)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to init webhook client")
	}
	return &alertSender{
		client:     client,
		webhookUrl: webhookUrl,
	}, nil
}
