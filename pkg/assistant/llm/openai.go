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

// OpenAIProvider implements Provider for OpenAI's GPT models
type OpenAIProvider struct {
	*BaseProvider // Embed base functionality
	client        *openai.LLM
	config        Config
	model         string
	apiKey        string
	baseURL       string
}

// NewOpenAIProvider creates a new OpenAI provider
func NewOpenAIProvider() Provider {
	return &OpenAIProvider{
		BaseProvider: NewBaseProvider("openai"), // Use base provider
		model:        "gpt-3.5-turbo",
	}
}

// Configure configures the OpenAI provider
func (p *OpenAIProvider) Configure(config Config) error {
	// Validate required configuration
	if config.APIKey == "" {
		return fmt.Errorf("OpenAI API key is required")
	}

	// Set default model if not specified
	if config.Model == "" {
		config.Model = "gpt-3.5-turbo"
	}

	// Create OpenAI client using base provider helper (maintains consistency)
	llm, err := p.CreateOpenAICompatibleClient(config, "", true) // No custom baseURL for OpenAI
	if err != nil {
		return fmt.Errorf("failed to create OpenAI client: %w", err)
	}

	p.client = llm
	p.config = config
	p.model = config.Model
	p.apiKey = config.APIKey
	p.baseURL = config.BaseURL
	p.SetConfigured(true) // Use base provider method

	return nil
}

// Chat sends messages to OpenAI and returns a response
func (p *OpenAIProvider) Chat(ctx context.Context, messages []Message) (*ChatResponse, error) {
	// Use base validation
	if err := p.ValidateConfiguration(); err != nil {
		return nil, err
	}

	// Convert messages using base provider helper (eliminates 15+ lines of duplication)
	llmMessages := p.ConvertMessagesToLangChainGo(messages)

	// Call OpenAI
	startTime := time.Now()
	response, err := p.client.GenerateContent(ctx, llmMessages,
		llms.WithMaxTokens(p.config.MaxTokens),
		llms.WithTemperature(float64(p.config.Temperature)),
	)
	if err != nil {
		return nil, enhanceOpenAIError(err)
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
		calculateOpenAICost,
		p.model,
	)

	// Build response using base provider helper (eliminates construction duplication)
	metadata := map[string]string{
		"latency_ms": fmt.Sprintf("%.0f", time.Since(startTime).Seconds()*1000),
	}

	return p.BuildChatResponse(content, p.model, "stop", usage, []ToolCall{}, metadata), nil
}

// ChatWithTools sends messages to OpenAI with tool support and returns a response
func (p *OpenAIProvider) ChatWithTools(ctx context.Context, messages []Message, tools []Tool) (*ChatResponse, error) {
	// Use base validation
	if err := p.ValidateConfiguration(); err != nil {
		return nil, err
	}

	// Convert messages using base provider helper
	llmMessages := p.ConvertMessagesToLangChainGo(messages)

	// Convert tools using base provider helper (eliminates duplication)
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

	// Call OpenAI with tools
	startTime := time.Now()
	response, err := p.client.GenerateContent(ctx, llmMessages, options...)
	if err != nil {
		return nil, enhanceOpenAIError(err)
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
		calculateOpenAICost,
		p.model,
	)

	// Extract tool calls from response if any
	var toolCalls []ToolCall
	if len(response.Choices) > 0 && len(response.Choices[0].ToolCalls) > 0 {
		toolCalls = make([]ToolCall, len(response.Choices[0].ToolCalls))
		for i, tc := range response.Choices[0].ToolCalls {
			// Parse function arguments
			var args map[string]interface{}
			if tc.FunctionCall != nil && tc.FunctionCall.Arguments != "" {
				_ = json.Unmarshal([]byte(tc.FunctionCall.Arguments), &args)
			}

			toolCalls[i] = ToolCall{
				ID:   tc.FunctionCall.Name, // Use function name as ID for now
				Type: "function",
				Function: FunctionCall{
					Name:      tc.FunctionCall.Name,
					Arguments: args,
				},
			}
		}
	}

	// Build response using base provider helper
	metadata := map[string]string{
		"latency_ms": fmt.Sprintf("%.0f", time.Since(startTime).Seconds()*1000),
	}

	return p.BuildChatResponse(content, p.model, "stop", usage, toolCalls, metadata), nil
}

// StreamChat sends messages to OpenAI and streams the response via callback
func (p *OpenAIProvider) StreamChat(ctx context.Context, messages []Message, callback StreamCallback) (*ChatResponse, error) {
	// Call StreamChatWithTools with empty tools
	return p.StreamChatWithTools(ctx, messages, []Tool{}, callback)
}

// StreamChatWithTools sends messages to OpenAI with tool support and streams the response via callback
func (p *OpenAIProvider) StreamChatWithTools(ctx context.Context, messages []Message, tools []Tool, callback StreamCallback) (*ChatResponse, error) {
	// Use base validation
	if err := p.ValidateConfiguration(); err != nil {
		return nil, err
	}

	// Convert messages using base provider helper (eliminates 15+ lines of duplication)
	llmMessages := p.ConvertMessagesToLangChainGo(messages)

	startTime := time.Now()
	var fullContent strings.Builder
	var completionTokens int
	toolFilter := NewToolCallFilter() // Provider-agnostic tool call filtering

	// Convert tools using base provider helper (eliminates duplication)
	langchainTools := p.ConvertToolsToLangChainGo(tools)

	// Build options
	options := []llms.CallOption{
		llms.WithMaxTokens(p.config.MaxTokens),
		llms.WithTemperature(float64(p.config.Temperature)),
		llms.WithStreamingFunc(func(ctx context.Context, chunk []byte) error {
			chunkStr := string(chunk)
			if chunkStr == "" {
				return nil
			}

			// Use provider-agnostic tool call filtering
			if toolFilter.ShouldFilterChunk(chunkStr) {
				// Don't add tool call content to fullContent or send to user
				return nil
			}

			// Only process actual text content (not tool calls)
			fullContent.WriteString(chunkStr)
			completionTokens += estimateTokens(chunkStr)

			// Send chunk to callback only if it's actual text content
			streamChunk := StreamChunk{
				Content:    fullContent.String(),
				Delta:      chunkStr,
				IsComplete: false,
				Metadata: map[string]string{
					"provider": "openai",
				},
				GeneratedAt: time.Now(),
			}

			return callback(streamChunk)
		}),
	}

	// Add tools if provided
	if len(langchainTools) > 0 {
		options = append(options, llms.WithTools(langchainTools))
	}

	// Use streaming generation
	response, err := p.client.GenerateContent(ctx, llmMessages, options...)
	if err != nil {
		return nil, enhanceOpenAIError(err)
	}

	// Calculate final token usage
	usage := TokenUsage{
		PromptTokens:     estimateTokens(messagesToString(messages)),
		CompletionTokens: completionTokens,
	}
	usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	usage.Cost = calculateOpenAICost(p.model, usage.TotalTokens)

	// Extract tool calls from response if any
	var toolCalls []ToolCall
	if len(response.Choices) > 0 && len(response.Choices[0].ToolCalls) > 0 {
		toolCalls = make([]ToolCall, len(response.Choices[0].ToolCalls))
		for i, tc := range response.Choices[0].ToolCalls {
			// Parse function arguments
			var args map[string]interface{}
			if tc.FunctionCall != nil && tc.FunctionCall.Arguments != "" {
				_ = json.Unmarshal([]byte(tc.FunctionCall.Arguments), &args)
			}

			toolCalls[i] = ToolCall{
				ID:   tc.ID,
				Type: tc.Type,
				Function: FunctionCall{
					Name:      tc.FunctionCall.Name,
					Arguments: args,
				},
			}
		}
	}

	// Send final chunk
	finalChunk := StreamChunk{
		Content:    fullContent.String(),
		Delta:      "",
		IsComplete: true,
		Usage:      &usage,
		Metadata: map[string]string{
			"provider":   "openai",
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
			"provider":   "openai",
			"latency_ms": fmt.Sprintf("%.0f", time.Since(startTime).Seconds()*1000),
		},
		ToolCalls: toolCalls,
	}, nil
}

// GetCapabilities returns OpenAI capabilities
func (p *OpenAIProvider) GetCapabilities() Capabilities {
	return Capabilities{
		Name:              "OpenAI",
		Models:            []string{}, // Models fetched via API using ListModels()
		MaxTokens:         128000,     // Max for gpt-4-turbo and newer
		SupportsStreaming: true,
		SupportsFunctions: true, // Tool calling support implemented
		CostPerToken:      0.000002,
		RequiresAuth:      true,
	}
}

// GetModel returns the current model
func (p *OpenAIProvider) GetModel() string {
	return p.model
}

// IsAvailable checks if the provider is available
func (p *OpenAIProvider) IsAvailable() bool {
	return p.configured && p.client != nil
}

// ListModels returns available models from OpenAI API
func (p *OpenAIProvider) ListModels(ctx context.Context) ([]string, error) {
	if p.client == nil {
		return nil, fmt.Errorf("provider not configured")
	}

	// Create HTTP request to list models
	baseURL := "https://api.openai.com/v1"
	if p.baseURL != "" {
		baseURL = p.baseURL
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

	// Filter to chat models only
	var chatModels []string
	for _, model := range modelsResp.Data {
		id := model.ID
		// Include GPT models and O1 models
		if strings.HasPrefix(id, "gpt-") || strings.HasPrefix(id, "o1") {
			// Exclude fine-tuned models (contain ':')
			if !strings.Contains(id, ":") {
				chatModels = append(chatModels, id)
			}
		}
	}

	return chatModels, nil
}

// Close cleans up resources
func (p *OpenAIProvider) Close() error {
	// Nothing to clean up for OpenAI client
	return nil
}

// Helper functions

func messagesToString(messages []Message) string {
	var parts []string
	for _, msg := range messages {
		parts = append(parts, msg.Content)
	}
	return strings.Join(parts, " ")
}

// estimateTokens provides a rough estimate of token count
// In a production system, you'd want to use tiktoken or similar
func estimateTokens(text string) int {
	// Rough approximation: 1 token ≈ 4 characters for English text
	return len(text) / 4
}

// calculateOpenAICost calculates approximate cost for OpenAI models
func calculateOpenAICost(model string, tokens int) float64 {
	var costPer1KTokens float64

	switch model {
	case "gpt-4":
		costPer1KTokens = 0.03
	case "gpt-4-turbo-preview":
		costPer1KTokens = 0.01
	case "gpt-3.5-turbo":
		costPer1KTokens = 0.002
	case "gpt-3.5-turbo-16k":
		costPer1KTokens = 0.004
	default:
		costPer1KTokens = 0.002 // Default to gpt-3.5-turbo pricing
	}

	return (float64(tokens) / 1000.0) * costPer1KTokens
}

// enhanceOpenAIError adds more context to OpenAI API errors
func enhanceOpenAIError(err error) error {
	errStr := err.Error()

	// Check for common error patterns
	if strings.Contains(errStr, "401") || strings.Contains(errStr, "Incorrect API key") {
		return fmt.Errorf("OpenAI API error: Invalid API key. Please check your API key at https://platform.openai.com/")
	}
	if strings.Contains(errStr, "402") || strings.Contains(errStr, "billing") {
		return fmt.Errorf("OpenAI API error: Payment required. Please add payment method at https://platform.openai.com/account/billing")
	}
	if strings.Contains(errStr, "429") || strings.Contains(errStr, "rate limit") || strings.Contains(errStr, "quota") {
		return fmt.Errorf("OpenAI API error: Rate limit or quota exceeded. Please check your usage at https://platform.openai.com/account/usage")
	}
	if strings.Contains(errStr, "500") || strings.Contains(errStr, "503") {
		return fmt.Errorf("OpenAI API error: Service temporarily unavailable. Please try again later")
	}

	return fmt.Errorf("OpenAI API error: %w", err)
}

// Register OpenAI provider with global registry
func init() {
	GlobalRegistry.Register("openai", NewOpenAIProvider)
}
