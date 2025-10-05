package chat

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/simple-container-com/api/pkg/api/logger/color"
	"github.com/simple-container-com/api/pkg/assistant/analysis"
	"github.com/simple-container-com/api/pkg/assistant/embeddings"
	"github.com/simple-container-com/api/pkg/assistant/generation"
	"github.com/simple-container-com/api/pkg/assistant/llm"
	"github.com/simple-container-com/api/pkg/assistant/llm/prompts"
)

// ChatInterface implements the interactive chat experience
type ChatInterface struct {
	llm        llm.Provider
	context    *ConversationContext
	embeddings *embeddings.Database
	analyzer   *analysis.ProjectAnalyzer
	generator  *generation.FileGenerator
	commands   map[string]*ChatCommand
	config     SessionConfig
}

// NewChatInterface creates a new chat interface
func NewChatInterface(config SessionConfig) (*ChatInterface, error) {
	// Initialize LLM provider
	provider := llm.GlobalRegistry.Create(config.LLMProvider)
	if provider == nil {
		return nil, fmt.Errorf("unsupported LLM provider: %s", config.LLMProvider)
	}

	// Configure provider
	apiKey := config.APIKey
	if apiKey == "" {
		apiKey = os.Getenv("OPENAI_API_KEY")
	}

	llmConfig := llm.Config{
		Provider:    config.LLMProvider,
		MaxTokens:   config.MaxTokens,
		Temperature: config.Temperature,
		APIKey:      apiKey,
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

	// Create chat interface
	chat := &ChatInterface{
		llm:        provider,
		embeddings: embeddingsDB,
		analyzer:   analysis.NewProjectAnalyzer(),
		generator:  generation.NewFileGenerator(),
		commands:   make(map[string]*ChatCommand),
		config:     config,
	}

	// Initialize conversation context
	chat.context = &ConversationContext{
		ProjectPath: config.ProjectPath,
		Mode:        config.Mode,
		History:     []Message{},
		SessionID:   uuid.New().String(),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	// Register commands
	chat.registerCommands()

	// Add system prompt
	systemPrompt := prompts.GenerateContextualPrompt(config.Mode, nil, []string{})
	chat.addMessage("system", systemPrompt)

	return chat, nil
}

// StartSession starts an interactive chat session
func (c *ChatInterface) StartSession(ctx context.Context) error {
	c.printWelcome()

	// Analyze project if path is provided
	if c.config.ProjectPath != "" {
		if err := c.analyzeProject(ctx); err != nil {
			fmt.Printf("%s Failed to analyze project: %v\n", color.YellowString("⚠️"), err)
		}
	}

	// Start chat loop
	return c.chatLoop(ctx)
}

// chatLoop handles the main conversation loop
func (c *ChatInterface) chatLoop(ctx context.Context) error {
	scanner := bufio.NewScanner(os.Stdin)

	for {
		// Show prompt
		fmt.Printf("\n%s ", color.CyanString("💬"))

		// Read user input
		if !scanner.Scan() {
			break
		}

		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}

		// Check for exit commands
		if input == "exit" || input == "quit" || input == "/exit" {
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

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("input error: %w", err)
	}

	return nil
}

// handleChat processes regular chat messages
func (c *ChatInterface) handleChat(ctx context.Context, input string) error {
	// Add user message to context
	c.addMessage("user", input)

	// Show thinking indicator
	fmt.Print(color.YellowString("🤔 Thinking..."))

	// Get response from LLM
	response, err := c.llm.Chat(ctx, c.context.History)
	if err != nil {
		fmt.Print("\r") // Clear thinking indicator
		return fmt.Errorf("LLM error: %w", err)
	}

	// Clear thinking indicator and show response
	fmt.Print("\r")
	fmt.Printf("%s %s\n", color.BlueString("🤖"), response.Content)

	// Add assistant response to context
	c.addMessage("assistant", response.Content)

	// Update context
	c.context.UpdatedAt = time.Now()

	return nil
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

	// Handle generated files
	if len(result.Files) > 0 {
		fmt.Printf("\n%s Generated files:\n", color.CyanString("📁"))
		for _, file := range result.Files {
			fmt.Printf("  - %s (%s)\n", color.WhiteString(file.Path), file.Type)
		}
	}

	// Show next step if available
	if result.NextStep != "" {
		fmt.Printf("\n%s %s\n", color.BlueString("💡"), result.NextStep)
	}

	return nil
}

// printWelcome displays the welcome message
func (c *ChatInterface) printWelcome() {
	fmt.Println()
	fmt.Println(color.BlueBold("🚀 Simple Container AI Assistant"))
	fmt.Println(color.WhiteString("I'll help you set up your project with Simple Container."))
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
	fmt.Println(color.GrayString("Type 'exit' to quit"))
}

// analyzeProject analyzes the current project
func (c *ChatInterface) analyzeProject(ctx context.Context) error {
	fmt.Printf("%s Analyzing project at %s...\n", color.YellowString("🔍"), c.config.ProjectPath)

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

	// Update system prompt with project context
	contextualPrompt := prompts.GenerateContextualPrompt(c.config.Mode, projectInfo, c.context.Resources)
	c.context.History[0].Content = contextualPrompt

	return nil
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
}

// GetContext returns the current conversation context
func (c *ChatInterface) GetContext() *ConversationContext {
	return c.context
}

// Close cleans up resources
func (c *ChatInterface) Close() error {
	if c.llm != nil {
		return c.llm.Close()
	}
	return nil
}
