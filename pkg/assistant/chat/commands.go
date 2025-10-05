package chat

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/term"

	"github.com/fatih/color"

	"github.com/simple-container-com/api/pkg/assistant/analysis"
	"github.com/simple-container-com/api/pkg/assistant/config"
	"github.com/simple-container-com/api/pkg/assistant/embeddings"
	"github.com/simple-container-com/api/pkg/assistant/llm/prompts"
	"github.com/simple-container-com/api/pkg/assistant/modes"
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

	c.commands["history"] = &ChatCommand{
		Name:        "history",
		Description: "Show command history",
		Usage:       "/history [clear]",
		Handler:     c.handleHistory,
		Args: []CommandArg{
			{Name: "action", Type: "string", Required: false, Description: "Action: clear to clear history"},
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
		NextStep: "Review the generated files and run 'docker-compose up -d' from ${project:root} to test locally, then 'sc deploy -e staging' to deploy",
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
		message += "\n\n🚀 Developer Mode: I'll help you set up your application with client.yaml, ${project:root}/docker-compose.yaml, and ${project:root}/Dockerfile"
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
		UseStreaming:   false,
		BackupExisting: false, // Chat interface handles file writing separately
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

	// Generate client.yaml using DeveloperMode logic
	clientYaml, err := c.developerMode.GenerateClientYAMLWithLLM(opts, projectAnalysis)
	if err == nil {
		files = append(files, GeneratedFile{
			Path:        ".sc/stacks/" + projectName + "/client.yaml",
			Type:        "yaml",
			Description: "Simple Container client configuration",
			Generated:   true,
			Content:     clientYaml,
		})
	}

	// Generate docker-compose.yaml using DeveloperMode logic
	if !opts.SkipCompose {
		composeYaml, err := c.developerMode.GenerateComposeYAMLWithLLM(projectAnalysis)
		if err == nil {
			files = append(files, GeneratedFile{
				Path:        "docker-compose.yaml",
				Type:        "yaml",
				Description: "Local development environment",
				Generated:   true,
				Content:     composeYaml,
			})
		}
	}

	// Generate Dockerfile using DeveloperMode logic
	if !opts.SkipDockerfile {
		dockerfile, err := c.developerMode.GenerateDockerfileWithLLM(projectAnalysis)
		if err == nil {
			files = append(files, GeneratedFile{
				Path:        "Dockerfile",
				Type:        "dockerfile",
				Description: "Container image definition",
				Generated:   true,
				Content:     dockerfile,
			})
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
    type: aws-ecs-fargate
    config:
      ecsClusterResource: ecs-cluster
      ecrRepositoryResource: app-registry
    
  api-service:
    type: aws-ecs-fargate
    config:
      ecsClusterResource: ecs-cluster
      ecrRepositoryResource: api-registry

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
        # Compute cluster
        ecs-cluster:
          type: aws-ecs-cluster
          config:
            name: myapp-staging-cluster
            
        # Container registry
        app-registry:
          type: aws-ecr-repository
          config:
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
        # Compute cluster
        ecs-cluster:
          type: aws-ecs-cluster
          config:
            name: myapp-prod-cluster
            
        # Container registry
        app-registry:
          type: aws-ecr-repository
          config:
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
	secretsContent := `# Authentication for cloud providers
auth:
  aws:
    account: "123456789012"
    accessKey: "${secret:aws-access-key}"
    secretAccessKey: "${secret:aws-secret-key}"
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
		selectedProvider, err := selectProvider(cfg)
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

		// Prompt for API key
		fmt.Print(color.CyanString(fmt.Sprintf("🔑 Enter your %s API key: ", providerName)))
		apiKey, err := readSecureInput()
		if err != nil {
			return &CommandResult{
				Success: false,
				Message: fmt.Sprintf("Failed to read API key: %v", err),
			}, nil
		}

		if apiKey == "" {
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

		configPath, _ := config.ConfigPath()
		return &CommandResult{
			Success:  true,
			Message:  fmt.Sprintf("✅ %s API key saved successfully to %s", providerName, configPath),
			NextStep: fmt.Sprintf("Provider '%s' is now set as default. Use '/provider switch' to change providers.", provider),
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
			selectedProvider, err := selectConfiguredProvider(cfg)
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
func selectProvider(cfg *config.Config) (string, error) {
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
	fmt.Print(color.CyanString("Enter number (1-5) or 'q' to cancel: "))

	// Read user input
	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}

	input = strings.TrimSpace(input)

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
func selectConfiguredProvider(cfg *config.Config) (string, error) {
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
	fmt.Print(color.CyanString(fmt.Sprintf("Enter number (1-%d) or 'q' to cancel: ", len(configuredProviders))))

	// Read user input
	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}

	input = strings.TrimSpace(input)

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
