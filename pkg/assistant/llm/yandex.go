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

// YandexProvider implements Provider for Yandex ChatGPT (OpenAI-compatible API)
type YandexProvider struct {
	client     *openai.LLM
	config     Config
	model      string
	apiKey     string
	baseURL    string
	configured bool
}

// NewYandexProvider creates a new Yandex provider
func NewYandexProvider() Provider {
	return &YandexProvider{}
}

// Configure configures the Yandex provider
func (p *YandexProvider) Configure(config Config) error {
	// Validate required configuration
	if config.APIKey == "" {
		return fmt.Errorf("Yandex API key is required")
	}

	// Set default base URL if not specified
	if config.BaseURL == "" {
		config.BaseURL = "https://llm.api.cloud.yandex.net/foundationModels/v1"
	}

	// Set default model if not specified
	if config.Model == "" {
		config.Model = "yandexgpt/latest"
	}

	// Create Yandex client (using OpenAI client with custom base URL)
	llm, err := openai.New(
		openai.WithToken(config.APIKey),
		openai.WithModel(config.Model),
		openai.WithBaseURL(config.BaseURL),
	)
	if err != nil {
		return fmt.Errorf("failed to create Yandex client: %w", err)
	}

	p.client = llm
	p.config = config
	p.model = config.Model
	p.apiKey = config.APIKey
	p.baseURL = config.BaseURL
	p.configured = true

	return nil
}

// ChatWithTools sends messages to Yandex with tools (not supported)
func (p *YandexProvider) ChatWithTools(ctx context.Context, messages []Message, tools []Tool) (*ChatResponse, error) {
	return nil, fmt.Errorf("Yandex provider does not support function/tool calling")
}

// Chat sends messages to Yandex and returns a response
func (p *YandexProvider) Chat(ctx context.Context, messages []Message) (*ChatResponse, error) {
	if !p.configured {
		return nil, fmt.Errorf("Yandex provider not configured")
	}

	// Convert messages to langchaingo format
	llmMessages := make([]llms.MessageContent, 0, len(messages))

	for _, msg := range messages {
		var msgType llms.ChatMessageType
		switch strings.ToLower(msg.Role) {
		case "user":
			msgType = llms.ChatMessageTypeHuman
		case "assistant":
			msgType = llms.ChatMessageTypeAI
		case "system":
			msgType = llms.ChatMessageTypeSystem
		default:
			msgType = llms.ChatMessageTypeHuman
		}

		llmMessages = append(llmMessages, llms.TextParts(msgType, msg.Content))
	}

	// Call Yandex
	startTime := time.Now()
	response, err := p.client.GenerateContent(ctx, llmMessages,
		llms.WithMaxTokens(p.config.MaxTokens),
		llms.WithTemperature(float64(p.config.Temperature)),
	)
	if err != nil {
		return nil, fmt.Errorf("Yandex API error: %w", err)
	}

	// Extract response content
	var content string
	if len(response.Choices) > 0 {
		content = response.Choices[0].Content
	}

	// Calculate token usage
	usage := TokenUsage{
		PromptTokens:     estimateTokens(messagesToString(messages)),
		CompletionTokens: estimateTokens(content),
	}
	usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	usage.Cost = calculateYandexCost(p.model, usage.TotalTokens)

	return &ChatResponse{
		Content:      content,
		Usage:        usage,
		Model:        p.model,
		FinishReason: "stop",
		Metadata: map[string]string{
			"provider":   "yandex",
			"latency_ms": fmt.Sprintf("%.0f", time.Since(startTime).Seconds()*1000),
		},
		GeneratedAt: time.Now(),
	}, nil
}

// StreamChat sends messages to Yandex and streams the response via callback
func (p *YandexProvider) StreamChat(ctx context.Context, messages []Message, callback StreamCallback) (*ChatResponse, error) {
	if !p.configured {
		return nil, fmt.Errorf("Yandex provider not configured")
	}

	// Convert messages to langchaingo format
	llmMessages := make([]llms.MessageContent, 0, len(messages))

	for _, msg := range messages {
		var msgType llms.ChatMessageType
		switch strings.ToLower(msg.Role) {
		case "user":
			msgType = llms.ChatMessageTypeHuman
		case "assistant":
			msgType = llms.ChatMessageTypeAI
		case "system":
			msgType = llms.ChatMessageTypeSystem
		default:
			msgType = llms.ChatMessageTypeHuman
		}

		llmMessages = append(llmMessages, llms.TextParts(msgType, msg.Content))
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

			fullContent.WriteString(chunkStr)
			completionTokens += estimateTokens(chunkStr)

			// Send chunk to callback
			streamChunk := StreamChunk{
				Content:    fullContent.String(),
				Delta:      chunkStr,
				IsComplete: false,
				Metadata: map[string]string{
					"provider": "yandex",
				},
				GeneratedAt: time.Now(),
			}

			return callback(streamChunk)
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("Yandex streaming API error: %w", err)
	}

	// Calculate final token usage
	usage := TokenUsage{
		PromptTokens:     estimateTokens(messagesToString(messages)),
		CompletionTokens: completionTokens,
	}
	usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	usage.Cost = calculateYandexCost(p.model, usage.TotalTokens)

	// Send final chunk
	finalChunk := StreamChunk{
		Content:    fullContent.String(),
		Delta:      "",
		IsComplete: true,
		Usage:      &usage,
		Metadata: map[string]string{
			"provider":   "yandex",
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
			"provider":   "yandex",
			"latency_ms": fmt.Sprintf("%.0f", time.Since(startTime).Seconds()*1000),
		},
		GeneratedAt: time.Now(),
	}, nil
}

// StreamChatWithTools sends messages to Yandex with tool support and streams the response via callback
func (p *YandexProvider) StreamChatWithTools(ctx context.Context, messages []Message, tools []Tool, callback StreamCallback) (*ChatResponse, error) {
	// TODO: Implement proper tool support for Yandex
	// For now, fallback to regular streaming (Yandex may not support tools yet)
	if len(tools) > 0 {
		// Use non-streaming with tools as fallback
		response, err := p.ChatWithTools(ctx, messages, tools)
		if err != nil {
			return nil, err
		}

		// Simulate streaming by sending the full response as one chunk
		finalChunk := StreamChunk{
			Content:    response.Content,
			Delta:      response.Content,
			IsComplete: true,
			Usage:      &response.Usage,
			Metadata: map[string]string{
				"provider": "yandex",
			},
			GeneratedAt: time.Now(),
		}

		if err := callback(finalChunk); err != nil {
			return nil, fmt.Errorf("callback error: %w", err)
		}

		return response, nil
	}

	// No tools, use regular streaming
	return p.StreamChat(ctx, messages, callback)
}

// GetCapabilities returns Yandex capabilities
func (p *YandexProvider) GetCapabilities() Capabilities {
	return Capabilities{
		Name:              "Yandex ChatGPT",
		Models:            []string{},
		MaxTokens:         8000,
		SupportsStreaming: true,
		SupportsFunctions: false,
		CostPerToken:      0.0000012, // Approximate pricing
		RequiresAuth:      true,
	}
}

// GetModel returns the current model
func (p *YandexProvider) GetModel() string {
	return p.model
}

// IsAvailable checks if the provider is available
func (p *YandexProvider) IsAvailable() bool {
	return p.configured && p.client != nil
}

// ListModels returns available models from Yandex API
func (p *YandexProvider) ListModels(ctx context.Context) ([]string, error) {
	if !p.configured {
		return nil, fmt.Errorf("provider not configured")
	}

	// Try to fetch from Yandex Foundation Models API
	baseURL := p.baseURL
	if baseURL == "" {
		baseURL = "https://llm.api.cloud.yandex.net/foundationModels/v1"
	}

	// Yandex uses /models endpoint
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
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	// Parse response
	var modelsResp struct {
		Models []struct {
			Name        string `json:"name"`
			URI         string `json:"uri"`
			Description string `json:"description"`
		} `json:"models"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&modelsResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Extract model names/URIs
	var models []string
	for _, model := range modelsResp.Models {
		if model.URI != "" {
			models = append(models, model.URI)
		} else if model.Name != "" {
			models = append(models, model.Name)
		}
	}

	if len(models) == 0 {
		return nil, fmt.Errorf("no models found in Yandex API response")
	}

	return models, nil
}

// Close cleans up resources
func (p *YandexProvider) Close() error {
	return nil
}

// calculateYandexCost calculates approximate cost for Yandex models
func calculateYandexCost(model string, tokens int) float64 {
	// Yandex pricing varies by model
	// YandexGPT: ~$1.20 per 1M tokens
	// YandexGPT Lite: ~$0.60 per 1M tokens
	var costPer1MTokens float64

	if strings.Contains(model, "lite") {
		costPer1MTokens = 0.60
	} else {
		costPer1MTokens = 1.20
	}

	return (float64(tokens) / 1000000.0) * costPer1MTokens
}

// Register Yandex provider with global registry
func init() {
	GlobalRegistry.Register("yandex", NewYandexProvider)
}
