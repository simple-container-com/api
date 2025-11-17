package telegram

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/simple-container-com/api/pkg/api"
)

func TestEscapeHTML(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "no special characters",
			input:    "hello world",
			expected: "hello world",
		},
		{
			name:     "ampersand escaping",
			input:    "test & more",
			expected: "test &amp; more",
		},
		{
			name:     "less than escaping",
			input:    "value < 10",
			expected: "value &lt; 10",
		},
		{
			name:     "greater than escaping",
			input:    "value > 5",
			expected: "value &gt; 5",
		},
		{
			name:     "quote escaping",
			input:    `say "hello"`,
			expected: "say &quot;hello&quot;",
		},
		{
			name:     "dashes should NOT be escaped",
			input:    "white-label-staging",
			expected: "white-label-staging",
		},
		{
			name:     "dots should NOT be escaped",
			input:    "1m43.466630034s",
			expected: "1m43.466630034s",
		},
		{
			name:     "parentheses should NOT be escaped",
			input:    "function(param)",
			expected: "function(param)",
		},
		{
			name:     "underscores should NOT be escaped",
			input:    "test_variable",
			expected: "test_variable",
		},
		{
			name:     "asterisks should NOT be escaped",
			input:    "test*bold*text",
			expected: "test*bold*text",
		},
		{
			name:     "complex deployment name",
			input:    "monitoring-staging",
			expected: "monitoring-staging",
		},
		{
			name:     "complex time duration",
			input:    "6m19.535263328s",
			expected: "6m19.535263328s",
		},
		{
			name:     "mixed characters",
			input:    "deploy < 5min & status > ok with \"quotes\"",
			expected: "deploy &lt; 5min &amp; status &gt; ok with &quot;quotes&quot;",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := escapeHTML(tt.input)
			if result != tt.expected {
				t.Errorf("escapeHTML(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestEscapeMarkdown(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "no special characters",
			input:    "hello world",
			expected: "hello world",
		},
		{
			name:     "underscore escaping",
			input:    "test_variable",
			expected: "test\\_variable",
		},
		{
			name:     "asterisk escaping",
			input:    "test*bold*text",
			expected: "test\\*bold\\*text",
		},
		{
			name:     "square brackets escaping",
			input:    "[link text]",
			expected: "\\[link text\\]",
		},
		{
			name:     "backtick escaping",
			input:    "`code block`",
			expected: "\\`code block\\`",
		},
		{
			name:     "backslash escaping",
			input:    "path\\to\\file",
			expected: "path\\\\to\\\\file",
		},
		{
			name:     "dashes should NOT be escaped",
			input:    "white-label-staging",
			expected: "white-label-staging",
		},
		{
			name:     "dots should NOT be escaped",
			input:    "1m43.466630034s",
			expected: "1m43.466630034s",
		},
		{
			name:     "parentheses should NOT be escaped",
			input:    "function(param)",
			expected: "function(param)",
		},
		{
			name:     "complex deployment name",
			input:    "monitoring-staging",
			expected: "monitoring-staging",
		},
		{
			name:     "complex time duration",
			input:    "6m19.535263328s",
			expected: "6m19.535263328s",
		},
		{
			name:     "mixed special characters",
			input:    "test_var with *bold* and [link] and `code` but keep-dashes and dots.txt",
			expected: "test\\_var with \\*bold\\* and \\[link\\] and \\`code\\` but keep-dashes and dots.txt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := escapeMarkdown(tt.input)
			if result != tt.expected {
				t.Errorf("escapeMarkdown(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestFormatAlertMessage(t *testing.T) {
	sender := &alertSender{
		chatId: "test-chat",
		token:  "test-token",
	}

	tests := []struct {
		name        string
		alert       api.Alert
		contains    []string
		notContains []string
	}{
		{
			name: "deployment success alert",
			alert: api.Alert{
				Name:        "monitoring-staging",
				Title:       "Deploy Succeeded",
				Description: "Successfully deployed monitoring to staging in 6m19.535263328s",
				AlertType:   api.BuildSucceeded,
				StackName:   "monitoring",
				StackEnv:    "staging",
			},
			contains: []string{
				"🎉 <b>Simple Container Build</b>",
				"<b>Name:</b> monitoring-staging",
				"<b>Title:</b> Deploy Succeeded",
				"<b>Description:</b> Successfully deployed monitoring to staging in 6m19.535263328s",
				"<b>Stack:</b> <code>monitoring</code>",
				"<b>Environment:</b> <code>staging</code>",
			},
			notContains: []string{
				"monitoring\\-staging", // Should NOT have escaped dashes
				"6m19\\.535263328s",    // Should NOT have escaped dots
			},
		},
		{
			name: "deployment failure alert",
			alert: api.Alert{
				Name:        "white-label-staging",
				Title:       "Deploy Failed",
				Description: "Deploy of white-label failed after 1m43.466630034s: deployment failed",
				AlertType:   api.BuildFailed,
				StackName:   "white-label",
				StackEnv:    "staging",
			},
			contains: []string{
				"❌ <b>Simple Container Build</b>",
				"<b>Name:</b> white-label-staging",
				"<b>Title:</b> Deploy Failed",
				"<b>Description:</b> Deploy of white-label failed after 1m43.466630034s: deployment failed",
			},
			notContains: []string{
				"white\\-label\\-staging", // Should NOT have escaped dashes
				"1m43\\.466630034s",       // Should NOT have escaped dots
			},
		},
		{
			name: "alert with special HTML characters",
			alert: api.Alert{
				Name:        "test_service",
				Title:       "Test <bold> and \"quotes\"",
				Description: "Alert with & ampersand and > greater than",
				AlertType:   api.AlertTriggered,
			},
			contains: []string{
				"🚨 <b>Simple Container Alert</b>",
				"<b>Name:</b> test_service",                                            // Underscore should NOT be escaped in HTML
				"<b>Title:</b> Test &lt;bold&gt; and &quot;quotes&quot;",               // HTML chars should be escaped
				"<b>Description:</b> Alert with &amp; ampersand and &gt; greater than", // HTML chars should be escaped
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sender.formatAlertMessage(tt.alert)

			for _, expected := range tt.contains {
				if !strings.Contains(result, expected) {
					t.Errorf("formatAlertMessage() result should contain %q, but got:\n%s", expected, result)
				}
			}

			for _, notExpected := range tt.notContains {
				if strings.Contains(result, notExpected) {
					t.Errorf("formatAlertMessage() result should NOT contain %q, but got:\n%s", notExpected, result)
				}
			}
		})
	}
}

func TestTruncateMessage(t *testing.T) {
	sender := &alertSender{}

	tests := []struct {
		name     string
		input    string
		maxLen   int
		expected bool // whether truncation should occur
	}{
		{
			name:     "short message no truncation",
			input:    "Short message",
			maxLen:   4000,
			expected: false,
		},
		{
			name:     "long message gets truncated",
			input:    strings.Repeat("a", 5000),
			maxLen:   4000,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sender.truncateMessage(tt.input)

			if tt.expected {
				// Should be truncated
				if len(result) >= len(tt.input) {
					t.Errorf("truncateMessage() should have truncated message, input len: %d, result len: %d", len(tt.input), len(result))
				}
				if len(result) > maxTelegramMessageLength {
					t.Errorf("truncateMessage() result too long: %d > %d", len(result), maxTelegramMessageLength)
				}
			} else {
				// Should not be truncated
				if result != tt.input {
					t.Errorf("truncateMessage() should not have modified short message")
				}
			}
		})
	}
}

func TestSendAlert(t *testing.T) {
	// Create a test server to mock Telegram API
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify the request
		if r.Method != "POST" {
			t.Errorf("Expected POST request, got %s", r.Method)
		}

		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Expected Content-Type application/json, got %s", r.Header.Get("Content-Type"))
		}

		// Parse the request body
		var telegramMsg TelegramMessage
		if err := json.NewDecoder(r.Body).Decode(&telegramMsg); err != nil {
			t.Errorf("Failed to decode request body: %v", err)
		}

		// Verify the message content
		if telegramMsg.ChatID != "test-chat-id" {
			t.Errorf("Expected chat_id 'test-chat-id', got %s", telegramMsg.ChatID)
		}

		if telegramMsg.ParseMode != "HTML" {
			t.Errorf("Expected parse_mode 'HTML', got %s", telegramMsg.ParseMode)
		}

		// Check that the message doesn't contain over-escaped characters
		if strings.Contains(telegramMsg.Text, "monitoring\\-staging") {
			t.Errorf("Message contains over-escaped dashes: %s", telegramMsg.Text)
		}

		if strings.Contains(telegramMsg.Text, "6m19\\.535") {
			t.Errorf("Message contains over-escaped dots: %s", telegramMsg.Text)
		}

		// Return success response
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":123}}`))
	}))
	defer server.Close()

	// Create sender with test server URL
	sender := &alertSender{
		chatId: "test-chat-id",
		token:  "123456789:test-token",
	}

	// Override the URL for testing (we'd need to modify the code to make this configurable)
	// For now, this test verifies the message formatting

	alert := api.Alert{
		Name:        "monitoring-staging",
		Title:       "Deploy Succeeded",
		Description: "Successfully deployed monitoring to staging in 6m19.535263328s",
		AlertType:   api.BuildSucceeded,
		StackName:   "monitoring",
		StackEnv:    "staging",
	}

	// Test message formatting (we can't easily test the HTTP call without modifying the production code)
	message := sender.formatAlertMessage(alert)

	// Verify no over-escaping
	if strings.Contains(message, "monitoring\\-staging") {
		t.Errorf("Message contains over-escaped dashes: %s", message)
	}

	if strings.Contains(message, "6m19\\.535") {
		t.Errorf("Message contains over-escaped dots: %s", message)
	}

	// Verify proper content
	if !strings.Contains(message, "monitoring-staging") {
		t.Errorf("Message should contain unescaped 'monitoring-staging': %s", message)
	}

	if !strings.Contains(message, "6m19.535263328s") {
		t.Errorf("Message should contain unescaped time duration: %s", message)
	}
}

func TestValidateConfiguration(t *testing.T) {
	tests := []struct {
		name      string
		chatId    string
		token     string
		expectErr bool
	}{
		{
			name:      "valid configuration",
			chatId:    "test-chat-id",
			token:     "123456789:AAE_test_token_here",
			expectErr: false,
		},
		{
			name:      "missing token",
			chatId:    "test-chat-id",
			token:     "",
			expectErr: true,
		},
		{
			name:      "missing chat id",
			chatId:    "",
			token:     "123456789:AAE_test_token_here",
			expectErr: true,
		},
		{
			name:      "invalid token format",
			chatId:    "test-chat-id",
			token:     "invalid-token",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sender := &alertSender{
				chatId: tt.chatId,
				token:  tt.token,
			}

			err := sender.ValidateConfiguration()

			if tt.expectErr && err == nil {
				t.Errorf("ValidateConfiguration() should have returned an error")
			}

			if !tt.expectErr && err != nil {
				t.Errorf("ValidateConfiguration() should not have returned an error: %v", err)
			}
		})
	}
}

func TestNew(t *testing.T) {
	chatId := "test-chat-id"
	token := "test-token"

	sender := New(chatId, token)

	if sender == nil {
		t.Errorf("New() should return a non-nil sender")
	}

	// Type assertion to access internal fields for testing
	if alertSender, ok := sender.(*alertSender); ok {
		if alertSender.chatId != chatId {
			t.Errorf("New() chatId = %s, want %s", alertSender.chatId, chatId)
		}
		if alertSender.token != token {
			t.Errorf("New() token = %s, want %s", alertSender.token, token)
		}
	} else {
		t.Errorf("New() should return an *alertSender")
	}
}

// Benchmark tests to ensure performance is acceptable
func BenchmarkEscapeMarkdown(b *testing.B) {
	text := "Deploy of white-label failed after 1m43.466630034s: deployment failed with error (exit code 255)"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		escapeMarkdown(text)
	}
}

func BenchmarkFormatAlertMessage(b *testing.B) {
	sender := &alertSender{}
	alert := api.Alert{
		Name:          "monitoring-staging",
		Title:         "Deploy Succeeded",
		Description:   "Successfully deployed monitoring to staging in 6m19.535263328s",
		AlertType:     api.BuildSucceeded,
		StackName:     "monitoring",
		StackEnv:      "staging",
		CommitAuthor:  "developer@example.com",
		CommitMessage: "Fix deployment configuration",
		DetailsUrl:    "https://github.com/example/repo/actions/runs/123",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sender.formatAlertMessage(alert)
	}
}
