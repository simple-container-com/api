package cmd_assistant

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/simple-container-com/api/pkg/api/logger/color"
	"github.com/simple-container-com/api/pkg/assistant/chat"
	"github.com/simple-container-com/api/pkg/assistant/embeddings"
	"github.com/simple-container-com/api/pkg/assistant/mcp"
	"github.com/simple-container-com/api/pkg/assistant/modes"
	"github.com/simple-container-com/api/pkg/cmd/root_cmd"
)

type AssistantCmd struct {
	rootCmd       *root_cmd.RootCmd
	developerMode *modes.DeveloperMode
	devopsMode    *modes.DevOpsMode
	mode          string // Current mode (dev, devops, general)
}

func NewAssistantCmd(rootCmd *root_cmd.RootCmd) *cobra.Command {
	assistantCmd := &AssistantCmd{
		rootCmd:       rootCmd,
		developerMode: modes.NewDeveloperMode(),
		devopsMode:    modes.NewDevOpsMode(),
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
			return a.developerMode.Setup(cmd.Context(), opts)
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

	return cmd
}

func (a *AssistantCmd) newDevAnalyzeCmd() *cobra.Command {
	var opts modes.AnalyzeOptions

	cmd := &cobra.Command{
		Use:   "analyze",
		Short: "Analyze project structure and tech stack",
		Long:  "Detect technology stack, dependencies, and architecture patterns",
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.developerMode.Analyze(cmd.Context(), opts)
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
			return a.runMCP(cmd, host, port)
		},
	}

	cmd.Flags().IntVar(&port, "port", 9999, "Port to listen on")
	cmd.Flags().StringVar(&host, "host", "localhost", "Host to bind to")

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

	// Handle OpenAI API key
	if apiKey, _ := cmd.Flags().GetString("openai-key"); apiKey != "" {
		os.Setenv("OPENAI_API_KEY", apiKey)
	}

	// Check if API key is available
	if llmProvider == "openai" && os.Getenv("OPENAI_API_KEY") == "" {
		fmt.Println(color.YellowFmt("⚠️  OpenAI API key not found"))
		fmt.Println("Please set your OpenAI API key:")
		fmt.Println("  export OPENAI_API_KEY=sk-your-key-here")
		fmt.Println("  or use: sc assistant chat --openai-key sk-your-key-here")
		fmt.Println()
		fmt.Println("Get your API key from: https://platform.openai.com/api-keys")
		return fmt.Errorf("OpenAI API key required for chat mode")
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

	// Start interactive session
	ctx := context.Background()
	return chatInterface.StartSession(ctx)
}

func (a *AssistantCmd) runSearch(cmd *cobra.Command, query string, limit int, docType string) error {
	fmt.Printf("🔍 Searching documentation for: %s\n", color.CyanFmt(query))
	if docType != "" {
		fmt.Printf("📋 Document type: %s\n", color.YellowFmt(docType))
	}
	fmt.Println()

	// Load embedded documentation database
	db, err := embeddings.LoadEmbeddedDatabase()
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

func (a *AssistantCmd) runMCP(cmd *cobra.Command, host string, port int) error {
	fmt.Printf("🌐 Starting MCP server on %s:%d\n", color.CyanFmt(host), port)
	fmt.Println("This will expose Simple Container context to external LLM tools.\n")

	// Create MCP server instance
	mcpServer := mcp.NewMCPServer(host, port)

	// Start the server
	ctx := cmd.Context()
	return mcpServer.Start(ctx)
}

// Helper function to check if embedded documentation is available
func (a *AssistantCmd) checkEmbeddingsAvailable() bool {
	_, err := embeddings.LoadEmbeddedDatabase()
	return err == nil
}

func init() {
	// This will be called when the package is imported
	// Can be used for initialization if needed
}
