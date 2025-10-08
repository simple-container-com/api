package chat

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/term"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"gopkg.in/yaml.v2"

	"github.com/simple-container-com/api/pkg/api/logger/color"
	"github.com/simple-container-com/api/pkg/assistant/analysis"
	"github.com/simple-container-com/api/pkg/assistant/config"
	"github.com/simple-container-com/api/pkg/assistant/embeddings"
	"github.com/simple-container-com/api/pkg/assistant/llm"
	"github.com/simple-container-com/api/pkg/assistant/llm/prompts"
	"github.com/simple-container-com/api/pkg/assistant/modes"
	"github.com/simple-container-com/api/pkg/assistant/resources"
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
		Usage:       "/analyze [--full]",
		Handler:     c.handleAnalyze,
		Aliases:     []string{"a"},
		Args: []CommandArg{
			{Name: "full", Type: "flag", Required: false, Description: "Run full analysis (slower but comprehensive)"},
		},
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
		Description: "Switch conversation mode (dev/devops) or change deployment type preference for new setups (cloud-compose/static/single-image)",
		Usage:       "/switch <mode_or_deployment_type>",
		Handler:     c.handleSwitch,
		Args: []CommandArg{
			{Name: "target", Type: "string", Required: true, Description: "Mode (dev|devops) or deployment type preference (cloud-compose|static|single-image)"},
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

	c.commands["config"] = &ChatCommand{
		Name:        "config",
		Description: "Display current Simple Container configuration with AI analysis",
		Usage:       "/config [--type client|server] [--stack <name>] [--explain]",
		Handler:     c.handleConfig,
		Aliases:     []string{"cfg"},
		Args: []CommandArg{
			{Name: "type", Type: "string", Required: false, Description: "Configuration type: 'client' or 'server'", Default: "client"},
			{Name: "stack", Type: "string", Required: false, Description: "Specific stack name (e.g. 'myapp', 'service-name')"},
			{Name: "explain", Type: "boolean", Required: false, Description: "Include AI-powered configuration analysis"},
		},
	}

	c.commands["apikey"] = &ChatCommand{
		Name:        "apikey",
		Description: "Manage LLM provider API keys",
		Usage:       "/apikey <set|delete|status> [provider]",
		Handler:     c.handleAPIKey,
		Args: []CommandArg{
			{Name: "action", Type: "string", Required: true, Description: "Action: set, delete, or status"},
			{Name: "provider", Type: "string", Required: false, Description: "Provider: openai, ollama, anthropic, deepseek, yandex"},
		},
	}

	c.commands["provider"] = &ChatCommand{
		Name:        "provider",
		Description: "Manage LLM provider settings",
		Usage:       "/provider <list|switch|info> [provider]",
		Handler:     c.handleProvider,
		Args: []CommandArg{
			{Name: "action", Type: "string", Required: true, Description: "Action: list, switch, or info"},
			{Name: "provider", Type: "string", Required: false, Description: "Provider name for switch/info"},
		},
	}

	c.commands["model"] = &ChatCommand{
		Name:        "model",
		Description: "Manage LLM model selection",
		Usage:       "/model <list|switch|info> [model]",
		Handler:     c.handleModel,
		Args: []CommandArg{
			{Name: "action", Type: "string", Required: true, Description: "Action: list, switch, or info"},
			{Name: "model", Type: "string", Required: false, Description: "Model name for switch"},
		},
	}

	c.commands["history"] = &ChatCommand{
		Name:        "history",
		Description: "Show command history",
		Usage:       "/history [clear]",
		Handler:     c.handleHistory,
		Args: []CommandArg{
			{Name: "action", Type: "string", Required: false, Description: "Action: clear to clear history"},
		},
	}

	// Configuration modification commands (aligned with MCP tools)
	c.commands["config"] = &ChatCommand{
		Name:        "config",
		Description: "Get current Simple Container configuration",
		Usage:       "/config [client|server] [stack_name]",
		Handler:     c.handleGetConfig,
		Args: []CommandArg{
			{Name: "type", Type: "string", Required: false, Description: "Configuration type: client or server", Default: "client"},
			{Name: "stack", Type: "string", Required: false, Description: "Specific stack name (for client config)"},
		},
	}

	c.commands["addenv"] = &ChatCommand{
		Name:        "addenv",
		Description: "Add new environment/stack to client.yaml",
		Usage:       "/addenv <stack_name> <deployment_type> <parent> <parent_env>",
		Handler:     c.handleAddEnvironment,
		Args: []CommandArg{
			{Name: "stack_name", Type: "string", Required: true, Description: "Name of the new stack/environment"},
			{Name: "deployment_type", Type: "string", Required: true, Description: "Deployment type: static, single-image, or cloud-compose"},
			{Name: "parent", Type: "string", Required: true, Description: "Parent stack reference (project/stack format)"},
			{Name: "parent_env", Type: "string", Required: true, Description: "Parent environment to map to"},
		},
	}

	c.commands["modifystack"] = &ChatCommand{
		Name:        "modifystack",
		Description: "Modify existing stack environment configuration in client.yaml files (not for changing deployment preferences - use /switch for that). Use this to modify environment properties like parent stack references, resource usage, scaling, etc. The stack_name refers to the directory in .sc/stacks/<stack-name>, and environment_name is the key in the stacks section (staging, prod, etc).",
		Usage:       "/modifystack <stack_name> <environment_name> <key=value> [key=value...]",
		Handler:     c.handleModifyStack,
		Args: []CommandArg{
			{Name: "stack_name", Type: "string", Required: true, Description: "Name of the stack directory in .sc/stacks/<stack-name>"},
			{Name: "environment_name", Type: "string", Required: true, Description: "Environment name (staging, prod, dev, etc.) - the key in client.yaml stacks section"},
			{Name: "parent", Type: "string", Required: false, Description: "Parent stack reference (e.g. 'infrastructure', 'mycompany/shared')"},
			{Name: "parentEnv", Type: "string", Required: false, Description: "Parent environment to map to (e.g. 'staging', 'prod', 'shared')"},
			{Name: "type", Type: "string", Required: false, Description: "Deployment type (cloud-compose, static, single-image)"},
			{Name: "config.uses", Type: "string", Required: false, Description: "Comma-separated list of resources the stack should use (e.g. 'postgres,redis' or empty '' to remove all)"},
			{Name: "config.scale.min", Type: "string", Required: false, Description: "Minimum number of instances"},
			{Name: "config.scale.max", Type: "string", Required: false, Description: "Maximum number of instances"},
			{Name: "config.env", Type: "string", Required: false, Description: "Environment variables in key=value format"},
			{Name: "config.secrets", Type: "string", Required: false, Description: "Secret references in key=value format"},
			{Name: "config.ports", Type: "string", Required: false, Description: "Port mappings (e.g. '8080:80,9000:9000')"},
			{Name: "config.healthCheck", Type: "string", Required: false, Description: "Health check endpoint path"},
		},
	}

	c.commands["addresource"] = &ChatCommand{
		Name:        "addresource",
		Description: "Add new resource to server.yaml",
		Usage:       "/addresource <resource_name> <resource_type> <environment>",
		Handler:     c.handleAddResource,
		Args: []CommandArg{
			{Name: "resource_name", Type: "string", Required: true, Description: "Name of the resource"},
			{Name: "resource_type", Type: "string", Required: true, Description: "Type of resource (e.g., mongodb-atlas, redis)"},
			{Name: "environment", Type: "string", Required: true, Description: "Environment to add resource to"},
		},
	}

	// Missing commands for MCP parity
	c.commands["context"] = &ChatCommand{
		Name:        "context",
		Description: "Get basic project context information",
		Usage:       "/context",
		Handler:     c.handleGetProjectContext,
		Args:        []CommandArg{},
	}

	c.commands["resources"] = &ChatCommand{
		Name:        "resources",
		Description: "List all supported Simple Container resources",
		Usage:       "/resources",
		Handler:     c.handleGetSupportedResources,
		Args:        []CommandArg{},
	}

	// Search docs command
	c.commands["search_docs"] = &ChatCommand{
		Name:        "search_docs",
		Description: "Search Simple Container documentation for specific information",
		Usage:       "/search_docs <query>",
		Handler:     c.handleSearchDocs,
		Args: []CommandArg{
			{Name: "query", Type: "string", Required: true, Description: "Search query for documentation"},
		},
	}

	// Theme command
	c.commands["theme"] = &ChatCommand{
		Name:        "theme",
		Description: "Change chat color theme",
		Usage:       "/theme [list|set <name>]",
		Handler:     c.handleTheme,
		Args: []CommandArg{
			{Name: "action", Type: "string", Required: false, Description: "Action: list or set"},
			{Name: "name", Type: "string", Required: false, Description: "Theme name for set action"},
		},
	}

	// Sessions command
	c.commands["sessions"] = &ChatCommand{
		Name:        "sessions",
		Description: "Manage chat sessions",
		Usage:       "/sessions [list|new|delete [<id>|all]|config <max>]",
		Handler:     c.handleSessions,
		Args: []CommandArg{
			{Name: "action", Type: "string", Required: false, Description: "Action: list, new, delete, or config"},
			{Name: "value", Type: "string", Required: false, Description: "Session ID, 'all' (for delete all), or max sessions count"},
		},
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

	// Default: use all args as query
	query := strings.Join(args, " ")
	limit := 5

	// Check if last arg is a number (limit)
	if len(args) > 1 {
		if num, err := strconv.Atoi(args[len(args)-1]); err == nil && num > 0 && num <= 20 {
			// Last argument is a valid limit, use remaining args as query
			query = strings.Join(args[:len(args)-1], " ")
			limit = num
		}
		// If last arg is not a valid number, use all args as query (already set above)
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

	// Check for --full flag
	fullAnalysis := false
	for _, arg := range args {
		if arg == "--full" || arg == "-f" {
			fullAnalysis = true
			break
		}
	}

	// Configure analyzer based on mode
	if fullAnalysis {
		fmt.Printf("🔍 Running comprehensive analysis (this may take longer)...\n")
		c.analyzer.EnableFullAnalysis()
		// Set up progress reporter for full analysis
		progressReporter := analysis.NewStreamingProgressReporter(os.Stdout)
		c.analyzer.SetProgressReporter(progressReporter)
	} else {
		fmt.Printf("🔍 Running quick analysis...\n")
		// Ensure we're in QuickMode
		c.analyzer.SetAnalysisMode(analysis.QuickMode)
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
		// Add deployment type confirmation for developer mode
		if err := c.confirmDeploymentTypeForChat(context); err != nil {
			return &CommandResult{
				Success: false,
				Message: fmt.Sprintf("Setup cancelled: %v", err),
			}, nil
		}
		files, err = c.generateDeveloperFiles(context)
	case "devops":
		files, err = c.generateDevOpsFiles(context)
	default:
		// Auto-detect based on project
		if context.ProjectInfo.PrimaryStack != nil {
			// Add deployment type confirmation for auto-detected developer mode
			if err := c.confirmDeploymentTypeForChat(context); err != nil {
				return &CommandResult{
					Success: false,
					Message: fmt.Sprintf("Setup cancelled: %v", err),
				}, nil
			}
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
		NextStep: "Review the generated files and run 'docker-compose up -d' from ${project:root} to test locally, then 'sc deploy -s ${project:name} -e staging' to deploy",
	}, nil
}

// handleSwitch switches between modes or deployment types
func (c *ChatInterface) handleSwitch(ctx context.Context, args []string, context *ConversationContext) (*CommandResult, error) {
	if len(args) == 0 {
		return &CommandResult{
			Success: false,
			Message: "Please specify a mode (dev|devops) or deployment type (cloud-compose|static|single-image)",
		}, nil
	}

	target := strings.ToLower(args[0])

	// Check if it's a mode switch
	validModes := []string{"dev", "devops", "developer"}
	isMode := false
	for _, mode := range validModes {
		if target == mode {
			isMode = true
			break
		}
	}

	// Check if it's a deployment type switch
	validDeploymentTypes := []string{"cloud-compose", "static", "single-image"}
	isDeploymentType := false
	for _, deployType := range validDeploymentTypes {
		if target == deployType {
			isDeploymentType = true
			break
		}
	}

	if !isMode && !isDeploymentType {
		return &CommandResult{
			Success: false,
			Message: "Invalid target. Use mode (dev|devops) or deployment type (cloud-compose|static|single-image)",
		}, nil
	}

	if isMode {
		// Handle mode switching
		newMode := target
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

		return &CommandResult{
			Success: true,
			Message: fmt.Sprintf("✅ Switched from %s mode to %s mode", oldMode, newMode),
		}, nil
	} else {
		// Handle deployment type switching
		if context.Metadata == nil {
			context.Metadata = make(map[string]interface{})
		}

		oldDeploymentType := "unknown"
		if confirmedType, exists := context.Metadata["confirmed_deployment_type"]; exists {
			if typeStr, ok := confirmedType.(string); ok {
				oldDeploymentType = typeStr
			}
		}

		// Update deployment type in context
		context.Metadata["confirmed_deployment_type"] = target

		// Provide description of the new deployment type
		var description string
		switch target {
		case "cloud-compose":
			description = "🐳 Multi-container deployment (docker-compose based) - Best for: Full-stack apps, databases, complex services"
		case "static":
			description = "📄 Static site deployment (HTML/CSS/JS files) - Best for: React, Vue, Angular, static sites"
		case "single-image":
			description = "🚀 Single container deployment (serverless/lambda style) - Best for: AWS Lambda, simple APIs, microservices"
		}

		return &CommandResult{
			Success: true,
			Message: fmt.Sprintf("✅ Switched deployment type from %s to %s\n%s\n\n💡 Use `/setup` to regenerate configuration with the new deployment type", oldDeploymentType, target, description),
		}, nil
	}
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
	message := "Session Status:"
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

// handleConfig displays current Simple Container configuration
func (c *ChatInterface) handleConfig(ctx context.Context, args []string, context *ConversationContext) (*CommandResult, error) {
	// Parse arguments
	configType := "client"
	stackName := ""
	explainFlag := false

	for i, arg := range args {
		if arg == "--type" && i+1 < len(args) {
			configType = args[i+1]
		} else if arg == "--stack" && i+1 < len(args) {
			stackName = args[i+1]
		} else if arg == "--explain" {
			explainFlag = true
		}
	}

	// Additional validation and sanitization for common LLM errors
	switch configType {
	case "client", "server":
		// Valid types, keep as-is
	case "--type":
		// LLM sometimes provides the flag name instead of value
		configType = "client"
	default:
		// Invalid type, default to client
		configType = "client"
	}

	// Get configuration content directly from filesystem
	result, configContent, err := c.getConfigurationContentWithRaw(configType, stackName)
	if err != nil {
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("❌ Failed to read configuration: %v", err),
		}, nil
	}

	// If --explain flag is provided and LLM is available, add AI analysis
	if explainFlag && c.llm != nil {
		fmt.Printf("🤔 Analyzing configuration with AI...\n")

		explanation, err := c.generateConfigurationExplanation(ctx, configType, stackName, string(configContent), context)
		if err != nil {
			// Don't fail the command, just show a warning
			fmt.Printf("⚠️ AI analysis failed: %v\n", err)
		} else if explanation != "" {
			result += "\n\n🤖 **AI Configuration Analysis**\n" + explanation
		}
	} else if explainFlag && c.llm == nil {
		result += "\n\n💡 **AI Analysis Unavailable**: Connect an LLM provider to get intelligent configuration analysis"
	}

	return &CommandResult{
		Success: true,
		Message: result,
	}, nil
}

// Helper function to get configuration content with raw data
func (c *ChatInterface) getConfigurationContentWithRaw(configType, stackName string) (string, []byte, error) {
	var message strings.Builder

	// Determine configuration file path based on type
	var configPath string
	if configType == "client" {
		if stackName != "" {
			configPath = filepath.Join(".sc", "stacks", stackName, "client.yaml")
		} else {
			// Try to find any client.yaml file
			projectName := "myapp"
			if c.context.ProjectPath != "" {
				projectName = filepath.Base(c.context.ProjectPath)
			}
			configPath = filepath.Join(".sc", "stacks", projectName, "client.yaml")
		}
	} else {
		configPath = filepath.Join(".sc", "stacks", "infrastructure", "server.yaml")
	}

	// Check if file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return fmt.Sprintf("❌ Configuration file not found: %s\n\n💡 Use `/setup` to generate configuration files", configPath), nil, nil
	}

	// Read configuration file
	content, err := os.ReadFile(configPath)
	if err != nil {
		return "", nil, fmt.Errorf("failed to read configuration file: %w", err)
	}

	// Build result message
	message.WriteString(fmt.Sprintf("📋 **%s Configuration**\n", cases.Title(language.English).String(configType)))
	message.WriteString(fmt.Sprintf("📁 **File**: %s\n\n", color.CyanFmt(configPath)))

	// Generate summary
	summary, err := c.generateSimpleConfigSummary(configType, content)
	if err == nil && summary != "" {
		message.WriteString(fmt.Sprintf("📊 **Summary**\n%s\n", summary))
	}

	// Display configuration content with line numbers
	message.WriteString("📄 **Configuration Content**\n")
	message.WriteString("```yaml\n")

	lines := strings.Split(string(content), "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) != "" {
			message.WriteString(fmt.Sprintf("%3d: %s\n", i+1, line))
		}
	}
	message.WriteString("```")

	return message.String(), content, nil
}

// Generate a simple summary of the configuration
func (c *ChatInterface) generateSimpleConfigSummary(configType string, content []byte) (string, error) {
	// Parse YAML content
	var config map[string]interface{}
	if err := yaml.Unmarshal(content, &config); err != nil {
		return "", err
	}

	var summary strings.Builder

	if configType == "client" {
		// Client configuration summary
		if stacks, ok := config["stacks"].(map[string]interface{}); ok {
			var stackNames []string
			templates := make(map[string]bool)
			resources := make(map[string]bool)

			for stackName, stackConfig := range stacks {
				stackNames = append(stackNames, stackName)

				if stack, ok := stackConfig.(map[string]interface{}); ok {
					// Check template type
					if stackType, ok := stack["type"].(string); ok {
						templates[stackType] = true
					}

					// Check resource usage
					if stackConfig, ok := stack["config"].(map[string]interface{}); ok {
						if uses, ok := stackConfig["uses"].([]interface{}); ok {
							for _, resource := range uses {
								if resourceStr, ok := resource.(string); ok {
									resources[resourceStr] = true
								}
							}
						}
					}
				}
			}

			summary.WriteString(fmt.Sprintf("• **Environments**: %d (%s)\n",
				len(stackNames), strings.Join(stackNames, ", ")))

			if len(templates) > 0 {
				var templateList []string
				for template := range templates {
					templateList = append(templateList, template)
				}
				summary.WriteString(fmt.Sprintf("• **Deployment Types**: %s\n",
					strings.Join(templateList, ", ")))
			}

			if len(resources) > 0 {
				var resourceList []string
				for resource := range resources {
					resourceList = append(resourceList, resource)
				}
				summary.WriteString(fmt.Sprintf("• **Resources Used**: %s\n",
					strings.Join(resourceList, ", ")))
			}
		}
	} else if configType == "server" {
		// Server configuration summary
		if templates, ok := config["templates"].(map[string]interface{}); ok {
			var templateNames []string
			for name := range templates {
				templateNames = append(templateNames, name)
			}
			summary.WriteString(fmt.Sprintf("• **Templates**: %d (%s)\n",
				len(templateNames), strings.Join(templateNames, ", ")))
		}

		if resources, ok := config["resources"].(map[string]interface{}); ok {
			if resourcesSection, ok := resources["resources"].(map[string]interface{}); ok {
				var envNames []string
				for env := range resourcesSection {
					envNames = append(envNames, env)
				}
				summary.WriteString(fmt.Sprintf("• **Resource Environments**: %d (%s)\n",
					len(envNames), strings.Join(envNames, ", ")))
			}
		}

		if provisioner, ok := config["provisioner"].(map[string]interface{}); ok {
			if provType, ok := provisioner["type"].(string); ok {
				summary.WriteString(fmt.Sprintf("• **Provisioner**: %s\n", provType))
			}
		}
	}

	return summary.String(), nil
}

// generateConfigurationExplanation uses LLM to analyze and explain the configuration
func (c *ChatInterface) generateConfigurationExplanation(ctx context.Context, configType, stackName, configContent string, context *ConversationContext) (string, error) {
	// Create a specialized prompt for configuration analysis
	prompt := c.buildConfigAnalysisPrompt(configType, stackName, configContent, context)

	// Create a temporary conversation history for this analysis
	analysisHistory := []llm.Message{
		{Role: "system", Content: prompt},
		{Role: "user", Content: "Please analyze this Simple Container configuration and provide insights."},
	}

	// Call LLM for analysis
	response, err := c.llm.Chat(ctx, analysisHistory)
	if err != nil {
		return "", fmt.Errorf("failed to generate configuration analysis: %w", err)
	}

	return response.Content, nil
}

// buildConfigAnalysisPrompt creates a specialized prompt for configuration analysis
func (c *ChatInterface) buildConfigAnalysisPrompt(configType, stackName, configContent string, context *ConversationContext) string {
	var prompt strings.Builder

	prompt.WriteString("You are a Simple Container expert analyzing configuration files. ")
	prompt.WriteString("Provide a comprehensive but concise analysis of this configuration. ")
	prompt.WriteString("Focus on practical insights, potential issues, and recommendations.\n\n")

	prompt.WriteString("**Analysis Guidelines:**\n")
	prompt.WriteString("- Explain what this configuration does in plain language\n")
	prompt.WriteString("- Identify the deployment pattern and architecture\n")
	prompt.WriteString("- Point out any potential issues or missing components\n")
	prompt.WriteString("- Suggest improvements or best practices\n")
	prompt.WriteString("- Explain resource dependencies and relationships\n")
	prompt.WriteString("- Note any security considerations\n\n")

	// Add context about the configuration
	prompt.WriteString(fmt.Sprintf("**Configuration Type:** %s\n", configType))
	if stackName != "" {
		prompt.WriteString(fmt.Sprintf("**Stack Name:** %s\n", stackName))
	}

	// Add project context if available
	if context.ProjectInfo != nil && context.ProjectInfo.PrimaryStack != nil {
		prompt.WriteString(fmt.Sprintf("**Detected Project:** %s (%s)\n",
			context.ProjectInfo.PrimaryStack.Language,
			context.ProjectInfo.PrimaryStack.Framework))
	}

	prompt.WriteString("\n**Configuration Content:**\n")
	prompt.WriteString("```yaml\n")
	prompt.WriteString(configContent)
	prompt.WriteString("\n```\n\n")

	prompt.WriteString("Provide your analysis in a clear, structured format using markdown. ")
	prompt.WriteString("Use appropriate emojis for visual clarity but don't overuse them.")

	return prompt.String()
}

// Helper functions

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (c *ChatInterface) generateDeveloperFiles(context *ConversationContext) ([]GeneratedFile, error) {
	// Use DeveloperMode for consistent file generation
	projectPath := "."
	if context.ProjectPath != "" {
		projectPath = context.ProjectPath
	}

	// Initialize metadata if it doesn't exist
	if context.Metadata == nil {
		context.Metadata = make(map[string]interface{})
	}

	// Get confirmed deployment type from context (if available)
	var deploymentType string
	if confirmedType, exists := context.Metadata["confirmed_deployment_type"]; exists {
		if typeStr, ok := confirmedType.(string); ok {
			deploymentType = typeStr
		}
	}

	// Create SetupOptions for DeveloperMode
	opts := &modes.SetupOptions{
		Interactive:    false, // Chat interface handles interactivity differently
		Environment:    "staging",
		Parent:         "infrastructure",
		SkipAnalysis:   false,
		SkipDockerfile: false,
		SkipCompose:    false,
		OutputDir:      projectPath,
		GenerateAll:    false,
		UseStreaming:   true,           // Enable streaming for better UX in chat mode
		DeploymentType: deploymentType, // Use the confirmed deployment type
	}

	// Capture the generated files by temporarily redirecting the DeveloperMode output
	// We'll use a custom approach to get the generated content without writing files
	return c.generateFilesUsingDeveloperMode(context, opts)
}

func (c *ChatInterface) generateFilesUsingDeveloperMode(context *ConversationContext, opts *modes.SetupOptions) ([]GeneratedFile, error) {
	// Get or create project analysis
	var projectAnalysis *analysis.ProjectAnalysis
	if context.ProjectInfo != nil {
		projectAnalysis = context.ProjectInfo
	} else {
		// Add progress reporting for better UX during analysis
		fmt.Printf("🔄 Analyzing project for configuration generation...\n")
		progressReporter := analysis.NewStreamingProgressReporter(os.Stdout)
		c.analyzer.SetProgressReporter(progressReporter)

		var err error
		projectAnalysis, err = c.analyzer.AnalyzeProject(opts.OutputDir)
		if err != nil {
			// Use a basic fallback analysis
			projectAnalysis = &analysis.ProjectAnalysis{
				Name: "my-app",
				PrimaryStack: &analysis.TechStackInfo{
					Language: "javascript",
				},
			}
		}
	}

	// Determine proper project name for file paths
	projectName := projectAnalysis.Name
	if projectName == "." || projectName == "" {
		if absPath, err := filepath.Abs(opts.OutputDir); err == nil {
			projectName = filepath.Base(absPath)
		} else {
			projectName = "my-app"
		}
	}

	files := []GeneratedFile{}

	// Start all file generations in parallel
	fmt.Printf("📄 Generating configuration files in parallel...\n")

	type fileGenResult struct {
		file GeneratedFile
		err  error
		name string
	}

	var wg sync.WaitGroup
	resultChan := make(chan fileGenResult, 3)

	// Generate client.yaml
	wg.Add(1)
	go func() {
		defer wg.Done()
		fmt.Printf("   📄 Generating client.yaml...\n")
		clientYaml, err := c.developerMode.GenerateClientYAMLWithLLM(opts, projectAnalysis)
		result := fileGenResult{name: "client.yaml", err: err}
		if err == nil {
			result.file = GeneratedFile{
				Path:        ".sc/stacks/" + projectName + "/client.yaml",
				Type:        "yaml",
				Description: "Simple Container client configuration",
				Generated:   true,
				Content:     clientYaml,
			}
		}
		resultChan <- result
	}()

	// Generate docker-compose.yaml
	if !opts.SkipCompose {
		wg.Add(1)
		go func() {
			defer wg.Done()
			fmt.Printf("   🐳 Generating docker-compose.yaml...\n")
			composeYaml, err := c.developerMode.GenerateComposeYAMLWithLLM(projectAnalysis)
			result := fileGenResult{name: "docker-compose.yaml", err: err}
			if err == nil {
				result.file = GeneratedFile{
					Path:        "docker-compose.yaml",
					Type:        "yaml",
					Description: "Local development environment",
					Generated:   true,
					Content:     composeYaml,
				}
			}
			resultChan <- result
		}()
	}

	// Generate Dockerfile
	if !opts.SkipDockerfile {
		wg.Add(1)
		go func() {
			defer wg.Done()
			fmt.Printf("   🐳 Generating Dockerfile...\n")
			dockerfile, err := c.developerMode.GenerateDockerfileWithLLM(projectAnalysis)
			result := fileGenResult{name: "Dockerfile", err: err}
			if err == nil {
				result.file = GeneratedFile{
					Path:        "Dockerfile",
					Type:        "dockerfile",
					Description: "Container image definition",
					Generated:   true,
					Content:     dockerfile,
				}
			}
			resultChan <- result
		}()
	}

	// Wait for all generations to complete and collect results
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Collect results as they complete
	for result := range resultChan {
		if result.err == nil {
			fmt.Printf("   ✅ %s generated\n", result.name)
			files = append(files, result.file)
		} else {
			fmt.Printf("   ⚠️ Failed to generate %s: %v\n", result.name, result.err)
		}
	}

	return files, nil
}

func (c *ChatInterface) generateDevOpsFiles(context *ConversationContext) ([]GeneratedFile, error) {
	// Use DevOps mode to generate infrastructure files
	files := []GeneratedFile{}

	// Generate server.yaml using proper schema structure
	serverContent := `schemaVersion: 1.0

# Provisioner configuration
provisioner:
  type: pulumi
  config:
    state-storage:
      type: s3-bucket
      config:
        bucketName: simple-container-state
        region: us-east-1
    secrets-provider:
      type: aws-kms
      config:
        keyId: "arn:aws:kms:us-east-1:123456789012:key/simple-container-secrets"

# Reusable templates for application teams
templates:
  web-app:
    type: ecs-fargate
    
  api-service:
    type: ecs-fargate

# Secrets management configuration
secrets:
  type: aws-kms
  config:
    keyId: "alias/simple-container"

# CI/CD integration
cicd:
  type: github-actions
  config:
    auth-token: "${secret:GITHUB_TOKEN}"

# Shared infrastructure resources
resources:
  # Domain registrar (optional)
  registrar:
    type: cloudflare
    config:
      credentials: "${secret:CLOUDFLARE_API_TOKEN}"
      accountId: "your-cloudflare-account-id"
      zoneName: "example.com"
  
  # Environment-specific resources
  resources:
    # Staging environment
    staging:
      template: web-app
      resources:
        # Container registry
        app-registry:
          type: ecr-repository
          name: myapp-apps-staging
            
        # Database
        postgres-db:
          type: aws-rds-postgres
          config:
            name: myapp-staging-db
            instanceClass: db.t3.micro
            allocatedStorage: 20
            engineVersion: "15.4"
            username: dbadmin
            password: "${secret:staging-db-password}"
            databaseName: myapp
            
        # Cache
        redis-cache:
          type: aws-elasticache-redis
          config:
            name: myapp-staging-cache
            nodeType: cache.t3.micro
            numCacheNodes: 1
            
        # Storage
        uploads-bucket:
          type: s3-bucket
          config:
            name: myapp-staging-uploads
            allowOnlyHttps: true

    # Production environment
    production:
      template: web-app
      resources:
        # Container registry
        app-registry:
          type: ecr-repository
          name: myapp-apps-prod
            
        # Database with high availability
        postgres-db:
          type: aws-rds-postgres
          config:
            name: myapp-prod-db
            instanceClass: db.r5.large
            allocatedStorage: 100
            multiAZ: true
            backupRetentionPeriod: 7
            engineVersion: "15.4"
            username: dbadmin
            password: "${secret:prod-db-password}"
            databaseName: myapp
            
        # Cache cluster
        redis-cache:
          type: aws-elasticache-redis
          config:
            name: myapp-prod-cache
            nodeType: cache.r5.large
            numCacheNodes: 3
            
        # Storage
        uploads-bucket:
          type: s3-bucket
          config:
            name: myapp-prod-uploads
            allowOnlyHttps: true

# Configuration variables
variables:
  app-prefix:
    type: string
    value: myapp`

	files = append(files, GeneratedFile{
		Path:        ".sc/stacks/infrastructure/server.yaml",
		Type:        "yaml",
		Description: "Infrastructure configuration",
		Generated:   true,
		Content:     serverContent,
	})

	// Generate secrets.yaml
	secretsContent := `# Simple Container secrets configuration
schemaVersion: 1.0

# Authentication for cloud providers
auth:
  aws:
    type: aws-token
    config:
      account: "123456789012"
      accessKey: "AKIA..."  # Replace with actual AWS access key
      secretAccessKey: "wJa..."  # Replace with actual AWS secret key
      region: us-east-1

# Secret values (managed with sc secrets add)
values:
  # Database passwords
  staging-db-password: "${STAGING_DB_PASSWORD}"
  prod-db-password: "${PROD_DB_PASSWORD}"
  
  # Cloud credentials  
  aws-access-key: "${AWS_ACCESS_KEY}"
  aws-secret-key: "${AWS_SECRET_KEY}"
  
  # Application secrets
  jwt-secret: "${JWT_SECRET}"`

	files = append(files, GeneratedFile{
		Path:        ".sc/stacks/infrastructure/secrets.yaml",
		Type:        "yaml",
		Description: "Authentication and secrets",
		Generated:   true,
		Content:     secretsContent,
	})

	return files, nil
}

// handleAPIKey manages LLM provider API key storage
func (c *ChatInterface) handleAPIKey(ctx context.Context, args []string, context *ConversationContext) (*CommandResult, error) {
	if len(args) == 0 {
		return &CommandResult{
			Success: false,
			Message: "Please specify an action: set, delete, or status\nUsage: /apikey <set|delete|status> [provider]",
		}, nil
	}

	action := strings.ToLower(args[0])

	// Load config first
	cfg, err := config.Load()
	if err != nil {
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("Failed to load config: %v", err),
		}, nil
	}

	// Determine provider
	var provider string
	if len(args) > 1 {
		// Provider specified in command
		provider = strings.ToLower(args[1])
		if !config.IsValidProvider(provider) {
			return &CommandResult{
				Success: false,
				Message: fmt.Sprintf("Invalid provider: %s\nValid providers: openai, ollama, anthropic, deepseek, yandex", args[1]),
			}, nil
		}
	} else if action == "set" {
		// No provider specified for 'set' - show interactive menu
		selectedProvider, err := c.selectProvider(cfg)
		if err != nil {
			return &CommandResult{
				Success: false,
				Message: fmt.Sprintf("Failed to select provider: %v", err),
			}, nil
		}
		if selectedProvider == "" {
			return &CommandResult{
				Success: false,
				Message: "No provider selected",
			}, nil
		}
		provider = selectedProvider
	} else {
		// For other actions, use default provider
		provider = cfg.GetDefaultProvider()
		if provider == "" {
			provider = config.ProviderOpenAI
		}
	}

	switch action {
	case "set":
		providerName := config.GetProviderDisplayName(provider)

		// For Ollama, API key is optional
		var apiKey string
		var err error
		if provider == config.ProviderOllama {
			fmt.Print(color.CyanString(fmt.Sprintf("🔑 Enter your %s API key (press Enter to skip for local instance): ", providerName)))
		} else {
			fmt.Print(color.CyanString(fmt.Sprintf("🔑 Enter your %s API key: ", providerName)))
		}

		apiKey, err = readSecureInput()
		if err != nil {
			return &CommandResult{
				Success: false,
				Message: fmt.Sprintf("Failed to read API key: %v", err),
			}, nil
		}

		// API key is required for all providers except Ollama
		if apiKey == "" && provider != config.ProviderOllama {
			return &CommandResult{
				Success: false,
				Message: "API key cannot be empty",
			}, nil
		}

		// For Ollama, also ask for base URL
		providerCfg := config.ProviderConfig{APIKey: apiKey}
		if provider == config.ProviderOllama {
			fmt.Print(color.CyanString("🌐 Enter Ollama base URL (press Enter for http://localhost:11434): "))
			reader := bufio.NewReader(os.Stdin)
			baseURL, _ := reader.ReadString('\n')
			baseURL = strings.TrimSpace(baseURL)
			if baseURL == "" {
				baseURL = "http://localhost:11434"
			}
			providerCfg.BaseURL = baseURL

			fmt.Print(color.CyanString("🤖 Enter default model (press Enter for llama2): "))
			model, _ := reader.ReadString('\n')
			model = strings.TrimSpace(model)
			if model == "" {
				model = "llama2"
			}
			providerCfg.Model = model
		}

		// Save provider config
		if err := cfg.SetProviderConfig(provider, providerCfg); err != nil {
			return &CommandResult{
				Success: false,
				Message: fmt.Sprintf("Failed to save API key: %v", err),
			}, nil
		}

		// Set as default provider
		if err := cfg.SetDefaultProvider(provider); err != nil {
			return &CommandResult{
				Success: false,
				Message: fmt.Sprintf("Failed to set default provider: %v", err),
			}, nil
		}

		// Reload LLM provider immediately
		if err := c.ReloadLLMProvider(); err != nil {
			configPath, _ := config.ConfigPath()
			return &CommandResult{
				Success: false,
				Message: fmt.Sprintf("⚠️  %s API key saved to %s but failed to reload: %v\nPlease use '/provider switch %s' to activate.", providerName, configPath, err, provider),
			}, nil
		}

		configPath, _ := config.ConfigPath()
		return &CommandResult{
			Success:  true,
			Message:  fmt.Sprintf("✅ %s API key saved to %s and activated successfully!\nYou can now chat with %s.", providerName, configPath, providerName),
			NextStep: "Start chatting or use '/model list' to see available models",
		}, nil

	case "delete", "remove":
		if !cfg.HasProviderConfig(provider) {
			providerName := config.GetProviderDisplayName(provider)
			return &CommandResult{
				Success: false,
				Message: fmt.Sprintf("No API key is stored for %s", providerName),
			}, nil
		}

		if err := cfg.DeleteProviderConfig(provider); err != nil {
			return &CommandResult{
				Success: false,
				Message: fmt.Sprintf("Failed to delete API key: %v", err),
			}, nil
		}

		providerName := config.GetProviderDisplayName(provider)
		return &CommandResult{
			Success: true,
			Message: fmt.Sprintf("✅ %s API key deleted successfully", providerName),
		}, nil

	case "status", "show":
		// Show status for specific provider or all
		if len(args) > 1 {
			// Show specific provider
			if cfg.HasProviderConfig(provider) {
				providerCfg, _ := cfg.GetProviderConfig(provider)
				masked := maskAPIKey(providerCfg.APIKey)
				providerName := config.GetProviderDisplayName(provider)
				message := fmt.Sprintf("✅ %s API key is configured: %s", providerName, masked)
				if providerCfg.BaseURL != "" {
					message += fmt.Sprintf("\n   Base URL: %s", providerCfg.BaseURL)
				}
				if providerCfg.Model != "" {
					message += fmt.Sprintf("\n   Default Model: %s", providerCfg.Model)
				}
				configPath, _ := config.ConfigPath()
				message += fmt.Sprintf("\n   Stored in: %s", configPath)
				return &CommandResult{
					Success: true,
					Message: message,
				}, nil
			}
			providerName := config.GetProviderDisplayName(provider)
			return &CommandResult{
				Success: false,
				Message: fmt.Sprintf("❌ No API key is stored for %s\nUse '/apikey set %s' to configure one", providerName, provider),
			}, nil
		}

		// Show all configured providers
		providers := cfg.ListProviders()
		if len(providers) == 0 {
			return &CommandResult{
				Success: false,
				Message: "❌ No API keys are currently stored\nUse '/apikey set [provider]' to configure one",
			}, nil
		}

		message := "📋 Configured Providers:\n"
		defaultProvider := cfg.GetDefaultProvider()
		for _, p := range providers {
			providerCfg, _ := cfg.GetProviderConfig(p)
			masked := maskAPIKey(providerCfg.APIKey)
			providerName := config.GetProviderDisplayName(p)
			defaultMark := ""
			if p == defaultProvider {
				defaultMark = " (default)"
			}
			message += fmt.Sprintf("\n  • %s%s: %s", providerName, defaultMark, masked)
			if providerCfg.BaseURL != "" {
				message += fmt.Sprintf("\n    Base URL: %s", providerCfg.BaseURL)
			}
		}
		configPath, _ := config.ConfigPath()
		message += fmt.Sprintf("\n\nStored in: %s", configPath)

		return &CommandResult{
			Success: true,
			Message: message,
		}, nil

	default:
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("Unknown action: %s\nValid actions: set, delete, status", action),
		}, nil
	}
}

// handleProvider manages LLM provider settings
func (c *ChatInterface) handleProvider(ctx context.Context, args []string, context *ConversationContext) (*CommandResult, error) {
	if len(args) == 0 {
		return &CommandResult{
			Success: false,
			Message: "Please specify an action: list, switch, or info\nUsage: /provider <list|switch|info> [provider]",
		}, nil
	}

	action := strings.ToLower(args[0])
	cfg, err := config.Load()
	if err != nil {
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("Failed to load config: %v", err),
		}, nil
	}

	switch action {
	case "list":
		providers := cfg.ListProviders()
		if len(providers) == 0 {
			return &CommandResult{
				Success: false,
				Message: "❌ No providers configured\nUse '/apikey set [provider]' to configure a provider",
			}, nil
		}

		message := "📋 Available Providers:\n"
		defaultProvider := cfg.GetDefaultProvider()
		for _, p := range providers {
			providerName := config.GetProviderDisplayName(p)
			defaultMark := ""
			if p == defaultProvider {
				defaultMark = " ⭐ (current)"
			}
			message += fmt.Sprintf("\n  • %s%s", providerName, defaultMark)
		}
		message += "\n\nUse '/provider switch <provider>' to change the default provider"

		return &CommandResult{
			Success: true,
			Message: message,
		}, nil

	case "switch":
		var provider string

		if len(args) < 2 {
			// No provider specified - show interactive menu
			selectedProvider, err := c.selectConfiguredProvider(cfg)
			if err != nil {
				return &CommandResult{
					Success: false,
					Message: fmt.Sprintf("Failed to select provider: %v", err),
				}, nil
			}
			if selectedProvider == "" {
				return &CommandResult{
					Success: false,
					Message: "No provider selected",
				}, nil
			}
			provider = selectedProvider
		} else {
			// Provider specified directly
			provider = strings.ToLower(args[1])
			if !config.IsValidProvider(provider) {
				return &CommandResult{
					Success: false,
					Message: fmt.Sprintf("Invalid provider: %s\nValid providers: openai, ollama, anthropic, deepseek, yandex", args[1]),
				}, nil
			}

			if !cfg.HasProviderConfig(provider) {
				providerName := config.GetProviderDisplayName(provider)
				return &CommandResult{
					Success: false,
					Message: fmt.Sprintf("❌ %s is not configured\nUse '/apikey set %s' to configure it first", providerName, provider),
				}, nil
			}
		}

		if err := cfg.SetDefaultProvider(provider); err != nil {
			return &CommandResult{
				Success: false,
				Message: fmt.Sprintf("Failed to switch provider: %v", err),
			}, nil
		}

		// Reload LLM provider immediately
		if err := c.ReloadLLMProvider(); err != nil {
			return &CommandResult{
				Success: false,
				Message: fmt.Sprintf("⚠️  Provider switched in config but failed to reload: %v\nPlease restart the chat session.", err),
			}, nil
		}

		providerName := config.GetProviderDisplayName(provider)
		return &CommandResult{
			Success: true,
			Message: fmt.Sprintf("✅ Switched to %s and reloaded successfully!\nYou can continue chatting with the new provider.", providerName),
		}, nil

	case "info":
		provider := cfg.GetDefaultProvider()
		if len(args) > 1 {
			provider = strings.ToLower(args[1])
			if !config.IsValidProvider(provider) {
				return &CommandResult{
					Success: false,
					Message: fmt.Sprintf("Invalid provider: %s", args[1]),
				}, nil
			}
		}

		if provider == "" {
			return &CommandResult{
				Success: false,
				Message: "❌ No default provider set\nUse '/apikey set [provider]' to configure one",
			}, nil
		}

		if !cfg.HasProviderConfig(provider) {
			providerName := config.GetProviderDisplayName(provider)
			return &CommandResult{
				Success: false,
				Message: fmt.Sprintf("❌ %s is not configured", providerName),
			}, nil
		}

		providerCfg, _ := cfg.GetProviderConfig(provider)
		providerName := config.GetProviderDisplayName(provider)
		message := fmt.Sprintf("ℹ️  %s Configuration:\n", providerName)
		message += fmt.Sprintf("\n  Provider: %s", provider)
		message += fmt.Sprintf("\n  API Key: %s", maskAPIKey(providerCfg.APIKey))
		if providerCfg.BaseURL != "" {
			message += fmt.Sprintf("\n  Base URL: %s", providerCfg.BaseURL)
		}
		if providerCfg.Model != "" {
			message += fmt.Sprintf("\n  Default Model: %s", providerCfg.Model)
		}

		return &CommandResult{
			Success: true,
			Message: message,
		}, nil

	default:
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("Unknown action: %s\nValid actions: list, switch, info", action),
		}, nil
	}
}

// readSecureInput reads input securely (hidden) from terminal
func readSecureInput() (string, error) {
	// Check if we're running in a terminal
	if !term.IsTerminal(int(syscall.Stdin)) {
		// Not a terminal, read from stdin normally
		reader := bufio.NewReader(os.Stdin)
		input, err := reader.ReadString('\n')
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(input), nil
	}

	// Read password from terminal (hidden input)
	bytePassword, err := term.ReadPassword(int(syscall.Stdin))
	if err != nil {
		return "", err
	}

	fmt.Println() // Add newline after hidden input
	return strings.TrimSpace(string(bytePassword)), nil
}

// maskAPIKey masks an API key for display purposes
func maskAPIKey(apiKey string) string {
	if len(apiKey) <= 8 {
		return "sk-****"
	}
	return apiKey[:7] + "..." + apiKey[len(apiKey)-4:]
}

// selectProvider shows an interactive menu to select a provider
func (c *ChatInterface) selectProvider(cfg *config.Config) (string, error) {
	// Get all valid providers
	allProviders := []string{
		config.ProviderOpenAI,
		config.ProviderOllama,
		config.ProviderAnthropic,
		config.ProviderDeepseek,
		config.ProviderYandex,
	}

	// Get configured providers
	configuredProviders := cfg.ListProviders()
	configuredMap := make(map[string]bool)
	for _, p := range configuredProviders {
		configuredMap[p] = true
	}

	// Display menu
	fmt.Println(color.CyanString("\n📋 Select a provider to configure:"))
	fmt.Println()

	for i, provider := range allProviders {
		providerName := config.GetProviderDisplayName(provider)
		status := ""
		if configuredMap[provider] {
			status = color.GreenString(" ✓ (configured)")
		} else {
			status = color.YellowString(" (not configured)")
		}
		fmt.Printf("  %d. %s%s\n", i+1, providerName, status)
	}

	fmt.Println()

	// Read user input using inputHandler
	input, err := c.inputHandler.ReadSimple(color.CyanString("Enter number (1-5) or 'q' to cancel: "))
	if err != nil {
		return "", err
	}

	// Check for cancel
	if input == "q" || input == "Q" || input == "quit" || input == "cancel" {
		return "", nil
	}

	// Parse selection
	selection, err := strconv.Atoi(input)
	if err != nil || selection < 1 || selection > len(allProviders) {
		return "", fmt.Errorf("invalid selection: %s", input)
	}

	selectedProvider := allProviders[selection-1]
	fmt.Println(color.GreenString(fmt.Sprintf("✓ Selected: %s", config.GetProviderDisplayName(selectedProvider))))
	fmt.Println()

	return selectedProvider, nil
}

// handleHistory shows or clears command history
func (c *ChatInterface) handleHistory(ctx context.Context, args []string, context *ConversationContext) (*CommandResult, error) {
	if len(args) > 0 && strings.ToLower(args[0]) == "clear" {
		c.inputHandler.ClearHistory()
		return &CommandResult{
			Success: true,
			Message: "✅ Command history cleared",
		}, nil
	}

	history := c.inputHandler.GetHistory()
	if len(history) == 0 {
		return &CommandResult{
			Success: true,
			Message: "No command history yet",
		}, nil
	}

	message := fmt.Sprintf("📜 Command History (%d commands):\n", len(history))

	// Show last 20 commands
	start := 0
	if len(history) > 20 {
		start = len(history) - 20
		message += fmt.Sprintf("\n(Showing last 20 of %d commands)\n", len(history))
	}

	for i := start; i < len(history); i++ {
		message += fmt.Sprintf("\n  %d. %s", i+1, history[i])
	}

	message += "\n\n💡 Tip: Use ↑/↓ arrow keys to navigate history, Tab for autocomplete"

	return &CommandResult{
		Success: true,
		Message: message,
	}, nil
}

// selectConfiguredProvider shows an interactive menu to select from configured providers only
func (c *ChatInterface) selectConfiguredProvider(cfg *config.Config) (string, error) {
	// Get configured providers
	configuredProviders := cfg.ListProviders()

	if len(configuredProviders) == 0 {
		return "", fmt.Errorf("no providers configured. Use '/apikey set' to configure a provider first")
	}

	if len(configuredProviders) == 1 {
		// Only one provider configured, no need to show menu
		return configuredProviders[0], nil
	}

	// Get current default
	defaultProvider := cfg.GetDefaultProvider()

	// Display menu
	fmt.Println(color.CyanString("\n📋 Select a provider to switch to:"))
	fmt.Println()

	for i, provider := range configuredProviders {
		providerName := config.GetProviderDisplayName(provider)
		defaultMark := ""
		if provider == defaultProvider {
			defaultMark = color.YellowString(" ⭐ (current)")
		}
		fmt.Printf("  %d. %s%s\n", i+1, providerName, defaultMark)
	}

	fmt.Println()

	// Read user input using inputHandler to properly handle stdin
	input, err := c.inputHandler.ReadSimple(color.CyanString(fmt.Sprintf("Enter number (1-%d) or 'q' to cancel: ", len(configuredProviders))))
	if err != nil {
		return "", err
	}

	// Check for cancel
	if input == "q" || input == "Q" || input == "quit" || input == "cancel" {
		return "", nil
	}

	// Parse selection
	selection, err := strconv.Atoi(input)
	if err != nil || selection < 1 || selection > len(configuredProviders) {
		return "", fmt.Errorf("invalid selection: %s", input)
	}

	selectedProvider := configuredProviders[selection-1]
	fmt.Println(color.GreenString(fmt.Sprintf("✓ Selected: %s", config.GetProviderDisplayName(selectedProvider))))
	fmt.Println()

	return selectedProvider, nil
}

// confirmDeploymentTypeForChat handles deployment type confirmation in chat interface
func (c *ChatInterface) confirmDeploymentTypeForChat(context *ConversationContext) error {
	// Initialize metadata if it doesn't exist
	if context.Metadata == nil {
		context.Metadata = make(map[string]interface{})
	}
	// Determine deployment type using simple heuristic (since internal method is private)
	detectedType := "cloud-compose" // Default fallback
	if context.ProjectInfo != nil {
		// Simple heuristic based on project analysis
		if context.ProjectInfo.PrimaryStack != nil {
			lang := strings.ToLower(context.ProjectInfo.PrimaryStack.Language)
			if lang == "html" || lang == "javascript" || lang == "typescript" {
				// Check for static site indicators
				detectedType = "static"
			} else if lang == "go" || lang == "python" || lang == "java" {
				detectedType = "single-image"
			}
		}
	}

	// Display detected type with description
	fmt.Printf("🔍 Detected deployment type: %s\n", detectedType)

	switch detectedType {
	case "static":
		fmt.Printf("   📄 Static site deployment (HTML/CSS/JS files)\n")
		fmt.Printf("   💡 Best for: React, Vue, Angular, static sites\n")
	case "single-image":
		fmt.Printf("   🚀 Single container deployment (serverless/lambda style)\n")
		fmt.Printf("   💡 Best for: AWS Lambda, simple APIs, microservices\n")
	case "cloud-compose":
		fmt.Printf("   🐳 Multi-container deployment (docker-compose based)\n")
		fmt.Printf("   💡 Best for: Full-stack apps, databases, complex services\n")
	}

	// Use chat interface's ReadSimple for input (fixes Y/N prompt issue)
	response, err := c.inputHandler.ReadSimple("\n   Is this correct? [Y/n]: ")
	if err != nil {
		// If there's an error reading input, default to "yes" and store the detected type
		context.Metadata["confirmed_deployment_type"] = detectedType
		return nil
	}

	response = strings.ToLower(strings.TrimSpace(response))
	if response == "n" || response == "no" {
		// Let user choose the deployment type using chat interface
		return c.selectDeploymentTypeForChat(context)
	}

	// Store the confirmed deployment type in context for use during generation
	context.Metadata["confirmed_deployment_type"] = detectedType
	return nil
}

// selectDeploymentTypeForChat handles manual deployment type selection in chat interface
func (c *ChatInterface) selectDeploymentTypeForChat(context *ConversationContext) error {
	// Initialize metadata if it doesn't exist
	if context.Metadata == nil {
		context.Metadata = make(map[string]interface{})
	}
	fmt.Printf("\n📋 Available deployment types:\n")
	fmt.Printf("   1. static - Static site (HTML/CSS/JS files)\n")
	fmt.Printf("   2. single-image - Single container (serverless/lambda style)\n")
	fmt.Printf("   3. cloud-compose - Multi-container (docker-compose based)\n")

	// Use chat interface's ReadSimple for menu selection
	response, err := c.inputHandler.ReadSimple("\n   Select deployment type [1-3]: ")
	if err != nil {
		return fmt.Errorf("failed to read selection: %v", err)
	}

	response = strings.TrimSpace(response)

	var selectedType string
	switch response {
	case "1":
		selectedType = "static"
		fmt.Printf("✅ Selected: static\n")
	case "2":
		selectedType = "single-image"
		fmt.Printf("✅ Selected: single-image\n")
	case "3", "":
		selectedType = "cloud-compose"
		fmt.Printf("✅ Selected: cloud-compose\n")
	default:
		selectedType = "cloud-compose"
		fmt.Printf("Invalid selection. Using cloud-compose as default.\n")
	}

	// Store the selected deployment type in context for use during generation
	context.Metadata["confirmed_deployment_type"] = selectedType

	return nil
}

// handleModel handles model management commands
func (c *ChatInterface) handleModel(ctx context.Context, args []string, context *ConversationContext) (*CommandResult, error) {
	cfg, err := config.Load()
	if err != nil {
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("Failed to load config: %v", err),
		}, nil
	}

	if len(args) == 0 {
		return &CommandResult{
			Success: false,
			Message: "Usage: /model <list|switch|info> [model]",
		}, nil
	}

	action := strings.ToLower(args[0])

	switch action {
	case "list":
		// Get current provider
		provider := cfg.GetDefaultProvider()
		if provider == "" {
			return &CommandResult{
				Success: false,
				Message: "No provider configured. Use '/provider switch' first.",
			}, nil
		}

		// Get provider instance to list available models
		providerInstance := llm.GlobalRegistry.Create(provider)
		if providerInstance == nil {
			return &CommandResult{
				Success: false,
				Message: fmt.Sprintf("Provider %s not available", provider),
			}, nil
		}

		// Configure provider to enable API calls
		providerCfg, _ := cfg.GetProviderConfig(provider)
		if err := providerInstance.Configure(llm.Config{
			Provider: provider,
			APIKey:   providerCfg.APIKey,
			BaseURL:  providerCfg.BaseURL,
		}); err != nil {
			return &CommandResult{
				Success: false,
				Message: fmt.Sprintf("Failed to configure provider %s: %v", provider, err),
			}, nil
		}

		// Get models from API
		models, err := providerInstance.ListModels(ctx)
		if err != nil {
			// Fallback to capabilities if API fails
			capabilities := providerInstance.GetCapabilities()
			models = capabilities.Models
		}

		currentModel := providerCfg.Model
		if currentModel == "" {
			currentModel = providerInstance.GetModel()
		}

		capabilities := providerInstance.GetCapabilities()
		message := fmt.Sprintf("📋 Available Models for %s:\n\n", capabilities.Name)
		for i, model := range models {
			mark := ""
			if model == currentModel {
				mark = color.YellowString(" ⭐ (current)")
			}
			message += fmt.Sprintf("  %d. %s%s\n", i+1, model, mark)
		}
		message += "\n💡 Use '/model switch <model>' to change the model"

		return &CommandResult{
			Success: true,
			Message: message,
		}, nil

	case "switch":
		// Get current provider
		provider := cfg.GetDefaultProvider()
		if provider == "" {
			return &CommandResult{
				Success: false,
				Message: "No provider configured. Use '/provider switch' first.",
			}, nil
		}

		var modelName string
		if len(args) > 1 {
			// Model specified directly
			modelName = args[1]
		} else {
			// Show interactive menu
			selectedModel, err := c.selectModel(cfg, provider)
			if err != nil {
				return &CommandResult{
					Success: false,
					Message: fmt.Sprintf("Failed to select model: %v", err),
				}, nil
			}
			if selectedModel == "" {
				return &CommandResult{
					Success: false,
					Message: "No model selected",
				}, nil
			}
			modelName = selectedModel
		}

		// Update provider config with new model
		providerCfg, exists := cfg.GetProviderConfig(provider)
		if !exists {
			return &CommandResult{
				Success: false,
				Message: fmt.Sprintf("Provider %s not configured", provider),
			}, nil
		}

		providerCfg.Model = modelName
		if err := cfg.SetProviderConfig(provider, providerCfg); err != nil {
			return &CommandResult{
				Success: false,
				Message: fmt.Sprintf("Failed to save model: %v", err),
			}, nil
		}

		// Reload provider with new model
		if err := c.ReloadLLMProvider(); err != nil {
			return &CommandResult{
				Success: false,
				Message: fmt.Sprintf("⚠️  Model switched in config but failed to reload: %v\nPlease restart the chat session.", err),
			}, nil
		}

		return &CommandResult{
			Success: true,
			Message: fmt.Sprintf("✅ Switched to model %s and reloaded successfully!", modelName),
		}, nil

	case "info":
		// Get current model info
		provider := cfg.GetDefaultProvider()
		if provider == "" {
			return &CommandResult{
				Success: false,
				Message: "No provider configured",
			}, nil
		}

		providerCfg, _ := cfg.GetProviderConfig(provider)
		currentModel := providerCfg.Model
		if currentModel == "" {
			currentModel = c.llm.GetModel()
		}

		capabilities := c.llm.GetCapabilities()

		message := "ℹ️  Current Model Information:\n\n"
		message += fmt.Sprintf("Provider: %s\n", capabilities.Name)
		message += fmt.Sprintf("Model: %s\n", currentModel)
		message += fmt.Sprintf("Max Tokens: %d\n", capabilities.MaxTokens)
		message += fmt.Sprintf("Supports Streaming: %v\n", capabilities.SupportsStreaming)

		return &CommandResult{
			Success: true,
			Message: message,
		}, nil

	default:
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("Unknown action: %s\nUsage: /model <list|switch|info> [model]", action),
		}, nil
	}
}

// selectModel shows an interactive menu to select a model for current provider
func (c *ChatInterface) selectModel(cfg *config.Config, provider string) (string, error) {
	// Get provider instance
	providerInstance := llm.GlobalRegistry.Create(provider)
	if providerInstance == nil {
		return "", fmt.Errorf("provider %s not available", provider)
	}

	// Configure provider to enable API calls
	providerCfg, _ := cfg.GetProviderConfig(provider)
	if err := providerInstance.Configure(llm.Config{
		Provider: provider,
		APIKey:   providerCfg.APIKey,
		BaseURL:  providerCfg.BaseURL,
	}); err != nil {
		return "", fmt.Errorf("failed to configure provider %s: %w", provider, err)
	}

	// Get models from API
	ctx := context.Background()
	models, err := providerInstance.ListModels(ctx)
	if err != nil {
		// Fallback to capabilities
		capabilities := providerInstance.GetCapabilities()
		models = capabilities.Models
	}

	if len(models) == 0 {
		return "", fmt.Errorf("no models available for provider %s", provider)
	}

	capabilities := providerInstance.GetCapabilities()

	if len(models) == 1 {
		return models[0], nil
	}

	// Get current model
	currentModel := providerCfg.Model

	// Display menu
	fmt.Println(color.CyanString(fmt.Sprintf("\n📋 Select a model for %s:", capabilities.Name)))
	fmt.Println()

	for i, model := range models {
		mark := ""
		if model == currentModel {
			mark = color.YellowString(" ⭐ (current)")
		}
		fmt.Printf("  %d. %s%s\n", i+1, model, mark)
	}

	fmt.Println()

	// Read user input
	input, err := c.inputHandler.ReadSimple(color.CyanString(fmt.Sprintf("Enter number (1-%d) or 'q' to cancel: ", len(models))))
	if err != nil {
		return "", err
	}

	// Check for cancel
	if input == "q" || input == "Q" || input == "quit" || input == "cancel" {
		return "", nil
	}

	// Parse selection
	selection, err := strconv.Atoi(input)
	if err != nil || selection < 1 || selection > len(models) {
		return "", fmt.Errorf("invalid selection: %s", input)
	}

	selectedModel := models[selection-1]
	fmt.Println(color.GreenString(fmt.Sprintf("✓ Selected: %s", selectedModel)))
	fmt.Println()

	return selectedModel, nil
}

// handleGetConfig gets current Simple Container configuration using unified handler
func (c *ChatInterface) handleGetConfig(ctx context.Context, args []string, context *ConversationContext) (*CommandResult, error) {
	if c.commandHandler == nil {
		return &CommandResult{
			Success: false,
			Message: "❌ Command handler not available",
		}, nil
	}

	configType := "client"
	stackName := ""

	// Parse flag-based arguments (same logic as handleConfig)
	for i, arg := range args {
		if arg == "--type" && i+1 < len(args) {
			configType = args[i+1]
		} else if arg == "--stack" && i+1 < len(args) {
			stackName = args[i+1]
		}
	}

	// Additional validation and sanitization
	switch configType {
	case "client", "server":
		// Valid types, keep as-is
	case "--type":
		// LLM sometimes provides the flag name instead of value
		configType = "client"
	default:
		// Invalid type, default to client
		configType = "client"
	}

	// Use unified command handler
	result, err := c.commandHandler.GetCurrentConfig(ctx, configType, stackName)
	if err != nil {
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("❌ Failed to get configuration: %v", err),
		}, nil
	}

	return &CommandResult{
		Success: result.Success,
		Message: result.Message,
		Data:    result.Data, // CRITICAL: Include the actual configuration data
	}, nil
}

// handleAddEnvironment adds a new environment/stack using unified handler
func (c *ChatInterface) handleAddEnvironment(ctx context.Context, args []string, context *ConversationContext) (*CommandResult, error) {
	if c.commandHandler == nil {
		return &CommandResult{
			Success: false,
			Message: "❌ Command handler not available",
		}, nil
	}

	if len(args) < 4 {
		return &CommandResult{
			Success: false,
			Message: "❌ Usage: /addenv <stack_name> <deployment_type> <parent> <parent_env>",
		}, nil
	}

	stackName := args[0]
	deploymentType := args[1]
	parent := args[2]
	parentEnv := args[3]

	// Validate deployment type
	validTypes := []string{"static", "single-image", "cloud-compose"}
	isValid := false
	for _, validType := range validTypes {
		if deploymentType == validType {
			isValid = true
			break
		}
	}
	if !isValid {
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("❌ Invalid deployment type '%s'. Valid types: %v", deploymentType, validTypes),
		}, nil
	}

	// Use unified command handler
	result, err := c.commandHandler.AddEnvironment(ctx, stackName, deploymentType, parent, parentEnv, nil)
	if err != nil {
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("❌ Failed to add environment: %v", err),
		}, nil
	}

	return &CommandResult{
		Success:  result.Success,
		Message:  result.Message,
		NextStep: "Environment added successfully! You can now deploy to this stack.",
	}, nil
}

// handleModifyStack modifies existing stack configuration using unified handler
func (c *ChatInterface) handleModifyStack(ctx context.Context, args []string, context *ConversationContext) (*CommandResult, error) {
	if c.commandHandler == nil {
		return &CommandResult{
			Success: false,
			Message: "❌ Command handler not available",
		}, nil
	}

	if len(args) < 3 {
		return &CommandResult{
			Success: false,
			Message: "❌ Usage: /modifystack <stack_name> <environment_name> <key=value> [key=value...]\n" +
				"Examples:\n" +
				"  /modifystack myapp staging parent=infrastructure\n" +
				"  /modifystack myapp prod parentEnv=production\n" +
				"  /modifystack myapp staging config.uses=postgres,redis\n" +
				"  /modifystack myapp prod config.scale.max=10",
		}, nil
	}

	stackName := args[0]
	environmentName := args[1]
	changes := make(map[string]interface{})

	// Parse key=value pairs
	for _, arg := range args[2:] {
		parts := strings.SplitN(arg, "=", 2)
		if len(parts) != 2 {
			return &CommandResult{
				Success: false,
				Message: fmt.Sprintf("❌ Invalid format '%s'. Use key=value format.", arg),
			}, nil
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Try to parse value as different types
		if value == "true" {
			changes[key] = true
		} else if value == "false" {
			changes[key] = false
		} else if num, err := strconv.Atoi(value); err == nil {
			changes[key] = num
		} else {
			changes[key] = value
		}
	}

	// Use unified command handler
	result, err := c.commandHandler.ModifyStackConfig(ctx, stackName, environmentName, changes)
	if err != nil {
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("❌ Failed to modify stack: %v", err),
		}, nil
	}

	return &CommandResult{
		Success:  result.Success,
		Message:  result.Message,
		NextStep: "Stack configuration updated successfully!",
	}, nil
}

// handleAddResource adds a new resource to server.yaml using unified handler
func (c *ChatInterface) handleAddResource(ctx context.Context, args []string, context *ConversationContext) (*CommandResult, error) {
	if c.commandHandler == nil {
		return &CommandResult{
			Success: false,
			Message: "❌ Command handler not available",
		}, nil
	}

	if len(args) < 3 {
		return &CommandResult{
			Success: false,
			Message: "❌ Usage: /addresource <resource_name> <resource_type> <environment> [key=value...]\n" +
				"Examples:\n" +
				"  /addresource mongodb-prod mongodb-atlas production tier=M10 region=us-east-1\n" +
				"  /addresource redis-cache redis staging",
		}, nil
	}

	resourceName := args[0]
	resourceType := args[1]
	environment := args[2]
	config := make(map[string]interface{})

	// Parse additional key=value pairs for resource configuration
	for _, arg := range args[3:] {
		parts := strings.SplitN(arg, "=", 2)
		if len(parts) != 2 {
			return &CommandResult{
				Success: false,
				Message: fmt.Sprintf("❌ Invalid format '%s'. Use key=value format.", arg),
			}, nil
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Try to parse value as different types
		if value == "true" {
			config[key] = true
		} else if value == "false" {
			config[key] = false
		} else if num, err := strconv.Atoi(value); err == nil {
			config[key] = num
		} else {
			config[key] = value
		}
	}

	// Use unified command handler
	result, err := c.commandHandler.AddResource(ctx, resourceName, resourceType, environment, config)
	if err != nil {
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("❌ Failed to add resource: %v", err),
		}, nil
	}

	return &CommandResult{
		Success:  result.Success,
		Message:  result.Message,
		NextStep: "Resource added successfully! You can now reference it in your application stacks.",
	}, nil
}

// handleGetProjectContext gets basic project context information using unified handler
func (c *ChatInterface) handleGetProjectContext(ctx context.Context, args []string, context *ConversationContext) (*CommandResult, error) {
	if c.commandHandler == nil {
		return &CommandResult{
			Success: false,
			Message: "❌ Command handler not available",
		}, nil
	}

	// Use unified command handler
	result, err := c.commandHandler.GetProjectContext(ctx, ".")
	if err != nil {
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("❌ Failed to get project context: %v", err),
		}, nil
	}

	return &CommandResult{
		Success: result.Success,
		Message: result.Message,
	}, nil
}

// handleGetSupportedResources lists all supported Simple Container resources
func (c *ChatInterface) handleGetSupportedResources(ctx context.Context, args []string, context *ConversationContext) (*CommandResult, error) {
	// Use the resources package for consistency
	supportedResources, err := resources.GetSupportedResourcesFromSchemas(ctx)
	if err != nil {
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("❌ Failed to get supported resources: %v", err),
		}, nil
	}

	// Format the response for chat interface
	message := fmt.Sprintf("📦 **Simple Container Supported Resources** (%d providers, %d resources)\n\n",
		len(supportedResources.Providers), len(supportedResources.Resources))

	// Create a title caser for proper capitalization
	caser := cases.Title(language.English)

	for _, provider := range supportedResources.Providers {
		message += fmt.Sprintf("### %s (%d resources)\n", caser.String(provider.Name), len(provider.Resources))
		for _, resource := range provider.Resources {
			message += fmt.Sprintf("- `%s`\n", resource)
		}
		message += "\n"
	}

	// Add usage information
	message += "**Usage:**\n"
	message += "- These resources can be referenced in `client.yaml` under the `uses` section\n"
	message += "- Format: `uses: [resource-type, another-resource]`\n"

	return &CommandResult{
		Success: true,
		Message: message,
	}, nil
}

// handleSearchDocs searches Simple Container documentation
func (c *ChatInterface) handleSearchDocs(ctx context.Context, args []string, context *ConversationContext) (*CommandResult, error) {
	if len(args) == 0 {
		return &CommandResult{
			Success: false,
			Message: "❌ Usage: /search_docs <query>\nExample: /search_docs mongodb configuration",
		}, nil
	}

	query := strings.Join(args, " ")

	// Check if embeddings are available
	if c.embeddings == nil {
		return &CommandResult{
			Success: false,
			Message: "❌ Documentation search is not available - embeddings database not loaded",
		}, nil
	}

	// Search for relevant documentation (limit to top 5 results for tool calls)
	results, err := embeddings.SearchDocumentation(c.embeddings, query, 5)
	if err != nil {
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("❌ Failed to search documentation: %v", err),
		}, nil
	}

	if len(results) == 0 {
		return &CommandResult{
			Success: true,
			Message: fmt.Sprintf("🔍 No documentation found for query: \"%s\"", query),
		}, nil
	}

	// Format results for LLM consumption
	var response strings.Builder
	response.WriteString(fmt.Sprintf("📚 **Found %d documentation results for \"%s\"**\n\n", len(results), query))

	for i, result := range results {
		score := int(result.Score * 100)
		title := "Unknown"
		if titleInterface, exists := result.Metadata["title"]; exists {
			if titleStr, ok := titleInterface.(string); ok {
				title = titleStr
			}
		}

		response.WriteString(fmt.Sprintf("**%d. %s** (%d%% relevance)\n", i+1, title, score))

		// Include relevant content snippet (truncated to 600 chars for tool response)
		content := result.Content
		if len(content) > 600 {
			content = content[:600] + "..."
		}

		response.WriteString(fmt.Sprintf("```\n%s\n```\n\n", content))
	}

	response.WriteString("💡 **Use this information to provide accurate, specific guidance based on Simple Container documentation.**")

	return &CommandResult{
		Success: true,
		Message: response.String(),
	}, nil
}

// handleTheme handles theme management commands
func (c *ChatInterface) handleTheme(ctx context.Context, args []string, context *ConversationContext) (*CommandResult, error) {
	if len(args) == 0 {
		// Show current theme
		theme := GetCurrentTheme()
		return &CommandResult{
			Success: true,
			Message: fmt.Sprintf("Current theme: %s - %s\nUse '/theme list' to see all themes or '/theme set <name>' to change theme", theme.Name, theme.Description),
		}, nil
	}

	action := strings.ToLower(args[0])

	switch action {
	case "list":
		// List all available themes
		themes := ListThemes()
		currentTheme := GetCurrentTheme()

		message := "📋 Available Themes:\n"
		for _, theme := range themes {
			mark := ""
			if theme.Name == currentTheme.Name {
				mark = " ⭐ (current)"
			}
			// Show theme with example colors
			message += fmt.Sprintf("\n  • %s%s - %s",
				theme.ApplyText(theme.Name),
				mark,
				theme.Description)
			message += fmt.Sprintf("\n    Example: %s %s %s",
				theme.ApplyText("text"),
				theme.ApplyCode("code"),
				theme.ApplyHeader("header"))
		}
		message += "\n\nUse '/theme set <name>' to change theme"

		return &CommandResult{
			Success: true,
			Message: message,
		}, nil

	case "set":
		if len(args) < 2 {
			return &CommandResult{
				Success: false,
				Message: "Please specify a theme name. Use '/theme list' to see available themes",
			}, nil
		}

		themeName := strings.ToLower(args[1])
		theme, err := GetTheme(themeName)
		if err != nil {
			return &CommandResult{
				Success: false,
				Message: fmt.Sprintf("Theme '%s' not found. Use '/theme list' to see available themes", themeName),
			}, nil
		}

		if err := SetCurrentTheme(themeName); err != nil {
			return &CommandResult{
				Success: false,
				Message: fmt.Sprintf("Failed to set theme: %v", err),
			}, nil
		}

		return &CommandResult{
			Success: true,
			Message: fmt.Sprintf("✅ Theme changed to '%s' - %s\n\nExample: %s %s %s",
				theme.Name,
				theme.Description,
				theme.ApplyText("text"),
				theme.ApplyCode("code"),
				theme.ApplyHeader("header")),
		}, nil

	default:
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("Unknown action '%s'. Use 'list' or 'set'", action),
		}, nil
	}
}

// handleSessions handles session management commands
func (c *ChatInterface) handleSessions(ctx context.Context, args []string, context *ConversationContext) (*CommandResult, error) {
	if len(args) == 0 {
		// Show current session info
		currentSession := c.sessionManager.GetCurrentSession()
		if currentSession == nil {
			return &CommandResult{
				Success: false,
				Message: "No active session",
			}, nil
		}

		message := fmt.Sprintf("📋 Current Session: %s\n", currentSession.Title)
		message += fmt.Sprintf("   Session ID: %s\n", currentSession.ID)
		message += fmt.Sprintf("   Mode: %s\n", currentSession.Mode)
		message += fmt.Sprintf("   Project: %s\n", currentSession.ProjectPath)
		message += fmt.Sprintf("   Messages: %d\n", len(currentSession.ConversationHistory))
		message += fmt.Sprintf("   Started: %s\n", currentSession.StartedAt.Format("2006-01-02 15:04"))
		message += "\nUse '/sessions list' to see all sessions"

		return &CommandResult{
			Success: true,
			Message: message,
		}, nil
	}

	action := strings.ToLower(args[0])

	switch action {
	case "new":
		// Save current session before creating new one
		if currentSession := c.sessionManager.GetCurrentSession(); currentSession != nil {
			if err := c.sessionManager.SaveSession(currentSession); err != nil {
				fmt.Printf("%s Failed to save current session: %v\n", color.YellowString("⚠️"), err)
			}
		}

		// Create new session
		newSession := c.sessionManager.CreateNewSession(c.config.ProjectPath, c.config.Mode)

		// Clear conversation history for new session
		c.context.History = make([]Message, 0)
		c.context.SessionID = newSession.ID
		c.context.CreatedAt = time.Now()
		c.context.UpdatedAt = time.Now()

		// Clear input history
		c.inputHandler.ClearHistory()
		c.inputHandler.history = newSession.CommandHistory

		// Re-add system prompt with project context
		if c.context.ProjectInfo != nil {
			systemPrompt := prompts.GenerateContextualPrompt(c.config.Mode, c.context.ProjectInfo, c.context.Resources)
			c.addMessage("system", systemPrompt)
		}

		return &CommandResult{
			Success: true,
			Message: fmt.Sprintf("✨ Started new session: %s", newSession.Title),
		}, nil

	case "list":
		sessions, err := c.sessionManager.ListSessions()
		if err != nil {
			return &CommandResult{
				Success: false,
				Message: fmt.Sprintf("Failed to list sessions: %v", err),
			}, nil
		}

		if len(sessions) == 0 {
			return &CommandResult{
				Success: true,
				Message: "No saved sessions found",
			}, nil
		}

		message := fmt.Sprintf("📚 Saved Sessions (%d):\n\n", len(sessions))
		currentSession := c.sessionManager.GetCurrentSession()

		for i, session := range sessions {
			age := time.Since(session.LastUsedAt)
			ageStr := formatDuration(age)

			projectName := filepath.Base(session.ProjectPath)
			if projectName == "" || projectName == "." {
				projectName = "no project"
			}

			mark := ""
			if currentSession != nil && session.ID == currentSession.ID {
				mark = " ⭐ (current)"
			}

			message += fmt.Sprintf("%d. %s%s\n", i+1, session.Title, mark)
			message += fmt.Sprintf("   ID: %s | Mode: %s | Project: %s\n", session.ID, session.Mode, projectName)
			message += fmt.Sprintf("   %d messages | Last used: %s ago\n\n", len(session.ConversationHistory), ageStr)
		}

		maxSessions := c.sessionManager.GetMaxSessions()
		message += fmt.Sprintf("Max saved sessions: %d\n", maxSessions)
		message += "\nUse '/sessions delete <id>' to delete a session"
		message += "\nUse '/sessions config <max>' to change max sessions"

		return &CommandResult{
			Success: true,
			Message: message,
		}, nil

	case "delete":
		// If no session ID provided, show interactive selection menu
		if len(args) < 2 {
			return c.handleSessionDeleteInteractive()
		}

		// Check for "all" keyword
		if strings.ToLower(args[1]) == "all" {
			return c.handleSessionDeleteAll()
		}

		// Direct deletion with provided ID
		sessionID := args[1]
		if err := c.sessionManager.DeleteSession(sessionID); err != nil {
			return &CommandResult{
				Success: false,
				Message: fmt.Sprintf("Failed to delete session: %v", err),
			}, nil
		}

		return &CommandResult{
			Success: true,
			Message: fmt.Sprintf("✅ Session %s deleted successfully", sessionID),
		}, nil

	case "config":
		if len(args) < 2 {
			maxSessions := c.sessionManager.GetMaxSessions()
			return &CommandResult{
				Success: true,
				Message: fmt.Sprintf("Current max sessions: %d\nUse '/sessions config <number>' to change", maxSessions),
			}, nil
		}

		maxSessions, err := strconv.Atoi(args[1])
		if err != nil || maxSessions < 1 {
			return &CommandResult{
				Success: false,
				Message: "Invalid number. Please specify a positive integer.",
			}, nil
		}

		c.sessionManager.SetMaxSessions(maxSessions)

		// Save to config
		cfg, err := config.Load()
		if err != nil {
			return &CommandResult{
				Success: false,
				Message: fmt.Sprintf("Failed to load config: %v", err),
			}, nil
		}

		cfg.MaxSavedSessions = maxSessions
		if err := cfg.Save(); err != nil {
			return &CommandResult{
				Success: false,
				Message: fmt.Sprintf("Failed to save config: %v", err),
			}, nil
		}

		return &CommandResult{
			Success: true,
			Message: fmt.Sprintf("✅ Max saved sessions set to %d", maxSessions),
		}, nil

	default:
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("Unknown action '%s'. Use 'list', 'new', 'delete', or 'config'", action),
		}, nil
	}
}

// handleSessionDeleteInteractive shows interactive session deletion menu
func (c *ChatInterface) handleSessionDeleteInteractive() (*CommandResult, error) {
	sessions, err := c.sessionManager.ListSessions()
	if err != nil {
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("Failed to list sessions: %v", err),
		}, nil
	}

	if len(sessions) == 0 {
		return &CommandResult{
			Success: false,
			Message: "No sessions to delete",
		}, nil
	}

	currentSession := c.sessionManager.GetCurrentSession()

	fmt.Println()
	fmt.Println(color.YellowBold("🗑️  Select session to delete"))
	fmt.Println()

	// Show available sessions
	for i, session := range sessions {
		age := time.Since(session.LastUsedAt)
		ageStr := formatDuration(age)

		projectName := filepath.Base(session.ProjectPath)
		if projectName == "" || projectName == "." {
			projectName = "no project"
		}

		mark := ""
		if currentSession != nil && session.ID == currentSession.ID {
			mark = " ⭐ (current)"
		}

		fmt.Printf("  %d. %s%s\n", i+1, color.GreenString(session.Title), mark)
		fmt.Printf("     %s | %s | %d messages | %s ago\n",
			color.YellowString(session.Mode),
			color.CyanString(projectName),
			len(session.ConversationHistory),
			ageStr)
	}
	fmt.Println()
	fmt.Printf("  %d. %s\n", len(sessions)+1, color.GrayString("Cancel"))
	fmt.Println()

	// Read user choice
	input, err := c.inputHandler.ReadSimple(color.CyanString(fmt.Sprintf("Select session to delete [1-%d]: ", len(sessions)+1)))
	if err != nil {
		return &CommandResult{
			Success: false,
			Message: "Failed to read input",
		}, nil
	}

	choice, err := strconv.Atoi(strings.TrimSpace(input))
	if err != nil || choice < 1 || choice > len(sessions)+1 {
		return &CommandResult{
			Success: false,
			Message: "Invalid choice",
		}, nil
	}

	// Cancel
	if choice == len(sessions)+1 {
		return &CommandResult{
			Success: true,
			Message: "Cancelled",
		}, nil
	}

	selectedSession := sessions[choice-1]

	// Confirm deletion
	fmt.Printf("\n%s Are you sure you want to delete '%s'? [y/N]: ",
		color.YellowString("⚠️"), selectedSession.Title)

	confirmation, err := c.inputHandler.ReadSimple("")
	if err != nil {
		return &CommandResult{
			Success: false,
			Message: "Failed to read confirmation",
		}, nil
	}

	confirmation = strings.ToLower(strings.TrimSpace(confirmation))
	if confirmation != "y" && confirmation != "yes" {
		return &CommandResult{
			Success: true,
			Message: "Cancelled",
		}, nil
	}

	// Delete the session
	if err := c.sessionManager.DeleteSession(selectedSession.ID); err != nil {
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("Failed to delete session: %v", err),
		}, nil
	}

	return &CommandResult{
		Success: true,
		Message: fmt.Sprintf("✅ Session '%s' deleted successfully", selectedSession.Title),
	}, nil
}

// handleSessionDeleteAll deletes all sessions with confirmation
func (c *ChatInterface) handleSessionDeleteAll() (*CommandResult, error) {
	sessions, err := c.sessionManager.ListSessions()
	if err != nil {
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("Failed to list sessions: %v", err),
		}, nil
	}

	if len(sessions) == 0 {
		return &CommandResult{
			Success: false,
			Message: "No sessions to delete",
		}, nil
	}

	currentSession := c.sessionManager.GetCurrentSession()

	fmt.Println()
	fmt.Printf("%s You are about to delete %s session(s)\n",
		color.YellowBold("⚠️  WARNING"),
		color.RedString(fmt.Sprintf("%d", len(sessions))))
	fmt.Println()

	// Show sessions that will be deleted
	for i, session := range sessions {
		projectName := filepath.Base(session.ProjectPath)
		if projectName == "" || projectName == "." {
			projectName = "no project"
		}

		mark := ""
		if currentSession != nil && session.ID == currentSession.ID {
			mark = " ⭐ (current)"
		}

		fmt.Printf("  %d. %s (%s)%s - %d messages\n", i+1, session.Title, projectName, mark, len(session.ConversationHistory))
	}
	fmt.Println()

	// Confirm deletion
	fmt.Printf("%s This action cannot be undone. Are you sure? Type 'DELETE ALL' to confirm: ",
		color.RedString("⚠️"))

	confirmation, err := c.inputHandler.ReadSimple("")
	if err != nil {
		return &CommandResult{
			Success: false,
			Message: "Failed to read confirmation",
		}, nil
	}

	confirmation = strings.TrimSpace(confirmation)
	if confirmation != "DELETE ALL" {
		return &CommandResult{
			Success: true,
			Message: "Cancelled",
		}, nil
	}

	// Delete all sessions
	deletedCount := 0
	for _, session := range sessions {
		if err := c.sessionManager.DeleteSession(session.ID); err != nil {
			fmt.Printf("%s Failed to delete session '%s': %v\n",
				color.YellowString("⚠️"), session.Title, err)
		} else {
			deletedCount++
		}
	}

	// Create a new session after deleting all
	newSession := c.sessionManager.CreateNewSession(c.config.ProjectPath, c.config.Mode)

	// Clear conversation history
	c.context.History = make([]Message, 0)
	c.context.SessionID = newSession.ID
	c.context.CreatedAt = time.Now()
	c.context.UpdatedAt = time.Now()

	// Clear input history
	c.inputHandler.ClearHistory()
	c.inputHandler.history = newSession.CommandHistory

	// Re-add system prompt with project context
	if c.context.ProjectInfo != nil {
		systemPrompt := prompts.GenerateContextualPrompt(c.config.Mode, c.context.ProjectInfo, c.context.Resources)
		c.addMessage("system", systemPrompt)
	}

	return &CommandResult{
		Success: true,
		Message: fmt.Sprintf("✅ Deleted %d session(s). Started new session: %s", deletedCount, newSession.Title),
	}, nil
}
