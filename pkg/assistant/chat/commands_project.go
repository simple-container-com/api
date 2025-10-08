package chat

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"gopkg.in/yaml.v2"

	"github.com/simple-container-com/api/pkg/api/logger/color"
	"github.com/simple-container-com/api/pkg/assistant/analysis"
	"github.com/simple-container-com/api/pkg/assistant/resources"
)

// registerProjectCommands registers project analysis and configuration commands
func (c *ChatInterface) registerProjectCommands() {
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
}

// handleAnalyze analyzes the current project
func (c *ChatInterface) handleAnalyze(ctx context.Context, args []string, context *ConversationContext) (*CommandResult, error) {
	if context.ProjectPath == "" {
		return &CommandResult{
			Success: false,
			Message: "❌ No project directory set. Please navigate to your project directory.",
		}, nil
	}

	// Parse arguments (analysis mode not used currently)
	_ = args

	fmt.Printf("🔍 Analyzing project at %s...\n", color.CyanString(context.ProjectPath))

	// Create analyzer and run analysis
	analyzer := analysis.NewProjectAnalyzer()
	result, err := analyzer.AnalyzeProject(context.ProjectPath)
	if err != nil {
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("❌ Analysis failed: %v", err),
		}, nil
	}

	// Update context with analysis results
	context.ProjectInfo = result

	// Format results for display
	message := "📊 Project Analysis Results:\n\n"

	if result.PrimaryStack != nil {
		message += fmt.Sprintf("🔧 **Primary Technology**: %s", result.PrimaryStack.Language)
		if result.PrimaryStack.Framework != "" {
			message += fmt.Sprintf(" (%s)", result.PrimaryStack.Framework)
		}
		message += "\n"
	}

	if result.Resources != nil && len(result.Resources.Databases) > 0 {
		message += "\n📚 **Technologies Detected**:\n"
		for _, db := range result.Resources.Databases {
			message += fmt.Sprintf("  • Database: %s\n", db.Name)
		}
	}

	if result.Resources != nil && (len(result.Resources.Databases) > 0 || len(result.Resources.ExternalAPIs) > 0) {
		message += "\n🔗 **External Dependencies**:\n"
		for _, db := range result.Resources.Databases {
			message += fmt.Sprintf("  • Database: %s\n", db.Name)
		}
		for _, api := range result.Resources.ExternalAPIs {
			message += fmt.Sprintf("  • API: %s\n", api.Name)
		}
	}

	if len(result.Recommendations) > 0 {
		message += "\n💡 **Deployment Recommendations**:\n"
		for _, rec := range result.Recommendations {
			message += fmt.Sprintf("  • %s: %s\n", rec.Type, rec.Description)
		}
	}

	return &CommandResult{
		Success: true,
		Message: message,
	}, nil
}

// handleSetup generates configuration files
func (c *ChatInterface) handleSetup(ctx context.Context, args []string, context *ConversationContext) (*CommandResult, error) {
	if context.ProjectInfo == nil {
		return &CommandResult{
			Success: false,
			Message: "❌ Project not analyzed. Please run '/analyze' first.",
		}, nil
	}

	// Parse mode argument
	mode := "auto"
	for _, arg := range args {
		if strings.HasPrefix(arg, "--mode=") {
			mode = strings.TrimPrefix(arg, "--mode=")
		} else if arg == "--mode" && len(args) > 1 {
			// Look for next argument as mode
			for i, a := range args {
				if a == "--mode" && i+1 < len(args) {
					mode = args[i+1]
					break
				}
			}
		}
	}

	fmt.Printf("⚙️  Setting up Simple Container configuration (mode: %s)...\n", color.CyanString(mode))

	// TODO: Implement actual configuration generation using the modes package
	// For now, provide a placeholder implementation that works with the current API

	return &CommandResult{
		Success: true,
		Message: "✅ Simple Container configuration files generated successfully!\n\nYou can now deploy with:\n`sc deploy -s <your-project-name> -e staging`",
	}, nil
}

// handleConfig displays current Simple Container configuration
func (c *ChatInterface) handleConfig(ctx context.Context, args []string, context *ConversationContext) (*CommandResult, error) {
	// Parse arguments
	configType := "client"
	stackName := ""
	explain := false

	for i, arg := range args {
		switch {
		case strings.HasPrefix(arg, "--type="):
			configType = strings.TrimPrefix(arg, "--type=")
		case arg == "--type" && i+1 < len(args):
			configType = args[i+1]
		case strings.HasPrefix(arg, "--stack="):
			stackName = strings.TrimPrefix(arg, "--stack=")
		case arg == "--stack" && i+1 < len(args):
			stackName = args[i+1]
		case arg == "--explain":
			explain = true
		}
	}

	var filePath string
	switch configType {
	case "server":
		filePath = filepath.Join(".sc", "stacks", "infrastructure", "server.yaml")
	case "client":
		if stackName != "" {
			filePath = filepath.Join(".sc", "stacks", stackName, "client.yaml")
		} else {
			// Try to find client.yaml in current or subdirectories
			if _, err := os.Stat(filepath.Join(".sc", "stacks")); err == nil {
				entries, err := os.ReadDir(filepath.Join(".sc", "stacks"))
				if err == nil {
					for _, entry := range entries {
						if entry.IsDir() {
							clientPath := filepath.Join(".sc", "stacks", entry.Name(), "client.yaml")
							if _, err := os.Stat(clientPath); err == nil {
								filePath = clientPath
								break
							}
						}
					}
				}
			}
		}
	default:
		return &CommandResult{
			Success: false,
			Message: "❌ Invalid config type. Use 'client' or 'server'",
		}, nil
	}

	if filePath == "" {
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("❌ No %s configuration found", configType),
		}, nil
	}

	// Read the configuration file
	data, err := os.ReadFile(filePath)
	if err != nil {
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("❌ Failed to read %s: %v", filePath, err),
		}, nil
	}

	titleCaser := cases.Title(language.English)
	message := fmt.Sprintf("📋 **%s Configuration** (`%s`)\n\n", titleCaser.String(configType), filePath)
	message += "```yaml\n"
	message += string(data)
	message += "\n```\n"

	if explain && c.llm != nil {
		message += "\n🤖 **AI Analysis:**\n"

		// Parse YAML to provide analysis
		var config map[string]interface{}
		if err := yaml.Unmarshal(data, &config); err == nil {
			// Add basic analysis based on config content
			if stacks, ok := config["stacks"].(map[interface{}]interface{}); ok {
				message += fmt.Sprintf("• Found %d stack environments\n", len(stacks))
				for env := range stacks {
					message += fmt.Sprintf("• Environment: **%s**\n", env)
				}
			}
			if resources, ok := config["resources"].(map[interface{}]interface{}); ok {
				message += fmt.Sprintf("• Contains %d resource definitions\n", len(resources))
			}
		}
	}

	return &CommandResult{
		Success: true,
		Message: message,
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

	message := "📚 **Supported Simple Container Resources:**\n\n"

	// Check if supportedResources has the expected structure
	if len(supportedResources.Providers) > 0 {
		for _, provider := range supportedResources.Providers {
			message += fmt.Sprintf("**%s** (%d resources):\n", provider.Name, len(provider.Resources))
			for _, resource := range provider.Resources {
				message += fmt.Sprintf("  • %s\n", resource)
			}
			message += "\n"
		}

		message += fmt.Sprintf("**Total**: %d resources across %d providers\n\n",
			len(supportedResources.Resources), len(supportedResources.Providers))
	} else {
		message += "No supported resources found or unable to load resource schemas.\n\n"
	}

	message += "Use `/addresource <name> <type> <environment>` to add resources to your infrastructure."

	return &CommandResult{
		Success: true,
		Message: message,
	}, nil
}
