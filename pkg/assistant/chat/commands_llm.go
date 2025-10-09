package chat

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/term"

	"github.com/simple-container-com/api/pkg/api/logger/color"
	"github.com/simple-container-com/api/pkg/assistant/config"
)

// registerLLMCommands registers LLM provider and model management commands
func (c *ChatInterface) registerLLMCommands() {
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
			Message: "Please specify an action: list, switch, or info\nUsage: /model <list|switch|info> [model]",
		}, nil
	}

	action := strings.ToLower(args[0])

	// Get current provider
	provider := cfg.GetDefaultProvider()
	if provider == "" {
		return &CommandResult{
			Success: false,
			Message: "❌ No provider configured\nUse '/apikey set [provider]' to configure one first",
		}, nil
	}

	// Check if we have an active LLM connection
	if c.llm == nil {
		return &CommandResult{
			Success: false,
			Message: "❌ No active LLM connection\nPlease check your provider configuration",
		}, nil
	}

	capabilities := c.llm.GetCapabilities()

	switch action {
	case "list":
		message := fmt.Sprintf("🤖 Available Models for %s:\n", capabilities.Name)

		if len(capabilities.Models) == 0 {
			message += "\n❌ No models available or unable to fetch model list"
			if provider == config.ProviderOllama {
				message += "\n\nFor Ollama, make sure:"
				message += "\n  • Ollama is running (ollama serve)"
				message += "\n  • Models are installed (ollama pull <model>)"
				message += "\n  • Base URL is correct in your configuration"
			}
		} else {
			providerCfg, _ := cfg.GetProviderConfig(provider)
			currentModel := providerCfg.Model
			if currentModel == "" {
				currentModel = c.llm.GetModel()
			}
			for i, model := range capabilities.Models {
				indicator := ""
				if model == currentModel {
					indicator = " ⭐ (current)"
				}
				message += fmt.Sprintf("\n  %d. %s%s", i+1, model, indicator)
			}
			message += "\n\nUse '/model switch <model>' to change the current model"
		}

		return &CommandResult{
			Success: true,
			Message: message,
		}, nil

	case "switch":
		if len(args) < 2 {
			return &CommandResult{
				Success: false,
				Message: "Please specify a model name\nUsage: /model switch <model>",
			}, nil
		}

		modelName := args[1]

		// For providers with dynamic model lists, validate the model
		if len(capabilities.Models) > 0 {
			validModel := false
			for _, availableModel := range capabilities.Models {
				if availableModel == modelName {
					validModel = true
					break
				}
			}
			if !validModel {
				return &CommandResult{
					Success: false,
					Message: fmt.Sprintf("❌ Model '%s' is not available for %s\nUse '/model list' to see available models", modelName, capabilities.Name),
				}, nil
			}
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
				Message: fmt.Sprintf("Failed to save model selection: %v", err),
			}, nil
		}

		// Reload LLM provider with new model
		if err := c.ReloadLLMProvider(); err != nil {
			return &CommandResult{
				Success: false,
				Message: fmt.Sprintf("⚠️  Model saved but failed to reload provider: %v\nThe model will be used on next restart.", err),
			}, nil
		}

		return &CommandResult{
			Success: true,
			Message: fmt.Sprintf("✅ Switched to model '%s' for %s\nYou can continue chatting with the new model.", modelName, capabilities.Name),
		}, nil

	case "info":
		providerCfg, _ := cfg.GetProviderConfig(provider)
		currentModel := providerCfg.Model
		if currentModel == "" {
			currentModel = c.llm.GetModel()
		}
		if currentModel == "" {
			currentModel = "default"
		}

		message := "ℹ️  Current Model Information:\n"
		message += fmt.Sprintf("\n  Provider: %s", capabilities.Name)
		message += fmt.Sprintf("\n  Model: %s", currentModel)
		message += fmt.Sprintf("\n  Max Tokens: %d", capabilities.MaxTokens)
		message += fmt.Sprintf("\n  Supports Streaming: %v", capabilities.SupportsStreaming)
		message += fmt.Sprintf("\n  Supports Functions: %v", capabilities.SupportsFunctions)
		if capabilities.CostPerToken > 0 {
			message += fmt.Sprintf("\n  Cost Per Token: $%.6f", capabilities.CostPerToken)
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

// selectConfiguredProvider shows an interactive menu to select from configured providers only
func (c *ChatInterface) selectConfiguredProvider(cfg *config.Config) (string, error) {
	providers := cfg.ListProviders()
	if len(providers) == 0 {
		return "", fmt.Errorf("no providers are configured")
	}

	// Display menu
	fmt.Println(color.CyanString("\n📋 Select a provider to switch to:"))
	fmt.Println()

	defaultProvider := cfg.GetDefaultProvider()
	for i, provider := range providers {
		providerName := config.GetProviderDisplayName(provider)
		current := ""
		if provider == defaultProvider {
			current = color.GreenString(" (current)")
		}
		fmt.Printf("  %d. %s%s\n", i+1, providerName, current)
	}

	fmt.Println()

	// Read user input using inputHandler
	input, err := c.inputHandler.ReadSimple(color.CyanString("Enter number or 'q' to cancel: "))
	if err != nil {
		return "", err
	}

	// Check for cancel
	if input == "q" || input == "Q" || input == "quit" || input == "cancel" {
		return "", nil
	}

	// Parse selection
	selection, err := strconv.Atoi(input)
	if err != nil || selection < 1 || selection > len(providers) {
		return "", fmt.Errorf("invalid selection: %s", input)
	}

	selectedProvider := providers[selection-1]
	fmt.Println(color.GreenString(fmt.Sprintf("✓ Selected: %s", config.GetProviderDisplayName(selectedProvider))))
	fmt.Println()

	return selectedProvider, nil
}
