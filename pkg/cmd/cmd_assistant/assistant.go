package cmd_assistant

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"golang.org/x/term"

	"github.com/spf13/cobra"

	"github.com/simple-container-com/api/pkg/api/logger"
	"github.com/simple-container-com/api/pkg/api/logger/color"
	"github.com/simple-container-com/api/pkg/assistant/chat"
	"github.com/simple-container-com/api/pkg/assistant/config"
	"github.com/simple-container-com/api/pkg/assistant/core"
	"github.com/simple-container-com/api/pkg/assistant/embeddings"
	"github.com/simple-container-com/api/pkg/assistant/mcp"
	"github.com/simple-container-com/api/pkg/assistant/modes"
	"github.com/simple-container-com/api/pkg/cmd/root_cmd"
)

type AssistantCmd struct {
	rootCmd       *root_cmd.RootCmd
	coreManager   *core.Manager
	developerMode *modes.DeveloperMode
	devopsMode    *modes.DevOpsMode
	mode          string // Current mode (dev, devops, general)
}

// initCoreManager ensures the core manager is initialized
func (a *AssistantCmd) initCoreManager() {
	if a.coreManager == nil {
		config := core.DefaultManagerConfig()
		a.coreManager = core.NewManager(config, a.rootCmd.Logger)
	}
}

// initCoreManagerWithTesting ensures the core manager is initialized with testing enabled
func (a *AssistantCmd) initCoreManagerWithTesting() {
	if a.coreManager == nil {
		config := core.DefaultManagerConfig()
		config.EnableTesting = true // Enable testing for test command
		a.coreManager = core.NewManager(config, a.rootCmd.Logger)
	}
}

// initDeveloperMode initializes developer mode lazily
func (a *AssistantCmd) initDeveloperMode() {
	if a.developerMode == nil {
		a.developerMode = modes.NewDeveloperMode()
	}
}

// initDevOpsMode initializes devops mode lazily
func (a *AssistantCmd) initDevOpsMode() {
	if a.devopsMode == nil {
		a.devopsMode = modes.NewDevOpsMode()
	}
}

func NewAssistantCmd(rootCmd *root_cmd.RootCmd) *cobra.Command {
	assistantCmd := &AssistantCmd{
		rootCmd:       rootCmd,
		coreManager:   nil, // Will be initialized lazily
		developerMode: nil, // Will be initialized lazily when needed
		devopsMode:    nil, // Will be initialized lazily when needed
	}

	cmd := &cobra.Command{
		Use:   "assistant",
		Short: "AI-powered project onboarding assistant",
		Long: `AI-powered assistant for Simple Container with two distinct modes:

👩‍💻 Developer Mode - For application teams:
  • Generate client.yaml and docker-compose files
  • Analyze project tech stack and dependencies
  • Set up application deployment configurations

🛠️ DevOps Mode - For infrastructure teams:
  • Generate server.yaml and secrets configuration
  • Set up shared resources and cloud infrastructure
  • Manage multi-environment deployments

🔍 Shared Features:
  • Semantic documentation search
  • MCP server for external tool integration`,
		Example: `  # Developer Mode
  sc assistant dev setup
  sc assistant dev analyze
  
  # DevOps Mode
  sc assistant devops setup
  sc assistant devops resources --add postgres
  
  # Shared Features
  sc assistant search "postgres configuration"
  sc assistant mcp --port 9999`,
	}

	// Add subcommands
	cmd.AddCommand(
		assistantCmd.newDeveloperCmd(),
		assistantCmd.newDevOpsCmd(),
		assistantCmd.newSearchCmd(),
		assistantCmd.newChatCmd(),
		assistantCmd.newMCPCmd(),
		assistantCmd.newTestCmd(),
		assistantCmd.newHealthCmd(),
		assistantCmd.newStatsCmd(),
	)

	return cmd
}

// Developer mode commands
func (a *AssistantCmd) newDeveloperCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dev",
		Short: "Developer mode commands",
		Long:  "Commands for application developers to set up client configurations",
	}

	cmd.AddCommand(
		a.newDevSetupCmd(),
		a.newDevAnalyzeCmd(),
	)

	return cmd
}

// DevOps mode commands
func (a *AssistantCmd) newDevOpsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "devops",
		Short: "DevOps mode commands",
		Long:  "Commands for infrastructure teams to set up shared resources",
	}

	cmd.AddCommand(
		a.newDevOpsSetupCmd(),
		a.newDevOpsResourcesCmd(),
		a.newDevOpsSecretsCmd(),
	)

	return cmd
}

func (a *AssistantCmd) newChatCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "chat [project-path]",
		Short: "Start interactive chat mode",
		Long: `Start an interactive conversation with the Simple Container AI assistant.
		
The AI assistant will help you:
- Analyze your project and detect technology stack
- Generate configuration files (client.yaml, docker-compose.yaml, Dockerfile)
- Search documentation and answer questions
- Provide guidance on Simple Container best practices

Examples:
  sc assistant chat                          # Start chat in current directory
  sc assistant chat /path/to/project        # Start chat for specific project
  sc assistant chat --mode dev              # Start in developer mode
  sc assistant chat --mode devops           # Start in DevOps mode
  sc assistant chat --openai-key sk-...     # Use specific OpenAI API key`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runChat(cmd, args)
		},
	}

	// Add flags
	cmd.Flags().StringVar(&a.mode, "mode", "general", "Chat mode: dev, devops, or general")
	cmd.Flags().String("openai-key", "", "OpenAI API key (or set OPENAI_API_KEY env var)")
	cmd.Flags().String("llm-provider", "openai", "LLM provider: openai")
	cmd.Flags().Int("max-tokens", 2048, "Maximum tokens per response")
	cmd.Flags().Float32("temperature", 0.7, "LLM temperature (0.0-1.0)")
	cmd.Flags().Bool("verbose", false, "Verbose output")

	return cmd
}

// Developer mode subcommands
func (a *AssistantCmd) newDevSetupCmd() *cobra.Command {
	var opts modes.SetupOptions

	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Generate application configuration files",
		Long:  "Analyze project and generate client.yaml, docker-compose.yaml, and Dockerfile",
		RunE: func(cmd *cobra.Command, args []string) error {
			a.initDeveloperMode()
			return a.developerMode.Setup(cmd.Context(), &opts)
		},
	}

	cmd.Flags().BoolVarP(&opts.Interactive, "interactive", "i", false, "Interactive mode with prompts")
	cmd.Flags().StringVar(&opts.Environment, "env", "staging", "Target environment")
	cmd.Flags().StringVar(&opts.Parent, "parent", "infrastructure", "Parent stack name")
	cmd.Flags().BoolVar(&opts.SkipAnalysis, "skip-analysis", false, "Skip automatic project analysis")
	cmd.Flags().BoolVar(&opts.SkipDockerfile, "skip-dockerfile", false, "Skip Dockerfile generation")
	cmd.Flags().BoolVar(&opts.SkipCompose, "skip-compose", false, "Skip docker-compose.yaml generation")
	cmd.Flags().StringVar(&opts.Language, "language", "", "Override detected language")
	cmd.Flags().StringVar(&opts.Framework, "framework", "", "Override detected framework")
	cmd.Flags().StringVar(&opts.CloudProvider, "cloud", "", "Target cloud provider")
	cmd.Flags().StringVar(&opts.OutputDir, "output-dir", "", "Output directory")

	// Multi-file generation options
	cmd.Flags().BoolVar(&opts.GenerateAll, "generate-all", false, "Generate all files using coordinated multi-file generation for better consistency")
	cmd.Flags().BoolVar(&opts.UseStreaming, "streaming", false, "Use streaming LLM responses for real-time progress feedback")
	cmd.Flags().BoolVar(&opts.BackupExisting, "backup-existing", true, "Backup existing files before overwriting")

	return cmd
}

func (a *AssistantCmd) newDevAnalyzeCmd() *cobra.Command {
	var opts modes.AnalyzeOptions

	cmd := &cobra.Command{
		Use:   "analyze",
		Short: "Analyze project structure and tech stack",
		Long:  "Detect technology stack, dependencies, and architecture patterns",
		RunE: func(cmd *cobra.Command, args []string) error {
			a.initDeveloperMode()
			return a.developerMode.Analyze(cmd.Context(), &opts)
		},
	}

	cmd.Flags().BoolVar(&opts.Detailed, "detailed", false, "Show detailed analysis output")
	cmd.Flags().StringVar(&opts.Path, "path", ".", "Project path to analyze")
	cmd.Flags().StringVar(&opts.Output, "output", "", "Export analysis to file")
	cmd.Flags().StringVar(&opts.Format, "format", "table", "Output format (table, json, yaml)")

	return cmd
}

// DevOps mode subcommands
func (a *AssistantCmd) newDevOpsSetupCmd() *cobra.Command {
	var opts modes.DevOpsSetupOptions
	var envString string
	var resourceString string
	var templateString string

	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Set up infrastructure configuration",
		Long:  "Interactive wizard to set up server.yaml, secrets.yaml, and shared resources",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Parse comma-separated values
			if envString != "" {
				opts.Environments = strings.Split(envString, ",")
			}
			if resourceString != "" {
				opts.Resources = strings.Split(resourceString, ",")
			}
			if templateString != "" {
				opts.Templates = strings.Split(templateString, ",")
			}

			a.initDevOpsMode()
			return a.devopsMode.Setup(cmd.Context(), opts)
		},
	}

	cmd.Flags().BoolVarP(&opts.Interactive, "interactive", "i", true, "Interactive wizard mode")
	cmd.Flags().StringVar(&opts.CloudProvider, "cloud", "", "Cloud provider (aws, gcp, k8s)")
	cmd.Flags().StringVar(&envString, "envs", "", "Comma-separated environments")
	cmd.Flags().StringVar(&resourceString, "resources", "", "Comma-separated resource types")
	cmd.Flags().StringVar(&templateString, "templates", "", "Template names to create")
	cmd.Flags().StringVar(&opts.Prefix, "prefix", "", "Resource name prefix")
	cmd.Flags().StringVar(&opts.Region, "region", "", "Default cloud region")
	cmd.Flags().StringVar(&opts.OutputDir, "output-dir", "", "Output directory")

	return cmd
}

func (a *AssistantCmd) newDevOpsResourcesCmd() *cobra.Command {
	var opts modes.ResourceOptions

	cmd := &cobra.Command{
		Use:   "resources",
		Short: "Manage shared infrastructure resources",
		Long:  "Add, remove, or update shared infrastructure resources",
		RunE: func(cmd *cobra.Command, args []string) error {
			a.initDevOpsMode()
			return a.devopsMode.Resources(cmd.Context(), opts)
		},
	}

	cmd.Flags().BoolVar(&opts.Interactive, "list", false, "List available resource types")
	cmd.Flags().StringVar(&opts.ResourceType, "add", "", "Add resource type")
	cmd.Flags().StringVar(&opts.ResourceName, "remove", "", "Remove resource")
	cmd.Flags().StringVar(&opts.ResourceName, "update", "", "Update resource")
	cmd.Flags().StringVar(&opts.Environment, "env", "", "Target environment")
	cmd.Flags().BoolVarP(&opts.Interactive, "interactive", "i", false, "Interactive resource configuration")

	return cmd
}

func (a *AssistantCmd) newDevOpsSecretsCmd() *cobra.Command {
	var opts modes.SecretsOptions
	var secretString string

	cmd := &cobra.Command{
		Use:   "secrets",
		Short: "Manage authentication credentials and secrets",
		Long:  "Initialize, configure, and manage secrets for cloud providers",
		RunE: func(cmd *cobra.Command, args []string) error {
			if secretString != "" {
				opts.SecretNames = strings.Split(secretString, ",")
			}
			a.initDevOpsMode()
			return a.devopsMode.Secrets(cmd.Context(), opts)
		},
	}

	cmd.Flags().BoolVar(&opts.Interactive, "init", false, "Initialize secrets configuration")
	cmd.Flags().StringVar(&opts.Provider, "auth", "", "Configure cloud provider authentication")
	cmd.Flags().StringVar(&secretString, "generate", "", "Generate random secrets (comma-separated)")
	cmd.Flags().BoolVarP(&opts.Interactive, "interactive", "i", false, "Interactive secret entry")
	cmd.Flags().IntVar(&opts.Length, "length", 32, "Generated secret length")

	return cmd
}

func (a *AssistantCmd) newSearchCmd() *cobra.Command {
	var limit int
	var docType string

	cmd := &cobra.Command{
		Use:   "search [query]",
		Short: "Search documentation semantically",
		Long:  "Search Simple Container documentation using semantic similarity",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			query := strings.Join(args, " ")
			return a.runSearch(cmd, query, limit, docType)
		},
	}

	cmd.Flags().IntVar(&limit, "limit", 5, "Maximum number of results to return")
	cmd.Flags().StringVar(&docType, "type", "", "Document type (docs, examples, schemas)")

	return cmd
}

func (a *AssistantCmd) newMCPCmd() *cobra.Command {
	var (
		port int
		host string
	)

	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Start MCP (Model Context Protocol) server",
		Long:  "Start a JSON-RPC server that exposes Simple Container context to external LLM tools",
		RunE: func(cmd *cobra.Command, args []string) error {
			host, _ := cmd.Flags().GetString("host")
			port, _ := cmd.Flags().GetInt("port")
			stdio, _ := cmd.Flags().GetBool("stdio")

			// Setup signal handling for graceful shutdown
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			if stdio {
				// Use stdin/stdout mode for IDE integration
				// No output to stdout - only JSON-RPC responses

				// Handle CTRL+C gracefully in stdio mode
				sigCh := make(chan os.Signal, 1)
				signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

				// Initialize MCP server
				server := mcp.NewMCPServer(host, port)

				// Start MCP server in goroutine
				errCh := make(chan error, 1)
				go func() {
					errCh <- server.StartStdio(ctx)
				}()

				// Wait for completion or signal
				select {
				case err := <-errCh:
					return err
				case <-sigCh:
					cancel()
					// Give server a moment to shut down gracefully
					time.Sleep(100 * time.Millisecond)
					return nil
				}
			} else {
				// Use HTTP mode
				fmt.Printf("🚀 Starting Simple Container MCP Server...\n")
				fmt.Printf("   Host: %s\n", host)
				fmt.Printf("   Port: %d\n", port)
				fmt.Printf("   Protocol: JSON-RPC 2.0 over HTTP\n\n")

				// Handle CTRL+C gracefully
				sigCh := make(chan os.Signal, 1)
				signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

				// Initialize MCP server
				server := mcp.NewMCPServer(host, port)

				fmt.Printf("📡 MCP Server ready for Windsurf integration\n")
				fmt.Printf("   Health: http://%s:%d/health\n", host, port)
				fmt.Printf("   Capabilities: http://%s:%d/capabilities\n", host, port)
				fmt.Printf("   MCP Endpoint: http://%s:%d/mcp\n", host, port)
				fmt.Printf("\n🔗 To integrate with Windsurf, add this MCP server configuration\n")
				fmt.Printf("   Press Ctrl+C to stop\n\n")

				// Start MCP server in goroutine
				errCh := make(chan error, 1)
				go func() {
					errCh <- server.Start(ctx)
				}()

				// Wait for completion or signal
				select {
				case err := <-errCh:
					return err
				case <-sigCh:
					fmt.Println("\n🛑 Shutting down MCP server...")
					cancel()
					// Give server a moment to shut down gracefully
					time.Sleep(100 * time.Millisecond)
					return nil
				}
			}
		},
	}

	cmd.Flags().IntVar(&port, "port", 9999, "Port to listen on")
	cmd.Flags().StringVar(&host, "host", "localhost", "Host to bind to")
	cmd.Flags().Bool("stdio", false, "Use stdin/stdout for JSON-RPC communication (for IDE integration)")

	return cmd
}

// Implementation of command handlers

func (a *AssistantCmd) runChat(cmd *cobra.Command, args []string) error {
	// Get project path from args or current directory
	projectPath := "."
	if len(args) > 0 {
		projectPath = args[0]
	}

	// Read configuration from flags
	llmProvider, _ := cmd.Flags().GetString("llm-provider")
	maxTokens, _ := cmd.Flags().GetInt("max-tokens")
	temperature, _ := cmd.Flags().GetFloat32("temperature")
	verbose, _ := cmd.Flags().GetBool("verbose")

	// Handle OpenAI API key with priority:
	// 1. Command line flag
	// 2. Environment variable
	// 3. Stored config
	// 4. Interactive prompt
	apiKey := ""
	if flagKey, _ := cmd.Flags().GetString("openai-key"); flagKey != "" {
		apiKey = flagKey
		os.Setenv("OPENAI_API_KEY", apiKey)
	} else if envKey := os.Getenv("OPENAI_API_KEY"); envKey != "" {
		apiKey = envKey
	} else {
		// Try to load from config
		cfg, err := config.Load()
		if err == nil {
			// Get default provider or use openai
			provider := cfg.GetDefaultProvider()
			if provider == "" {
				provider = config.ProviderOpenAI
			}

			// Load provider config
			if providerCfg, exists := cfg.GetProviderConfig(provider); exists && providerCfg.APIKey != "" {
				apiKey = providerCfg.APIKey
				os.Setenv("OPENAI_API_KEY", apiKey)
				providerName := config.GetProviderDisplayName(provider)
				fmt.Println(color.GreenFmt(fmt.Sprintf("✅ Using stored %s API key", providerName)))

				// Show provider info
				if providerCfg.BaseURL != "" {
					fmt.Println(color.CyanFmt(fmt.Sprintf("   Base URL: %s", providerCfg.BaseURL)))
				}
				if providerCfg.Model != "" {
					fmt.Println(color.CyanFmt(fmt.Sprintf("   Model: %s", providerCfg.Model)))
				}
			}
		}
	}

	// If still no API key and using OpenAI, prompt for it
	if llmProvider == "openai" && apiKey == "" {
		fmt.Println(color.YellowFmt("⚠️  OpenAI API key not found"))
		fmt.Println("You can provide your OpenAI API key in several ways:")
		fmt.Println("  1. Stored config: Use '/apikey set' command in chat to save it permanently")
		fmt.Println("  2. Environment variable: export OPENAI_API_KEY=sk-your-key-here")
		fmt.Println("  3. Command line flag: sc assistant chat --openai-key sk-your-key-here")
		fmt.Println("  4. Enter it interactively now (not saved)")
		fmt.Println()
		fmt.Println("Get your API key from: " + color.CyanFmt("https://platform.openai.com/api-keys"))
		fmt.Println()

		// Prompt for interactive input
		apiKey, err := promptForOpenAIKey()
		if err != nil {
			return fmt.Errorf("failed to read OpenAI API key: %w", err)
		}

		if apiKey == "" {
			fmt.Println(color.RedFmt("❌ OpenAI API key is required for chat mode"))
			return fmt.Errorf("OpenAI API key required for chat mode")
		}

		// Set the API key for this session
		os.Setenv("OPENAI_API_KEY", apiKey)

		// Ask if user wants to save it permanently
		fmt.Print(color.YellowFmt("💾 Save this API key for future sessions? (Y/n): "))
		reader := bufio.NewReader(os.Stdin)
		response, err := reader.ReadString('\n')
		if err == nil {
			response = strings.ToLower(strings.TrimSpace(response))
			if response == "" || response == "y" || response == "yes" {
				// Save to config
				cfg, err := config.Load()
				if err == nil {
					if err := cfg.SetOpenAIAPIKey(apiKey); err == nil {
						configPath, _ := config.ConfigPath()
						fmt.Println(color.GreenFmt("✅ API key saved to " + configPath))
					} else {
						fmt.Println(color.YellowFmt("⚠️  Failed to save API key: " + err.Error()))
					}
				}
			} else {
				fmt.Println(color.YellowFmt("💡 Tip: Use '/apikey set' in chat to save it later"))
			}
		}
		fmt.Println()
	}

	// Create session config
	config := chat.DefaultSessionConfig()
	config.ProjectPath = projectPath
	config.Mode = a.mode
	config.LLMProvider = llmProvider
	config.MaxTokens = maxTokens
	config.Temperature = temperature

	if verbose {
		config.LogLevel = "debug"
	}

	// Create chat interface
	chatInterface, err := chat.NewChatInterface(config)
	if err != nil {
		return fmt.Errorf("failed to initialize chat interface: %w", err)
	}
	defer chatInterface.Close()

	// Setup signal handling for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle CTRL+C gracefully
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	// Start chat session in goroutine
	errCh := make(chan error, 1)
	go func() {
		errCh <- chatInterface.StartSession(ctx)
	}()

	// Wait for completion or signal
	select {
	case err := <-errCh:
		return err
	case <-sigCh:
		fmt.Println("\n\n👋 Goodbye! Chat session ended.")
		cancel()
		return nil
	}
}

func (a *AssistantCmd) runSearch(cmd *cobra.Command, query string, limit int, docType string) error {
	fmt.Printf("🔍 Searching documentation for: %s\n", color.CyanFmt(query))
	if docType != "" {
		fmt.Printf("📋 Document type: %s\n", color.YellowFmt(docType))
	}
	fmt.Println()

	// Set up logging context based on verbose flag
	ctx := context.Background()
	log := logger.New()
	verbose, _ := cmd.Flags().GetBool("verbose")
	if verbose {
		ctx = log.SetLogLevel(ctx, logger.LogLevelDebug)
	}

	// Load embedded documentation database
	db, err := embeddings.LoadEmbeddedDatabase(ctx)
	if err != nil {
		return fmt.Errorf("failed to load documentation database: %w", err)
	}

	// Perform semantic search
	results, err := embeddings.SearchDocumentation(db, query, limit)
	if err != nil {
		return fmt.Errorf("failed to search documentation: %w", err)
	}

	// Filter by document type if specified
	if docType != "" {
		filtered := []embeddings.SearchResult{}
		for _, result := range results {
			if result.Metadata["type"] == docType {
				filtered = append(filtered, result)
			}
		}
		results = filtered
	}

	if len(results) == 0 {
		fmt.Println(color.YellowFmt("No relevant documentation found."))
		return nil
	}

	// Display results
	fmt.Printf("Found %d relevant documents:\n\n", len(results))

	for i, result := range results {
		title, ok := result.Metadata["title"].(string)
		if !ok || title == "" {
			title = result.ID // Fallback to document ID
		}
		fmt.Printf("%s%d. %s%s\n", color.GreenFmt(""), i+1, color.BoldFmt(""), title)
		fmt.Printf("   Path: %s\n", result.Metadata["path"])
		fmt.Printf("   Type: %s\n", result.Metadata["type"])
		fmt.Printf("   Similarity: %.3f\n", result.Similarity)

		// Show content preview (first 200 chars)
		content := result.Content
		if len(content) > 200 {
			content = content[:200] + "..."
		}
		fmt.Printf("   Preview: %s\n\n", color.GrayFmt(content))
	}

	return nil
}

// promptForOpenAIKey prompts the user to enter their OpenAI API key securely
func promptForOpenAIKey() (string, error) {
	fmt.Print(color.CyanFmt("🔑 Enter your OpenAI API key: "))

	// Check if we're running in a terminal
	if !term.IsTerminal(int(syscall.Stdin)) {
		// Not a terminal, read from stdin normally
		reader := bufio.NewReader(os.Stdin)
		apiKey, err := reader.ReadString('\n')
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(apiKey), nil
	}

	// Read password from terminal (hidden input)
	bytePassword, err := term.ReadPassword(int(syscall.Stdin))
	if err != nil {
		return "", err
	}

	fmt.Println() // Add newline after hidden input
	apiKey := strings.TrimSpace(string(bytePassword))

	// Basic validation - OpenAI keys should start with "sk-"
	if apiKey != "" && !strings.HasPrefix(apiKey, "sk-") {
		fmt.Println(color.YellowFmt("⚠️  Warning: OpenAI API keys typically start with 'sk-'"))
		fmt.Print("Continue anyway? (y/N): ")

		reader := bufio.NewReader(os.Stdin)
		response, err := reader.ReadString('\n')
		if err != nil {
			return "", err
		}

		response = strings.ToLower(strings.TrimSpace(response))
		if response != "y" && response != "yes" {
			return "", fmt.Errorf("API key validation failed")
		}
	}

	return apiKey, nil
}

// Test command for comprehensive testing
func (a *AssistantCmd) newTestCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "test [project-path]",
		Short: "Run comprehensive AI Assistant tests",
		Long: `Run comprehensive tests of all AI Assistant components including:
- Embeddings system functionality
- Project analysis capabilities  
- Performance benchmarks
- Schema validation
- Memory usage analysis`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			// Initialize core manager
			a.initCoreManagerWithTesting()
			if err := a.coreManager.Initialize(ctx); err != nil {
				return fmt.Errorf("failed to initialize core manager: %w", err)
			}
			defer func() { _ = a.coreManager.Shutdown(ctx) }()

			// Determine project path
			projectPath := "."
			if len(args) > 0 {
				projectPath = args[0]
			}

			fmt.Println(color.CyanFmt("🧪 Starting AI Assistant comprehensive test suite..."))
			fmt.Printf("   Project path: %s\n\n", projectPath)

			// Run tests
			results, err := a.coreManager.RunTests(ctx, projectPath)
			if err != nil {
				return fmt.Errorf("test execution failed: %w", err)
			}

			// Display summary
			totalTests := 0
			totalPassed := 0
			totalFailed := 0

			for suiteName, suite := range results {
				totalTests += suite.Total
				totalPassed += suite.Passed
				totalFailed += suite.Failed

				status := color.GreenFmt("PASS")
				if suite.Failed > 0 {
					status = color.RedFmt("FAIL")
				}

				fmt.Printf("Suite %s: %s (%d/%d passed, %v duration)\n",
					suiteName, status, suite.Passed, suite.Total, suite.Duration)
			}

			fmt.Printf("\n%s Overall: %d/%d tests passed (%.1f%% success rate)\n",
				color.CyanFmt("📊"), totalPassed, totalTests,
				float64(totalPassed)/float64(totalTests)*100)

			if totalFailed > 0 {
				return fmt.Errorf("%d tests failed", totalFailed)
			}

			return nil
		},
	}

	return cmd
}

// Health command for system health check
func (a *AssistantCmd) newHealthCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "health",
		Short: "Check AI Assistant system health",
		Long:  "Check the health status of all AI Assistant components",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			// Initialize core manager
			a.initCoreManager()
			if err := a.coreManager.Initialize(ctx); err != nil {
				return fmt.Errorf("failed to initialize core manager: %w", err)
			}
			defer func() { _ = a.coreManager.Shutdown(ctx) }()

			fmt.Println(color.CyanFmt("🏥 AI Assistant Health Check"))
			fmt.Println(strings.Repeat("=", 40))

			health := a.coreManager.GetSystemHealth(ctx)

			// Overall status
			status := health["status"].(string)
			statusColor := color.GreenFmt
			if status != "healthy" {
				statusColor = color.YellowFmt
			}
			fmt.Printf("Overall Status: %s\n", statusColor(strings.ToUpper(status)))

			// Components
			fmt.Println("\nComponent Status:")
			if components, ok := health["components"].(map[string]interface{}); ok {
				for name, status := range components {
					statusStr := status.(string)
					statusColor := color.GreenFmt
					if statusStr != "healthy" {
						statusColor = color.YellowFmt
					}
					fmt.Printf("  %s: %s\n", name, statusColor(statusStr))
				}
			}

			// Memory usage
			if memUsage, ok := health["memory_usage_mb"]; ok {
				fmt.Printf("\nMemory Usage: %.2f MB\n", memUsage.(float64))
			}

			// Timestamp
			if timestamp, ok := health["timestamp"]; ok {
				fmt.Printf("Check Time: %s\n", timestamp.(time.Time).Format("2006-01-02 15:04:05"))
			}

			if status != "healthy" {
				return fmt.Errorf("system health check failed: %s", status)
			}

			return nil
		},
	}

	return cmd
}

// Stats command for performance and usage statistics
func (a *AssistantCmd) newStatsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stats",
		Short: "Show AI Assistant performance statistics",
		Long:  "Display detailed performance metrics, cache statistics, and usage information",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			// Initialize core manager
			a.initCoreManager()
			if err := a.coreManager.Initialize(ctx); err != nil {
				return fmt.Errorf("failed to initialize core manager: %w", err)
			}
			defer func() { _ = a.coreManager.Shutdown(ctx) }()

			fmt.Println(color.CyanFmt("📊 AI Assistant Statistics"))
			fmt.Println(strings.Repeat("=", 40))

			// Performance metrics
			fmt.Println(color.GreenFmt("Performance Metrics:"))
			perfMetrics := a.coreManager.GetPerformanceMetrics()
			if perfData, ok := perfMetrics["performance_metrics"]; ok {
				fmt.Printf("  Embedding Load Time: %s\n",
					getMetricDuration(perfData, "EmbeddingLoadTime"))
				fmt.Printf("  Schema Load Time: %s\n",
					getMetricDuration(perfData, "SchemaLoadTime"))
				fmt.Printf("  LLM Response Time: %s\n",
					getMetricDuration(perfData, "LLMResponseTime"))
				fmt.Printf("  Semantic Search Time: %s\n",
					getMetricDuration(perfData, "SemanticSearchTime"))
			}

			// Memory analysis
			if memData, ok := perfMetrics["memory_analysis"].(map[string]interface{}); ok {
				fmt.Println(color.GreenFmt("\nMemory Analysis:"))
				if currentMem, ok := memData["current_alloc_mb"].(float64); ok {
					fmt.Printf("  Current Usage: %.2f MB\n", currentMem)
				}
				if avgMem, ok := memData["average_alloc_mb"].(float64); ok {
					fmt.Printf("  Average Usage: %.2f MB\n", avgMem)
				}
				if gcCount, ok := memData["gc_count"].(uint32); ok {
					fmt.Printf("  GC Count: %d\n", gcCount)
				}
			}

			// Cache statistics
			fmt.Println(color.GreenFmt("\nCache Statistics:"))
			cacheStats := a.coreManager.GetCacheStats()
			for cacheName, stats := range cacheStats {
				if statsMap, ok := stats.(map[string]interface{}); ok {
					if entries, ok := statsMap["total_entries"].(int); ok {
						fmt.Printf("  %s: %d entries\n", cacheName, entries)
					}
				}
			}

			// Security status
			fmt.Println(color.GreenFmt("\nSecurity Status:"))
			secStats := a.coreManager.GetSecurityStats()
			if enabled, ok := secStats["security_enabled"].(bool); ok {
				fmt.Printf("  Security Enabled: %v\n", enabled)
				if level, ok := secStats["security_level"].(string); ok {
					fmt.Printf("  Security Level: %s\n", level)
				}
			}

			return nil
		},
	}

	return cmd
}

// Helper function to extract duration metrics
func getMetricDuration(data interface{}, key string) string {
	if metricsMap, ok := data.(map[string]interface{}); ok {
		if duration, ok := metricsMap[key].(time.Duration); ok {
			return duration.String()
		}
	}
	return "N/A"
}

func init() {
	// This will be called when the package is imported
	// Can be used for initialization if needed
}
