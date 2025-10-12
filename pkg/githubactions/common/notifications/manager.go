package notifications

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/simple-container-com/api/pkg/githubactions/common/git"
	"github.com/simple-container-com/api/pkg/githubactions/config"
	"github.com/simple-container-com/api/pkg/githubactions/utils/logging"
)

// Status represents notification status types
type Status string

const (
	StatusStarted   Status = "started"
	StatusSuccess   Status = "success"
	StatusFailure   Status = "failure"
	StatusCancelled Status = "cancelled"
)

// Manager handles sending notifications to various platforms
type Manager struct {
	cfg    *config.Config
	logger logging.Logger
	client *http.Client
}

// SlackPayload represents a Slack webhook payload
type SlackPayload struct {
	Blocks []SlackBlock `json:"blocks"`
}

// SlackBlock represents a Slack message block
type SlackBlock struct {
	Type string    `json:"type"`
	Text SlackText `json:"text"`
}

// SlackText represents Slack text content
type SlackText struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// DiscordPayload represents a Discord webhook payload
type DiscordPayload struct {
	Embeds []DiscordEmbed `json:"embeds"`
}

// DiscordEmbed represents a Discord embed
type DiscordEmbed struct {
	Title       string        `json:"title"`
	Description string        `json:"description"`
	URL         string        `json:"url"`
	Color       int           `json:"color"`
	Timestamp   string        `json:"timestamp"`
	Footer      DiscordFooter `json:"footer,omitempty"`
}

// DiscordFooter represents a Discord embed footer
type DiscordFooter struct {
	Text string `json:"text"`
}

// NewManager creates a new notifications manager
func NewManager(cfg *config.Config, logger logging.Logger) *Manager {
	return &Manager{
		cfg:    cfg,
		logger: logger,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

// SendNotification sends a notification with the given status
func (n *Manager) SendNotification(ctx context.Context, status Status, metadata *git.Metadata, version string, duration time.Duration) error {
	n.logger.Info("Sending notifications", "status", status, "version", version)

	var errs []error

	// Send Slack notification if configured
	if n.cfg.SlackWebhookURL != "" {
		if err := n.sendSlackNotification(ctx, status, metadata, version, duration); err != nil {
			n.logger.Warn("Slack notification failed", "error", err)
			errs = append(errs, fmt.Errorf("slack notification failed: %w", err))
		}
	}

	// Send Discord notification if configured
	if n.cfg.DiscordWebhookURL != "" {
		if err := n.sendDiscordNotification(ctx, status, metadata, version, duration); err != nil {
			n.logger.Warn("Discord notification failed", "error", err)
			errs = append(errs, fmt.Errorf("discord notification failed: %w", err))
		}
	}

	// If no webhooks configured, just log
	if n.cfg.SlackWebhookURL == "" && n.cfg.DiscordWebhookURL == "" {
		n.logger.Info("No notification webhooks configured, skipping notifications")
	}

	// Return first error if any occurred
	if len(errs) > 0 {
		return errs[0]
	}

	return nil
}

// sendSlackNotification sends a notification to Slack
func (n *Manager) sendSlackNotification(ctx context.Context, status Status, metadata *git.Metadata, version string, duration time.Duration) error {
	emoji := n.getEmoji(status)
	message := n.formatSlackMessage(status, emoji, metadata, version, duration)

	payload := SlackPayload{
		Blocks: []SlackBlock{
			{
				Type: "section",
				Text: SlackText{
					Type: "mrkdwn",
					Text: message,
				},
			},
		},
	}

	return n.sendWebhook(ctx, n.cfg.SlackWebhookURL, payload)
}

// sendDiscordNotification sends a notification to Discord
func (n *Manager) sendDiscordNotification(ctx context.Context, status Status, metadata *git.Metadata, version string, duration time.Duration) error {
	emoji := n.getEmoji(status)
	title := fmt.Sprintf("Simple Container Deployment - %s %s", strings.ToUpper(string(status)), emoji)
	description := n.formatDiscordDescription(status, metadata, version, duration)
	color := n.getDiscordColor(status)

	embed := DiscordEmbed{
		Title:       title,
		Description: description,
		URL:         metadata.BuildURL,
		Color:       color,
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
		Footer: DiscordFooter{
			Text: "Simple Container GitHub Actions",
		},
	}

	payload := DiscordPayload{
		Embeds: []DiscordEmbed{embed},
	}

	return n.sendWebhook(ctx, n.cfg.DiscordWebhookURL, payload)
}

// formatSlackMessage formats a message for Slack
func (n *Manager) formatSlackMessage(status Status, emoji string, metadata *git.Metadata, version string, duration time.Duration) string {
	statusText := strings.ToUpper(string(status))
	buildURL := metadata.BuildURL
	stackName := n.cfg.StackName
	environment := n.cfg.Environment
	author := metadata.Author

	baseMessage := fmt.Sprintf("%s *<%s|%s>* deploy *%s* to *%s* (v%s) by %s",
		emoji, buildURL, statusText, stackName, environment, version, author)

	switch status {
	case StatusStarted:
		if n.cfg.CCOnStart {
			baseMessage += n.getCCDevs("start")
		}
	case StatusSuccess:
		branch := metadata.Branch
		commitMessage := metadata.Message
		durationText := n.formatDuration(duration)
		baseMessage = fmt.Sprintf("%s *<%s|%s>* deploy *%s* to *%s* (v%s) (%s) - %s by %s (took: %s)",
			emoji, buildURL, statusText, stackName, environment, version, branch, commitMessage, author, durationText)
	case StatusFailure, StatusCancelled:
		branch := metadata.Branch
		commitMessage := metadata.Message
		baseMessage = fmt.Sprintf("%s *<%s|%s>* deploy *%s* to *%s* (%s) - %s by %s",
			emoji, buildURL, statusText, stackName, environment, branch, commitMessage, author)
		baseMessage += n.getCCDevs("failure")
	}

	return baseMessage
}

// formatDiscordDescription formats a description for Discord
func (n *Manager) formatDiscordDescription(status Status, metadata *git.Metadata, version string, duration time.Duration) string {
	var description strings.Builder

	description.WriteString(fmt.Sprintf("**Stack**: %s\n", n.cfg.StackName))
	description.WriteString(fmt.Sprintf("**Environment**: %s\n", n.cfg.Environment))
	description.WriteString(fmt.Sprintf("**Version**: %s\n", version))
	description.WriteString(fmt.Sprintf("**Branch**: %s\n", metadata.Branch))
	description.WriteString(fmt.Sprintf("**Author**: %s\n", metadata.Author))

	if status == StatusSuccess {
		description.WriteString(fmt.Sprintf("**Duration**: %s\n", n.formatDuration(duration)))
	}

	if metadata.Message != "" {
		description.WriteString(fmt.Sprintf("**Commit**: %s\n", metadata.Message))
	}

	// Add PR preview URL if applicable
	if n.cfg.PRPreview && n.cfg.PRNumber != "" {
		previewURL := fmt.Sprintf("https://pr%s-%s", n.cfg.PRNumber, n.cfg.PreviewDomainBase)
		description.WriteString(fmt.Sprintf("**Preview URL**: %s\n", previewURL))
	}

	return description.String()
}

// sendWebhook sends a webhook payload to the specified URL
func (n *Manager) sendWebhook(ctx context.Context, webhookURL string, payload interface{}) error {
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", webhookURL, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := n.client.Do(req)
	if err != nil {
		return fmt.Errorf("webhook request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}

	return nil
}

// getEmoji returns an appropriate emoji for the status
func (n *Manager) getEmoji(status Status) string {
	switch status {
	case StatusStarted:
		return "🚧"
	case StatusSuccess:
		return "✅"
	case StatusFailure:
		return "❗"
	case StatusCancelled:
		return "❌"
	default:
		return "ℹ️"
	}
}

// getDiscordColor returns an appropriate color for Discord embeds
func (n *Manager) getDiscordColor(status Status) int {
	switch status {
	case StatusStarted:
		return 0xFFA500 // Orange
	case StatusSuccess:
		return 0x00FF00 // Green
	case StatusFailure:
		return 0xFF0000 // Red
	case StatusCancelled:
		return 0x808080 // Gray
	default:
		return 0x0099FF // Blue
	}
}

// getCCDevs returns CC text for relevant team members
func (n *Manager) getCCDevs(notificationType string) string {
	// This could be enhanced to load actual user mappings from configuration
	// For now, returning a generic CC message
	switch notificationType {
	case "start":
		// Only CC on start if configured
		if n.cfg.CCOnStart {
			return " (deployment started)"
		}
		return ""
	case "failure":
		return " (cc: DevOps team)"
	default:
		return ""
	}
}

// formatDuration formats a duration in a human-readable format
func (n *Manager) formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}

	minutes := int(d.Minutes())
	seconds := int(d.Seconds()) % 60

	if minutes < 60 {
		return fmt.Sprintf("%dm%ds", minutes, seconds)
	}

	hours := minutes / 60
	minutes = minutes % 60

	return fmt.Sprintf("%dh%dm%ds", hours, minutes, seconds)
}
