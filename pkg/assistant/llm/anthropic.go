package llm

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/anthropic"
)

// AnthropicProvider implements the Provider interface for Anthropic's Claude API
type AnthropicProvider struct {
	*BaseProvider // Embed base functionality
	client        *anthropic.LLM
	config        Config
	model         string
	apiKey        string
}

// calculateAnthropicCostWrapper is a wrapper for the cost calculation to match the expected signature
func calculateAnthropicCostWrapper(model string, totalTokens int) float64 {
	// For wrapper, we'll estimate input/output tokens as roughly equal
	inputTokens := totalTokens / 2
	outputTokens := totalTokens / 2
	return calculateAnthropicCost(model, inputTokens, outputTokens)
}

// NewAnthropicProvider creates a new Anthropic provider
func NewAnthropicProvider() Provider {
	return &AnthropicProvider{
		BaseProvider: NewBaseProvider("anthropic"), // Use base provider
		model:        "claude-3-5-sonnet-20241022",
	}
}

// Configure configures the Anthropic provider
func (p *AnthropicProvider) Configure(config Config) error {
	if config.APIKey == "" {
		return fmt.Errorf("API key is required for Anthropic")
	}

	// Set default model if not specified
	if config.Model == "" {
		config.Model = "claude-3-5-sonnet-20241022"
	}

	// Create Anthropic client using langchaingo
	client, err := anthropic.New(
		anthropic.WithToken(config.APIKey),
		anthropic.WithModel(config.Model),
	)
	if err != nil {
		return fmt.Errorf("failed to create Anthropic client: %w", err)
	}

	p.client = client
	p.config = config
	p.model = config.Model
	p.apiKey = config.APIKey
	p.SetConfigured(true) // Use base provider method
	return nil
}

// Chat sends a chat request to Anthropic
func (p *AnthropicProvider) Chat(ctx context.Context, messages []Message) (*ChatResponse, error) {
	// Use base validation
	if err := p.ValidateConfiguration(); err != nil {
		return nil, err
	}

	// Validate messages
	if len(messages) == 0 {
		return nil, fmt.Errorf("at least one message is required")
	}

	// Convert messages using base provider helper
	llmMessages := p.ConvertMessagesToLangChainGo(messages)

	// Validate converted messages
	if len(llmMessages) == 0 {
		return nil, fmt.Errorf("no valid messages after conversion (all messages were empty or filtered)")
	}

	// Call Anthropic
	startTime := time.Now()
	response, err := p.client.GenerateContent(ctx, llmMessages,
		llms.WithMaxTokens(p.config.MaxTokens),
		llms.WithTemperature(float64(p.config.Temperature)),
	)
	if err != nil {
		return nil, fmt.Errorf("Anthropic API error: %w", err)
	}

	// Extract response content
	var content string
	if len(response.Choices) > 0 {
		content = response.Choices[0].Content
	}

	// Calculate token usage using base provider helper
	usage := p.CalculateUsageWithCost(
		estimateTokens(messagesToString(messages)),
		estimateTokens(content),
		calculateAnthropicCostWrapper,
		p.model,
	)

	// Build response using base provider helper
	metadata := map[string]string{
		"latency_ms": fmt.Sprintf("%.0f", time.Since(startTime).Seconds()*1000),
	}

	return p.BuildChatResponse(content, p.model, "stop", usage, []ToolCall{}, metadata), nil
}

// ChatWithTools sends a chat request to Anthropic with tool support
func (p *AnthropicProvider) ChatWithTools(ctx context.Context, messages []Message, tools []Tool) (*ChatResponse, error) {
	// Use base validation
	if err := p.ValidateConfiguration(); err != nil {
		return nil, err
	}

	// Validate messages
	if len(messages) == 0 {
		return nil, fmt.Errorf("at least one message is required")
	}

	// Convert messages using base provider helper
	llmMessages := p.ConvertMessagesToLangChainGo(messages)

	// Validate converted messages
	if len(llmMessages) == 0 {
		return nil, fmt.Errorf("no valid messages after conversion (all messages were empty or filtered)")
	}

	// Convert tools using base provider helper
	langchainTools := p.ConvertToolsToLangChainGo(tools)

	// Build options
	options := []llms.CallOption{
		llms.WithMaxTokens(p.config.MaxTokens),
		llms.WithTemperature(float64(p.config.Temperature)),
	}

	// Add tools if provided
	if len(langchainTools) > 0 {
		options = append(options, llms.WithTools(langchainTools))
	}

	// Call Anthropic with tools
	startTime := time.Now()
	response, err := p.client.GenerateContent(ctx, llmMessages, options...)
	if err != nil {
		return nil, fmt.Errorf("Anthropic API error: %w", err)
	}

	// Extract response content
	var content string
	if len(response.Choices) > 0 {
		content = response.Choices[0].Content
	}

	// Calculate token usage using base provider helper
	usage := p.CalculateUsageWithCost(
		estimateTokens(messagesToString(messages)),
		estimateTokens(content),
		calculateAnthropicCostWrapper,
		p.model,
	)

	// Extract tool calls using base provider helper (eliminates 20+ lines of duplication)
	toolCalls := p.ExtractToolCallsFromLangChainResponse(response)

	// Build response using base provider helper
	metadata := map[string]string{
		"latency_ms": fmt.Sprintf("%.0f", time.Since(startTime).Seconds()*1000),
	}

	return p.BuildChatResponse(content, p.model, "stop", usage, toolCalls, metadata), nil
}

// StreamChat streams a chat response from Anthropic
func (p *AnthropicProvider) StreamChat(ctx context.Context, messages []Message, callback StreamCallback) (*ChatResponse, error) {
	// Use base validation
	if err := p.ValidateConfiguration(); err != nil {
		return nil, err
	}

	// Validate messages
	if len(messages) == 0 {
		return nil, fmt.Errorf("at least one message is required")
	}

	// Convert messages using base provider helper
	llmMessages := p.ConvertMessagesToLangChainGo(messages)

	// Validate converted messages
	if len(llmMessages) == 0 {
		return nil, fmt.Errorf("no valid messages after conversion (all messages were empty or filtered)")
	}

	startTime := time.Now()
	var fullContent strings.Builder
	var completionTokens int

	// Use streaming generation
	_, err := p.client.GenerateContent(ctx, llmMessages,
		llms.WithMaxTokens(p.config.MaxTokens),
		llms.WithTemperature(float64(p.config.Temperature)),
		llms.WithStreamingFunc(func(ctx context.Context, chunk []byte) error {
			chunkStr := string(chunk)
			if chunkStr == "" {
				return nil
			}

			// Add to full content and estimate tokens
			fullContent.WriteString(chunkStr)
			completionTokens += estimateTokens(chunkStr)

			// Send chunk to callback
			streamChunk := StreamChunk{
				Content:    fullContent.String(),
				Delta:      chunkStr,
				IsComplete: false,
				Metadata: map[string]string{
					"provider": "anthropic",
				},
				GeneratedAt: time.Now(),
			}

			return callback(streamChunk)
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("Anthropic API error: %w", err)
	}

	// Calculate final token usage using base provider helper
	usage := p.CalculateUsageWithCost(
		estimateTokens(messagesToString(messages)),
		completionTokens,
		calculateAnthropicCostWrapper,
		p.model,
	)

	// Send final chunk
	finalChunk := StreamChunk{
		Content:    fullContent.String(),
		Delta:      "",
		IsComplete: true,
		Usage:      &usage,
		Metadata: map[string]string{
			"provider":   "anthropic",
			"latency_ms": fmt.Sprintf("%.0f", time.Since(startTime).Seconds()*1000),
		},
		GeneratedAt: time.Now(),
	}

	if err := callback(finalChunk); err != nil {
		return nil, fmt.Errorf("callback error: %w", err)
	}

	// Build response using base provider helper
	metadata := map[string]string{
		"latency_ms": fmt.Sprintf("%.0f", time.Since(startTime).Seconds()*1000),
	}

	return p.BuildChatResponse(fullContent.String(), p.model, "stop", usage, []ToolCall{}, metadata), nil
}

// StreamChatWithTools sends messages to Anthropic with tool support and streams the response via callback
// NOTE: langchaingo v0.1.13 has a bug with Anthropic streaming+tools - it fails with
// "invalid delta text field type" when processing tool_use deltas because it expects
// a "text" field that doesn't exist in tool_use events. We use fallback to non-streaming.
// See: https://github.com/tmc/langchaingo/blob/v0.1.13/llms/anthropic/internal/anthropicclient/messages.go#L232-238
func (p *AnthropicProvider) StreamChatWithTools(ctx context.Context, messages []Message, tools []Tool, callback StreamCallback) (*ChatResponse, error) {
	// Use base provider's standardized implementation (fallback to non-streaming with tools)
	return p.DefaultStreamChatWithTools(ctx, messages, tools, callback, p.ChatWithTools, p.StreamChat)
}

// GetCapabilities returns the provider's capabilities
func (p *AnthropicProvider) GetCapabilities() Capabilities {
	return Capabilities{
		Name:              "Anthropic Claude",
		Models:            []string{}, // Models fetched via API
		MaxTokens:         200000,
		SupportsStreaming: true,
		SupportsFunctions: true, // Tool calling now fully supported via langchaingo
		RequiresAuth:      true,
	}
}

// GetModel returns the current model name
func (p *AnthropicProvider) GetModel() string {
	return p.model
}

// IsAvailable checks if the provider is configured and available
func (p *AnthropicProvider) IsAvailable() bool {
	// Use base provider's configuration validation
	return p.ValidateConfiguration() == nil
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
