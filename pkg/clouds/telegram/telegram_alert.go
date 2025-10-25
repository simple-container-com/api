package telegram

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/pkg/errors"

	"github.com/simple-container-com/api/pkg/api"
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

	// Check if the request was successful
	if resp.StatusCode != http.StatusOK {
		return errors.Errorf("Telegram API returned status %d", resp.StatusCode)
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

	if alert.DetailsUrl != "" {
		message.WriteString(fmt.Sprintf("**Details:** %s\n", alert.DetailsUrl))
	}

	message.WriteString(fmt.Sprintf("\n⏰ *%s*", time.Now().Format("2006-01-02 15:04:05 MST")))

	return message.String()
}

func New(chatId, token string) api.AlertSender {
	return &alertSender{
		chatId: chatId,
		token:  token,
	}
}
