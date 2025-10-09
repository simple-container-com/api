package chat

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"

	"github.com/simple-container-com/api/pkg/api/logger/color"
	"github.com/simple-container-com/api/pkg/assistant/analysis"
	"github.com/simple-container-com/api/pkg/assistant/config"
	"github.com/simple-container-com/api/pkg/assistant/core"
	"github.com/simple-container-com/api/pkg/assistant/embeddings"
	"github.com/simple-container-com/api/pkg/assistant/generation"
	"github.com/simple-container-com/api/pkg/assistant/llm"
	"github.com/simple-container-com/api/pkg/assistant/llm/prompts"
	"github.com/simple-container-com/api/pkg/assistant/modes"
	"github.com/simple-container-com/api/pkg/assistant/resources"
)

// ChatInterface implements the interactive chat experience
type ChatInterface struct {
	llm              llm.Provider
	context          *ConversationContext
	embeddings       *embeddings.Database
	analyzer         *analysis.ProjectAnalyzer
	generator        *generation.FileGenerator
	developerMode    *modes.DeveloperMode
	commandHandler   *core.UnifiedCommandHandler // New unified command handler
	commands         map[string]*ChatCommand
	config           SessionConfig
	inputHandler     *InputHandler
	toolCallHandler  *ToolCallHandler
	docCache         map[string][]embeddings.SearchResult // Cache for documentation search results
	markdownRenderer *MarkdownRenderer
	streamRenderer   *StreamRenderer
	sessionManager   *SessionManager
}

// NewChatInterface creates a new chat interface
func NewChatInterface(sessionConfig SessionConfig) (*ChatInterface, error) {
	// Initialize LLM provider
	provider := llm.GlobalRegistry.Create(sessionConfig.LLMProvider)
	if provider == nil {
		return nil, fmt.Errorf("unsupported LLM provider: %s", sessionConfig.LLMProvider)
	}

	// Configure provider
	apiKey := sessionConfig.APIKey
	baseURL := ""
	model := ""

	// Load provider config to get additional settings
	cfg, err := config.Load()
	if err == nil {
		if providerCfg, exists := cfg.GetProviderConfig(sessionConfig.LLMProvider); exists {
			if apiKey == "" {
				apiKey = providerCfg.APIKey
			}
			baseURL = providerCfg.BaseURL
			model = providerCfg.Model
		}
	}

	if apiKey == "" {
		// Try to get API key from environment based on provider
		switch sessionConfig.LLMProvider {
		case "anthropic":
			apiKey = os.Getenv("ANTHROPIC_API_KEY")
		case "openai":
			apiKey = os.Getenv("OPENAI_API_KEY")
		case "deepseek":
			apiKey = os.Getenv("DEEPSEEK_API_KEY")
		case "yandex":
			apiKey = os.Getenv("YANDEX_API_KEY")
		default:
			apiKey = os.Getenv("OPENAI_API_KEY")
		}
	}

	llmConfig := llm.Config{
		Provider:    sessionConfig.LLMProvider,
		Model:       model,
		MaxTokens:   sessionConfig.MaxTokens,
		Temperature: sessionConfig.Temperature,
		APIKey:      apiKey,
		BaseURL:     baseURL,
	}

	if err := provider.Configure(llmConfig); err != nil {
		return nil, fmt.Errorf("failed to configure LLM provider: %w", err)
	}

	// Load embeddings database
	ctx := context.Background()
	embeddingsDB, err := embeddings.LoadEmbeddedDatabase(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load embeddings database: %w", err)
	}

	// Create chat interface components (reuse embeddings to avoid reloading)
	analyzer := analysis.NewProjectAnalyzerWithEmbeddings(embeddingsDB)
	developerMode := modes.NewDeveloperModeWithComponents(provider, embeddingsDB, analyzer)
	generator := generation.NewFileGeneratorWithMode(developerMode)

	// Initialize unified command handler
	commandHandler, err := core.NewUnifiedCommandHandler()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize unified command handler: %w", err)
	}

	// Initialize session manager
	sessionManager, err := NewSessionManager()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize session manager: %w", err)
	}

	// Load config to get max sessions setting
	if cfg, err := config.Load(); err == nil && cfg.MaxSavedSessions > 0 {
		sessionManager.SetMaxSessions(cfg.MaxSavedSessions)
	}

	// Cleanup old sessions on startup
	if err := sessionManager.CleanupOldSessions(); err != nil {
		// Log warning but don't fail initialization
		fmt.Printf("Warning: failed to cleanup old sessions: %v\n", err)
	}

	// Create chat interface
	chat := &ChatInterface{
		llm:              provider,
		embeddings:       embeddingsDB,
		analyzer:         analyzer,
		generator:        generator,
		developerMode:    developerMode,
		commandHandler:   commandHandler,
		commands:         make(map[string]*ChatCommand),
		config:           sessionConfig,
		docCache:         make(map[string][]embeddings.SearchResult),
		markdownRenderer: NewMarkdownRenderer(),
		streamRenderer:   NewStreamRenderer(),
		sessionManager:   sessionManager,
	}

	// Get available resources for context
	availableResources := chat.getAvailableResources()

	// Initialize conversation context
	chat.context = &ConversationContext{
		ProjectPath: sessionConfig.ProjectPath,
		Mode:        sessionConfig.Mode,
		History:     []Message{},
		Resources:   availableResources,
		SessionID:   uuid.New().String(),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	// Register commands
	chat.registerCommands()

	// Initialize tool call handler
	chat.toolCallHandler = NewToolCallHandler(chat.commands)

	// Initialize input handler with commands
	chat.inputHandler = NewInputHandler(chat.commands)

	// Note: System prompt will be added in StartSession after project analysis

	return chat, nil
}

// StartSession starts an interactive chat session
func (c *ChatInterface) StartSession(ctx context.Context) error {
	// Set up signal handling for graceful terminal cleanup
	signalCtx, signalCancel := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer signalCancel()

	// Ensure cleanup on exit
	defer c.cleanup()

	// Prompt for session selection or creation
	if err := c.selectOrCreateSession(); err != nil {
		return fmt.Errorf("failed to setup session: %w", err)
	}

	c.printWelcome()

	// Analyze project if path is provided
	var projectInfo *analysis.ProjectAnalysis
	if c.config.ProjectPath != "" {
		if err := c.analyzeProject(signalCtx); err != nil {
			fmt.Printf("%s Failed to analyze project: %v\n", color.YellowString("⚠️"), err)
		} else {
			projectInfo = c.context.ProjectInfo
		}
	}

	// Smart mode inference based on analysis results
	intelligentMode := c.inferMode(projectInfo, c.config.Mode)
	if intelligentMode != c.config.Mode {
		c.config.Mode = intelligentMode
		c.context.Mode = intelligentMode
		fmt.Printf("%s Auto-detected %s mode based on project analysis\n",
			color.CyanString("🧠"),
			color.BlueBold(intelligentMode))
	}

	// Add system prompt with project context (if available)
	// Check if we need to add system message (only if not already present or if it's empty)
	needsSystemMessage := true
	if len(c.context.History) > 0 && c.context.History[0].Role == "system" {
		// System message already exists, check if it's not empty
		if strings.TrimSpace(c.context.History[0].Content) != "" {
			needsSystemMessage = false
		} else {
			// Remove empty system message
			c.context.History = c.context.History[1:]
		}
	}

	if needsSystemMessage {
		systemPrompt := prompts.GenerateContextualPrompt(c.config.Mode, projectInfo, c.context.Resources)
		// Insert at the beginning instead of appending
		if len(c.context.History) > 0 {
			c.context.History = append([]Message{{
				Role:      "system",
				Content:   systemPrompt,
				Timestamp: time.Now(),
				Metadata:  make(map[string]interface{}),
			}}, c.context.History...)
		} else {
			c.addMessage("system", systemPrompt)
		}
	}

	// Start chat loop with signal-aware context
	return c.chatLoop(signalCtx)
}

// chatLoop handles the main conversation loop
func (c *ChatInterface) chatLoop(ctx context.Context) error {
	for {
		// Check if context was cancelled (e.g., by signal)
		select {
		case <-ctx.Done():
			fmt.Println(color.GreenString("\n👋 Goodbye! Happy coding with Simple Container!"))
			return ctx.Err()
		default:
		}

		// Read user input with autocomplete and history
		input, err := c.inputHandler.ReadLine(color.CyanString("\n💬 "))
		if err != nil {
			if err.Error() == "interrupted" {
				fmt.Println(color.GreenString("👋 Goodbye! Happy coding with Simple Container!"))
				return nil
			}
			return fmt.Errorf("input error: %w", err)
		}

		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}

		// Save message to session history
		if err := c.sessionManager.AddMessage(input); err != nil {
			// Log but don't fail on history save errors
			fmt.Printf("%s Failed to save to history: %v\n", color.YellowString("⚠️"), err)
		}

		// Check for exit commands
		if input == "exit" || input == "quit" || input == "/exit" {
			// Save session before exiting
			if err := c.sessionManager.SaveSession(c.sessionManager.GetCurrentSession()); err != nil {
				fmt.Printf("%s Failed to save session: %v\n", color.YellowString("⚠️"), err)
			}
			// Cleanup old sessions
			if err := c.sessionManager.CleanupOldSessions(); err != nil {
				fmt.Printf("%s Failed to cleanup old sessions: %v\n", color.YellowString("⚠️"), err)
			}
			fmt.Println(color.GreenString("👋 Goodbye! Happy coding with Simple Container!"))
			break
		}

		// Handle command or regular chat
		if strings.HasPrefix(input, "/") {
			if err := c.handleCommand(ctx, input); err != nil {
				fmt.Printf("%s %v\n", color.RedString("❌"), err)
			}
		} else {
			if err := c.handleChat(ctx, input); err != nil {
				fmt.Printf("%s %v\n", color.RedString("❌"), err)
			}
		}
	}

	return nil
}

// handleChat processes regular chat messages
func (c *ChatInterface) handleChat(ctx context.Context, input string) error {
	// Add user message to context
	c.addMessage("user", input)

	// Ensure we have at least a system message if history was empty
	if len(c.context.History) == 1 && c.context.History[0].Role == "user" {
		// No system message exists, add one
		systemPrompt := prompts.GenerateContextualPrompt(c.config.Mode, c.context.ProjectInfo, c.context.Resources)
		// Insert at the beginning
		c.context.History = append([]Message{{
			Role:      "system",
			Content:   systemPrompt,
			Timestamp: time.Now(),
			Metadata:  make(map[string]interface{}),
		}}, c.context.History...)
	}

	// Enrich context with relevant documentation examples (RAG)
	if err := c.enrichContextWithDocumentation(input); err != nil {
		// Log error but don't fail - continue with regular chat
		// Only show warning in debug mode to avoid cluttering output
		if c.config.LogLevel == "debug" {
			fmt.Printf("📚 RAG Warning: Failed to retrieve relevant documentation: %v\n", err)
		}
	}

	// Check if provider supports streaming
	caps := c.llm.GetCapabilities()

	if caps.SupportsStreaming {
		return c.handleStreamingChat(ctx)
	} else {
		return c.handleNonStreamingChat(ctx)
	}
}

// handleStreamingChat processes chat with streaming responses
func (c *ChatInterface) handleStreamingChat(ctx context.Context) error {
	fmt.Print(color.YellowString("🤔 Thinking..."))

	var fullResponse string
	var firstChunk bool = true

	// Reset stream renderer for new response
	c.streamRenderer.Reset()

	// Create streaming callback
	callback := func(chunk llm.StreamChunk) error {
		if firstChunk {
			// Clear thinking indicator and show bot prefix on first chunk
			fmt.Print("\r")
			fmt.Printf("%s ", color.BlueString("🤖"))
			firstChunk = false
		}

		// Process chunk with stream renderer for colored output
		colored := c.streamRenderer.ProcessChunk(chunk.Delta)
		fmt.Print(colored)
		fullResponse += chunk.Delta

		if chunk.IsComplete {
			// Flush any remaining buffered content
			fmt.Print(c.streamRenderer.Flush())
			fmt.Print("\n") // New line after complete response
		}

		return nil
	}

	// Get available tools for the LLM
	tools := c.toolCallHandler.GetAvailableTools()

	// Estimate tokens used by tools/functions
	toolsTokens := 0
	if len(tools) > 0 {
		for _, tool := range tools {
			// Rough estimate: function name + description + parameters JSON
			toolsTokens += len(tool.Function.Name) / 4
			toolsTokens += len(tool.Function.Description) / 4
			// Parameters are map[string]interface{}, estimate size
			toolsTokens += 50 // Conservative estimate for parameters structure
		}
	}

	// Trim conversation history to fit model's context window
	// Reserve space for: response tokens + tools tokens
	modelName := c.llm.GetModel()
	reserveTokens := c.config.MaxTokens + toolsTokens
	trimmedHistory := llm.TrimMessagesToContextSize(c.context.History, modelName, reserveTokens)

	// Log if messages were trimmed
	if len(trimmedHistory) < len(c.context.History) {
		trimmedCount := len(c.context.History) - len(trimmedHistory)
		if c.config.LogLevel == "debug" {
			fmt.Printf("\n📊 Trimmed %d old messages to fit context window (model: %s, context: %d tokens, tools: %d tokens)\n",
				trimmedCount, modelName, llm.GetModelContextSize(modelName), toolsTokens)
		}
	}

	// Get streaming response from LLM with tools if the provider supports function calling
	var response *llm.ChatResponse
	var err error

	if c.llm.GetCapabilities().SupportsFunctions && len(tools) > 0 {
		response, err = c.llm.StreamChatWithTools(ctx, trimmedHistory, tools, callback)
		// If tools are not supported by this specific model, fallback to regular chat
		if err != nil && strings.Contains(err.Error(), "does not support tools") {
			if c.config.LogLevel == "debug" {
				fmt.Printf("\n⚠️  Model does not support tools, falling back to regular chat\n")
			}
			response, err = c.llm.StreamChat(ctx, trimmedHistory, callback)
		}
	} else {
		response, err = c.llm.StreamChat(ctx, trimmedHistory, callback)
	}

	if err != nil {
		fmt.Print("\r") // Clear thinking indicator
		return fmt.Errorf("LLM error: %w", err)
	}

	// Handle tool calls if present
	if len(response.ToolCalls) > 0 {
		return c.handleToolCalls(ctx, response)
	}

	// Add assistant response to context (use full response or response content)
	responseContent := fullResponse
	if responseContent == "" && response != nil {
		responseContent = response.Content
	}
	c.addMessage("assistant", responseContent)

	// Update context
	c.context.UpdatedAt = time.Now()

	return nil
}

// handleNonStreamingChat processes chat without streaming (fallback)
func (c *ChatInterface) handleNonStreamingChat(ctx context.Context) error {
	// Show thinking indicator
	fmt.Print(color.YellowString("🤔 Thinking..."))

	// Get available tools for the LLM
	tools := c.toolCallHandler.GetAvailableTools()

	// Estimate tokens used by tools/functions
	toolsTokens := 0
	if len(tools) > 0 {
		for _, tool := range tools {
			// Rough estimate: function name + description + parameters JSON
			toolsTokens += len(tool.Function.Name) / 4
			toolsTokens += len(tool.Function.Description) / 4
			// Parameters are map[string]interface{}, estimate size
			toolsTokens += 50 // Conservative estimate for parameters structure
		}
	}

	// Trim conversation history to fit model's context window
	// Reserve space for: response tokens + tools tokens
	modelName := c.llm.GetModel()
	reserveTokens := c.config.MaxTokens + toolsTokens
	trimmedHistory := llm.TrimMessagesToContextSize(c.context.History, modelName, reserveTokens)

	// Log if messages were trimmed
	if len(trimmedHistory) < len(c.context.History) {
		trimmedCount := len(c.context.History) - len(trimmedHistory)
		if c.config.LogLevel == "debug" {
			fmt.Printf("\n📊 Trimmed %d old messages to fit context window (model: %s, context: %d tokens, tools: %d tokens)\n",
				trimmedCount, modelName, llm.GetModelContextSize(modelName), toolsTokens)
		}
	}

	// Get response from LLM with tools if the provider supports function calling
	var response *llm.ChatResponse
	var err error

	if c.llm.GetCapabilities().SupportsFunctions && len(tools) > 0 {
		response, err = c.llm.ChatWithTools(ctx, trimmedHistory, tools)
		// If tools are not supported by this specific model, fallback to regular chat
		if err != nil && strings.Contains(err.Error(), "does not support tools") {
			if c.config.LogLevel == "debug" {
				fmt.Printf("\n⚠️  Model does not support tools, falling back to regular chat\n")
			}
			response, err = c.llm.Chat(ctx, trimmedHistory)
		}
	} else {
		response, err = c.llm.Chat(ctx, trimmedHistory)
	}

	if err != nil {
		fmt.Print("\r") // Clear thinking indicator
		return fmt.Errorf("LLM error: %w", err)
	}

	// Clear thinking indicator
	fmt.Print("\r")

	// Handle tool calls if present
	if len(response.ToolCalls) > 0 {
		return c.handleToolCalls(ctx, response)
	}

	// Regular text response with markdown rendering
	rendered := c.markdownRenderer.Render(response.Content)
	fmt.Printf("%s %s\n", color.BlueString("🤖"), rendered)

	// Add assistant response to context
	c.addMessage("assistant", response.Content)

	// Update context
	c.context.UpdatedAt = time.Now()

	return nil
}

// handleToolCalls processes tool calls from the LLM
func (c *ChatInterface) handleToolCalls(ctx context.Context, response *llm.ChatResponse) error {
	fmt.Printf("%s Executing requested actions...\n", color.CyanString("🔧"))

	// Add the assistant message with tool calls to conversation history
	// This is needed for the LLM to see what tools it called
	c.addMessage("assistant", response.Content)

	// Execute each tool call and add results to conversation
	for _, toolCall := range response.ToolCalls {
		fmt.Printf("  • Running %s...\n", color.YellowString(toolCall.Function.Name))

		// Execute the tool call
		result, err := c.toolCallHandler.ExecuteToolCall(ctx, toolCall, c.context)
		if err != nil {
			fmt.Printf("    ❌ Error: %v\n", err)
			// Add error result to conversation for LLM context
			c.addMessage("tool", fmt.Sprintf("Tool call %s failed: %v", toolCall.Function.Name, err))
			continue
		}

		// Display result
		if result.Success {
			fmt.Printf("    ✅ %s\n", result.Message)
		} else {
			fmt.Printf("    ❌ %s\n", result.Message)
		}

		// Handle generated files - actually write them to disk
		if len(result.Files) > 0 {
			c.handleGeneratedFiles(result)
		}

		// Add tool result to conversation history for LLM context
		toolResultMessage := c.toolCallHandler.FormatToolCallResult(toolCall, result)

		// Optional: Debug tool results for troubleshooting
		// fmt.Printf("DEBUG: Adding tool result (%d chars) to LLM context\n", len(toolResultMessage))

		c.addMessage("tool", toolResultMessage)
	}

	// Now continue LLM generation with the tool results in context
	fmt.Printf("\n%s Continuing response with tool results...\n", color.CyanString("🤖"))

	// Make another LLM call to continue generation with tool results
	if c.llm.GetCapabilities().SupportsStreaming {
		return c.handleStreamingContinuation(ctx)
	} else {
		return c.handleNonStreamingContinuation(ctx)
	}
}

// handleStreamingContinuation continues LLM generation in streaming mode after tool execution
func (c *ChatInterface) handleStreamingContinuation(ctx context.Context) error {
	var firstChunk bool = true
	var fullResponse string

	// Create streaming callback (following the same pattern as original)
	callback := func(chunk llm.StreamChunk) error {
		if firstChunk {
			firstChunk = false
		}

		// Process chunk with stream renderer for colored output (use Delta, not Content!)
		colored := c.streamRenderer.ProcessChunk(chunk.Delta)
		fmt.Print(colored)
		fullResponse += chunk.Delta

		if chunk.IsComplete {
			// Flush any remaining buffered content
			fmt.Print(c.streamRenderer.Flush())
			fmt.Print("\n") // New line after complete response
		}

		return nil
	}

	// Trim context before continuation to fit model's context window
	modelName := c.llm.GetModel()
	reserveTokens := c.config.MaxTokens
	trimmedHistory := llm.TrimMessagesToContextSize(c.context.History, modelName, reserveTokens)

	// Stream the continuation response with trimmed history
	response, err := c.llm.StreamChat(ctx, trimmedHistory, callback)
	if err != nil {
		return fmt.Errorf("LLM streaming continuation failed: %w", err)
	}

	// Handle tool calls if present in continuation response
	if len(response.ToolCalls) > 0 {
		return c.handleToolCalls(ctx, response)
	}

	// Add continuation response to context (use accumulated fullResponse, not response.Content)
	c.addMessage("assistant", fullResponse)

	// Update context
	c.context.UpdatedAt = time.Now()

	return nil
}

// handleNonStreamingContinuation continues LLM generation in non-streaming mode after tool execution
func (c *ChatInterface) handleNonStreamingContinuation(ctx context.Context) error {
	// Trim context before continuation to fit model's context window
	modelName := c.llm.GetModel()
	reserveTokens := c.config.MaxTokens
	trimmedHistory := llm.TrimMessagesToContextSize(c.context.History, modelName, reserveTokens)

	// Get the continuation response with trimmed history
	response, err := c.llm.Chat(ctx, trimmedHistory)
	if err != nil {
		return fmt.Errorf("LLM continuation failed: %w", err)
	}

	// Handle tool calls if present in continuation response
	if len(response.ToolCalls) > 0 {
		return c.handleToolCalls(ctx, response)
	}

	// Add continuation response to context (use full response or response content)
	responseContent := response.Content
	if responseContent == "" && len(response.ToolCalls) == 0 {
		responseContent = "I've completed the requested actions."
	}
	c.addMessage("assistant", responseContent)

	// Display continuation response with markdown rendering
	rendered := c.markdownRenderer.Render(responseContent)
	fmt.Printf("\n%s %s\n", color.BlueString("🤖"), rendered)

	// Update context
	c.context.UpdatedAt = time.Now()

	return nil
}

// handleGeneratedFiles processes generated files from command results
func (c *ChatInterface) handleGeneratedFiles(result *CommandResult) {
	// Ask for confirmation for each existing file individually
	overwriteDecisions := make(map[string]bool)
	for _, file := range result.Files {
		filename := filepath.Base(file.Path)
		if _, err := os.Stat(file.Path); err == nil {
			fmt.Printf("\n⚠️  %s already exists. Overwrite? [y/N]: ", color.YellowString(filename))

			reader := bufio.NewReader(os.Stdin)
			input, _ := reader.ReadString('\n')
			input = strings.TrimSpace(strings.ToLower(input))

			overwriteDecisions[filename] = (input == "y" || input == "yes")
		} else {
			overwriteDecisions[filename] = true
		}
	}

	// Write files to disk based on user decisions
	for _, file := range result.Files {
		filename := filepath.Base(file.Path)

		// Check if we should write this file based on user decision
		if shouldOverwrite, exists := overwriteDecisions[filename]; exists && !shouldOverwrite {
			fmt.Printf("  ⚠️  Skipped: %s (user chose not to overwrite)\n", color.YellowString(file.Path))
			continue
		}

		// Create directory if it doesn't exist
		dir := filepath.Dir(file.Path)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			fmt.Printf("Error creating directory %s: %v\n", dir, err)
			continue
		}

		// Write file
		if err := os.WriteFile(file.Path, []byte(file.Content), 0o644); err != nil {
			fmt.Printf("Error writing file %s: %v\n", file.Path, err)
		} else {
			fmt.Printf("  📄 Created: %s\n", color.GreenString(file.Path))
		}
	}
}

// handleCommand processes chat commands
func (c *ChatInterface) handleCommand(ctx context.Context, input string) error {
	parts := strings.Fields(input)
	if len(parts) == 0 {
		return fmt.Errorf("empty command")
	}

	commandName := strings.TrimPrefix(parts[0], "/")
	args := parts[1:]

	// Find command
	command, exists := c.commands[commandName]
	if !exists {
		return fmt.Errorf("unknown command: %s (type /help for available commands)", commandName)
	}

	// Execute command
	result, err := command.Handler(ctx, args, c.context)
	if err != nil {
		return err
	}

	// Display result
	if result.Success {
		fmt.Printf("%s %s\n", color.GreenString("✅"), result.Message)
	} else {
		fmt.Printf("%s %s\n", color.RedString("❌"), result.Message)
	}

	// Handle generated files - actually write them to disk
	if len(result.Files) > 0 {
		// Ask for confirmation for each existing file individually
		overwriteDecisions := make(map[string]bool)
		for _, file := range result.Files {
			filename := filepath.Base(file.Path)
			if _, err := os.Stat(file.Path); err == nil {
				fmt.Printf("\n⚠️  %s already exists. Overwrite? [y/N]: ", color.YellowString(filename))

				var response string
				if _, err := fmt.Scanln(&response); err != nil {
					// If there's an error reading input, default to "no"
					overwriteDecisions[filename] = false
				} else {
					response = strings.ToLower(strings.TrimSpace(response))
					overwriteDecisions[filename] = response == "y" || response == "yes"
				}
			} else {
				overwriteDecisions[filename] = true
			}
		}

		fmt.Printf("\n%s Generated files:\n", color.CyanString("📁"))
		for _, file := range result.Files {
			filename := filepath.Base(file.Path)

			// Check if we should write this file based on user decision
			if shouldOverwrite, exists := overwriteDecisions[filename]; exists && !shouldOverwrite {
				fmt.Printf("  - %s (%s) - %s\n", color.YellowString(file.Path), file.Type, color.YellowString("⚠ Skipped"))
				continue
			}

			// Create directory if needed
			dir := filepath.Dir(file.Path)
			if dir != "." {
				if err := os.MkdirAll(dir, 0o755); err != nil {
					fmt.Printf("  - %s (%s) - %s\n", color.RedString(file.Path), file.Type, color.RedString("Failed to create directory: "+err.Error()))
					continue
				}
			}

			// Write file content
			if err := os.WriteFile(file.Path, []byte(file.Content), 0o644); err != nil {
				fmt.Printf("  - %s (%s) - %s\n", color.RedString(file.Path), file.Type, color.RedString("Failed to write: "+err.Error()))
			} else {
				fmt.Printf("  - %s (%s) - %s\n", color.GreenString(file.Path), file.Type, color.GreenString("✓ Written"))
			}
		}
	}

	// Show next step if available
	if result.NextStep != "" {
		fmt.Printf("\n%s %s\n", color.BlueString("💡"), result.NextStep)
	}

	return nil
}

// selectOrCreateSession prompts user to select or create a session
func (c *ChatInterface) selectOrCreateSession() error {
	sessions, err := c.sessionManager.ListSessions()
	if err != nil {
		return err
	}

	fmt.Println()
	fmt.Println(color.BlueBold("📚 Session History"))
	fmt.Println()

	if len(sessions) == 0 {
		fmt.Println(color.GrayString("No previous sessions found. Starting a new session..."))
		session := c.sessionManager.CreateNewSession(c.config.ProjectPath, c.config.Mode)

		// Initialize input handler with session history
		if c.inputHandler == nil {
			c.inputHandler = NewInputHandler(c.commands)
		}
		c.inputHandler.history = session.CommandHistory

		return nil
	}

	// Show available sessions
	fmt.Println(color.CyanString("Available sessions:"))
	fmt.Println()
	for i, session := range sessions {
		age := time.Since(session.LastUsedAt)
		ageStr := formatDuration(age)

		projectName := filepath.Base(session.ProjectPath)
		if projectName == "" || projectName == "." {
			projectName = "no project"
		}

		fmt.Printf("  %d. %s\n", i+1, color.GreenString(session.Title))
		fmt.Printf("     %s | %s | %d messages | %s ago\n",
			color.YellowString(session.Mode),
			color.CyanString(projectName),
			len(session.ConversationHistory),
			ageStr)
	}
	fmt.Println()
	fmt.Printf("  %d. %s\n", len(sessions)+1, color.GreenString("Start a new session"))
	fmt.Println()

	// Read user choice
	input, err := c.inputHandler.ReadSimple(color.CyanString(fmt.Sprintf("Select session [1-%d]: ", len(sessions)+1)))
	if err != nil {
		return err
	}

	choice, err := strconv.Atoi(strings.TrimSpace(input))
	if err != nil || choice < 1 || choice > len(sessions)+1 {
		fmt.Println(color.YellowString("Invalid choice, starting new session..."))
		session := c.sessionManager.CreateNewSession(c.config.ProjectPath, c.config.Mode)
		c.inputHandler.history = session.CommandHistory
		return nil
	}

	if choice == len(sessions)+1 {
		// Create new session
		session := c.sessionManager.CreateNewSession(c.config.ProjectPath, c.config.Mode)
		c.inputHandler.history = session.CommandHistory
		fmt.Println(color.GreenString("✨ Started new session"))
	} else {
		// Load existing session
		session, err := c.sessionManager.LoadSession(sessions[choice-1].ID)
		if err != nil {
			return fmt.Errorf("failed to load session: %w", err)
		}
		c.inputHandler.history = session.CommandHistory

		// Restore conversation context from session
		c.restoreConversationContext(session)

		fmt.Printf("%s Resumed session: %s (%d messages in context)\n",
			color.GreenString("✅"), session.Title, len(session.ConversationHistory))
	}

	fmt.Println()
	return nil
}

// formatDuration formats a duration in human-readable form
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return "just now"
	}
	if d < time.Hour {
		mins := int(d.Minutes())
		if mins == 1 {
			return "1 minute"
		}
		return fmt.Sprintf("%d minutes", mins)
	}
	if d < 24*time.Hour {
		hours := int(d.Hours())
		if hours == 1 {
			return "1 hour"
		}
		return fmt.Sprintf("%d hours", hours)
	}
	days := int(d.Hours() / 24)
	if days == 1 {
		return "1 day"
	}
	return fmt.Sprintf("%d days", days)
}

// printWelcome displays the welcome message
func (c *ChatInterface) printWelcome() {
	fmt.Println()
	fmt.Println(color.BlueBold("🚀 Simple Container AI Assistant"))
	fmt.Println(color.WhiteString("I'll help you set up your project with Simple Container."))
	fmt.Println()

	// Display provider and model information
	providerName := c.config.LLMProvider
	modelName := c.llm.GetModel()
	capabilities := c.llm.GetCapabilities()

	// Get display name for provider
	displayName := providerName
	if capabilities.Name != "" {
		displayName = capabilities.Name
	}

	fmt.Printf("%s %s", color.GrayString("🤖 Provider:"), color.CyanString(displayName))
	if modelName != "" {
		fmt.Printf(" | %s %s\n", color.GrayString("Model:"), color.YellowString(modelName))
	} else {
		fmt.Println()
	}
	fmt.Println()

	if c.config.Mode == "dev" {
		fmt.Println(color.CyanString("📱 Developer Mode") + " - I'll help you set up your application")
	} else if c.config.Mode == "devops" {
		fmt.Println(color.YellowString("🛠️  DevOps Mode") + " - I'll help you set up infrastructure")
	} else {
		fmt.Println(color.GreenString("💬 General Mode") + " - Ask me anything about Simple Container")
	}

	fmt.Println()
	fmt.Println(color.WhiteString("Type '/help' for commands or just ask me questions!"))
	fmt.Println(color.GrayString("💡 Use Tab for autocomplete, ↑/↓ for history"))
	fmt.Println(color.GrayString("Type 'exit' or Ctrl+C to quit"))
}

// analyzeProject analyzes the current project
func (c *ChatInterface) analyzeProject(ctx context.Context) error {
	// Set up progress reporter for nice analysis feedback
	progressReporter := analysis.NewStreamingProgressReporter(os.Stdout)
	c.analyzer.SetProgressReporter(progressReporter)

	// Use CachedMode for chat interface to load from cache when available
	c.analyzer.SetAnalysisMode(analysis.CachedMode)

	projectInfo, err := c.analyzer.AnalyzeProject(c.config.ProjectPath)
	if err != nil {
		return err
	}

	c.context.ProjectInfo = projectInfo

	if projectInfo.PrimaryStack != nil {
		fmt.Printf("%s Detected: %s (%s) - %.0f%% confidence\n",
			color.GreenString("✅"),
			projectInfo.PrimaryStack.Language,
			projectInfo.PrimaryStack.Framework,
			projectInfo.PrimaryStack.Confidence*100)
	}

	// Note: System prompt is now added in StartSession with this project context

	return nil
}

// inferMode intelligently determines the appropriate mode based on project analysis
func (c *ChatInterface) inferMode(projectInfo *analysis.ProjectAnalysis, currentMode string) string {
	// If mode is explicitly set, respect it
	if currentMode != "" && currentMode != "general" {
		return currentMode
	}

	// If no project info available, stay in general mode
	if projectInfo == nil {
		return "general"
	}

	// Check for development indicators
	developerIndicators := 0
	devopsIndicators := 0

	// Language/framework detection suggests developer work
	if projectInfo.PrimaryStack != nil {
		switch projectInfo.PrimaryStack.Language {
		case "go", "javascript", "python", "java", "ruby", "php", "rust", "swift", "kotlin":
			developerIndicators += 3
		case "dockerfile", "yaml", "terraform", "kubernetes":
			devopsIndicators += 2
		}

		// Framework detection
		switch projectInfo.PrimaryStack.Framework {
		case "express", "react", "vue", "angular", "django", "flask", "gin", "echo", "fastapi", "spring", "rails":
			developerIndicators += 2
		case "terraform", "ansible", "kubernetes", "helm":
			devopsIndicators += 3
		}
	}

	// Resource detection patterns
	if projectInfo.Resources != nil {
		// Applications typically have databases, APIs, env vars
		if len(projectInfo.Resources.Databases) > 0 {
			developerIndicators += 1
		}
		if len(projectInfo.Resources.ExternalAPIs) > 0 {
			developerIndicators += 1
		}
		if len(projectInfo.Resources.EnvironmentVars) > 5 {
			developerIndicators += 1
		}

		// Infrastructure projects might have more complex storage/queue setups
		if len(projectInfo.Resources.Storage) > 1 {
			devopsIndicators += 1
		}
		if len(projectInfo.Resources.Queues) > 0 {
			devopsIndicators += 1
		}
	}

	// Architecture patterns
	switch projectInfo.Architecture {
	case "microservice", "api", "web-app", "standard-web-app", "single-page-app":
		developerIndicators += 2
	case "infrastructure", "platform", "multi-service":
		devopsIndicators += 2
	}

	// File patterns (check project name/path for clues)
	projectName := strings.ToLower(projectInfo.Name)
	if strings.Contains(projectName, "service") ||
		strings.Contains(projectName, "app") ||
		strings.Contains(projectName, "api") ||
		strings.Contains(projectName, "frontend") ||
		strings.Contains(projectName, "backend") {
		developerIndicators += 1
	}

	if strings.Contains(projectName, "infra") ||
		strings.Contains(projectName, "deploy") ||
		strings.Contains(projectName, "ops") ||
		strings.Contains(projectName, "platform") ||
		strings.Contains(projectName, "terraform") {
		devopsIndicators += 2
	}

	// Make decision based on indicators
	if developerIndicators > devopsIndicators {
		return "dev"
	} else if devopsIndicators > developerIndicators {
		return "devops"
	}

	// Default to dev mode if working on a codebase with a detected language
	if projectInfo.PrimaryStack != nil && projectInfo.PrimaryStack.Language != "" {
		return "dev"
	}

	return "general"
}

// addMessage adds a message to the conversation history
func (c *ChatInterface) addMessage(role, content string) {
	message := Message{
		Role:      role,
		Content:   content,
		Timestamp: time.Now(),
		Metadata:  make(map[string]interface{}),
	}

	c.context.History = append(c.context.History, message)
	c.context.UpdatedAt = time.Now()

	// Save to session history
	if err := c.sessionManager.AddConversationMessage(role, content, message.Metadata); err != nil {
		// Log but don't fail on save errors
		fmt.Printf("%s Failed to save message to session: %v\n", color.YellowString("⚠️"), err)
	}
}

// trimContextIfNeeded trims old messages from history if context is too large
// Keeps system messages and recent messages within token limit
func (c *ChatInterface) trimContextIfNeeded(maxTokens int) {
	if len(c.context.History) <= 2 {
		return // Keep at least system message and one exchange
	}

	// Estimate tokens (rough estimate: ~4 chars per token)
	estimatedTokens := 0
	for _, msg := range c.context.History {
		estimatedTokens += len(msg.Content) / 4
	}

	if estimatedTokens <= maxTokens {
		return // Within limit
	}

	// Keep system message(s) at the beginning
	systemMessages := []Message{}
	otherMessages := []Message{}

	for _, msg := range c.context.History {
		if msg.Role == "system" {
			systemMessages = append(systemMessages, msg)
		} else {
			otherMessages = append(otherMessages, msg)
		}
	}

	// Calculate how many recent messages we can keep
	// Reserve 50% for system messages
	maxOtherTokens := maxTokens / 2

	// Keep recent messages that fit in the limit
	keptMessages := []Message{}
	currentTokens := 0

	// Start from the end (most recent messages)
	for i := len(otherMessages) - 1; i >= 0; i-- {
		msgTokens := len(otherMessages[i].Content) / 4
		if currentTokens+msgTokens > maxOtherTokens {
			break
		}
		keptMessages = append([]Message{otherMessages[i]}, keptMessages...)
		currentTokens += msgTokens
	}

	// Rebuild history: system messages + recent messages
	c.context.History = append(systemMessages, keptMessages...)

	if len(keptMessages) < len(otherMessages) {
		fmt.Printf("%s Trimmed context: kept %d of %d messages (estimated %d tokens)\n",
			color.YellowString("⚠️"), len(keptMessages), len(otherMessages), currentTokens)
	}
}

// restoreConversationContext restores conversation history from session
func (c *ChatInterface) restoreConversationContext(session *SessionHistory) {
	// Clear current history
	c.context.History = make([]Message, 0, len(session.ConversationHistory))

	// Convert SavedMessage to Message
	for _, savedMsg := range session.ConversationHistory {
		message := Message{
			Role:      savedMsg.Role,
			Content:   savedMsg.Content,
			Timestamp: savedMsg.Timestamp,
			Metadata:  savedMsg.Metadata,
		}
		if message.Metadata == nil {
			message.Metadata = make(map[string]interface{})
		}
		c.context.History = append(c.context.History, message)
	}
}

// enrichContextWithDocumentation searches for relevant documentation based on user message
// and adds it to the conversation context using RAG (Retrieval-Augmented Generation).
//
// This function implements a comprehensive RAG system that:
// 1. Analyzes user messages for question indicators and relevant keywords
// 2. Performs semantic search against the embeddings database
// 3. Caches results to avoid redundant searches
// 4. Enriches the system prompt with relevant documentation examples
//
// Debug logging: Set LogLevel to "debug" (c.config.LogLevel = "debug") to see detailed
// RAG operation logs prefixed with "📚 RAG:" showing query extraction, search results,
// cache usage, and documentation enhancement details.
func (c *ChatInterface) enrichContextWithDocumentation(userMessage string) error {
	if c.embeddings == nil {
		return fmt.Errorf("embeddings database not available")
	}

	// Extract search query from user message
	searchQuery := c.extractSearchQuery(userMessage)
	if searchQuery == "" {
		// No relevant keywords found, skip documentation search
		if c.config.LogLevel == "debug" {
			fmt.Printf("📚 RAG: No relevant keywords found in message, skipping documentation search\n")
		}
		return nil
	}

	if c.config.LogLevel == "debug" {
		fmt.Printf("📚 RAG: Extracted search query: '%s'\n", searchQuery)
	}

	// Check cache first (with normalized query key)
	cacheKey := strings.ToLower(strings.TrimSpace(searchQuery))
	var results []embeddings.SearchResult
	if cachedResults, exists := c.docCache[cacheKey]; exists {
		results = cachedResults
		if c.config.LogLevel == "debug" {
			fmt.Printf("📚 RAG: Using cached results for query (found %d results)\n", len(results))
		}
	} else {
		// Search for relevant documentation (limit to top 3 most relevant results)
		var err error
		results, err = embeddings.SearchDocumentation(c.embeddings, searchQuery, 3)
		if err != nil {
			return fmt.Errorf("failed to search documentation: %w", err)
		}

		if c.config.LogLevel == "debug" {
			fmt.Printf("📚 RAG: Found %d documentation results from embeddings search\n", len(results))
		}

		// Cache the results (limit cache size to prevent memory bloat)
		if len(c.docCache) < 50 {
			c.docCache[cacheKey] = results
			if c.config.LogLevel == "debug" {
				fmt.Printf("📚 RAG: Cached results (cache size: %d/50)\n", len(c.docCache))
			}
		}
	}

	if len(results) == 0 {
		// No relevant documentation found
		if c.config.LogLevel == "debug" {
			fmt.Printf("📚 RAG: No relevant documentation found for query\n")
		}
		return nil
	}

	// Format documentation examples for LLM context
	docContext := c.formatDocumentationForLLM(results, userMessage)

	if c.config.LogLevel == "debug" {
		fmt.Printf("📚 RAG: Enhanced system prompt with %d documentation examples\n", len(results))
		// Show brief summary of retrieved docs
		for i, result := range results {
			title, ok := result.Metadata["title"].(string)
			if !ok || title == "" {
				title = result.ID
			}
			score := int(result.Score * 100)
			fmt.Printf("📚 RAG:   %d. %s (%d%% relevance)\n", i+1, title, score)
		}
	}

	// Update the system message with relevant documentation
	c.updateSystemMessageWithDocumentation(docContext)

	return nil
}

// extractSearchQuery extracts relevant keywords from user message for documentation search
func (c *ChatInterface) extractSearchQuery(message string) string {
	originalMessage := message
	message = strings.ToLower(message)

	// First check if this looks like a question that would benefit from documentation
	questionIndicators := []string{
		"how", "what", "where", "when", "why", "can you", "could you", "would you",
		"show me", "example", "help", "configure", "setup", "create", "generate",
		"explain", "tell me", "i need", "i want", "looking for", "?",
	}

	hasQuestionIndicator := false
	matchedIndicator := ""
	for _, indicator := range questionIndicators {
		if strings.Contains(message, indicator) {
			hasQuestionIndicator = true
			matchedIndicator = indicator
			break
		}
	}

	// If it doesn't look like a question needing examples, skip documentation search
	if !hasQuestionIndicator {
		return ""
	}

	// Keywords that indicate user needs documentation examples
	relevantKeywords := []string{
		"client.yaml", "server.yaml", "secrets.yaml", "docker-compose",
		"configuration", "config", "setup", "deploy", "example", "how to",
		"template", "resource", "stack", "environment", "secret",
		"mongodb", "redis", "postgres", "s3", "aws", "gcp", "kubernetes",
		"dockerfile", "compose", "helm", "terraform", "pulumi",
		"yaml", "file", "syntax", "format", "structure",
	}

	var foundKeywords []string
	for _, keyword := range relevantKeywords {
		if strings.Contains(message, keyword) {
			foundKeywords = append(foundKeywords, keyword)
		}
	}

	if len(foundKeywords) == 0 {
		// If no specific keywords but it's a question, use the original message for search
		if c.config.LogLevel == "debug" {
			fmt.Printf("📚 RAG: Question detected ('%s') but no specific keywords, using full message for search\n", matchedIndicator)
		}
		return originalMessage
	}

	// Return the most relevant keywords for search
	searchQuery := strings.Join(foundKeywords, " ")
	if c.config.LogLevel == "debug" {
		fmt.Printf("📚 RAG: Question detected ('%s'), found keywords: %v -> search query: '%s'\n", matchedIndicator, foundKeywords, searchQuery)
	}
	return searchQuery
}

// formatDocumentationForLLM formats search results for inclusion in LLM context
func (c *ChatInterface) formatDocumentationForLLM(results []embeddings.SearchResult, userMessage string) string {
	var docBuilder strings.Builder

	docBuilder.WriteString("\n\nRELEVANT DOCUMENTATION EXAMPLES (based on user question):\n")
	docBuilder.WriteString("Use these examples to provide accurate, specific guidance:\n\n")

	for i, result := range results {
		score := int(result.Score * 100)
		title, ok := result.Metadata["title"].(string)
		if !ok || title == "" {
			title = result.ID
		}

		docBuilder.WriteString(fmt.Sprintf("%d. **%s** (%d%% relevance)\n", i+1, title, score))

		// Include relevant content snippet
		content := result.Content
		if len(content) > 800 {
			content = content[:800] + "..."
		}

		docBuilder.WriteString(fmt.Sprintf("```\n%s\n```\n\n", content))
	}

	docBuilder.WriteString("END OF DOCUMENTATION EXAMPLES\n")
	docBuilder.WriteString("Use the above examples to provide specific, accurate guidance based on actual Simple Container patterns.\n")

	return docBuilder.String()
}

// updateSystemMessageWithDocumentation updates or creates system message with documentation context
func (c *ChatInterface) updateSystemMessageWithDocumentation(docContext string) {
	// Find existing system message (should be first message)
	if len(c.context.History) > 0 && c.context.History[0].Role == "system" {
		// Update existing system message
		originalContent := c.context.History[0].Content

		// Remove any previous documentation section
		if idx := strings.Index(originalContent, "\n\nRELEVANT DOCUMENTATION EXAMPLES"); idx != -1 {
			originalContent = originalContent[:idx]
		}

		// Add new documentation context
		c.context.History[0].Content = originalContent + docContext
		c.context.History[0].Timestamp = time.Now()
	} else {
		// Create new system message with documentation context
		systemMessage := Message{
			Role:      "system",
			Content:   "You are the Simple Container AI Assistant. " + docContext,
			Timestamp: time.Now(),
			Metadata:  make(map[string]interface{}),
		}

		// Insert at the beginning
		c.context.History = append([]Message{systemMessage}, c.context.History...)
	}
}

// GetContext returns the current conversation context
func (c *ChatInterface) GetContext() *ConversationContext {
	return c.context
}

// ReloadLLMProvider reloads the LLM provider with current config
func (c *ChatInterface) ReloadLLMProvider() error {
	// Load current config
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Get default provider
	provider := cfg.GetDefaultProvider()
	if provider == "" {
		return fmt.Errorf("no default provider configured")
	}

	// Get provider config
	providerCfg, exists := cfg.GetProviderConfig(provider)
	if !exists {
		return fmt.Errorf("provider %s not configured", provider)
	}
	// Check API key requirement (Ollama doesn't need one)
	if provider != config.ProviderOllama && providerCfg.APIKey == "" {
		return fmt.Errorf("provider %s not configured (API key missing)", provider)
	}

	// Close old provider
	if c.llm != nil {
		c.llm.Close()
	}

	// Create new provider
	newProvider := llm.GlobalRegistry.Create(provider)
	if newProvider == nil {
		return fmt.Errorf("unsupported LLM provider: %s", provider)
	}

	// Configure provider
	llmConfig := llm.Config{
		Provider:    provider,
		Model:       providerCfg.Model, // Use model from provider config
		MaxTokens:   c.config.MaxTokens,
		Temperature: c.config.Temperature,
		APIKey:      providerCfg.APIKey,
		BaseURL:     providerCfg.BaseURL,
	}

	if err := newProvider.Configure(llmConfig); err != nil {
		return fmt.Errorf("failed to configure LLM provider: %w", err)
	}

	// Update LLM provider
	c.llm = newProvider
	c.config.LLMProvider = provider

	return nil
}

// getAvailableResources retrieves list of available Simple Container resources
func (c *ChatInterface) getAvailableResources() []string {
	// Use the resources package to get available resource types
	return resources.GetAvailableResourceTypes()
}

// cleanup performs graceful cleanup of terminal state and resources
func (c *ChatInterface) cleanup() {
	// Close input handler to restore terminal state
	if c.inputHandler != nil {
		if err := c.inputHandler.Close(); err != nil {
			// If regular close fails, try to restore terminal with stty
			if cmd := exec.Command("stty", "sane"); cmd != nil {
				// Ignore error - this is emergency cleanup
				_ = cmd.Run()
			}
		}
	}
}

// Close cleans up resources
func (c *ChatInterface) Close() error {
	// Perform cleanup first
	c.cleanup()

	// Close LLM provider
	if c.llm != nil {
		return c.llm.Close()
	}
	return nil
}
