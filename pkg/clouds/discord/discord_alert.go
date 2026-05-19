package discord

import (
	"fmt"
	"strings"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/webhook"
	"github.com/pkg/errors"

	"github.com/simple-container-com/api/pkg/api"
)

const (
	// Discord's message content limit is 2000 characters
	// We use 1900 to leave room for truncation indicator
	maxDiscordMessageLength = 1900
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

	// Ensure message doesn't exceed Discord's limit
	fullMessage := icon + message
	if len(fullMessage) > maxDiscordMessageLength {
		// Truncate description intelligently
		truncationIndicator := "\n\n⚠️ **[Error details truncated - check GitHub Actions logs for full output]**"

		// Calculate how much space we have for description
		baseMessage := icon + fmt.Sprintf(" **%s** [%s](%s) for **%s** in *%s*",
			alert.AlertType, alert.Title, alert.DetailsUrl, alert.StackName, alert.StackEnv)

		if alert.CommitAuthor != "" || alert.CommitMessage != "" {
			baseMessage += "\n"
			if alert.CommitAuthor != "" {
				baseMessage += fmt.Sprintf("👤 Author: %s", alert.CommitAuthor)
			}
			if alert.CommitMessage != "" {
				commitMsg := alert.CommitMessage
				if len(commitMsg) > 100 {
					commitMsg = commitMsg[:97] + "..."
				}
				if alert.CommitAuthor != "" {
					baseMessage += " • "
				}
				baseMessage += fmt.Sprintf("💬 %s", commitMsg)
			}
		}

		availableSpace := maxDiscordMessageLength - len(baseMessage) - len(truncationIndicator) - 10 // safety margin

		if availableSpace > 50 && alert.Description != "" {
			// Use intelligent truncation to show both beginning and end
			truncatedDesc := alert.Description
			if len(truncatedDesc) > availableSpace {
				truncatedDesc = intelligentTruncate(truncatedDesc, availableSpace)
			}
			fullMessage = baseMessage + fmt.Sprintf("\n%s", truncatedDesc) + truncationIndicator
		} else {
			// Very little space, just send essentials
			fullMessage = baseMessage + truncationIndicator
		}

		// Final safety check
		if len(fullMessage) > maxDiscordMessageLength {
			fullMessage = fullMessage[:maxDiscordMessageLength-3] + "..."
		}
	}

	_, err := a.client.CreateMessage(discord.WebhookMessageCreate{
		Content: fullMessage,
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

// intelligentTruncate truncates long text to show both beginning and end
// since the most important information (actual error) is usually at the end
func intelligentTruncate(text string, maxLength int) string {
	if len(text) <= maxLength {
		return text
	}

	// For long text, show beginning and end with separator
	// This ensures we capture both context (beginning) and the actual error (end)

	// Reserve space for the separator
	separator := "\n\n[... truncated ...]\n\n"
	separatorLen := len(separator)

	// Calculate space for beginning and end portions
	availableSpace := maxLength - separatorLen
	beginningLen := availableSpace / 3      // 1/3 for beginning
	endLen := availableSpace - beginningLen // 2/3 for end (more important)

	// Ensure minimum lengths
	if beginningLen < 50 {
		beginningLen = 50
		endLen = availableSpace - beginningLen
	}
	if endLen < 100 {
		endLen = 100
		beginningLen = availableSpace - endLen
	}

	// Fall back to a simple end-trim if maxLength is too small for the
	// 50-byte beginning / 100-byte end minimums to fit. Without this the
	// floor clamps above push beginningLen / endLen negative and the
	// slice operations below panic. The production caller (Send) guards
	// against this via `availableSpace > 50`, but a defensive fallback
	// here keeps the helper safe for any caller.
	if beginningLen < 0 || endLen < 0 || beginningLen+endLen > len(text) {
		if maxLength <= 3 {
			if maxLength < 0 {
				return ""
			}
			return text[:maxLength]
		}
		return text[:maxLength-3] + "..."
	}

	// Extract beginning and end portions
	beginning := text[:beginningLen]
	end := text[len(text)-endLen:]

	// Try to break at line boundaries for cleaner truncation
	if lastNewline := strings.LastIndex(beginning, "\n"); lastNewline > beginningLen-100 {
		beginning = beginning[:lastNewline]
	}
	if firstNewline := strings.Index(end, "\n"); firstNewline < 100 && firstNewline > 0 {
		end = end[firstNewline+1:]
	}

	return beginning + separator + end
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
