package chat

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/fatih/color"

	"github.com/simple-container-com/api/pkg/assistant/embeddings"
	"github.com/simple-container-com/api/pkg/assistant/llm/prompts"
)

// registerCommands registers all available chat commands
func (c *ChatInterface) registerCommands() {
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

	c.commands["analyze"] = &ChatCommand{
		Name:        "analyze",
		Description: "Analyze current project tech stack",
		Usage:       "/analyze",
		Handler:     c.handleAnalyze,
		Aliases:     []string{"a"},
	}

	c.commands["setup"] = &ChatCommand{
		Name:        "setup",
		Description: "Generate configuration files for current project",
		Usage:       "/setup [--mode dev|devops]",
		Handler:     c.handleSetup,
		Aliases:     []string{"generate", "g"},
		Args: []CommandArg{
			{Name: "mode", Type: "string", Required: false, Description: "Setup mode", Default: "auto"},
		},
	}

	c.commands["switch"] = &ChatCommand{
		Name:        "switch",
		Description: "Switch between dev and devops modes",
		Usage:       "/switch <mode>",
		Handler:     c.handleSwitch,
		Args: []CommandArg{
			{Name: "mode", Type: "string", Required: true, Description: "Mode to switch to (dev|devops)"},
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
			color.WhiteString(command.Usage))

		if len(command.Args) > 0 {
			message += "\n\nArguments:"
			for _, arg := range command.Args {
				required := ""
				if arg.Required {
					required = " (required)"
				}
				message += fmt.Sprintf("\n  %s: %s%s", arg.Name, arg.Description, required)
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

	// Show general help
	message := prompts.CommandHelpPrompt()
	message += "\n\n" + color.CyanString("Available Commands:")

	for _, command := range c.commands {
		aliases := ""
		if len(command.Aliases) > 0 {
			aliases = fmt.Sprintf(" (aliases: %s)", strings.Join(command.Aliases, ", "))
		}
		message += fmt.Sprintf("\n  /%s%s - %s", command.Name, aliases, command.Description)
	}

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
			Message: "Please provide a search query. Usage: /search <query>",
		}, nil
	}

	query := strings.Join(args[:len(args)-1], " ")
	limit := 5

	// Check if last arg is a number (limit)
	if len(args) > 1 {
		if num, err := strconv.Atoi(args[len(args)-1]); err == nil && num > 0 && num <= 20 {
			query = strings.Join(args[:len(args)-1], " ")
			limit = num
		} else {
			query = strings.Join(args, " ")
		}
	}

	// Perform search
	results, err := embeddings.SearchDocumentation(c.embeddings, query, limit)
	if err != nil {
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("Search failed: %v", err),
		}, nil
	}

	if len(results) == 0 {
		return &CommandResult{
			Success: true,
			Message: "No relevant documentation found. Try different keywords.",
		}, nil
	}

	// Format results
	message := fmt.Sprintf("Found %d results for '%s':", len(results), query)
	for i, result := range results {
		score := int(result.Score * 100)
		title, ok := result.Metadata["title"].(string)
		if !ok || title == "" {
			title = result.ID // Fallback to document ID
		}
		message += fmt.Sprintf("\n\n%d. %s (%d%% match)",
			i+1,
			color.CyanString(title),
			score)
		message += fmt.Sprintf("\n   %s", result.Content[:min(200, len(result.Content))])
		if len(result.Content) > 200 {
			message += "..."
		}
	}

	return &CommandResult{
		Success:  true,
		Message:  message,
		NextStep: "Ask me questions about these topics or use /setup to generate configurations",
	}, nil
}

// handleAnalyze analyzes the current project
func (c *ChatInterface) handleAnalyze(ctx context.Context, args []string, context *ConversationContext) (*CommandResult, error) {
	if context.ProjectPath == "" {
		return &CommandResult{
			Success: false,
			Message: "No project path configured. Please restart with a project path or use 'sc assistant chat /path/to/project'",
		}, nil
	}

	// Re-analyze project
	projectInfo, err := c.analyzer.AnalyzeProject(context.ProjectPath)
	if err != nil {
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("Failed to analyze project: %v", err),
		}, nil
	}

	context.ProjectInfo = projectInfo
	context.UpdatedAt = context.UpdatedAt

	// Format analysis results
	message := fmt.Sprintf("Project Analysis for %s:", color.CyanString(projectInfo.Name))
	message += fmt.Sprintf("\n📍 Path: %s", projectInfo.Path)

	if projectInfo.PrimaryStack != nil {
		stack := projectInfo.PrimaryStack
		message += fmt.Sprintf("\n🎯 Primary Stack: %s (%s) - %.0f%% confidence",
			stack.Language, stack.Framework, stack.Confidence*100)
	}

	if len(projectInfo.TechStacks) > 1 {
		message += "\n\n📊 All detected stacks:"
		for _, stack := range projectInfo.TechStacks {
			message += fmt.Sprintf("\n  - %s (%s) - %.0f%%",
				stack.Language, stack.Framework, stack.Confidence*100)
		}
	}

	if len(projectInfo.Files) > 0 {
		message += "\n\n📦 Key Files:"
		for _, file := range projectInfo.Files[:min(5, len(projectInfo.Files))] {
			message += fmt.Sprintf("\n  - %s", file.Path)
		}
		if len(projectInfo.Files) > 5 {
			message += fmt.Sprintf("\n  ... and %d more", len(projectInfo.Files)-5)
		}
	}

	// Update system prompt with new context
	contextualPrompt := prompts.GenerateContextualPrompt(context.Mode, projectInfo, context.Resources)
	if len(c.context.History) > 0 {
		c.context.History[0].Content = contextualPrompt
	}

	return &CommandResult{
		Success:  true,
		Message:  message,
		NextStep: "Use /setup to generate configuration files based on this analysis",
	}, nil
}

// handleSetup generates configuration files
func (c *ChatInterface) handleSetup(ctx context.Context, args []string, context *ConversationContext) (*CommandResult, error) {
	if context.ProjectInfo == nil {
		return &CommandResult{
			Success: false,
			Message: "Project not analyzed yet. Run /analyze first.",
		}, nil
	}

	// Determine setup mode
	mode := context.Mode
	for _, arg := range args {
		if arg == "--mode" || strings.HasPrefix(arg, "--mode=") {
			if strings.Contains(arg, "=") {
				mode = strings.Split(arg, "=")[1]
			}
		} else if arg == "dev" || arg == "devops" {
			mode = arg
		}
	}

	var files []GeneratedFile
	var err error

	switch mode {
	case "dev", "developer":
		files, err = c.generateDeveloperFiles(context)
	case "devops":
		files, err = c.generateDevOpsFiles(context)
	default:
		// Auto-detect based on project
		if context.ProjectInfo.PrimaryStack != nil {
			files, err = c.generateDeveloperFiles(context)
		} else {
			return &CommandResult{
				Success: false,
				Message: "Unable to determine setup mode. Specify --mode dev or --mode devops",
			}, nil
		}
	}

	if err != nil {
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("Setup failed: %v", err),
		}, nil
	}

	message := fmt.Sprintf("Generated %d configuration files for %s mode", len(files), mode)

	return &CommandResult{
		Success:  true,
		Message:  message,
		Files:    files,
		NextStep: "Review the generated files and run 'docker-compose up -d' to test locally, then 'sc deploy -e staging' to deploy",
	}, nil
}

// handleSwitch switches between modes
func (c *ChatInterface) handleSwitch(ctx context.Context, args []string, context *ConversationContext) (*CommandResult, error) {
	if len(args) == 0 {
		return &CommandResult{
			Success: false,
			Message: "Please specify a mode: dev or devops",
		}, nil
	}

	newMode := strings.ToLower(args[0])
	if newMode != "dev" && newMode != "devops" && newMode != "developer" {
		return &CommandResult{
			Success: false,
			Message: "Invalid mode. Use 'dev' or 'devops'",
		}, nil
	}

	if newMode == "developer" {
		newMode = "dev"
	}

	oldMode := context.Mode
	context.Mode = newMode

	// Update system prompt
	contextualPrompt := prompts.GenerateContextualPrompt(newMode, context.ProjectInfo, context.Resources)
	if len(c.context.History) > 0 {
		c.context.History[0].Content = contextualPrompt
	}

	message := fmt.Sprintf("Switched from %s mode to %s mode", oldMode, newMode)
	if newMode == "dev" {
		message += "\n\n🚀 Developer Mode: I'll help you set up your application with client.yaml, docker-compose.yaml, and Dockerfile"
	} else {
		message += "\n\n🛠️  DevOps Mode: I'll help you set up infrastructure with server.yaml, secrets.yaml, and shared resources"
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
		Message: "Conversation history cleared. How can I help you?",
	}, nil
}

// handleStatus shows session status
func (c *ChatInterface) handleStatus(ctx context.Context, args []string, context *ConversationContext) (*CommandResult, error) {
	message := fmt.Sprintf("Session Status:")
	message += fmt.Sprintf("\n🆔 Session ID: %s", context.SessionID)
	message += fmt.Sprintf("\n⚡ Mode: %s", strings.ToTitle(context.Mode))
	message += fmt.Sprintf("\n📁 Project: %s", func() string {
		if context.ProjectPath != "" {
			return filepath.Base(context.ProjectPath)
		}
		return "None"
	}())

	if context.ProjectInfo != nil && context.ProjectInfo.PrimaryStack != nil {
		message += fmt.Sprintf("\n🎯 Detected: %s (%s)",
			context.ProjectInfo.PrimaryStack.Language,
			context.ProjectInfo.PrimaryStack.Framework)
	}

	message += fmt.Sprintf("\n💬 Messages: %d", len(context.History))
	message += fmt.Sprintf("\n⏰ Started: %s", context.CreatedAt.Format("15:04:05"))

	if len(context.Resources) > 0 {
		message += fmt.Sprintf("\n🔧 Resources: %s", strings.Join(context.Resources, ", "))
	}

	return &CommandResult{
		Success: true,
		Message: message,
	}, nil
}

// Helper functions

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (c *ChatInterface) generateDeveloperFiles(context *ConversationContext) ([]GeneratedFile, error) {
	// TODO: Implement actual file generation using c.generator
	// This is a placeholder that would integrate with the existing generation package

	files := []GeneratedFile{
		{
			Path:        ".sc/stacks/my-app/client.yaml",
			Type:        "yaml",
			Description: "Simple Container client configuration",
			Generated:   true,
			Content: `schemaVersion: 1.0
stacks:
  my-app:
    parent: infrastructure
    parentEnv: staging
    type: cloud-compose
    config:
      uses: [postgres-db]
      runs: [app]
      scale:
        min: 1
        max: 3
      env:
        NODE_ENV: production
        PORT: "3000"`,
		},
		{
			Path:        "docker-compose.yaml",
			Type:        "yaml",
			Description: "Local development environment",
			Generated:   true,
			Content: `services:
  app:
    build: .
    ports:
      - "3000:3000"
    environment:
      - NODE_ENV=development`,
		},
		{
			Path:        "Dockerfile",
			Type:        "dockerfile",
			Description: "Container image definition",
			Generated:   true,
			Content: `FROM node:18-alpine
WORKDIR /app
COPY package*.json ./
RUN npm ci --production
COPY . .
EXPOSE 3000
CMD ["npm", "start"]`,
		},
	}

	return files, nil
}

func (c *ChatInterface) generateDevOpsFiles(context *ConversationContext) ([]GeneratedFile, error) {
	// TODO: Implement actual DevOps file generation

	files := []GeneratedFile{
		{
			Path:        ".sc/stacks/infrastructure/server.yaml",
			Type:        "yaml",
			Description: "Infrastructure configuration",
			Generated:   true,
			Content: `schemaVersion: 1.0
provisioner:
  type: pulumi
templates:
  ecs-fargate:
    type: aws-ecs-fargate
resources:
  postgres-db:
    type: aws-rds-postgres
    staging:
      name: myapp-staging-db
      instanceClass: db.t3.micro
    production:
      name: myapp-prod-db
      instanceClass: db.t3.small`,
		},
		{
			Path:        ".sc/stacks/infrastructure/secrets.yaml",
			Type:        "yaml",
			Description: "Authentication and secrets",
			Generated:   true,
			Content: `schemaVersion: 1.0
auth:
  aws-account:
    credentials: "${auth:aws}"
values:
  db-password: "secure-password-123"`,
		},
	}

	return files, nil
}
