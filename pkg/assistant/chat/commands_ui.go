package chat

import (
	"context"
	"fmt"
	"strings"

	"github.com/simple-container-com/api/pkg/assistant/llm/prompts"
)

// registerUICommands registers UI management commands
func (c *ChatInterface) registerUICommands() {
	c.commands["switch"] = &ChatCommand{
		Name:        "switch",
		Description: "Switch conversation mode (dev/devops) or change deployment type preference for new setups (cloud-compose/static/single-image)",
		Usage:       "/switch <mode_or_deployment_type>",
		Handler:     c.handleSwitch,
		Args: []CommandArg{
			{Name: "target", Type: "string", Required: true, Description: "Mode (dev|devops) or deployment type preference (cloud-compose|static|single-image)"},
		},
	}

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
