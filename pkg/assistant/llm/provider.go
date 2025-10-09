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
// It simulates streaming by breaking the response into chunks and sending them with realistic timing
func (b *BaseProvider) FallbackToNonStreaming(ctx context.Context, messages []Message, tools []Tool, callback StreamCallback, chatWithTools func(context.Context, []Message, []Tool) (*ChatResponse, error)) (*ChatResponse, error) {
	response, err := chatWithTools(ctx, messages, tools)
	if err != nil {
		return nil, err
	}

	// Simulate realistic streaming by breaking content into chunks
	content := response.Content
	if content == "" {
		// Send empty completion chunk
		finalChunk := StreamChunk{
			Content:    "",
			Delta:      "",
			IsComplete: true,
			Usage:      &response.Usage,
			Metadata: map[string]string{
				"provider": b.name,
			},
			GeneratedAt: time.Now(),
		}
		return response, callback(finalChunk)
	}

	// Break content into words for reliable streaming with natural timing
	words := strings.Fields(content)
	if len(words) == 0 {
		// Handle empty content
		return response, nil
	}

	var currentContent strings.Builder

	for i, word := range words {
		// Add word to current content
		if i > 0 {
			currentContent.WriteString(" ") // Add space before word (except first)
		}
		currentContent.WriteString(word)

		// Create streaming chunk
		chunk := StreamChunk{
			Content: currentContent.String(),
			Delta: word + (func() string {
				if i < len(words)-1 {
					return " "
				} else {
					return ""
				}
			})(),
			IsComplete: i == len(words)-1,
			Usage:      nil, // Usage only in final chunk
			Metadata: map[string]string{
				"provider": b.name,
			},
			GeneratedAt: time.Now(),
		}

		// Add usage to final chunk
		if chunk.IsComplete {
			chunk.Usage = &response.Usage
		}

		if err := callback(chunk); err != nil {
			return nil, fmt.Errorf("callback error: %w", err)
		}

		// No delays - let the frontend handle the streaming display timing
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
		// Skip messages with empty content
		if strings.TrimSpace(msg.Content) == "" {
			continue
		}

		var msgType llms.ChatMessageType
		switch strings.ToLower(msg.Role) {
		case "user":
			msgType = llms.ChatMessageTypeHuman
		case "assistant":
			msgType = llms.ChatMessageTypeAI
		case "system":
			msgType = llms.ChatMessageTypeSystem
		case "tool":
			// Skip tool messages for now - they may cause issues with some providers
			continue
		default:
			msgType = llms.ChatMessageTypeHuman
		}

		llmMessages = append(llmMessages, llms.TextParts(msgType, msg.Content))
	}

	return llmMessages
}

// ConvertToolsToLangChainGo converts our Tool format to langchaingo Tool format
// This eliminates tool conversion duplication across providers
func (b *BaseProvider) ConvertToolsToLangChainGo(tools []Tool) []llms.Tool {
	if len(tools) == 0 {
		return nil
	}

	langchainTools := make([]llms.Tool, len(tools))
	for i, tool := range tools {
		langchainTools[i] = llms.Tool{
			Type: "function",
			Function: &llms.FunctionDefinition{
				Name:        tool.Function.Name,
				Description: tool.Function.Description,
				Parameters:  tool.Function.Parameters,
			},
		}
	}

	return langchainTools
}

// DefaultStreamChatWithTools provides a standard implementation for providers that don't have native streaming+tools
// This eliminates the duplicate fallback pattern across Anthropic, DeepSeek, Ollama, and Yandex providers
func (b *BaseProvider) DefaultStreamChatWithTools(
	ctx context.Context,
	messages []Message,
	tools []Tool,
	callback StreamCallback,
	chatWithToolsFunc func(context.Context, []Message, []Tool) (*ChatResponse, error),
	streamChatFunc func(context.Context, []Message, StreamCallback) (*ChatResponse, error),
) (*ChatResponse, error) {
	// Use base validation
	if err := b.ValidateConfiguration(); err != nil {
		return nil, err
	}

	// Standard fallback pattern: if tools are provided, use non-streaming with tools
	if len(tools) > 0 {
		return b.FallbackToNonStreaming(ctx, messages, tools, callback, chatWithToolsFunc)
	}

	// No tools, use regular streaming
	return streamChatFunc(ctx, messages, callback)
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

// ExtractToolCallsFromLangChainResponse extracts tool calls from langchaingo response
// This eliminates the duplicate tool call extraction logic across OpenAI, Anthropic, DeepSeek, and Ollama
func (b *BaseProvider) ExtractToolCallsFromLangChainResponse(response *llms.ContentResponse) []ToolCall {
	var toolCalls []ToolCall

	// Iterate through all choices to find tool calls
	// Some providers (like Anthropic) may put tool calls in different choices
	for _, choice := range response.Choices {
		if len(choice.ToolCalls) > 0 {
			for _, tc := range choice.ToolCalls {
				// Parse function arguments
				var args map[string]interface{}
				if tc.FunctionCall != nil && tc.FunctionCall.Arguments != "" {
					_ = json.Unmarshal([]byte(tc.FunctionCall.Arguments), &args)
				}

				// Extract ID, handling different provider patterns
				id := tc.ID
				if id == "" && tc.FunctionCall != nil {
					id = tc.FunctionCall.Name // Fallback to function name as ID
				}

				toolCalls = append(toolCalls, ToolCall{
					ID:   id,
					Type: "function",
					Function: FunctionCall{
						Name:      tc.FunctionCall.Name,
						Arguments: args,
					},
				})
			}
		}
	}

	return toolCalls
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

// TrimMessagesToContextSize intelligently trims message history using priority-based sliding window
//
// Priority order (highest to lowest):
// 1. System prompt - always included fully (critical for model behavior)
// 2. Last user message - always included fully (API requirement)
// 3. Messages with tool calls - high priority for context
// 4. Recent history - sliding window from newest to oldest
//
// Strategy:
// - Reserve space for system prompt + last message first
// - Fill remaining space with prioritized history
// - Ensures optimal context usage while maintaining coherence
func TrimMessagesToContextSize(messages []Message, model string, reserveTokens int) []Message {
	contextSize := GetModelContextSize(model)
	maxTokens := contextSize - reserveTokens

	if len(messages) == 0 || maxTokens <= 0 {
		return messages
	}

	// Step 1: Separate system message (FIRST one only) from conversation
	var systemMsg *Message
	var conversationMsgs []Message

	for i := range messages {
		// FIX: Take FIRST system message only, not last
		if messages[i].Role == "system" && systemMsg == nil {
			systemMsg = &messages[i]
		} else if messages[i].Role != "system" {
			conversationMsgs = append(conversationMsgs, messages[i])
		}
	}

	// If no conversation messages, return system only
	if len(conversationMsgs) == 0 {
		if systemMsg != nil {
			return []Message{*systemMsg}
		}
		return messages
	}

	// Step 2: Reserve space for critical elements (system + last message)
	totalTokens := 0

	// Reserve system prompt fully (priority #1)
	var finalSystemMsg *Message
	if systemMsg != nil {
		systemTokens := estimateMessageTokens(*systemMsg)

		// Try to keep system prompt fully
		if systemTokens <= maxTokens*3/10 { // Max 30% for system
			finalSystemMsg = systemMsg
			totalTokens += systemTokens
		} else if systemTokens <= maxTokens/2 { // Max 50% in extreme case
			finalSystemMsg = systemMsg
			totalTokens += systemTokens
		} else {
			// System prompt is huge, truncate to 30% of context
			maxSystemChars := (maxTokens*3/10 - 20) * 4
			if maxSystemChars > 100 {
				truncatedContent := systemMsg.Content[:maxSystemChars] + "\n\n[System prompt truncated to fit context]"
				systemMsgCopy := *systemMsg
				systemMsgCopy.Content = truncatedContent
				finalSystemMsg = &systemMsgCopy
				totalTokens += maxTokens * 3 / 10
			}
		}
	}

	// Reserve last message fully (priority #2)
	lastMsg := conversationMsgs[len(conversationMsgs)-1]
	lastMsgTokens := estimateMessageTokens(lastMsg)

	// Ensure room for last message
	if totalTokens+lastMsgTokens > maxTokens {
		if finalSystemMsg != nil {
			// Reduce system message to make room
			availableForSystem := maxTokens - lastMsgTokens - 50
			if availableForSystem > 50 {
				maxSystemChars := (availableForSystem - 10) * 4
				if maxSystemChars > 0 && maxSystemChars < len(finalSystemMsg.Content) {
					systemMsgCopy := *finalSystemMsg
					systemMsgCopy.Content = finalSystemMsg.Content[:maxSystemChars] + "..."
					finalSystemMsg = &systemMsgCopy
					totalTokens = availableForSystem
				}
			} else {
				// Drop system message if necessary to keep last message
				finalSystemMsg = nil
				totalTokens = 0
			}
		}
	}

	totalTokens += lastMsgTokens

	// Step 3: Fill remaining space with prioritized history
	availableForHistory := maxTokens - totalTokens
	historyMsgs := selectHistoryWithPriorities(conversationMsgs[:len(conversationMsgs)-1], availableForHistory)

	// Step 4: Reconstruct final message list
	var result []Message
	if finalSystemMsg != nil {
		result = append(result, *finalSystemMsg)
	}
	result = append(result, historyMsgs...)
	result = append(result, lastMsg)

	return result
}

// selectHistoryWithPriorities selects history messages using priority-based sliding window
func selectHistoryWithPriorities(history []Message, availableTokens int) []Message {
	if len(history) == 0 || availableTokens <= 0 {
		return []Message{}
	}

	// Categorize messages by priority
	type priorityMsg struct {
		msg      Message
		tokens   int
		priority int
		index    int
	}

	var highPriority []priorityMsg // Messages with tool calls
	var normalPriority []priorityMsg // Regular messages

	for i, msg := range history {
		tokens := estimateMessageTokens(msg)
		priority := 1 // Normal priority

		// High priority: messages with tool calls or tool results
		if hasToolCalls(msg) || msg.Role == "tool" {
			priority = 2
			highPriority = append(highPriority, priorityMsg{msg, tokens, priority, i})
		} else {
			normalPriority = append(normalPriority, priorityMsg{msg, tokens, priority, i})
		}
	}

	// Select messages to keep
	var selected []priorityMsg
	usedTokens := 0

	// Phase 1: Add high priority messages (from newest to oldest)
	for i := len(highPriority) - 1; i >= 0; i-- {
		pm := highPriority[i]
		if usedTokens+pm.tokens <= availableTokens {
			selected = append(selected, pm)
			usedTokens += pm.tokens
		}
	}

	// Phase 2: Add normal priority messages (from newest to oldest)
	for i := len(normalPriority) - 1; i >= 0; i-- {
		pm := normalPriority[i]
		if usedTokens+pm.tokens <= availableTokens {
			selected = append(selected, pm)
			usedTokens += pm.tokens
		}
	}

	// Sort selected messages by original index to maintain chronological order
	for i := 0; i < len(selected); i++ {
		for j := i + 1; j < len(selected); j++ {
			if selected[i].index > selected[j].index {
				selected[i], selected[j] = selected[j], selected[i]
			}
		}
	}

	// Extract messages
	result := make([]Message, len(selected))
	for i, pm := range selected {
		result[i] = pm.msg
	}

	return result
}

// hasToolCalls checks if a message contains tool calls
func hasToolCalls(msg Message) bool {
	if msg.Metadata == nil {
		return false
	}

	// Check for tool_calls in metadata
	if toolCalls, ok := msg.Metadata["tool_calls"]; ok {
		// Check if it's not nil and not empty
		switch v := toolCalls.(type) {
		case []interface{}:
			return len(v) > 0
		case map[string]interface{}:
			return len(v) > 0
		case string:
			return v != ""
		default:
			return toolCalls != nil
		}
	}

	// Check for function_call in metadata (older format)
	if funcCall, ok := msg.Metadata["function_call"]; ok {
		return funcCall != nil
	}

	return false
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
