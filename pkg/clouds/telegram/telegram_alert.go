package telegram

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/pkg/errors"

	"github.com/simple-container-com/api/pkg/api"
)

const (
	// Telegram API limit is 4096 characters per message
	// We use 4000 to leave room for truncation indicator
	maxTelegramMessageLength = 4000
)

type alertSender struct {
	chatId string
	token  string
}

// TelegramMessage represents the structure for sending messages to Telegram Bot API
type TelegramMessage struct {
	ChatID    string `json:"chat_id"`
	Text      string `json:"text"`
	ParseMode string `json:"parse_mode"`
}

func (a *alertSender) Send(alert api.Alert) error {
	if a.token == "" {
		return errors.New("Telegram bot token is required")
	}
	if a.chatId == "" {
		return errors.New("Telegram chat ID is required")
	}

	// Format the alert message with proper Telegram formatting
	message := a.formatAlertMessage(alert)

	// Ensure message doesn't exceed Telegram's limit
	message = a.truncateMessage(message)

	// Create the request payload
	telegramMsg := TelegramMessage{
		ChatID:    a.chatId,
		Text:      message,
		ParseMode: "Markdown",
	}

	// Convert to JSON
	jsonData, err := json.Marshal(telegramMsg)
	if err != nil {
		return errors.Wrap(err, "failed to marshal Telegram message")
	}

	// Make HTTP request to Telegram Bot API
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", a.token)

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return errors.Wrap(err, "failed to send request to Telegram API")
	}
	defer resp.Body.Close()

	// Check if the request was successful and provide detailed error info
	if resp.StatusCode != http.StatusOK {
		// Try to read response body for more details
		var responseBody []byte
		if resp.Body != nil {
			responseBody, _ = io.ReadAll(resp.Body)
		}

		// Extract bot ID from token for debugging (safe to show)
		botID := a.getBotID()

		switch resp.StatusCode {
		case 404:
			return errors.Errorf("Telegram API returned 404 - Bot token is likely invalid or bot doesn't exist. Bot ID: %s, Token format should be like '123456789:AAE...' Response: %s", botID, string(responseBody))
		case 400:
			return errors.Errorf("Telegram API returned 400 - Bad request (check chat_id format). Bot ID: %s, Response: %s", botID, string(responseBody))
		case 401:
			return errors.Errorf("Telegram API returned 401 - Unauthorized (invalid bot token). Bot ID: %s, Response: %s", botID, string(responseBody))
		case 403:
			return errors.Errorf("Telegram API returned 403 - Bot blocked or chat not found. Bot ID: %s, Response: %s", botID, string(responseBody))
		default:
			return errors.Errorf("Telegram API returned status %d. Bot ID: %s, Response: %s", resp.StatusCode, botID, string(responseBody))
		}
	}

	return nil
}

// formatAlertMessage formats the alert into a readable Telegram message with Markdown formatting
func (a *alertSender) formatAlertMessage(alert api.Alert) string {
	var message bytes.Buffer

	// Add emoji based on alert type
	var emoji string
	switch alert.AlertType {
	case api.AlertTriggered:
		emoji = "🚨"
	case api.AlertResolved:
		emoji = "✅"
	case api.BuildStarted:
		emoji = "🔨"
	case api.BuildSucceeded:
		emoji = "🎉"
	case api.BuildFailed:
		emoji = "❌"
	case api.BuildCancelled:
		emoji = "⏸️"
	default:
		emoji = "📢"
	}

	// Use appropriate title based on alert type
	var title string
	switch alert.AlertType {
	case api.BuildStarted, api.BuildSucceeded, api.BuildFailed, api.BuildCancelled:
		title = "Simple Container Build"
	case api.AlertTriggered, api.AlertResolved:
		title = "Simple Container Alert"
	default:
		title = "Simple Container Notification"
	}

	message.WriteString(fmt.Sprintf("%s *%s*\n\n", emoji, title))

	if alert.Name != "" {
		message.WriteString(fmt.Sprintf("**Name:** %s\n", alert.Name))
	}

	if alert.Title != "" {
		message.WriteString(fmt.Sprintf("**Title:** %s\n", alert.Title))
	}

	if alert.Description != "" {
		message.WriteString(fmt.Sprintf("**Description:** %s\n", alert.Description))
	}

	if alert.Reason != "" {
		message.WriteString(fmt.Sprintf("**Reason:** %s\n", alert.Reason))
	}

	if alert.AlertType != "" {
		message.WriteString(fmt.Sprintf("**Type:** `%s`\n", alert.AlertType))
	}

	if alert.StackName != "" {
		message.WriteString(fmt.Sprintf("**Stack:** `%s`\n", alert.StackName))
	}

	if alert.StackEnv != "" {
		message.WriteString(fmt.Sprintf("**Environment:** `%s`\n", alert.StackEnv))
	}

	if alert.CommitAuthor != "" {
		message.WriteString(fmt.Sprintf("**Author:** %s\n", alert.CommitAuthor))
	}

	if alert.CommitMessage != "" {
		// Truncate long commit messages to first line
		commitMsg := alert.CommitMessage
		if len(commitMsg) > 100 {
			commitMsg = commitMsg[:97] + "..."
		}
		message.WriteString(fmt.Sprintf("**Commit:** %s\n", commitMsg))
	}

	if alert.DetailsUrl != "" {
		message.WriteString(fmt.Sprintf("**Details:** %s\n", alert.DetailsUrl))
	}

	message.WriteString(fmt.Sprintf("\n⏰ *%s*", time.Now().Format("2006-01-02 15:04:05 MST")))

	return message.String()
}

// truncateMessage ensures the message doesn't exceed Telegram's character limit
// while preserving the most important information
func (a *alertSender) truncateMessage(message string) string {
	if len(message) <= maxTelegramMessageLength {
		return message
	}

	// Message is too long, need to truncate intelligently
	// Strategy: Keep header info, truncate Description/Reason, keep footer

	lines := strings.Split(message, "\n")
	if len(lines) < 3 {
		// Simple truncation if message structure is unexpected
		return message[:maxTelegramMessageLength-50] + "\n\n⚠️ *[Message truncated due to length]*"
	}

	// Find where Description and Reason fields are
	var headerLines []string
	var footerLines []string
	var descriptionLines []string
	var reasonLines []string

	inDescription := false
	inReason := false

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "**Description:**") {
			inDescription = true
			inReason = false
			descriptionLines = append(descriptionLines, line)
			continue
		} else if strings.HasPrefix(trimmed, "**Reason:**") {
			inDescription = false
			inReason = true
			reasonLines = append(reasonLines, line)
			continue
		} else if strings.HasPrefix(trimmed, "**Type:**") ||
			strings.HasPrefix(trimmed, "**Stack:**") ||
			strings.HasPrefix(trimmed, "**Environment:**") ||
			strings.HasPrefix(trimmed, "**Details:**") ||
			strings.HasPrefix(trimmed, "⏰") {
			inDescription = false
			inReason = false
			footerLines = append(footerLines, line)
			continue
		}

		if inDescription {
			descriptionLines = append(descriptionLines, line)
		} else if inReason {
			reasonLines = append(reasonLines, line)
		} else if i < len(lines)/2 {
			// Lines before middle are probably header
			headerLines = append(headerLines, line)
		} else {
			// Lines in second half go to footer
			footerLines = append(footerLines, line)
		}
	}

	// Calculate available space for Description and Reason
	headerSize := len(strings.Join(headerLines, "\n"))
	footerSize := len(strings.Join(footerLines, "\n"))
	truncationIndicator := "\n\n⚠️ *[Error details truncated - check GitHub Actions logs for full output]*"

	availableSpace := maxTelegramMessageLength - headerSize - footerSize - len(truncationIndicator) - 100 // safety margin

	if availableSpace < 200 {
		// Very little space, just keep essentials
		essentialLines := []string{}
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "🚨") || strings.HasPrefix(trimmed, "❌") ||
				strings.HasPrefix(trimmed, "✅") || strings.HasPrefix(trimmed, "🔨") ||
				strings.HasPrefix(trimmed, "🎉") || strings.HasPrefix(trimmed, "⏸️") ||
				strings.HasPrefix(trimmed, "**Name:**") ||
				strings.HasPrefix(trimmed, "**Title:**") ||
				strings.HasPrefix(trimmed, "**Type:**") ||
				strings.HasPrefix(trimmed, "**Stack:**") ||
				strings.HasPrefix(trimmed, "**Environment:**") ||
				strings.HasPrefix(trimmed, "**Details:**") ||
				strings.HasPrefix(trimmed, "⏰") {
				essentialLines = append(essentialLines, line)
			}
		}
		result := strings.Join(essentialLines, "\n") + truncationIndicator
		if len(result) > maxTelegramMessageLength {
			return result[:maxTelegramMessageLength-3] + "..."
		}
		return result
	}

	// Truncate Description and Reason to fit available space
	var truncatedDetails []string

	if len(descriptionLines) > 0 {
		descText := strings.Join(descriptionLines, "\n")
		if len(descText) > availableSpace/2 {
			descText = descText[:availableSpace/2] + "..."
		}
		truncatedDetails = append(truncatedDetails, descText)
	}

	if len(reasonLines) > 0 {
		reasonText := strings.Join(reasonLines, "\n")
		if len(reasonText) > availableSpace/2 {
			reasonText = reasonText[:availableSpace/2] + "..."
		}
		truncatedDetails = append(truncatedDetails, reasonText)
	}

	// Reconstruct message
	result := strings.Join(headerLines, "\n") + "\n" +
		strings.Join(truncatedDetails, "\n") +
		truncationIndicator + "\n" +
		strings.Join(footerLines, "\n")

	// Final safety check
	if len(result) > maxTelegramMessageLength {
		return result[:maxTelegramMessageLength-3] + "..."
	}

	return result
}

func New(chatId, token string) api.AlertSender {
	return &alertSender{
		chatId: chatId,
		token:  token,
	}
}

// ValidateConfiguration can be used to test the Telegram configuration
func (a *alertSender) ValidateConfiguration() error {
	if a.token == "" {
		return errors.New("Telegram bot token is required")
	}
	if a.chatId == "" {
		return errors.New("Telegram chat ID is required")
	}

	// Test the bot token format
	if len(a.token) < 10 || !contains(a.token, ":") {
		botID := a.getBotID()
		return errors.Errorf("Invalid Telegram bot token format. Bot ID: %s, Should be like '123456789:AAE...'", botID)
	}

	return nil
}

func contains(s, substr string) bool {
	for i := 0; i < len(s)-len(substr)+1; i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// getBotID extracts the bot ID (part before colon) from token for debugging
// This is safe to show as bot IDs are not sensitive
func (a *alertSender) getBotID() string {
	if a.token == "" {
		return "empty"
	}

	colonIndex := -1
	for i, c := range a.token {
		if c == ':' {
			colonIndex = i
			break
		}
	}

	if colonIndex > 0 {
		return a.token[:colonIndex]
	}

	// If no colon found, show first 10 chars max
	if len(a.token) > 10 {
		return a.token[:10] + "..."
	}
	return a.token
}
