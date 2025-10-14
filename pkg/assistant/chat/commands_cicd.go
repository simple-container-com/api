package chat

import (
	"context"
	"fmt"
	"strings"
)

// registerCICDCommands registers CI/CD pipeline management commands
func (c *ChatInterface) registerCICDCommands() {
	c.commands["cicd-generate"] = &ChatCommand{
		Name:        "cicd-generate",
		Description: "Generate CI/CD workflows for GitHub Actions",
		Usage:       "/cicd-generate [--stack <name>] [--config <file>]",
		Handler:     c.handleCICDGenerate,
		Aliases:     []string{"generate-cicd", "cicd-gen"},
		Args: []CommandArg{
			{Name: "stack", Type: "string", Required: false, Description: "Stack name to generate CI/CD for"},
			{Name: "config", Type: "string", Required: false, Description: "Path to server.yaml config file"},
		},
	}

	c.commands["cicd-validate"] = &ChatCommand{
		Name:        "cicd-validate",
		Description: "Validate CI/CD configuration in server.yaml",
		Usage:       "/cicd-validate [--stack <name>] [--config <file>] [--show-diff]",
		Handler:     c.handleCICDValidate,
		Aliases:     []string{"validate-cicd"},
		Args: []CommandArg{
			{Name: "stack", Type: "string", Required: false, Description: "Stack name to validate CI/CD for"},
			{Name: "config", Type: "string", Required: false, Description: "Path to server.yaml config file"},
			{Name: "show-diff", Type: "flag", Required: false, Description: "Show differences between current and expected configuration"},
		},
	}

	c.commands["cicd-preview"] = &ChatCommand{
		Name:        "cicd-preview",
		Description: "Preview CI/CD workflows that would be generated",
		Usage:       "/cicd-preview [--stack <name>] [--config <file>] [--show-content]",
		Handler:     c.handleCICDPreview,
		Aliases:     []string{"preview-cicd"},
		Args: []CommandArg{
			{Name: "stack", Type: "string", Required: false, Description: "Stack name to preview CI/CD for"},
			{Name: "config", Type: "string", Required: false, Description: "Path to server.yaml config file"},
			{Name: "show-content", Type: "flag", Required: false, Description: "Show full workflow file contents"},
		},
	}

	c.commands["cicd-sync"] = &ChatCommand{
		Name:        "cicd-sync",
		Description: "Sync CI/CD workflows to GitHub repository",
		Usage:       "/cicd-sync [--stack <name>] [--config <file>] [--dry-run]",
		Handler:     c.handleCICDSync,
		Aliases:     []string{"sync-cicd"},
		Args: []CommandArg{
			{Name: "stack", Type: "string", Required: false, Description: "Stack name to sync CI/CD for"},
			{Name: "config", Type: "string", Required: false, Description: "Path to server.yaml config file"},
			{Name: "dry-run", Type: "flag", Required: false, Description: "Show what would be synced without actually syncing"},
		},
	}

	c.commands["cicd-setup"] = &ChatCommand{
		Name:        "cicd-setup",
		Description: "Interactive CI/CD setup wizard for configuring GitHub Actions",
		Usage:       "/cicd-setup [--stack <name>]",
		Handler:     c.handleCICDSetup,
		Aliases:     []string{"setup-cicd"},
		Args: []CommandArg{
			{Name: "stack", Type: "string", Required: false, Description: "Stack name to setup CI/CD for"},
		},
	}
}

// handleCICDGenerate generates CI/CD workflows using the CLI command
func (c *ChatInterface) handleCICDGenerate(ctx context.Context, args []string, context *ConversationContext) (*CommandResult, error) {
	if c.commandHandler == nil {
		return &CommandResult{
			Success: false,
			Message: "❌ Command handler not available",
		}, nil
	}

	// Parse flags
	params := make(map[string]string)
	for i := 0; i < len(args); i++ {
		if strings.HasPrefix(args[i], "--") {
			key := strings.TrimPrefix(args[i], "--")
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
				params[key] = args[i+1]
				i++ // Skip the value
			} else {
				params[key] = "true" // Flag without value
			}
		}
	}

	// Use the existing CI/CD generation via command handler
	result, err := c.commandHandler.GenerateCICD(ctx, params["stack"], params["config"])
	if err != nil {
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("❌ Failed to generate CI/CD workflows: %v", err),
		}, nil
	}

	return &CommandResult{
		Success: result.Success,
		Message: result.Message,
		Data:    result.Data,
	}, nil
}

// handleCICDValidate validates CI/CD configuration
func (c *ChatInterface) handleCICDValidate(ctx context.Context, args []string, context *ConversationContext) (*CommandResult, error) {
	if c.commandHandler == nil {
		return &CommandResult{
			Success: false,
			Message: "❌ Command handler not available",
		}, nil
	}

	// Parse flags
	params := make(map[string]string)
	for i := 0; i < len(args); i++ {
		if strings.HasPrefix(args[i], "--") {
			key := strings.TrimPrefix(args[i], "--")
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
				params[key] = args[i+1]
				i++ // Skip the value
			} else {
				params[key] = "true" // Flag without value
			}
		}
	}

	result, err := c.commandHandler.ValidateCICD(ctx, params["stack"], params["config"], params["show-diff"] == "true")
	if err != nil {
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("❌ Failed to validate CI/CD configuration: %v", err),
		}, nil
	}

	return &CommandResult{
		Success: result.Success,
		Message: result.Message,
		Data:    result.Data,
	}, nil
}

// handleCICDPreview previews CI/CD workflows
func (c *ChatInterface) handleCICDPreview(ctx context.Context, args []string, context *ConversationContext) (*CommandResult, error) {
	if c.commandHandler == nil {
		return &CommandResult{
			Success: false,
			Message: "❌ Command handler not available",
		}, nil
	}

	// Parse flags
	params := make(map[string]string)
	for i := 0; i < len(args); i++ {
		if strings.HasPrefix(args[i], "--") {
			key := strings.TrimPrefix(args[i], "--")
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
				params[key] = args[i+1]
				i++ // Skip the value
			} else {
				params[key] = "true" // Flag without value
			}
		}
	}

	result, err := c.commandHandler.PreviewCICD(ctx, params["stack"], params["config"], params["show-content"] == "true")
	if err != nil {
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("❌ Failed to preview CI/CD workflows: %v", err),
		}, nil
	}

	return &CommandResult{
		Success: result.Success,
		Message: result.Message,
		Data:    result.Data,
	}, nil
}

// handleCICDSync syncs CI/CD workflows to repository
func (c *ChatInterface) handleCICDSync(ctx context.Context, args []string, context *ConversationContext) (*CommandResult, error) {
	if c.commandHandler == nil {
		return &CommandResult{
			Success: false,
			Message: "❌ Command handler not available",
		}, nil
	}

	// Parse flags
	params := make(map[string]string)
	for i := 0; i < len(args); i++ {
		if strings.HasPrefix(args[i], "--") {
			key := strings.TrimPrefix(args[i], "--")
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
				params[key] = args[i+1]
				i++ // Skip the value
			} else {
				params[key] = "true" // Flag without value
			}
		}
	}

	result, err := c.commandHandler.SyncCICD(ctx, params["stack"], params["config"], params["dry-run"] == "true")
	if err != nil {
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("❌ Failed to sync CI/CD workflows: %v", err),
		}, nil
	}

	return &CommandResult{
		Success: result.Success,
		Message: result.Message,
		Data:    result.Data,
	}, nil
}

// handleCICDSetup provides interactive CI/CD setup wizard
func (c *ChatInterface) handleCICDSetup(ctx context.Context, args []string, context *ConversationContext) (*CommandResult, error) {
	stackName := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "--") {
		stackName = args[0]
	}

	// Parse --stack flag if present
	for i := 0; i < len(args); i++ {
		if args[i] == "--stack" && i+1 < len(args) {
			stackName = args[i+1]
			break
		}
	}

	// Provide CI/CD setup guidance
	message := "🚀 **CI/CD Setup Wizard**\n\n"

	if stackName == "" {
		message += "To setup CI/CD for your project, you need to:\n\n"
		message += "1. **Add CI/CD configuration to your server.yaml:**\n"
		message += "```yaml\n"
		message += "cicd:\n"
		message += "  type: github-actions\n"
		message += "  config:\n"
		message += "    organization: \"your-github-org\"\n"
		message += "    environments:\n"
		message += "      staging:\n"
		message += "        type: staging\n"
		message += "        auto-deploy: true\n"
		message += "        runners: [\"ubuntu-latest\"]\n"
		message += "      production:\n"
		message += "        type: production\n"
		message += "        protection: true\n"
		message += "        auto-deploy: false\n"
		message += "        runners: [\"ubuntu-latest\"]\n"
		message += "    notifications:\n"
		message += "      slack: \"${secret:slack-webhook-url}\"\n"
		message += "      discord: \"${secret:discord-webhook-url}\"\n"
		message += "    workflow-generation:\n"
		message += "      enabled: true\n"
		message += "```\n\n"
		message += "2. **Generate the workflows:**\n"
		message += "   `/cicd-generate --stack your-stack-name`\n\n"
		message += "3. **Validate the configuration:**\n"
		message += "   `/cicd-validate --stack your-stack-name --show-diff`\n\n"
		message += "4. **Preview the workflows:**\n"
		message += "   `/cicd-preview --stack your-stack-name --show-content`\n\n"
		message += "5. **Sync to GitHub repository:**\n"
		message += "   `/cicd-sync --stack your-stack-name`\n\n"
	} else {
		message += fmt.Sprintf("Setting up CI/CD for stack: **%s**\n\n", stackName)
		message += "**Next Steps:**\n"
		message += "1. First validate your configuration:\n"
		message += fmt.Sprintf("   `/cicd-validate --stack %s --show-diff`\n\n", stackName)
		message += "2. Generate the workflows:\n"
		message += fmt.Sprintf("   `/cicd-generate --stack %s`\n\n", stackName)
		message += "3. Preview what will be created:\n"
		message += fmt.Sprintf("   `/cicd-preview --stack %s --show-content`\n\n", stackName)
		message += "4. Sync to your repository:\n"
		message += fmt.Sprintf("   `/cicd-sync --stack %s`\n\n", stackName)
	}

	message += "**📚 Need help with configuration?**\n"
	message += "- Use `/search cicd github actions` to find documentation\n"
	message += "- Use `/file server.yaml` to view your current server configuration\n"
	message += "- Use `/resources` to see available resource types for CI/CD\n"

	return &CommandResult{
		Success: true,
		Message: message,
	}, nil
}
