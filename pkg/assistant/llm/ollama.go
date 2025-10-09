package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/openai"
)

// OllamaProvider implements Provider for Ollama (OpenAI-compatible API)
type OllamaProvider struct {
	*BaseProvider // Embed base functionality
	client        *openai.LLM
	config        Config
	model         string
	baseURL       string
}

// NewOllamaProvider creates a new Ollama provider
func NewOllamaProvider() Provider {
	return &OllamaProvider{
		BaseProvider: NewBaseProvider("ollama"), // Use base provider
		model:        "llama3.2", // Default to llama3.2 which supports tool calling
		baseURL:      "http://localhost:11434",
	}
}

// Configure configures the Ollama provider
func (p *OllamaProvider) Configure(config Config) error {
	// Set default base URL if not specified
	baseURL := config.BaseURL
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}

	// Ensure /v1 suffix for OpenAI-compatible API
	if !strings.HasSuffix(baseURL, "/v1") {
		baseURL = baseURL + "/v1"
	}

	// Set default model if not specified
	if config.Model == "" {
		config.Model = "llama3.2" // Default to llama3.2 which supports tool calling
	}

	// Create Ollama client using base provider helper (eliminates 15+ lines of duplication)
	llm, err := p.CreateOpenAICompatibleClient(config, baseURL, false) // Ollama doesn't require API key
	if err != nil {
		return fmt.Errorf("failed to create Ollama client: %w", err)
	}

	p.client = llm
	p.config = config
	p.model = config.Model
	p.baseURL = baseURL
	p.SetConfigured(true) // Use base provider method

	return nil
}

// ChatWithTools sends messages to Ollama with tool support and returns a response
func (p *OllamaProvider) ChatWithTools(ctx context.Context, messages []Message, tools []Tool) (*ChatResponse, error) {
	// Use base validation
	if err := p.ValidateConfiguration(); err != nil {
		return nil, err
	}

	// Check if model supports tool calling
	if !supportsToolCalling(p.model) {
		fmt.Printf("\n⚠️  Warning: Model %s may not support tool calling. Consider using llama3.1, llama3.2, mistral-nemo, or qwen2.5\n", p.model)
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

	// Call Ollama with tools
	startTime := time.Now()
	response, err := p.client.GenerateContent(ctx, llmMessages, options...)
	if err != nil {
		return nil, fmt.Errorf("Ollama API error: %w", err)
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
		nil, // Ollama is free, so no cost calculator
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

// Chat sends messages to Ollama and returns a response
func (p *OllamaProvider) Chat(ctx context.Context, messages []Message) (*ChatResponse, error) {
	// Use base validation
	if err := p.ValidateConfiguration(); err != nil {
		return nil, err
	}

	// Convert messages using base provider helper (eliminates 15+ lines of duplication)
	llmMessages := p.ConvertMessagesToLangChainGo(messages)

	// Call Ollama
	startTime := time.Now()
	response, err := p.client.GenerateContent(ctx, llmMessages,
		llms.WithMaxTokens(p.config.MaxTokens),
		llms.WithTemperature(float64(p.config.Temperature)),
	)
	if err != nil {
		return nil, fmt.Errorf("Ollama API error: %w", err)
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
		nil, // Ollama is free, so no cost calculator
		p.model,
	)

	// Build response using base provider helper (eliminates construction duplication)
	metadata := map[string]string{
		"latency_ms": fmt.Sprintf("%.0f", time.Since(startTime).Seconds()*1000),
	}

	return p.BuildChatResponse(content, p.model, "stop", usage, []ToolCall{}, metadata), nil
}

// StreamChat sends messages to Ollama and streams the response via callback
func (p *OllamaProvider) StreamChat(ctx context.Context, messages []Message, callback StreamCallback) (*ChatResponse, error) {
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
					"provider": "ollama",
				},
				GeneratedAt: time.Now(),
			}

			return callback(streamChunk)
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("Ollama streaming API error: %w", err)
	}

	// Calculate final token usage
	usage := TokenUsage{
		PromptTokens:     estimateTokens(messagesToString(messages)),
		CompletionTokens: completionTokens,
	}
	usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	usage.Cost = 0.0 // Ollama is free

	// Send final chunk
	finalChunk := StreamChunk{
		Content:    fullContent.String(),
		Delta:      "",
		IsComplete: true,
		Usage:      &usage,
		Metadata: map[string]string{
			"provider":   "ollama",
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
			"provider":   "ollama",
			"latency_ms": fmt.Sprintf("%.0f", time.Since(startTime).Seconds()*1000),
		},
		GeneratedAt: time.Now(),
	}, nil
}

// StreamChatWithTools sends messages to Ollama with tool support and streams the response via callback
// NOTE: For reliability, we use fallback to non-streaming when tools are present.
// This ensures tool calls are properly extracted and processed.
func (p *OllamaProvider) StreamChatWithTools(ctx context.Context, messages []Message, tools []Tool, callback StreamCallback) (*ChatResponse, error) {
	// Use base provider's standardized implementation (fallback to non-streaming with tools)
	return p.DefaultStreamChatWithTools(ctx, messages, tools, callback, p.ChatWithTools, p.StreamChat)
}

// GetCapabilities returns Ollama capabilities
func (p *OllamaProvider) GetCapabilities() Capabilities {
	return Capabilities{
		Name:              "Ollama",
		Models:            []string{},
		MaxTokens:         4096,
		SupportsStreaming: true,
		SupportsFunctions: true, // Tool calling now fully supported (model-dependent) ⭐
		CostPerToken:      0.0,
		RequiresAuth:      false,
	}
}

// GetModel returns the current model
func (p *OllamaProvider) GetModel() string {
	return p.model
}

// IsAvailable checks if the provider is available
func (p *OllamaProvider) IsAvailable() bool {
	// Use base provider's configuration validation
	return p.ValidateConfiguration() == nil && p.client != nil
}

// ListModels returns available models from Ollama API
func (p *OllamaProvider) ListModels(ctx context.Context) ([]string, error) {
	// Ollama has /api/tags endpoint for listing models
	baseURL := p.baseURL
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}

	// Remove /v1 suffix if present for Ollama native API
	baseURL = strings.TrimSuffix(baseURL, "/v1")

	url := baseURL + "/api/tags"

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch models (is Ollama running?): %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	// Parse Ollama response format
	var ollamaResp struct {
		Models []struct {
			Name       string `json:"name"`
			Model      string `json:"model"`
			ModifiedAt string `json:"modified_at"`
			Size       int64  `json:"size"`
		} `json:"models"`
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if err := json.Unmarshal(body, &ollamaResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Extract model names
	var models []string
	for _, model := range ollamaResp.Models {
		if model.Name != "" {
			models = append(models, model.Name)
		}
	}

	if len(models) == 0 {
		return nil, fmt.Errorf("no models found on Ollama server")
	}

	return models, nil
}

// Close cleans up resources
func (p *OllamaProvider) Close() error {
	return nil
}

// supportsToolCalling checks if an Ollama model supports tool/function calling
func supportsToolCalling(model string) bool {
	// Models known to support tool calling
	toolSupportedModels := []string{
		"llama3.1", "llama3.2", // Llama 3.1 and 3.2 series
		"mistral-nemo", "mistral-large", // Mistral series
		"qwen2.5", // Qwen series
		"command-r", "command-r-plus", // Cohere Command R series
		"firefunction", // FireFunction series
	}

	modelLower := strings.ToLower(model)
	for _, supported := range toolSupportedModels {
		if strings.Contains(modelLower, supported) {
			return true
		}
	}
	return false
}

// Register Ollama provider with global registry
func init() {
	GlobalRegistry.Register("ollama", NewOllamaProvider)
}
