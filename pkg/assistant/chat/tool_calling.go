package chat

import (
	"context"
	"fmt"
	"strings"

	"gopkg.in/yaml.v2"

	"github.com/simple-container-com/api/pkg/assistant/llm"
)

// ToolCallHandler handles LLM tool calls by routing them to appropriate chat commands
type ToolCallHandler struct {
	commands map[string]*ChatCommand
}

// NewToolCallHandler creates a new tool call handler
func NewToolCallHandler(commands map[string]*ChatCommand) *ToolCallHandler {
	return &ToolCallHandler{
		commands: commands,
	}
}

// GetAvailableTools returns the tool definitions for available chat commands
func (h *ToolCallHandler) GetAvailableTools() []llm.Tool {
	tools := []llm.Tool{}

	// Define tools for each available command
	for name, command := range h.commands {
		// Skip help and exit commands as tools
		if name == "help" || name == "exit" || name == "clear" {
			continue
		}

		tool := llm.Tool{
			Type: "function",
			Function: llm.FunctionDef{
				Name:        name,
				Description: command.Description,
				Parameters:  h.generateParameterSchema(command),
			},
		}

		tools = append(tools, tool)
	}

	return tools
}

// generateParameterSchema creates a JSON schema for command parameters
func (h *ToolCallHandler) generateParameterSchema(command *ChatCommand) map[string]interface{} {
	schema := map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{},
		"required":   []string{},
	}

	properties := schema["properties"].(map[string]interface{})
	required := []string{}

	for _, arg := range command.Args {
		argSchema := map[string]interface{}{
			"type":        "string",
			"description": arg.Description,
		}

		// Add enum for specific values if needed
		switch arg.Name {
		case "mode":
			argSchema["enum"] = []string{"dev", "devops"}
		case "target":
			argSchema["enum"] = []string{"dev", "devops", "cloud-compose", "static", "single-image"}
		case "query":
			argSchema["description"] = "Search query for documentation or analysis"
		case "full":
			argSchema["type"] = "boolean"
			argSchema["description"] = "Run comprehensive analysis (slower but thorough)"
		case "config.uses":
			argSchema["description"] = "Comma-separated list of resources (e.g. 'postgres,redis') or empty string '' to remove all resources"
			argSchema["examples"] = []string{"postgres,redis", "postgres", ""}
		case "type":
			if command.Name == "modifystack" {
				argSchema["enum"] = []string{"cloud-compose", "static", "single-image"}
			} else if command.Name == "config" {
				argSchema["description"] = "Configuration type to display: 'client' for application config, 'server' for infrastructure config"
				argSchema["enum"] = []string{"client", "server"}
				argSchema["default"] = "client"
			}
		case "parent":
			if command.Name == "modifystack" {
				argSchema["description"] = "Parent stack reference (e.g. 'infrastructure', 'mycompany/shared')"
				argSchema["examples"] = []string{"infrastructure", "mycompany/shared", "parent-project/infrastructure"}
			}
		case "parentEnv":
			if command.Name == "modifystack" {
				argSchema["description"] = "Parent environment to map to (e.g. 'staging', 'prod', 'shared')"
				argSchema["enum"] = []string{"staging", "production", "prod", "dev", "development", "shared"}
			}
		case "config.maxMemory":
			if command.Name == "modifystack" {
				argSchema["description"] = "Lambda function memory allocation in MB - USE THIS FOR MEMORY CHANGES (e.g. '512', '1024', '2048')"
				argSchema["type"] = "string"
				argSchema["enum"] = []string{"512", "1024", "2048", "3008"}
				argSchema["examples"] = []string{"512", "1024", "2048"}
			}
		case "config.timeout":
			if command.Name == "modifystack" {
				argSchema["description"] = "Lambda function timeout in seconds (e.g. '30', '60', '120')"
				argSchema["type"] = "string"
				argSchema["examples"] = []string{"30", "60", "120", "300"}
			}
		case "config.scale.max":
			if command.Name == "modifystack" {
				argSchema["description"] = "Maximum number of container instances (NOT memory allocation!)"
				argSchema["type"] = "string"
				argSchema["examples"] = []string{"1", "5", "10", "20"}
			}
		case "stack":
			if command.Name == "config" {
				argSchema["description"] = "Stack name to display configuration for (e.g. 'myapp', 'service-name'). Leave empty for current project."
				argSchema["examples"] = []string{"myapp", "service-name", "api-service", "web-app"}
			}
		case "explain":
			if command.Name == "config" {
				argSchema["type"] = "boolean"
				argSchema["description"] = "Include AI-powered analysis of the configuration"
			}
		}

		properties[arg.Name] = argSchema

		if arg.Required {
			required = append(required, arg.Name)
		}
	}

	if len(required) > 0 {
		schema["required"] = required
	}

	return schema
}

// ExecuteToolCall executes a tool call by routing it to the appropriate command
func (h *ToolCallHandler) ExecuteToolCall(ctx context.Context, toolCall llm.ToolCall, chatContext *ConversationContext) (*CommandResult, error) {
	functionName := toolCall.Function.Name

	// Find the command
	command, exists := h.commands[functionName]
	if !exists {
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("Unknown tool function: %s", functionName),
		}, nil
	}

	// Convert arguments to string array format expected by commands
	args := h.convertArgumentsToArgs(functionName, toolCall.Function.Arguments)

	// Execute the command
	result, err := command.Handler(ctx, args, chatContext)
	if err != nil {
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("Error executing %s: %v", functionName, err),
		}, nil
	}

	return result, nil
}

// convertArgumentsToArgs converts JSON arguments to string array format expected by commands
func (h *ToolCallHandler) convertArgumentsToArgs(functionName string, arguments map[string]interface{}) []string {
	args := []string{}

	// Special handling for switch command - it expects target as positional argument
	if functionName == "switch" {
		if targetVal, exists := arguments["target"]; exists {
			if strVal, ok := targetVal.(string); ok {
				args = append(args, strVal)
			}
		}
		// Also check legacy "mode" parameter for backward compatibility
		if targetVal, exists := arguments["mode"]; exists {
			if strVal, ok := targetVal.(string); ok {
				args = append(args, strVal)
			}
		}
		return args
	}

	// Special handling for search_docs command - it expects query as positional argument
	if functionName == "search_docs" {
		if queryVal, exists := arguments["query"]; exists {
			if strVal, ok := queryVal.(string); ok {
				args = append(args, strVal)
			}
		}
		return args
	}

	// Special handling for modifystack command - it expects stack_name, environment_name, followed by key=value pairs
	if functionName == "modifystack" {
		// First add the stack name
		if stackNameVal, exists := arguments["stack_name"]; exists {
			if strVal, ok := stackNameVal.(string); ok {
				args = append(args, strVal)
			}
		}

		// Then add the environment name
		if envNameVal, exists := arguments["environment_name"]; exists {
			if strVal, ok := envNameVal.(string); ok {
				args = append(args, strVal)
			}
		}

		// Then add all other arguments as key=value pairs
		for key, value := range arguments {
			if key == "stack_name" || key == "environment_name" {
				continue // Skip these as we already handled them
			}

			// Convert value to string and create key=value format
			if strVal, ok := value.(string); ok {
				args = append(args, fmt.Sprintf("%s=%s", key, strVal))
			} else if boolVal, ok := value.(bool); ok {
				args = append(args, fmt.Sprintf("%s=%v", key, boolVal))
			} else if numVal, ok := value.(float64); ok {
				// JSON numbers are float64
				args = append(args, fmt.Sprintf("%s=%.0f", key, numVal))
			} else {
				// Fallback to string representation
				args = append(args, fmt.Sprintf("%s=%v", key, value))
			}
		}
		return args
	}

	// Special handling for config command - it expects flag-based arguments
	if functionName == "config" {
		// Handle type parameter with validation
		if typeVal, exists := arguments["type"]; exists {
			if strVal, ok := typeVal.(string); ok && strVal != "" {
				// Validate and clean the type value
				switch strVal {
				case "client", "server":
					args = append(args, "--type", strVal)
				case "--type": // LLM sometimes provides the flag name instead of value
					args = append(args, "--type", "client") // Default to client
				default:
					args = append(args, "--type", "client") // Default to client for invalid values
				}
			}
		} else {
			// No type specified, default to client
			args = append(args, "--type", "client")
		}

		// Handle stack parameter
		if stackVal, exists := arguments["stack"]; exists {
			if strVal, ok := stackVal.(string); ok && strVal != "" {
				args = append(args, "--stack", strVal)
			}
		}

		// Handle explain flag
		if explainVal, exists := arguments["explain"]; exists {
			if boolVal, ok := explainVal.(bool); ok && boolVal {
				args = append(args, "--explain")
			}
		}

		return args
	}

	// Default handling for other commands
	for key, value := range arguments {
		switch key {
		case "full":
			// Boolean flag
			if boolVal, ok := value.(bool); ok && boolVal {
				args = append(args, "--full")
			}
		case "mode":
			// Mode argument for non-switch commands
			if strVal, ok := value.(string); ok {
				args = append(args, "--mode", strVal)
			}
		case "type":
			// Handle type parameter for commands not specifically handled above
			if strVal, ok := value.(string); ok && strVal != "" {
				// Validate common types to prevent invalid values
				switch strVal {
				case "client", "server", "cloud-compose", "static", "single-image":
					args = append(args, strVal)
				case "--type", "--server", "--client":
					// LLM sometimes provides flag names instead of values
					args = append(args, "client") // Safe default
				default:
					args = append(args, strVal) // Pass through unknown types
				}
			}
		default:
			// Generic string argument
			if strVal, ok := value.(string); ok && strVal != "" {
				args = append(args, strVal)
			}
		}
	}

	return args
}

// FormatToolCallResult formats a tool call result for inclusion in conversation
func (h *ToolCallHandler) FormatToolCallResult(toolCall llm.ToolCall, result *CommandResult) string {
	var response strings.Builder

	response.WriteString(fmt.Sprintf("**Tool Call: %s**\n", toolCall.Function.Name))

	if result.Success {
		response.WriteString(fmt.Sprintf("✅ %s\n", result.Message))

		// Include actual configuration data for config command
		if toolCall.Function.Name == "config" && result.Data != nil {
			if content, exists := result.Data["content"].(map[string]interface{}); exists {
				// Convert to YAML and include in tool result so LLM can see actual data
				if yamlBytes, err := yaml.Marshal(content); err == nil {
					response.WriteString("\n**Actual Configuration Content:**\n")
					response.WriteString("```yaml\n")
					response.WriteString(string(yamlBytes))
					response.WriteString("```\n")
				}
			}
		}

		// Include file information if files were generated
		if len(result.Files) > 0 {
			response.WriteString("\n**Generated Files:**\n")
			for _, file := range result.Files {
				response.WriteString(fmt.Sprintf("- `%s`: %s\n", file.Path, file.Description))
			}
		}

		// Include next steps if provided
		if result.NextStep != "" {
			response.WriteString(fmt.Sprintf("\n**Next Steps:** %s\n", result.NextStep))
		}
	} else {
		response.WriteString(fmt.Sprintf("❌ %s\n", result.Message))
	}

	return response.String()
}

// FormatToolCallForConversation creates a formatted message about the tool call for the conversation
func (h *ToolCallHandler) FormatToolCallForConversation(toolCall llm.ToolCall, result *CommandResult) string {
	// Create a natural language description of what was executed
	functionName := toolCall.Function.Name

	switch functionName {
	case "setup":
		if result.Success {
			fileCount := len(result.Files)
			return fmt.Sprintf("I've successfully generated %d configuration files for your project. The setup includes client.yaml, docker-compose.yaml, and Dockerfile optimized for your detected tech stack.", fileCount)
		}
		return fmt.Sprintf("I attempted to set up your project configuration, but encountered an issue: %s", result.Message)

	case "analyze":
		if result.Success {
			return "I've completed a comprehensive analysis of your project and updated the context with the latest findings."
		}
		return fmt.Sprintf("I tried to analyze your project but ran into a problem: %s", result.Message)

	case "switch":
		if result.Success {
			return "I've successfully updated your preferences as requested."
		}
		return fmt.Sprintf("I tried to switch settings but encountered an error: %s", result.Message)

	case "search_docs":
		if result.Success {
			return "I've retrieved relevant documentation to help answer your question. Let me use this information to provide specific guidance."
		}
		return fmt.Sprintf("I tried to search the documentation but encountered an issue: %s", result.Message)

	default:
		if result.Success {
			return fmt.Sprintf("I've successfully executed the %s command: %s", functionName, result.Message)
		}
		return fmt.Sprintf("I tried to execute the %s command but encountered an error: %s", functionName, result.Message)
	}
}
