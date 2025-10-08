package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/openai"
)

// DeepSeekProvider implements Provider for DeepSeek (OpenAI-compatible API)
type DeepSeekProvider struct {
	*BaseProvider // Embed base functionality
	client        *openai.LLM
	config        Config
	model         string
	apiKey        string
	baseURL       string
}

// NewDeepSeekProvider creates a new DeepSeek provider
func NewDeepSeekProvider() Provider {
	return &DeepSeekProvider{
		BaseProvider: NewBaseProvider("deepseek"), // Use base provider
		model:        "deepseek-chat",
		baseURL:      "https://api.deepseek.com/v1",
	}
}

// Configure configures the DeepSeek provider
func (p *DeepSeekProvider) Configure(config Config) error {
	// Validate required configuration
	if config.APIKey == "" {
		return fmt.Errorf("DeepSeek API key is required")
	}

	// Set default base URL if not specified
	if config.BaseURL == "" {
		config.BaseURL = "https://api.deepseek.com/v1"
	}

	// Set default model if not specified
	if config.Model == "" {
		config.Model = "deepseek-chat"
	}

	// Create DeepSeek client using base provider helper (eliminates 8+ lines of duplication)
	llm, err := p.CreateOpenAICompatibleClient(config, config.BaseURL, true)
	if err != nil {
		return fmt.Errorf("failed to create DeepSeek client: %w", err)
	}

	p.client = llm
	p.config = config
	p.model = config.Model
	p.apiKey = config.APIKey
	p.baseURL = config.BaseURL
	p.SetConfigured(true) // Use base provider method

	return nil
}

// ChatWithTools sends messages to DeepSeek with tool support and returns a response
func (p *DeepSeekProvider) ChatWithTools(ctx context.Context, messages []Message, tools []Tool) (*ChatResponse, error) {
	// Use base validation
	if err := p.ValidateConfiguration(); err != nil {
		return nil, err
	}

	// Convert messages using base provider helper
	llmMessages := p.ConvertMessagesToLangChainGo(messages)

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

	// Call DeepSeek with tools
	startTime := time.Now()
	response, err := p.client.GenerateContent(ctx, llmMessages, options...)
	if err != nil {
		return nil, enhanceDeepSeekError(err)
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
		calculateDeepSeekCost,
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

// Chat sends messages to DeepSeek and returns a response
func (p *DeepSeekProvider) Chat(ctx context.Context, messages []Message) (*ChatResponse, error) {
	// Use base validation
	if err := p.ValidateConfiguration(); err != nil {
		return nil, err
	}

	// Convert messages using base provider helper (eliminates 15+ lines of duplication)
	llmMessages := p.ConvertMessagesToLangChainGo(messages)

	// Call DeepSeek
	startTime := time.Now()
	response, err := p.client.GenerateContent(ctx, llmMessages,
		llms.WithMaxTokens(p.config.MaxTokens),
		llms.WithTemperature(float64(p.config.Temperature)),
	)
	if err != nil {
		return nil, enhanceDeepSeekError(err)
	}

	// Extract response content
	var content string
	if len(response.Choices) > 0 {
		content = response.Choices[0].Content
	}

	// Calculate token usage using base provider helper (eliminates calculation duplication)
	usage := p.CalculateUsageWithCost(
		estimateTokens(messagesToString(messages)),
		estimateTokens(content),
		calculateDeepSeekCost,
		p.model,
	)

	// Build response using base provider helper (eliminates construction duplication)
	metadata := map[string]string{
		"latency_ms": fmt.Sprintf("%.0f", time.Since(startTime).Seconds()*1000),
	}

	return p.BuildChatResponse(content, p.model, "stop", usage, []ToolCall{}, metadata), nil
}

// StreamChat sends messages to DeepSeek and streams the response via callback
func (p *DeepSeekProvider) StreamChat(ctx context.Context, messages []Message, callback StreamCallback) (*ChatResponse, error) {
	// Use base validation
	if err := p.ValidateConfiguration(); err != nil {
		return nil, err
	}

	// Convert messages using base provider helper (eliminates 15+ lines of duplication)
	llmMessages := p.ConvertMessagesToLangChainGo(messages)

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

			fullContent.WriteString(chunkStr)
			completionTokens += estimateTokens(chunkStr)

			// Send chunk to callback
			streamChunk := StreamChunk{
				Content:    fullContent.String(),
				Delta:      chunkStr,
				IsComplete: false,
				Metadata: map[string]string{
					"provider": "deepseek",
				},
				GeneratedAt: time.Now(),
			}

			return callback(streamChunk)
		}),
	)
	if err != nil {
		return nil, enhanceDeepSeekError(err)
	}

	// Calculate final token usage
	usage := TokenUsage{
		PromptTokens:     estimateTokens(messagesToString(messages)),
		CompletionTokens: completionTokens,
	}
	usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	usage.Cost = calculateDeepSeekCost(p.model, usage.TotalTokens)

	// Send final chunk
	finalChunk := StreamChunk{
		Content:    fullContent.String(),
		Delta:      "",
		IsComplete: true,
		Usage:      &usage,
		Metadata: map[string]string{
			"provider":   "deepseek",
			"latency_ms": fmt.Sprintf("%.0f", time.Since(startTime).Seconds()*1000),
		},
		GeneratedAt: time.Now(),
	}

	if callbackErr := callback(finalChunk); callbackErr != nil {
		return nil, fmt.Errorf("callback error: %w", callbackErr)
	}

	return &ChatResponse{
		Content:      fullContent.String(),
		Usage:        usage,
		Model:        p.model,
		FinishReason: "stop",
		Metadata: map[string]string{
			"provider":   "deepseek",
			"latency_ms": fmt.Sprintf("%.0f", time.Since(startTime).Seconds()*1000),
		},
		GeneratedAt: time.Now(),
	}, nil
}

// StreamChatWithTools sends messages to DeepSeek with tool support and streams the response via callback
func (p *DeepSeekProvider) StreamChatWithTools(ctx context.Context, messages []Message, tools []Tool, callback StreamCallback) (*ChatResponse, error) {
	// Use base provider's standardized implementation (eliminates duplicate pattern)
	return p.DefaultStreamChatWithTools(ctx, messages, tools, callback, p.ChatWithTools, p.StreamChat)
}

// GetCapabilities returns DeepSeek capabilities
func (p *DeepSeekProvider) GetCapabilities() Capabilities {
	return Capabilities{
		Name:              "DeepSeek",
		Models:            []string{},
		MaxTokens:         4096,
		SupportsStreaming: true,
		SupportsFunctions: true,      // Tool calling now fully supported ⭐
		CostPerToken:      0.0000014, // $0.14 per 1M tokens
		RequiresAuth:      true,
	}
}

// GetModel returns the current model
func (p *DeepSeekProvider) GetModel() string {
	return p.model
}

// IsAvailable checks if the provider is available
func (p *DeepSeekProvider) IsAvailable() bool {
	// Use base provider's configuration validation
	return p.ValidateConfiguration() == nil && p.client != nil
}

// ListModels returns available models from DeepSeek API
func (p *DeepSeekProvider) ListModels(ctx context.Context) ([]string, error) {
	// Use base validation
	if err := p.ValidateConfiguration(); err != nil {
		return nil, err
	}

	// Create HTTP request to list models
	baseURL := p.baseURL
	if baseURL == "" {
		baseURL = "https://api.deepseek.com/v1"
	}

	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/models", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch models: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error: status %d", resp.StatusCode)
	}

	// Parse response
	var modelsResp struct {
		Data []struct {
			ID      string `json:"id"`
			Object  string `json:"object"`
			Created int64  `json:"created"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&modelsResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Extract model IDs
	var models []string
	for _, model := range modelsResp.Data {
		models = append(models, model.ID)
	}

	return models, nil
}

// Close cleans up resources
func (p *DeepSeekProvider) Close() error {
	return nil
}

// calculateDeepSeekCost calculates approximate cost for DeepSeek models
func calculateDeepSeekCost(model string, tokens int) float64 {
	// DeepSeek pricing: $0.14 per 1M tokens for deepseek-chat
	// $0.28 per 1M tokens for deepseek-coder
	var costPer1MTokens float64

	if strings.Contains(model, "coder") {
		costPer1MTokens = 0.28
	} else {
		costPer1MTokens = 0.14
	}

	return (float64(tokens) / 1000000.0) * costPer1MTokens
}

// enhanceDeepSeekError adds more context to DeepSeek API errors
func enhanceDeepSeekError(err error) error {
	errStr := err.Error()

	// Check for common error patterns
	if strings.Contains(errStr, "402") || strings.Contains(errStr, "Insufficient Balance") {
		return fmt.Errorf("DeepSeek API error: Insufficient balance. Please add credits to your DeepSeek account at https://platform.deepseek.com/")
	}
	if strings.Contains(errStr, "401") || strings.Contains(errStr, "Unauthorized") {
		return fmt.Errorf("DeepSeek API error: Invalid API key. Please check your API key")
	}
	if strings.Contains(errStr, "429") || strings.Contains(errStr, "rate limit") {
		return fmt.Errorf("DeepSeek API error: Rate limit exceeded. Please wait a moment and try again")
	}
	if strings.Contains(errStr, "500") || strings.Contains(errStr, "503") {
		return fmt.Errorf("DeepSeek API error: Service temporarily unavailable. Please try again later")
	}

	return fmt.Errorf("DeepSeek API error: %w", err)
}

// Register DeepSeek provider with global registry
func init() {
	GlobalRegistry.Register("deepseek", NewDeepSeekProvider)
}
