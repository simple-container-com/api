package chat

import (
	"context"
	"fmt"
	"strings"
)

// registerStackCommands registers stack and environment management commands
func (c *ChatInterface) registerStackCommands() {
	// Configuration modification commands (aligned with MCP tools)
	c.commands["getconfig"] = &ChatCommand{
		Name:        "getconfig",
		Description: "Get current Simple Container configuration",
		Usage:       "/getconfig [client|server] [stack_name]",
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
		Description: "Modify existing stack environment configuration in client.yaml files (not for changing deployment preferences - use /switch for that). Use this to modify environment properties like parent stack references, resource usage, memory, scaling, etc. MEMORY CONFIG: For single-image deployments use 'config.maxMemory' (Lambda memory), for cloud-compose deployments use 'config.size.memory' (container memory). To remove secrets/databases: use dotted notation like 'config.secrets.SECRET_NAME=' with empty value.",
		Usage:       "/modifystack <stack_name> <environment_name> <key=value> [key=value...]",
		Handler:     c.handleModifyStack,
		Args: []CommandArg{
			{Name: "stack_name", Type: "string", Required: true, Description: "Name of the stack directory in .sc/stacks/<stack-name>"},
			{Name: "environment_name", Type: "string", Required: true, Description: "Environment key from client.yaml stacks section - if multiple environments exist, user will be prompted to choose"},
			{Name: "parent", Type: "string", Required: false, Description: "Parent stack reference (e.g. 'infrastructure', 'mycompany/shared')"},
			{Name: "parentEnv", Type: "string", Required: false, Description: "Parent environment to map to (e.g. 'staging', 'prod', 'shared')"},
			{Name: "type", Type: "string", Required: false, Description: "Deployment type (cloud-compose, static, single-image)"},
			{Name: "config.uses", Type: "string", Required: false, Description: "Comma-separated list of resources the stack should use (e.g. 'postgres,redis' or empty '' to remove all)"},
			{Name: "config.maxMemory", Type: "string", Required: false, Description: "Lambda function memory allocation in MB (e.g. '512', '1024', '2048') - ONLY for single-image deployments"},
			{Name: "config.size.memory", Type: "string", Required: false, Description: "Container memory allocation in MB (e.g. '512', '1024', '2048') - ONLY for cloud-compose deployments"},
			{Name: "config.size.cpu", Type: "string", Required: false, Description: "Container CPU allocation in millicores (e.g. '500', '1000', '2000') - ONLY for cloud-compose deployments"},
			{Name: "config.size.limits.memory", Type: "string", Required: false, Description: "Container memory limit in MB (e.g. '2048', '4096') - ONLY for cloud-compose deployments"},
			{Name: "config.size.limits.cpu", Type: "string", Required: false, Description: "Container CPU limit in millicores (e.g. '1000', '2000') - ONLY for cloud-compose deployments"},
			{Name: "config.size.requests.memory", Type: "string", Required: false, Description: "Container memory request in MB (e.g. '512', '1024') - ONLY for cloud-compose deployments"},
			{Name: "config.size.requests.cpu", Type: "string", Required: false, Description: "Container CPU request in millicores (e.g. '250', '500') - ONLY for cloud-compose deployments"},
			{Name: "config.timeout", Type: "string", Required: false, Description: "Lambda function timeout in seconds"},
			{Name: "config.scale.min", Type: "string", Required: false, Description: "Minimum number of container instances (NOT memory!)"},
			{Name: "config.scale.max", Type: "string", Required: false, Description: "Maximum number of container instances (NOT memory!)"},
			{Name: "config.env", Type: "string", Required: false, Description: "Environment variables in key=value format"},
			{Name: "config.secrets", Type: "string", Required: false, Description: "Secret references in key=value format - use dotted notation like 'config.secrets.API_KEY' to modify specific secrets or empty string to remove"},
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

	c.commands["stack"] = &ChatCommand{
		Name:        "stack",
		Description: "Manage and view stack configurations",
		Usage:       "/stack [list|info] [stack_name]",
		Handler:     c.handleStack,
		Args: []CommandArg{
			{Name: "action", Type: "string", Required: false, Description: "Action: list (show all stacks) or info (show stack details)", Default: "list"},
			{Name: "stack_name", Type: "string", Required: false, Description: "Name of the stack (required for info action)"},
		},
	}

	c.commands["diff"] = &ChatCommand{
		Name:        "diff",
		Description: "Show configuration differences between versions or environments. Supports hierarchical stack groups and wildcards.",
		Usage:       "/diff [stack_group[:stack_name]|*|pattern] [config_type=client|server] [compare_with=HEAD~1] [format=split|unified|inline|compact]\n\nExamples:\n  /diff                            # Show diff for all stacks\n  /diff *                          # Show diff for all stacks\n  /diff simple-container           # Show diff for all stacks in 'simple-container' group\n  /diff simple-container:staging   # Show diff for 'staging' stack in 'simple-container' group\n  /diff simple-container/staging   # Alternative syntax with slash\n  /diff simple-container:test*     # Show diff for all stacks starting with 'test' in 'simple-container' group\n  /diff staging                    # Show diff for 'staging' stack (auto-detect group)\n  /diff test* client               # Show diff for all stacks starting with 'test'\n  /diff *prod* client              # Show diff for all stacks containing 'prod'",
		Handler:     c.handleConfigDiff,
		Args: []CommandArg{
			{Name: "stack_name", Type: "string", Required: false, Description: "Stack name or group:stack format (e.g., 'simple-container:staging'). Use '*' for all, wildcards like 'test*' or '*prod*', or group name alone to show all stacks in that group"},
			{Name: "config_type", Type: "string", Required: false, Description: "Configuration type: client or server", Default: "client"},
			{Name: "compare_with", Type: "string", Required: false, Description: "Git reference to compare with (e.g., HEAD~1, main, v1.0)", Default: "HEAD~1"},
			{Name: "format", Type: "string", Required: false, Description: "Output format: split, unified, inline, or compact", Default: "split"},
		},
	}
}

// handleGetConfig gets current Simple Container configuration using unified handler
func (c *ChatInterface) handleGetConfig(ctx context.Context, args []string, context *ConversationContext) (*CommandResult, error) {
	if c.commandHandler == nil {
		return &CommandResult{
			Success: false,
			Message: "❌ Command handler not available",
		}, nil
	}

	// Parse arguments
	configType := "client"
	stackName := ""
	if len(args) > 0 {
		configType = args[0]
	}
	if len(args) > 1 {
		stackName = args[1]
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

	// Additional config can be passed as key=value pairs
	config := make(map[string]interface{})
	for i := 4; i < len(args); i++ {
		if parts := strings.SplitN(args[i], "=", 2); len(parts) == 2 {
			config[parts[0]] = parts[1]
		}
	}

	result, err := c.commandHandler.AddEnvironment(ctx, stackName, deploymentType, parent, parentEnv, config)
	if err != nil {
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("❌ Failed to add environment: %v", err),
		}, nil
	}

	return &CommandResult{
		Success: result.Success,
		Message: result.Message,
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

	// Parse key=value pairs
	changes := make(map[string]interface{})
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
		changes[key] = value
	}

	if len(changes) == 0 {
		return &CommandResult{
			Success: false,
			Message: "❌ No changes specified. Please provide at least one key=value pair.",
		}, nil
	}

	result, err := c.commandHandler.ModifyStackConfig(ctx, stackName, environmentName, changes)
	if err != nil {
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("❌ Failed to modify stack: %v", err),
		}, nil
	}

	return &CommandResult{
		Success: result.Success,
		Message: result.Message,
	}, nil
}

// handleAddResource adds a new resource to server.yaml using unified handler
func (c *ChatInterface) handleAddResource(ctx context.Context, args []string, context *ConversationContext) (*CommandResult, error) {
	if c.commandHandler == nil {
		return &CommandResult{}, nil
	}

	resourceName := args[0]
	resourceType := args[1]
	environment := args[2]

	// Use unified command handler
	result, err := c.commandHandler.AddResource(ctx, resourceName, resourceType, environment, nil)
	if err != nil {
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("❌ Failed to add resource: %v", err),
		}, nil
	}

	return &CommandResult{
		Success: result.Success,
		Message: result.Message,
	}, nil
}

// handleConfigDiff shows configuration differences between versions or environments
func (c *ChatInterface) handleConfigDiff(ctx context.Context, args []string, context *ConversationContext) (*CommandResult, error) {
	if c.commandHandler == nil {
		return &CommandResult{
			Success: false,
			Message: "❌ Command handler not available",
		}, nil
	}

	// Default values
	params := map[string]string{
		"stack_name":   "", // Empty means show all stacks
		"config_type":  "client",
		"compare_with": "HEAD", // Compare with last commit by default
		"format":       "split",
	}

	// Parse arguments
	for _, arg := range args {
		if strings.Contains(arg, "=") {
			// Handle named parameters (e.g., config_type=server)
			parts := strings.SplitN(arg, "=", 2)
			if len(parts) == 2 {
				params[parts[0]] = parts[1]
			}
		} else if params["stack_name"] == "" {
			// The first non-parameter argument is treated as stack_name
			params["stack_name"] = arg
		}
	}

	// If no stack name is provided, show diff for all stacks
	if params["stack_name"] == "" {
		// Get current config to list available stacks
		result, err := c.commandHandler.GetCurrentConfig(ctx, params["config_type"], "")
		if err != nil {
			return &CommandResult{
				Success: false,
				Message: fmt.Sprintf("❌ Failed to get configuration: %v", err),
			}, nil
		}

		// Extract stack names from the config
		var stackNames []string
		// Check if stacks are in content first
		if content, ok := result.Data["content"].(map[string]interface{}); ok {
			if stacks, ok := content["stacks"].(map[string]interface{}); ok {
				for stackName := range stacks {
					stackNames = append(stackNames, stackName)
				}
			}
		}
		// Fallback: check direct stacks key
		if len(stackNames) == 0 {
			if stacks, ok := result.Data["stacks"].(map[string]interface{}); ok {
				for stackName := range stacks {
					stackNames = append(stackNames, stackName)
				}
			}
		}

		if len(stackNames) == 0 {
			return &CommandResult{
				Success: true,
				Message: "No stacks found in the configuration. Use `/getconfig` to view the current configuration.",
			}, nil
		}

		// Show diff for all stacks
		var allMessages []string

		for _, stackName := range stackNames {
			// Get diff for this stack
			result, err := c.commandHandler.ShowConfigDiff(
				ctx,
				stackName,
				params["config_type"],
				params["compare_with"],
				params["format"],
			)
			if err != nil {
				allMessages = append(allMessages, fmt.Sprintf("❌ Failed to get diff for stack '%s': %v", stackName, err))
				continue
			}

			if result.Success {
				allMessages = append(allMessages, result.Message)
			} else {
				allMessages = append(allMessages, fmt.Sprintf("❌ %s", result.Message))
			}
		}

		if len(allMessages) == 0 {
			return &CommandResult{
				Success: true,
				Message: "No changes found in any stacks.",
			}, nil
		}

		// Combine all messages
		finalMessage := fmt.Sprintf("🔍 Configuration diff for all stacks (comparing with %s):\n\n", params["compare_with"])
		finalMessage += strings.Join(allMessages, "\n\n"+strings.Repeat("═", 80)+"\n\n")

		return &CommandResult{
			Success: true,
			Message: finalMessage,
		}, nil
	}

	// Call the MCP handler with the specified stack
	result, err := c.commandHandler.ShowConfigDiff(
		ctx,
		params["stack_name"],
		params["config_type"],
		params["compare_with"],
		params["format"],
	)
	if err != nil {
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("❌ Failed to show config diff: %v", err),
		}, nil
	}

	return &CommandResult{
		Success: result.Success,
		Message: result.Message,
	}, nil
}

// handleStack manages and views stack configurations
func (c *ChatInterface) handleStack(ctx context.Context, args []string, context *ConversationContext) (*CommandResult, error) {
	if c.commandHandler == nil {
		return &CommandResult{
			Success: false,
			Message: "❌ Command handler not available",
		}, nil
	}

	// Default action is list
	action := "list"
	stackName := ""

	// Parse arguments
	if len(args) > 0 {
		action = args[0]
	}
	if len(args) > 1 {
		stackName = args[1]
	}

	switch action {
	case "list":
		// Get current config to list available stacks
		result, err := c.commandHandler.GetCurrentConfig(ctx, "client", "")
		if err != nil {
			return &CommandResult{
				Success: false,
				Message: fmt.Sprintf("❌ Failed to get configuration: %v", err),
			}, nil
		}

		// Extract stack names from the config
		var stackNames []string
		// Check if stacks are in content first
		if content, ok := result.Data["content"].(map[string]interface{}); ok {
			if stacks, ok := content["stacks"].(map[string]interface{}); ok {
				for stackName := range stacks {
					stackNames = append(stackNames, stackName)
				}
			}
		}
		// Fallback: check direct stacks key
		if len(stackNames) == 0 {
			if stacks, ok := result.Data["stacks"].(map[string]interface{}); ok {
				for stackName := range stacks {
					stackNames = append(stackNames, stackName)
				}
			}
		}

		if len(stackNames) == 0 {
			return &CommandResult{
				Success: true,
				Message: "No stacks found in the configuration. Use `/getconfig` to view the current configuration.",
			}, nil
		}

		// Show available stacks
		message := "📋 Available stacks:\n\n"
		for _, name := range stackNames {
			message += fmt.Sprintf("• **%s**\n", name)
		}
		message += "\n💡 Use `/stack info <stack_name>` to view details or `/diff <stack_name>` to see changes"

		return &CommandResult{
			Success: true,
			Message: message,
		}, nil

	case "info":
		if stackName == "" {
			return &CommandResult{
				Success: false,
				Message: "❌ Stack name is required for info action. Usage: `/stack info <stack_name>`",
			}, nil
		}

		// Get current config for the specific stack
		result, err := c.commandHandler.GetCurrentConfig(ctx, "client", stackName)
		if err != nil {
			return &CommandResult{
				Success: false,
				Message: fmt.Sprintf("❌ Failed to get stack configuration: %v", err),
			}, nil
		}

		return &CommandResult{
			Success: result.Success,
			Message: fmt.Sprintf("📊 Stack '%s' configuration:\n\n%s", stackName, result.Message),
		}, nil

	default:
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("❌ Unknown action '%s'. Available actions: list, info", action),
		}, nil
	}
}
