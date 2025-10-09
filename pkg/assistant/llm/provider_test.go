package llm

import (
	"testing"
	"time"
)

func TestGetModelContextSize(t *testing.T) {
	tests := []struct {
		model    string
		expected int
	}{
		// OpenAI models
		{"gpt-3.5-turbo", 16385},
		{"gpt-4", 8192},
		{"gpt-4-turbo", 128000},
		{"gpt-4o", 128000},

		// Anthropic models
		{"claude-3-5-sonnet-20241022", 200000},
		{"claude-3-opus", 200000},
		{"claude-3-haiku", 200000},

		// DeepSeek models
		{"deepseek-chat", 64000},

		// Prefix matching
		{"gpt-4-turbo-2024-04-09", 128000},
		{"claude-3-5-sonnet-20250101", 200000},

		// Unknown model (should return default)
		{"unknown-model", 4096},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			size := GetModelContextSize(tt.model)
			if size != tt.expected {
				t.Errorf("GetModelContextSize(%q) = %d, want %d", tt.model, size, tt.expected)
			}
		})
	}
}

func TestTrimMessagesToContextSize(t *testing.T) {
	// Create test messages
	messages := []Message{
		{Role: "system", Content: "You are a helpful assistant.", Timestamp: time.Now()},
		{Role: "user", Content: "Hello", Timestamp: time.Now()},
		{Role: "assistant", Content: "Hi there!", Timestamp: time.Now()},
		{Role: "user", Content: "How are you?", Timestamp: time.Now()},
		{Role: "assistant", Content: "I'm doing great!", Timestamp: time.Now()},
		{Role: "user", Content: "What's the weather?", Timestamp: time.Now()},
	}

	tests := []struct {
		name          string
		messages      []Message
		model         string
		reserveTokens int
		wantMinLen    int
		wantMaxLen    int
		wantSystem    bool
	}{
		{
			name:          "Small model, should trim heavily",
			messages:      messages,
			model:         "unknown-model", // 4096 tokens
			reserveTokens: 2048,
			wantMinLen:    2, // At least system + 1 message
			wantMaxLen:    len(messages),
			wantSystem:    true,
		},
		{
			name:          "Large model, should keep all",
			messages:      messages,
			model:         "gpt-4-turbo", // 128000 tokens
			reserveTokens: 2048,
			wantMinLen:    len(messages),
			wantMaxLen:    len(messages),
			wantSystem:    true,
		},
		{
			name:          "No system message",
			messages:      messages[1:], // Skip system message
			model:         "gpt-3.5-turbo",
			reserveTokens: 2048,
			wantMinLen:    1,
			wantMaxLen:    len(messages) - 1,
			wantSystem:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			trimmed := TrimMessagesToContextSize(tt.messages, tt.model, tt.reserveTokens)

			// Check length constraints
			if len(trimmed) < tt.wantMinLen {
				t.Errorf("TrimMessagesToContextSize() returned %d messages, want at least %d",
					len(trimmed), tt.wantMinLen)
			}
			if len(trimmed) > tt.wantMaxLen {
				t.Errorf("TrimMessagesToContextSize() returned %d messages, want at most %d",
					len(trimmed), tt.wantMaxLen)
			}

			// Check system message preservation
			if tt.wantSystem && len(trimmed) > 0 {
				if trimmed[0].Role != "system" {
					t.Errorf("TrimMessagesToContextSize() should preserve system message as first message")
				}
			}

			// Check that we kept the most recent messages
			if len(trimmed) > 1 && len(tt.messages) > len(trimmed) {
				lastTrimmed := trimmed[len(trimmed)-1]
				lastOriginal := tt.messages[len(tt.messages)-1]
				if lastTrimmed.Content != lastOriginal.Content {
					t.Errorf("TrimMessagesToContextSize() should keep most recent messages")
				}
			}
		})
	}
}

func TestEstimateMessageTokens(t *testing.T) {
	tests := []struct {
		name    string
		message Message
		wantMin int
		wantMax int
	}{
		{
			name: "Short message",
			message: Message{
				Role:    "user",
				Content: "Hello",
			},
			wantMin: 10, // At least overhead
			wantMax: 20, // Small message
		},
		{
			name: "Long message",
			message: Message{
				Role:    "user",
				Content: string(make([]byte, 4000)), // 4000 characters
			},
			wantMin: 1000, // ~1000 tokens
			wantMax: 1100, // Plus overhead
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens := estimateMessageTokens(tt.message)
			if tokens < tt.wantMin {
				t.Errorf("estimateMessageTokens() = %d, want at least %d", tokens, tt.wantMin)
			}
			if tokens > tt.wantMax {
				t.Errorf("estimateMessageTokens() = %d, want at most %d", tokens, tt.wantMax)
			}
		})
	}
}

func TestTrimMessagesToContextSize_EmptyInput(t *testing.T) {
	// Test with empty messages
	trimmed := TrimMessagesToContextSize([]Message{}, "gpt-4", 2048)
	if len(trimmed) != 0 {
		t.Errorf("TrimMessagesToContextSize(empty) should return empty slice, got %d messages", len(trimmed))
	}
}

func TestTrimMessagesToContextSize_PreservesSystemMessage(t *testing.T) {
	// Create messages with reasonable content to test system message preservation
	messages := []Message{
		{Role: "system", Content: "System prompt", Timestamp: time.Now()},
		{Role: "user", Content: "First question", Timestamp: time.Now()},
		{Role: "assistant", Content: "First answer", Timestamp: time.Now()},
		{Role: "user", Content: "Second question", Timestamp: time.Now()},
		{Role: "assistant", Content: "Second answer", Timestamp: time.Now()},
	}

	// Use small model to force trimming
	trimmed := TrimMessagesToContextSize(messages, "unknown-model", 2048)

	// Should keep system message when it fits
	if len(trimmed) == 0 {
		t.Errorf("TrimMessagesToContextSize() returned empty result")
	}

	// System message should be first if present
	if trimmed[0].Role == "system" && trimmed[0].Content != "System prompt" {
		t.Errorf("TrimMessagesToContextSize() altered system message content")
	}

	// Should always keep last message (API requirement)
	if len(trimmed) > 0 && trimmed[len(trimmed)-1].Content != "Second answer" {
		t.Errorf("TrimMessagesToContextSize() should always keep last message")
	}
}

func TestTrimMessagesToContextSize_PrioritizesToolCalls(t *testing.T) {
	// Create messages with tool calls that should be prioritized
	messages := []Message{
		{Role: "system", Content: "System prompt", Timestamp: time.Now()},
		{Role: "user", Content: "Old question 1", Timestamp: time.Now()},
		{Role: "assistant", Content: "Old answer 1", Timestamp: time.Now()},
		{Role: "user", Content: "Question with tool", Timestamp: time.Now()},
		{Role: "assistant", Content: "Calling tool", Timestamp: time.Now(), Metadata: map[string]interface{}{
			"tool_calls": []interface{}{
				map[string]interface{}{"name": "search", "args": "test"},
			},
		}},
		{Role: "tool", Content: "Tool result", Timestamp: time.Now()},
		{Role: "user", Content: "Old question 2", Timestamp: time.Now()},
		{Role: "assistant", Content: "Old answer 2", Timestamp: time.Now()},
		{Role: "user", Content: "Latest question", Timestamp: time.Now()},
	}

	// Use small context to force prioritization
	trimmed := TrimMessagesToContextSize(messages, "unknown-model", 3500)

	// Should keep system message
	if len(trimmed) == 0 || trimmed[0].Role != "system" {
		t.Errorf("Should preserve system message")
	}

	// Should keep last message
	if trimmed[len(trimmed)-1].Content != "Latest question" {
		t.Errorf("Should preserve last message")
	}

	// Should prioritize tool call messages
	hasToolCall := false
	hasToolResult := false
	for _, msg := range trimmed {
		if msg.Content == "Calling tool" {
			hasToolCall = true
		}
		if msg.Role == "tool" {
			hasToolResult = true
		}
	}

	if !hasToolCall {
		t.Errorf("Should prioritize message with tool calls")
	}
	if !hasToolResult {
		t.Errorf("Should prioritize tool result messages")
	}
}

func TestTrimMessagesToContextSize_HandlesHugeLastMessage(t *testing.T) {
	// Test case where last message is huge and system must be dropped
	hugeContent := string(make([]byte, 8000)) // ~2000 tokens
	messages := []Message{
		{Role: "system", Content: "System prompt", Timestamp: time.Now()},
		{Role: "user", Content: "First question", Timestamp: time.Now()},
		{Role: "assistant", Content: hugeContent, Timestamp: time.Now()},
	}

	// Small context where last message barely fits
	trimmed := TrimMessagesToContextSize(messages, "unknown-model", 2048)

	// Should keep at least the last message
	if len(trimmed) == 0 {
		t.Errorf("Should keep at least last message")
	}

	// Last message must be preserved
	if trimmed[len(trimmed)-1].Content != hugeContent {
		t.Errorf("Last message must be preserved even if huge")
	}
}

func TestTrimMessagesToContextSize_TakesFirstSystemMessage(t *testing.T) {
	// Test that FIRST system message is taken, not last
	messages := []Message{
		{Role: "system", Content: "First system prompt", Timestamp: time.Now()},
		{Role: "user", Content: "Question", Timestamp: time.Now()},
		{Role: "system", Content: "Second system prompt", Timestamp: time.Now()},
		{Role: "assistant", Content: "Answer", Timestamp: time.Now()},
	}

	trimmed := TrimMessagesToContextSize(messages, "gpt-4", 2048)

	// Should take FIRST system message
	if len(trimmed) == 0 || trimmed[0].Role != "system" {
		t.Errorf("Should have system message first")
	}
	if trimmed[0].Content != "First system prompt" {
		t.Errorf("Should take FIRST system message, got: %s", trimmed[0].Content)
	}
}
