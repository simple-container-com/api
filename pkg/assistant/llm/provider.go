package llm

import (
	"context"
	"time"
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

	// StreamChat sends messages to the LLM and streams the response via callback
	StreamChat(ctx context.Context, messages []Message, callback StreamCallback) (*ChatResponse, error)

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
}

// TokenUsage tracks token consumption
type TokenUsage struct {
	PromptTokens     int     `json:"prompt_tokens"`     // Tokens in the prompt
	CompletionTokens int     `json:"completion_tokens"` // Tokens in the completion
	TotalTokens      int     `json:"total_tokens"`      // Total tokens used
	Cost             float64 `json:"cost"`              // Estimated cost (if available)
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
