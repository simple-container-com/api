package chat

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/simple-container-com/api/pkg/api/logger/color"
	"github.com/simple-container-com/api/pkg/assistant/embeddings"
)

// registerCoreCommands registers core/basic chat commands
func (c *ChatInterface) registerCoreCommands() {
	c.commands["help"] = &ChatCommand{
		Name:        "help",
		Description: "Show available commands and usage",
		Usage:       "/help [command]",
		Handler:     c.handleHelp,
		Aliases:     []string{"h"},
	}

	c.commands["search"] = &ChatCommand{
		Name:        "search",
		Description: "Search Simple Container documentation",
		Usage:       "/search <query> [limit]",
		Handler:     c.handleSearch,
		Aliases:     []string{"s"},
		Args: []CommandArg{
			{Name: "query", Type: "string", Required: true, Description: "Search query"},
			{Name: "limit", Type: "int", Required: false, Description: "Number of results", Default: "5"},
		},
	}

	c.commands["clear"] = &ChatCommand{
		Name:        "clear",
		Description: "Clear conversation history",
		Usage:       "/clear",
		Handler:     c.handleClear,
		Aliases:     []string{"cls"},
	}

	c.commands["status"] = &ChatCommand{
		Name:        "status",
		Description: "Show current session status",
		Usage:       "/status",
		Handler:     c.handleStatus,
		Aliases:     []string{"info"},
	}
}

// handleHelp shows help information
func (c *ChatInterface) handleHelp(ctx context.Context, args []string, context *ConversationContext) (*CommandResult, error) {
	if len(args) > 0 {
		// Show help for specific command
		commandName := args[0]
		command, exists := c.commands[commandName]
		if !exists {
			return &CommandResult{
				Success: false,
				Message: fmt.Sprintf("Unknown command: %s", commandName),
			}, nil
		}

		message := fmt.Sprintf("%s\n\n%s\n\nUsage: %s",
			color.CyanString(command.Name),
			command.Description,
			command.Usage)

		if len(command.Args) > 0 {
			message += "\n\nArguments:"
			for _, arg := range command.Args {
				required := ""
				if arg.Required {
					required = " (required)"
				}
				defaultVal := ""
				if arg.Default != "" {
					defaultVal = fmt.Sprintf(" [default: %s]", arg.Default)
				}
				message += fmt.Sprintf("\n  %s: %s%s%s", arg.Name, arg.Description, required, defaultVal)
			}
		}

		if len(command.Aliases) > 0 {
			message += fmt.Sprintf("\n\nAliases: %s", strings.Join(command.Aliases, ", "))
		}

		return &CommandResult{
			Success: true,
			Message: message,
		}, nil
	}

	// Show all commands
	message := "Available commands:\n\n"
	for name, command := range c.commands {
		if name == command.Name { // Skip aliases
			aliases := ""
			if len(command.Aliases) > 0 {
				aliases = fmt.Sprintf(" (aliases: %s)", strings.Join(command.Aliases, ", "))
			}
			message += fmt.Sprintf("/%s - %s%s\n", command.Name, command.Description, aliases)
		}
	}
	message += "\nUse '/help <command>' for detailed information about a specific command."

	return &CommandResult{
		Success: true,
		Message: message,
	}, nil
}

// handleSearch searches documentation
func (c *ChatInterface) handleSearch(ctx context.Context, args []string, context *ConversationContext) (*CommandResult, error) {
	if len(args) == 0 {
		return &CommandResult{
			Success: false,
			Message: "❌ Usage: /search <query> [limit]\nExample: /search mongodb configuration",
		}, nil
	}

	query := strings.Join(args[:len(args)-1], " ")
	limit := 5

	// Check if last arg is a number (limit)
	if len(args) > 1 {
		if lastArg := args[len(args)-1]; lastArg != "" {
			if parsedLimit, err := strconv.Atoi(lastArg); err == nil && parsedLimit > 0 {
				limit = parsedLimit
				if limit > 10 {
					limit = 10 // Cap at 10 results
				}
				query = strings.Join(args[:len(args)-1], " ")
			} else {
				query = strings.Join(args, " ")
			}
		}
	}

	if c.embeddings == nil {
		return &CommandResult{
			Success: false,
			Message: "❌ Search is not available - embeddings database not loaded",
		}, nil
	}

	results, err := embeddings.SearchDocumentation(c.embeddings, query, limit)
	if err != nil {
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("❌ Search failed: %v", err),
		}, nil
	}

	if len(results) == 0 {
		return &CommandResult{
			Success: true,
			Message: fmt.Sprintf("🔍 No results found for: \"%s\"", query),
		}, nil
	}

	message := fmt.Sprintf("🔍 Search Results for \"%s\":\n\n", query)
	for i, result := range results {
		score := int(result.Score * 100)
		title := "Unknown"
		if titleInterface, exists := result.Metadata["title"]; exists {
			if titleStr, ok := titleInterface.(string); ok {
				title = titleStr
			}
		}

		path := "Unknown"
		if pathInterface, exists := result.Metadata["path"]; exists {
			if pathStr, ok := pathInterface.(string); ok {
				path = pathStr
			}
		}

		message += fmt.Sprintf("%d. **%s** (%d%% match)\n   File: %s\n   %s\n\n",
			i+1, title, score, path, strings.ReplaceAll(result.Content[:min(200, len(result.Content))], "\n", " "))
	}

	return &CommandResult{
		Success: true,
		Message: message,
	}, nil
}

// handleClear clears conversation history
func (c *ChatInterface) handleClear(ctx context.Context, args []string, context *ConversationContext) (*CommandResult, error) {
	// Keep only system prompt
	if len(context.History) > 1 {
		context.History = context.History[:1]
	}
	return &CommandResult{
		Success: true,
		Message: "✅ Conversation history cleared",
	}, nil
}

// handleStatus shows session status
func (c *ChatInterface) handleStatus(ctx context.Context, args []string, context *ConversationContext) (*CommandResult, error) {
	message := "Session Status:"
	message += fmt.Sprintf("\n🆔 Session ID: %s", context.SessionID)
	message += fmt.Sprintf("\n⚡ Mode: %s", strings.ToTitle(context.Mode))
	message += fmt.Sprintf("\n💬 Messages: %d", len(context.History))
	message += fmt.Sprintf("\n📁 Project: %s", func() string {
		if context.ProjectPath == "" {
			return "Not set"
		}
		return context.ProjectPath
	}())

	if context.ProjectInfo != nil {
		message += fmt.Sprintf("\n🔧 Tech Stack: %s", context.ProjectInfo.PrimaryStack.Language)
	}

	if c.llm != nil {
		capabilities := c.llm.GetCapabilities()
		currentModel := c.llm.GetModel()
		if currentModel == "" {
			currentModel = "default"
		}
		message += fmt.Sprintf("\n🤖 Provider: %s", capabilities.Name)
		message += fmt.Sprintf("\n🧠 Model: %s", currentModel)
		message += fmt.Sprintf("\n🎯 Max Tokens: %d", capabilities.MaxTokens)
		message += fmt.Sprintf("\n📡 Supports Streaming: %v", capabilities.SupportsStreaming)
	}

	return &CommandResult{
		Success: true,
		Message: message,
	}, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
