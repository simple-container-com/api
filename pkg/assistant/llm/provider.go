package llm

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/openai"
)

// Message represents a single message in a conversation
type Message struct {
	Role      string                 `json:"role"`      // "user", "assistant", "system"
	Content   string                 `json:"content"`   // Message content
	Timestamp time.Time              `json:"timestamp"` // When message was created
	Metadata  map[string]interface{} `json:"metadata"`  // Additional context
}

// StreamCallback is called for each chunk of streaming response
type StreamCallback func(chunk StreamChunk) error

// StreamChunk represents a chunk of streaming response
type StreamChunk struct {
	Content     string            `json:"content"`      // Partial content
	Delta       string            `json:"delta"`        // New content since last chunk
	IsComplete  bool              `json:"is_complete"`  // Whether this is the final chunk
	Usage       *TokenUsage       `json:"usage"`        // Token usage (only on final chunk)
	Metadata    map[string]string `json:"metadata"`     // Additional metadata
	GeneratedAt time.Time         `json:"generated_at"` // When chunk was generated
}

// Provider defines the interface for LLM providers
type Provider interface {
	// Chat sends messages to the LLM and returns a response
	Chat(ctx context.Context, messages []Message) (*ChatResponse, error)

	// ChatWithTools sends messages to the LLM with available tools and returns a response
	ChatWithTools(ctx context.Context, messages []Message, tools []Tool) (*ChatResponse, error)

	// StreamChat sends messages to the LLM and streams the response via callback
	StreamChat(ctx context.Context, messages []Message, callback StreamCallback) (*ChatResponse, error)

	// StreamChatWithTools sends messages to the LLM with available tools and streams the response via callback
	StreamChatWithTools(ctx context.Context, messages []Message, tools []Tool, callback StreamCallback) (*ChatResponse, error)

	// GetCapabilities returns the provider's capabilities
	GetCapabilities() Capabilities

	// Configure configures the provider with given settings
	Configure(config Config) error

	// GetModel returns the model name being used
	GetModel() string

	// ListModels returns available models from the provider (via API if possible)
	ListModels(ctx context.Context) ([]string, error)

	// IsAvailable checks if the provider is available and configured
	IsAvailable() bool

	// Close cleans up resources
	Close() error
}

// ChatResponse represents a response from the LLM
type ChatResponse struct {
	Content      string            `json:"content"`       // Response content
	Usage        TokenUsage        `json:"usage"`         // Token usage information
	Model        string            `json:"model"`         // Model used
	FinishReason string            `json:"finish_reason"` // Why the response ended
	Metadata     map[string]string `json:"metadata"`      // Additional metadata
	GeneratedAt  time.Time         `json:"generated_at"`  // When response was generated
	ToolCalls    []ToolCall        `json:"tool_calls"`    // Tool/function calls requested by LLM
}

// ToolCall represents a function/tool call requested by the LLM
type ToolCall struct {
	ID       string       `json:"id"`       // Unique ID for this tool call
	Type     string       `json:"type"`     // Type of call (usually "function")
	Function FunctionCall `json:"function"` // Function call details
}

// FunctionCall represents the function details in a tool call
type FunctionCall struct {
	Name      string                 `json:"name"`      // Function name
	Arguments map[string]interface{} `json:"arguments"` // Function arguments
}

// Tool defines a tool/function that can be called by the LLM
type Tool struct {
	Type     string      `json:"type"`     // Tool type (usually "function")
	Function FunctionDef `json:"function"` // Function definition
}

// FunctionDef defines a function that can be called by the LLM
type FunctionDef struct {
	Name        string                 `json:"name"`        // Function name
	Description string                 `json:"description"` // Function description
	Parameters  map[string]interface{} `json:"parameters"`  // JSON schema for parameters
}

// TokenUsage tracks token consumption
type TokenUsage struct {
	PromptTokens     int     `json:"prompt_tokens"`     // Tokens in the prompt
	CompletionTokens int     `json:"completion_tokens"` // Tokens in the completion
	TotalTokens      int     `json:"total_tokens"`      // Total tokens used
	Cost             float64 `json:"cost"`              // Estimated cost (if available)
}

// ToolCallFilter helps providers filter out tool call JSON from streaming content
// This provides a provider-agnostic way to handle the common issue where LLMs
// stream raw tool call JSON that should not be displayed to users.
type ToolCallFilter struct {
	inToolCall bool // Track if we're currently inside a tool call JSON
}

// NewToolCallFilter creates a new tool call filter for streaming content
func NewToolCallFilter() *ToolCallFilter {
	return &ToolCallFilter{inToolCall: false}
}

// ShouldFilterChunk determines if a streaming chunk should be filtered out
// because it's part of tool call JSON that shouldn't be shown to users.
// Returns true if the chunk should be filtered (not shown), false otherwise.
func (f *ToolCallFilter) ShouldFilterChunk(chunkStr string) bool {
	// Check for tool call start markers
	if strings.Contains(chunkStr, `[{"`) || strings.Contains(chunkStr, `"tool_calls"`) {
		f.inToolCall = true
	}

	// If we're in a tool call, check for end markers
	if f.inToolCall {
		if strings.Contains(chunkStr, `}]`) || strings.Contains(chunkStr, `"finish_reason"`) {
			f.inToolCall = false
			// Still filter this closing chunk
			return true
		}
		return true // Filter all chunks while in tool call
	}

	return false // Don't filter regular content
}

// Reset resets the filter state (useful when starting a new request)
func (f *ToolCallFilter) Reset() {
	f.inToolCall = false
}

// BaseProvider provides common functionality for all LLM providers
// This reduces code duplication and provides consistent behavior
type BaseProvider struct {
	name       string
	configured bool
	toolFilter *ToolCallFilter
}

// NewBaseProvider creates a new base provider
func NewBaseProvider(name string) *BaseProvider {
	return &BaseProvider{
		name:       name,
		configured: false,
		toolFilter: NewToolCallFilter(),
	}
}

// ValidateConfiguration checks if the provider is properly configured
func (b *BaseProvider) ValidateConfiguration() error {
	if !b.configured {
		return fmt.Errorf("%s provider not configured", b.name)
	}
	return nil
}

// ConvertMessages converts our Message format to a generic format
// Providers can override this if they need specific conversion logic
func (b *BaseProvider) ConvertMessages(messages []Message) []map[string]interface{} {
	converted := make([]map[string]interface{}, len(messages))
	for i, msg := range messages {
		converted[i] = map[string]interface{}{
			"role":    msg.Role,
			"content": msg.Content,
		}
	}
	return converted
}

// ConvertTools converts our Tool format to a generic format
// Providers can override this if they need specific conversion logic
func (b *BaseProvider) ConvertTools(tools []Tool) []map[string]interface{} {
	if len(tools) == 0 {
		return nil
	}

	converted := make([]map[string]interface{}, len(tools))
	for i, tool := range tools {
		converted[i] = map[string]interface{}{
			"type": tool.Type,
			"function": map[string]interface{}{
				"name":        tool.Function.Name,
				"description": tool.Function.Description,
				"parameters":  tool.Function.Parameters,
			},
		}
	}
	return converted
}

// CreateStreamingCallback creates a streaming callback with tool call filtering
func (b *BaseProvider) CreateStreamingCallback(callback StreamCallback, fullContent *strings.Builder) func(string) error {
	return func(chunkStr string) error {
		if chunkStr == "" {
			return nil
		}

		// Use provider-agnostic tool call filtering
		if b.toolFilter.ShouldFilterChunk(chunkStr) {
			// Don't add tool call content to fullContent or send to user
			return nil
		}

		// Only process actual text content (not tool calls)
		fullContent.WriteString(chunkStr)

		// Send chunk to callback only if it's actual text content
		streamChunk := StreamChunk{
			Content:    fullContent.String(),
			Delta:      chunkStr,
			IsComplete: false,
			Metadata: map[string]string{
				"provider": b.name,
			},
			GeneratedAt: time.Now(),
		}

		return callback(streamChunk)
	}
}

// FallbackToNonStreaming provides a standard fallback for providers that don't support streaming with tools
func (b *BaseProvider) FallbackToNonStreaming(ctx context.Context, messages []Message, tools []Tool, callback StreamCallback, chatWithTools func(context.Context, []Message, []Tool) (*ChatResponse, error)) (*ChatResponse, error) {
	response, err := chatWithTools(ctx, messages, tools)
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
			"provider": b.name,
		},
		GeneratedAt: time.Now(),
	}

	if err := callback(finalChunk); err != nil {
		return nil, fmt.Errorf("callback error: %w", err)
	}

	return response, nil
}

// EstimateTokens provides a basic token estimation
// Providers can override with more accurate estimation
func (b *BaseProvider) EstimateTokens(text string) int {
	// Rough estimation: ~4 characters per token for most models
	return len(text) / 4
}

// SetConfigured marks the provider as configured
func (b *BaseProvider) SetConfigured(configured bool) {
	b.configured = configured
}

// LangChainGo Helper Methods - Eliminates duplication across OpenAI-compatible providers

// ConvertMessagesToLangChainGo converts our Message format to langchaingo MessageContent
// This eliminates the 15+ line duplication across OpenAI, DeepSeek, Ollama, Yandex
func (b *BaseProvider) ConvertMessagesToLangChainGo(messages []Message) []llms.MessageContent {
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

	return llmMessages
}

// CreateOpenAICompatibleClient creates an OpenAI-compatible client with standard options
// This eliminates duplication across DeepSeek, Ollama, Yandex providers
func (b *BaseProvider) CreateOpenAICompatibleClient(config Config, baseURL string, requiresAPIKey bool) (*openai.LLM, error) {
	opts := []openai.Option{
		openai.WithModel(config.Model),
	}

	// Add base URL if specified (for providers like DeepSeek, Yandex)
	if baseURL != "" {
		opts = append(opts, openai.WithBaseURL(baseURL))
	}

	// Handle API key requirements (Ollama doesn't require real keys)
	if requiresAPIKey {
		if config.APIKey == "" {
			return nil, fmt.Errorf("API key is required for %s", b.name)
		}
		opts = append(opts, openai.WithToken(config.APIKey))
	} else {
		// Use dummy key for providers like Ollama
		apiKey := config.APIKey
		if apiKey == "" {
			apiKey = "dummy-key"
		}
		opts = append(opts, openai.WithToken(apiKey))
	}

	return openai.New(opts...)
}

// CreateStandardHTTPClient creates an HTTP client with standard timeout
// This eliminates duplication across all providers that make HTTP calls
func (b *BaseProvider) CreateStandardHTTPClient() *http.Client {
	return &http.Client{Timeout: 10 * time.Second}
}

// BuildChatResponse creates a standardized ChatResponse
// This eliminates response construction duplication
func (b *BaseProvider) BuildChatResponse(content, model, finishReason string, usage TokenUsage, toolCalls []ToolCall, metadata map[string]string) *ChatResponse {
	if metadata == nil {
		metadata = make(map[string]string)
	}

	// Ensure provider is set
	metadata["provider"] = b.name

	return &ChatResponse{
		Content:      content,
		Usage:        usage,
		Model:        model,
		FinishReason: finishReason,
		Metadata:     metadata,
		GeneratedAt:  time.Now(),
		ToolCalls:    toolCalls,
	}
}

// CalculateUsageWithCost calculates token usage with cost
func (b *BaseProvider) CalculateUsageWithCost(promptTokens, completionTokens int, costCalculator func(string, int) float64, model string) TokenUsage {
	totalTokens := promptTokens + completionTokens
	cost := 0.0
	if costCalculator != nil {
		cost = costCalculator(model, totalTokens)
	}

	return TokenUsage{
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      totalTokens,
		Cost:             cost,
	}
}

// Capabilities describes what a provider can do
type Capabilities struct {
	Name              string   `json:"name"`               // Provider name
	Models            []string `json:"models"`             // Available models
	MaxTokens         int      `json:"max_tokens"`         // Maximum tokens per request
	SupportsStreaming bool     `json:"supports_streaming"` // Whether streaming is supported
	SupportsFunctions bool     `json:"supports_functions"` // Whether function calling is supported
	CostPerToken      float64  `json:"cost_per_token"`     // Cost per token (if known)
	RequiresAuth      bool     `json:"requires_auth"`      // Whether authentication is required
}

// ModelContextSize maps model names to their context window sizes
var ModelContextSizes = map[string]int{
	// OpenAI models
	"gpt-3.5-turbo":       16385,
	"gpt-3.5-turbo-16k":   16385,
	"gpt-4":               8192,
	"gpt-4-32k":           32768,
	"gpt-4-turbo":         128000,
	"gpt-4-turbo-preview": 128000,
	"gpt-4o":              128000,
	"gpt-4o-mini":         128000,
	"o1":                  200000,
	"o1-mini":             128000,
	"o1-preview":          128000,

	// Anthropic Claude models
	"claude-3-5-sonnet-20241022": 200000,
	"claude-3-5-sonnet":          200000,
	"claude-3-opus":              200000,
	"claude-3-sonnet":            200000,
	"claude-3-haiku":             200000,
	"claude-2.1":                 200000,
	"claude-2":                   100000,
	"claude-instant":             100000,

	// DeepSeek models
	"deepseek-chat":  64000,
	"deepseek-coder": 16000,

	// Ollama models (common defaults)
	"llama2":    4096,
	"llama3":    8192,
	"mistral":   8192,
	"mixtral":   32768,
	"codellama": 16384,

	// Yandex models
	"yandexgpt":      8000,
	"yandexgpt-lite": 8000,
}

// GetModelContextSize returns the context window size for a given model
// If the model is not found, it tries to match by prefix, otherwise returns a conservative default
func GetModelContextSize(model string) int {
	// Direct match
	if size, ok := ModelContextSizes[model]; ok {
		return size
	}

	// Try prefix matching (e.g., "gpt-4-turbo-2024-04-09" matches "gpt-4-turbo")
	// Sort by length descending to match longest prefix first
	var bestMatch string
	var bestSize int
	for knownModel, size := range ModelContextSizes {
		if len(model) >= len(knownModel) &&
			len(knownModel) > len(bestMatch) &&
			model[:len(knownModel)] == knownModel {
			bestMatch = knownModel
			bestSize = size
		}
	}

	if bestMatch != "" {
		return bestSize
	}

	// Conservative default for unknown models
	return 4096
}

// TrimMessagesToContextSize trims message history to fit within the model's context window
// It preserves the system message (first) and keeps the most recent messages
func TrimMessagesToContextSize(messages []Message, model string, reserveTokens int) []Message {
	contextSize := GetModelContextSize(model)
	maxTokens := contextSize - reserveTokens // Reserve tokens for response

	if len(messages) == 0 {
		return messages
	}

	// Always keep system message if present
	var systemMsg *Message
	startIdx := 0
	if len(messages) > 0 && messages[0].Role == "system" {
		systemMsg = &messages[0]
		startIdx = 1
	}

	// Estimate tokens for all messages
	totalTokens := 0
	if systemMsg != nil {
		totalTokens += estimateMessageTokens(*systemMsg)
	}

	// Start from the end and work backwards to keep most recent messages
	var keptMessages []Message
	for i := len(messages) - 1; i >= startIdx; i-- {
		msgTokens := estimateMessageTokens(messages[i])
		if totalTokens+msgTokens > maxTokens {
			break
		}
		totalTokens += msgTokens
		keptMessages = append([]Message{messages[i]}, keptMessages...)
	}

	// Reconstruct final message list
	var result []Message
	if systemMsg != nil {
		result = append(result, *systemMsg)
	}
	result = append(result, keptMessages...)

	return result
}

// estimateMessageTokens estimates the number of tokens in a message
// Uses a rough approximation of 4 characters per token
func estimateMessageTokens(msg Message) int {
	// Count content length
	contentLen := len(msg.Content)

	// Add overhead for role and structure (JSON formatting)
	overhead := 10

	return (contentLen / 4) + overhead
}

// Config holds configuration for LLM providers
type Config struct {
	Provider    string            `json:"provider"`    // Provider name (openai, local, etc.)
	Model       string            `json:"model"`       // Model to use
	APIKey      string            `json:"api_key"`     // API key (if required)
	BaseURL     string            `json:"base_url"`    // Base URL for API calls
	MaxTokens   int               `json:"max_tokens"`  // Maximum tokens per request
	Temperature float32           `json:"temperature"` // Temperature setting (0.0-1.0)
	TopP        float32           `json:"top_p"`       // Top-p sampling parameter
	Timeout     time.Duration     `json:"timeout"`     // Request timeout
	Metadata    map[string]string `json:"metadata"`    // Additional configuration
}

// DefaultConfig returns default LLM configuration
func DefaultConfig() Config {
	return Config{
		Provider:    "openai",
		Model:       "gpt-3.5-turbo",
		MaxTokens:   2048,
		Temperature: 0.7,
		TopP:        1.0,
		Timeout:     30 * time.Second,
		Metadata:    make(map[string]string),
	}
}

// ProviderRegistry manages available LLM providers
type ProviderRegistry struct {
	providers map[string]func() Provider
}

// NewProviderRegistry creates a new provider registry
func NewProviderRegistry() *ProviderRegistry {
	return &ProviderRegistry{
		providers: make(map[string]func() Provider),
	}
}

// Register registers a new provider factory
func (r *ProviderRegistry) Register(name string, factory func() Provider) {
	r.providers[name] = factory
}

// Create creates a provider instance by name
func (r *ProviderRegistry) Create(name string) Provider {
	if factory, exists := r.providers[name]; exists {
		return factory()
	}
	return nil
}

// List returns available provider names
func (r *ProviderRegistry) List() []string {
	names := make([]string, 0, len(r.providers))
	for name := range r.providers {
		names = append(names, name)
	}
	return names
}

// Global provider registry
var GlobalRegistry = NewProviderRegistry()
