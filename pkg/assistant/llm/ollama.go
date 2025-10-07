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
	client     *openai.LLM
	config     Config
	model      string
	baseURL    string
	configured bool
}

// NewOllamaProvider creates a new Ollama provider
func NewOllamaProvider() Provider {
	return &OllamaProvider{}
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
		config.Model = "llama2"
	}

	// Create Ollama client (using OpenAI client with custom base URL)
	opts := []openai.Option{
		openai.WithBaseURL(baseURL),
		openai.WithModel(config.Model),
	}

	// Ollama doesn't require an API key, but langchaingo requires one
	// Use a dummy key if not provided
	if config.APIKey == "" {
		opts = append(opts, openai.WithToken("ollama"))
	} else {
		opts = append(opts, openai.WithToken(config.APIKey))
	}

	llm, err := openai.New(opts...)
	if err != nil {
		return fmt.Errorf("failed to create Ollama client: %w", err)
	}

	p.client = llm
	p.config = config
	p.model = config.Model
	p.baseURL = baseURL
	p.configured = true

	return nil
}

// Chat sends messages to Ollama and returns a response
func (p *OllamaProvider) Chat(ctx context.Context, messages []Message) (*ChatResponse, error) {
	if !p.configured {
		return nil, fmt.Errorf("Ollama provider not configured")
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

	// Ollama doesn't provide token counts, so estimate
	usage := TokenUsage{
		PromptTokens:     estimateTokens(messagesToString(messages)),
		CompletionTokens: estimateTokens(content),
	}
	usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	usage.Cost = 0.0 // Ollama is free

	return &ChatResponse{
		Content:      content,
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

// StreamChat sends messages to Ollama and streams the response via callback
func (p *OllamaProvider) StreamChat(ctx context.Context, messages []Message, callback StreamCallback) (*ChatResponse, error) {
	if !p.configured {
		return nil, fmt.Errorf("Ollama provider not configured")
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

// GetCapabilities returns Ollama capabilities
func (p *OllamaProvider) GetCapabilities() Capabilities {
	return Capabilities{
		Name:              "Ollama",
		Models:            []string{},
		MaxTokens:         4096,
		SupportsStreaming: true,
		SupportsFunctions: false,
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
	return p.configured && p.client != nil
}

// ListModels returns available models from Ollama API
func (p *OllamaProvider) ListModels(ctx context.Context) ([]string, error) {
	if !p.configured {
		return nil, fmt.Errorf("provider not configured")
	}

	// Ollama has /api/tags endpoint for listing models
	baseURL := p.baseURL
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}

	// Remove /v1 suffix if present for Ollama native API
	baseURL = strings.TrimSuffix(baseURL, "/v1")

	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/api/tags", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

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

	// Parse Ollama response format
	var ollamaResp struct {
		Models []struct {
			Name       string `json:"name"`
			Model      string `json:"model"`
			ModifiedAt string `json:"modified_at"`
			Size       int64  `json:"size"`
		} `json:"models"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
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

// Register Ollama provider with global registry
func init() {
	GlobalRegistry.Register("ollama", NewOllamaProvider)
}
