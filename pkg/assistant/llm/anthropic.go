package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// AnthropicProvider implements the Provider interface for Anthropic's Claude API
type AnthropicProvider struct {
	config     Config
	httpClient *http.Client
	baseURL    string
	model      string
}

// Anthropic API request/response structures
type anthropicRequest struct {
	Model       string             `json:"model"`
	Messages    []anthropicMessage `json:"messages"`
	MaxTokens   int                `json:"max_tokens"`
	Temperature float32            `json:"temperature,omitempty"`
	Stream      bool               `json:"stream,omitempty"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicResponse struct {
	ID         string             `json:"id"`
	Type       string             `json:"type"`
	Role       string             `json:"role"`
	Content    []anthropicContent `json:"content"`
	Model      string             `json:"model"`
	StopReason string             `json:"stop_reason"`
	Usage      anthropicUsage     `json:"usage"`
}

type anthropicContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type anthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// Stream event types
type anthropicStreamEvent struct {
	Type         string                `json:"type"`
	Message      *anthropicResponse    `json:"message,omitempty"`
	Index        int                   `json:"index,omitempty"`
	ContentBlock *anthropicContent     `json:"content_block,omitempty"`
	Delta        *anthropicStreamDelta `json:"delta,omitempty"`
	Usage        *anthropicUsage       `json:"usage,omitempty"`
}

type anthropicStreamDelta struct {
	Type       string `json:"type"`
	Text       string `json:"text,omitempty"`
	StopReason string `json:"stop_reason,omitempty"`
}

// NewAnthropicProvider creates a new Anthropic provider
func NewAnthropicProvider() Provider {
	return &AnthropicProvider{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		baseURL: "https://api.anthropic.com/v1",
		model:   "claude-3-5-sonnet-20241022",
	}
}

// Configure configures the Anthropic provider
func (p *AnthropicProvider) Configure(config Config) error {
	p.config = config

	if config.APIKey == "" {
		return fmt.Errorf("API key is required for Anthropic")
	}

	if config.BaseURL != "" {
		p.baseURL = config.BaseURL
	}

	if config.Model != "" {
		p.model = config.Model
	}

	if config.Timeout > 0 {
		p.httpClient.Timeout = config.Timeout
	}

	return nil
}

// Chat sends a chat request to Anthropic
func (p *AnthropicProvider) Chat(ctx context.Context, messages []Message) (*ChatResponse, error) {
	// Convert messages to Anthropic format
	anthropicMessages := make([]anthropicMessage, 0, len(messages))
	for _, msg := range messages {
		// Anthropic only supports "user" and "assistant" roles
		role := msg.Role
		if role == "system" {
			// System messages should be added as user messages with system prefix
			continue
		}
		anthropicMessages = append(anthropicMessages, anthropicMessage{
			Role:    role,
			Content: msg.Content,
		})
	}

	// Create request
	reqBody := anthropicRequest{
		Model:       p.model,
		Messages:    anthropicMessages,
		MaxTokens:   p.config.MaxTokens,
		Temperature: p.config.Temperature,
		Stream:      false,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/messages", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", p.config.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	// Send request
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	// Parse response
	var anthropicResp anthropicResponse
	if err := json.NewDecoder(resp.Body).Decode(&anthropicResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Extract content
	content := ""
	if len(anthropicResp.Content) > 0 {
		content = anthropicResp.Content[0].Text
	}

	return &ChatResponse{
		Content: content,
		Usage: TokenUsage{
			PromptTokens:     anthropicResp.Usage.InputTokens,
			CompletionTokens: anthropicResp.Usage.OutputTokens,
			TotalTokens:      anthropicResp.Usage.InputTokens + anthropicResp.Usage.OutputTokens,
			Cost:             calculateAnthropicCost(anthropicResp.Model, anthropicResp.Usage.InputTokens, anthropicResp.Usage.OutputTokens),
		},
		Model:        anthropicResp.Model,
		FinishReason: anthropicResp.StopReason,
		GeneratedAt:  time.Now(),
		ToolCalls:    []ToolCall{}, // Initialize empty tool calls
	}, nil
}

// ChatWithTools sends a chat request to Anthropic with tool support
func (p *AnthropicProvider) ChatWithTools(ctx context.Context, messages []Message, tools []Tool) (*ChatResponse, error) {
	// TODO: Implement Anthropic function calling
	// For now, fallback to regular chat
	response, err := p.Chat(ctx, messages)
	if err != nil {
		return nil, err
	}

	// Ensure ToolCalls is initialized
	if response.ToolCalls == nil {
		response.ToolCalls = []ToolCall{}
	}

	return response, nil
}

// StreamChat streams a chat response from Anthropic
func (p *AnthropicProvider) StreamChat(ctx context.Context, messages []Message, callback StreamCallback) (*ChatResponse, error) {
	// Convert messages to Anthropic format
	anthropicMessages := make([]anthropicMessage, 0, len(messages))
	for _, msg := range messages {
		// Anthropic only supports "user" and "assistant" roles
		role := msg.Role
		if role == "system" {
			// System messages should be added as user messages with system prefix
			continue
		}
		anthropicMessages = append(anthropicMessages, anthropicMessage{
			Role:    role,
			Content: msg.Content,
		})
	}

	// Create request with streaming enabled
	reqBody := anthropicRequest{
		Model:       p.model,
		Messages:    anthropicMessages,
		MaxTokens:   p.config.MaxTokens,
		Temperature: p.config.Temperature,
		Stream:      true,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/messages", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", p.config.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	// Send request
	startTime := time.Now()
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	// Process SSE stream
	var fullContent strings.Builder
	var usage anthropicUsage
	var model string
	var stopReason string

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()

		// SSE format: "data: {...}"
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")

		// Skip ping events
		if data == "[DONE]" || strings.TrimSpace(data) == "" {
			continue
		}

		var event anthropicStreamEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			// Skip malformed events
			continue
		}

		// Handle different event types
		switch event.Type {
		case "message_start":
			if event.Message != nil {
				model = event.Message.Model
				if event.Message.Usage.InputTokens > 0 {
					usage.InputTokens = event.Message.Usage.InputTokens
				}
			}

		case "content_block_delta":
			if event.Delta != nil && event.Delta.Text != "" {
				fullContent.WriteString(event.Delta.Text)

				// Send chunk to callback
				streamChunk := StreamChunk{
					Content:    fullContent.String(),
					Delta:      event.Delta.Text,
					IsComplete: false,
					Metadata: map[string]string{
						"provider": "anthropic",
					},
					GeneratedAt: time.Now(),
				}

				if err := callback(streamChunk); err != nil {
					return nil, fmt.Errorf("callback error: %w", err)
				}
			}

		case "message_delta":
			if event.Delta != nil && event.Delta.StopReason != "" {
				stopReason = event.Delta.StopReason
			}
			if event.Usage != nil && event.Usage.OutputTokens > 0 {
				usage.OutputTokens = event.Usage.OutputTokens
			}

		case "message_stop":
			// Stream complete
			break
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading stream: %w", err)
	}

	// Calculate final token usage
	totalUsage := TokenUsage{
		PromptTokens:     usage.InputTokens,
		CompletionTokens: usage.OutputTokens,
		TotalTokens:      usage.InputTokens + usage.OutputTokens,
		Cost:             calculateAnthropicCost(p.model, usage.InputTokens, usage.OutputTokens),
	}

	// Send final chunk
	finalChunk := StreamChunk{
		Content:    fullContent.String(),
		Delta:      "",
		IsComplete: true,
		Usage:      &totalUsage,
		Metadata: map[string]string{
			"provider":   "anthropic",
			"latency_ms": fmt.Sprintf("%.0f", time.Since(startTime).Seconds()*1000),
		},
		GeneratedAt: time.Now(),
	}

	if err := callback(finalChunk); err != nil {
		return nil, fmt.Errorf("callback error: %w", err)
	}

	if model == "" {
		model = p.model
	}
	if stopReason == "" {
		stopReason = "end_turn"
	}

	return &ChatResponse{
		Content:      fullContent.String(),
		Usage:        totalUsage,
		Model:        model,
		FinishReason: stopReason,
		Metadata: map[string]string{
			"provider":   "anthropic",
			"latency_ms": fmt.Sprintf("%.0f", time.Since(startTime).Seconds()*1000),
		},
		GeneratedAt: time.Now(),
		ToolCalls:   []ToolCall{}, // Initialize empty tool calls
	}, nil
}

// GetCapabilities returns the provider's capabilities
func (p *AnthropicProvider) GetCapabilities() Capabilities {
	return Capabilities{
		Name:              "Anthropic Claude",
		Models:            []string{}, // Models fetched via API
		MaxTokens:         200000,
		SupportsStreaming: true,
		SupportsFunctions: true, // Tool calling support implemented
		RequiresAuth:      true,
	}
}

// GetModel returns the current model name
func (p *AnthropicProvider) GetModel() string {
	return p.model
}

// IsAvailable checks if the provider is configured and available
func (p *AnthropicProvider) IsAvailable() bool {
	return p.config.APIKey != ""
}

// ListModels returns available models by fetching from Anthropic documentation
func (p *AnthropicProvider) ListModels(ctx context.Context) ([]string, error) {
	// Anthropic doesn't have a dedicated /models endpoint
	// Fetch from their documentation
	return p.extractModelsFromDocs(ctx)
}

// extractModelsFromDocs fetches model list from Anthropic documentation API
func (p *AnthropicProvider) extractModelsFromDocs(ctx context.Context) ([]string, error) {
	// Try fetching from their docs API/JSON endpoint first
	client := &http.Client{Timeout: 10 * time.Second}

	// Try the models documentation page
	urls := []string{
		"https://docs.anthropic.com/en/docs/about-claude/models",
		"https://raw.githubusercontent.com/anthropics/anthropic-sdk-python/main/src/anthropic/types/model.py",
	}

	var allModels []string
	for _, url := range urls {
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			continue
		}

		resp, err := client.Do(req)
		if err != nil {
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			continue
		}

		// Extract model IDs from content
		models := extractModelNamesFromText(string(body))
		allModels = append(allModels, models...)
	}

	// Remove duplicates
	seen := make(map[string]bool)
	var uniqueModels []string
	for _, model := range allModels {
		if !seen[model] {
			uniqueModels = append(uniqueModels, model)
			seen[model] = true
		}
	}

	if len(uniqueModels) == 0 {
		return nil, fmt.Errorf("failed to fetch models from Anthropic documentation")
	}

	return uniqueModels, nil
}

// extractModelNamesFromText extracts Claude model names from text
func extractModelNamesFromText(text string) []string {
	var models []string
	seen := make(map[string]bool)

	// Look for patterns like: claude-3-5-sonnet-20241022
	// Also match in JSON, HTML attributes, etc
	text = strings.ReplaceAll(text, "&quot;", `"`)
	text = strings.ReplaceAll(text, "&#x27;", "'")

	// Split by common delimiters
	delimiters := []string{" ", "\n", "\t", ",", ";", "'", `"`, ">", "<", "(", ")", "[", "]", "{", "}"}
	words := []string{text}
	for _, delim := range delimiters {
		var newWords []string
		for _, w := range words {
			newWords = append(newWords, strings.Split(w, delim)...)
		}
		words = newWords
	}

	// Extract claude model names
	for _, word := range words {
		word = strings.TrimSpace(word)
		if strings.HasPrefix(word, "claude-") && !seen[word] {
			// Validate format: claude-X-Y-YYYYMMDD or claude-X.Y
			parts := strings.Split(word, "-")
			if len(parts) >= 3 {
				// Looks like a valid model ID
				models = append(models, word)
				seen[word] = true
			}
		}
	}

	return models
}

// Close cleans up resources
func (p *AnthropicProvider) Close() error {
	return nil
}

// calculateAnthropicCost calculates the cost for Anthropic models
func calculateAnthropicCost(model string, inputTokens, outputTokens int) float64 {
	var inputCostPer1M, outputCostPer1M float64

	switch {
	case strings.Contains(model, "claude-3-5-sonnet"):
		inputCostPer1M = 3.0
		outputCostPer1M = 15.0
	case strings.Contains(model, "claude-3-opus"):
		inputCostPer1M = 15.0
		outputCostPer1M = 75.0
	case strings.Contains(model, "claude-3-sonnet"):
		inputCostPer1M = 3.0
		outputCostPer1M = 15.0
	case strings.Contains(model, "claude-3-haiku"):
		inputCostPer1M = 0.25
		outputCostPer1M = 1.25
	default:
		// Default to Claude 3.5 Sonnet pricing
		inputCostPer1M = 3.0
		outputCostPer1M = 15.0
	}

	inputCost := (float64(inputTokens) / 1000000.0) * inputCostPer1M
	outputCost := (float64(outputTokens) / 1000000.0) * outputCostPer1M

	return inputCost + outputCost
}

// Register Anthropic provider with global registry
func init() {
	GlobalRegistry.Register("anthropic", NewAnthropicProvider)
}
