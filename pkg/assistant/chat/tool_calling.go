package chat

import (
	"context"
	"fmt"
	"strings"

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
		case "full":
			argSchema["type"] = "boolean"
			argSchema["description"] = "Run comprehensive analysis (slower but thorough)"
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

	// Special handling for switch command - it expects mode as positional argument
	if functionName == "switch" {
		if modeVal, exists := arguments["mode"]; exists {
			if strVal, ok := modeVal.(string); ok {
				args = append(args, strVal)
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

	default:
		if result.Success {
			return fmt.Sprintf("I've successfully executed the %s command: %s", functionName, result.Message)
		}
		return fmt.Sprintf("I tried to execute the %s command but encountered an error: %s", functionName, result.Message)
	}
}
