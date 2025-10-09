package chat

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"gopkg.in/yaml.v2"

	"github.com/simple-container-com/api/pkg/api/logger/color"
	"github.com/simple-container-com/api/pkg/assistant/analysis"
	"github.com/simple-container-com/api/pkg/assistant/modes"
	"github.com/simple-container-com/api/pkg/assistant/resources"
)

// registerProjectCommands registers project analysis and configuration commands
func (c *ChatInterface) registerProjectCommands() {
	c.commands["analyze"] = &ChatCommand{
		Name:        "analyze",
		Description: "Analyze current project tech stack",
		Usage:       "/analyze [--full] [--force]",
		Handler:     c.handleAnalyze,
		Aliases:     []string{"a", "analyse"}, // Added British spelling
		Args: []CommandArg{
			{Name: "full", Type: "flag", Required: false, Description: "Run full analysis including resource detection (slower but comprehensive)"},
			{Name: "force", Type: "flag", Required: false, Description: "Force fresh analysis even if cache exists"},
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

	c.commands["file"] = &ChatCommand{
		Name:        "file",
		Description: "Read and display a project file (Dockerfile, docker-compose.yaml, etc.)",
		Usage:       "/file <filename>",
		Handler:     c.handleReadProjectFile,
		Aliases:     []string{"show", "cat"},
		Args: []CommandArg{
			{Name: "filename", Type: "string", Required: true, Description: "File name to read (e.g., Dockerfile, docker-compose.yaml, package.json)"},
		},
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

	// Parse arguments to determine analysis mode
	fullAnalysis := false
	forceAnalysis := false

	for _, arg := range args {
		switch arg {
		case "--full", "-f":
			fullAnalysis = true
		case "--force":
			forceAnalysis = true
		}
	}

	// Check if cache exists and its completeness
	cacheExists := analysis.CacheExists(context.ProjectPath)
	hasResourcesInCache := analysis.HasResourcesInCache(context.ProjectPath)

	// Determine if we need to run full analysis despite cache
	needsFullAnalysis := forceAnalysis || (fullAnalysis && (!cacheExists || !hasResourcesInCache))

	if cacheExists && !forceAnalysis {
		if fullAnalysis && !hasResourcesInCache {
			fmt.Printf("📋 Found incomplete cached analysis (missing resources) for %s\n", color.YellowString(context.ProjectPath))
			fmt.Printf("🔍 Running full analysis to detect resources and environment variables...\n")
		} else {
			fmt.Printf("📋 Found cached analysis for %s\n", color.CyanString(context.ProjectPath))
		}
	} else {
		if fullAnalysis || needsFullAnalysis {
			fmt.Printf("🔍 Running full analysis at %s...\n", color.CyanString(context.ProjectPath))
		} else {
			fmt.Printf("🔍 Analyzing project at %s...\n", color.CyanString(context.ProjectPath))
		}
	}

	// Create analyzer and configure mode
	analyzer := analysis.NewProjectAnalyzer()

	// Set up progress reporting for better UX
	progressReporter := analysis.NewStreamingProgressReporter(os.Stdout)
	analyzer.SetProgressReporter(progressReporter)

	if forceAnalysis && fullAnalysis {
		analyzer.SetAnalysisMode(analysis.ForceFullMode)
	} else if forceAnalysis {
		analyzer.SetAnalysisMode(analysis.FullMode)
	} else if fullAnalysis {
		// If cache exists but lacks resources, use ForceFullMode to ensure we get everything
		if cacheExists && !hasResourcesInCache {
			analyzer.SetAnalysisMode(analysis.ForceFullMode)
		} else {
			analyzer.SetAnalysisMode(analysis.FullMode)
		}
	} else {
		analyzer.SetAnalysisMode(analysis.CachedMode)
	}

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

	// Display comprehensive resource information
	if result.Resources != nil {
		resourceCount := 0

		// Environment Variables
		if len(result.Resources.EnvironmentVars) > 0 {
			if resourceCount == 0 {
				message += "\n📋 **Resources Detected**:\n"
			}
			message += fmt.Sprintf("  🌐 **Environment Variables**: %d found\n", len(result.Resources.EnvironmentVars))
			// Show first few environment variables as examples
			for i, envVar := range result.Resources.EnvironmentVars {
				if i >= 3 { // Limit to first 3 to avoid overwhelming output
					message += fmt.Sprintf("    • ... and %d more\n", len(result.Resources.EnvironmentVars)-3)
					break
				}
				message += fmt.Sprintf("    • %s\n", envVar.Name)
			}
			resourceCount++
		}

		// Databases
		if len(result.Resources.Databases) > 0 {
			if resourceCount == 0 {
				message += "\n📋 **Resources Detected**:\n"
			}
			message += fmt.Sprintf("  💾 **Databases**: %d found\n", len(result.Resources.Databases))
			for _, db := range result.Resources.Databases {
				message += fmt.Sprintf("    • %s (%s)\n", db.Name, db.Type)
			}
			resourceCount++
		}

		// External APIs
		if len(result.Resources.ExternalAPIs) > 0 {
			if resourceCount == 0 {
				message += "\n📋 **Resources Detected**:\n"
			}
			message += fmt.Sprintf("  🌐 **External APIs**: %d found\n", len(result.Resources.ExternalAPIs))
			for _, api := range result.Resources.ExternalAPIs {
				message += fmt.Sprintf("    • %s\n", api.Name)
			}
			resourceCount++
		}

		// Secrets
		if len(result.Resources.Secrets) > 0 {
			if resourceCount == 0 {
				message += "\n📋 **Resources Detected**:\n"
			}
			message += fmt.Sprintf("  🔐 **Secrets**: %d found\n", len(result.Resources.Secrets))
			for i, secret := range result.Resources.Secrets {
				if i >= 3 {
					message += fmt.Sprintf("    • ... and %d more\n", len(result.Resources.Secrets)-3)
					break
				}
				message += fmt.Sprintf("    • %s\n", secret.Name)
			}
			resourceCount++
		}

		// Storage
		if len(result.Resources.Storage) > 0 {
			if resourceCount == 0 {
				message += "\n📋 **Resources Detected**:\n"
			}
			message += fmt.Sprintf("  💽 **Storage**: %d found\n", len(result.Resources.Storage))
			for _, storage := range result.Resources.Storage {
				message += fmt.Sprintf("    • %s (%s)\n", storage.Name, storage.Type)
			}
			resourceCount++
		}

		// Queues
		if len(result.Resources.Queues) > 0 {
			if resourceCount == 0 {
				message += "\n📋 **Resources Detected**:\n"
			}
			message += fmt.Sprintf("  📨 **Queues**: %d found\n", len(result.Resources.Queues))
			for _, queue := range result.Resources.Queues {
				message += fmt.Sprintf("    • %s (%s)\n", queue.Name, queue.Type)
			}
			resourceCount++
		}

		if resourceCount == 0 {
			message += "\n📋 **Resources**: None detected (use `/analyze --full` for comprehensive resource detection)\n"
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
		// Add deployment type confirmation for developer mode
		if err := c.confirmDeploymentTypeForChat(context); err != nil {
			return &CommandResult{
				Success: false,
				Message: fmt.Sprintf("Setup cancelled: %v", err),
			}, nil
		}
		files, err = c.generateDeveloperFiles(context)
	case "devops":
		files, err = c.generateDevOpsFiles(context)
	default:
		// Auto-detect based on project
		if context.ProjectInfo.PrimaryStack != nil {
			// Add deployment type confirmation for auto-detected developer mode
			if err := c.confirmDeploymentTypeForChat(context); err != nil {
				return &CommandResult{
					Success: false,
					Message: fmt.Sprintf("Setup cancelled: %v", err),
				}, nil
			}
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
		NextStep: "Review the generated files and run 'docker-compose up -d' from ${project:root} to test locally, then 'sc deploy -s ${project:name} -e staging' to deploy",
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

// confirmDeploymentTypeForChat handles deployment type confirmation in chat interface
func (c *ChatInterface) confirmDeploymentTypeForChat(context *ConversationContext) error {
	// Initialize metadata if it doesn't exist
	if context.Metadata == nil {
		context.Metadata = make(map[string]interface{})
	}
	// Determine deployment type using simple heuristic (since internal method is private)
	detectedType := "cloud-compose" // Default fallback
	if context.ProjectInfo != nil {
		// Simple heuristic based on project analysis
		if context.ProjectInfo.PrimaryStack != nil {
			lang := strings.ToLower(context.ProjectInfo.PrimaryStack.Language)
			if lang == "html" || lang == "javascript" || lang == "typescript" {
				// Check for static site indicators
				detectedType = "static"
			} else if lang == "go" || lang == "python" || lang == "java" {
				detectedType = "single-image"
			}
		}
	}

	// Display detected type with description
	fmt.Printf("🔍 Detected deployment type: %s\n", detectedType)

	switch detectedType {
	case "static":
		fmt.Printf("   📄 Static site deployment (HTML/CSS/JS files)\n")
		fmt.Printf("   💡 Best for: React, Vue, Angular, static sites\n")
	case "single-image":
		fmt.Printf("   🚀 Single container deployment (serverless/lambda style)\n")
		fmt.Printf("   💡 Best for: AWS Lambda, simple APIs, microservices\n")
	case "cloud-compose":
		fmt.Printf("   🐳 Multi-container deployment (docker-compose based)\n")
		fmt.Printf("   💡 Best for: Full-stack apps, databases, complex services\n")
	}

	// Use chat interface's ReadSimple for input (fixes Y/N prompt issue)
	response, err := c.inputHandler.ReadSimple("\n   Is this correct? [Y/n]: ")
	if err != nil {
		// If there's an error reading input, default to "yes" and store the detected type
		context.Metadata["confirmed_deployment_type"] = detectedType
		return nil
	}

	response = strings.ToLower(strings.TrimSpace(response))
	if response == "n" || response == "no" {
		// Let user choose the deployment type using chat interface
		return c.selectDeploymentTypeForChat(context)
	}

	// Store the confirmed deployment type in context for use during generation
	context.Metadata["confirmed_deployment_type"] = detectedType
	return nil
}

// selectDeploymentTypeForChat handles manual deployment type selection in chat interface
func (c *ChatInterface) selectDeploymentTypeForChat(context *ConversationContext) error {
	// Initialize metadata if it doesn't exist
	if context.Metadata == nil {
		context.Metadata = make(map[string]interface{})
	}
	fmt.Printf("\n📋 Available deployment types:\n")
	fmt.Printf("   1. static - Static site (HTML/CSS/JS files)\n")
	fmt.Printf("   2. single-image - Single container (serverless/lambda style)\n")
	fmt.Printf("   3. cloud-compose - Multi-container (docker-compose based)\n")

	// Use chat interface's ReadSimple for menu selection
	response, err := c.inputHandler.ReadSimple("\n   Select deployment type [1-3]: ")
	if err != nil {
		return fmt.Errorf("failed to read selection: %v", err)
	}

	response = strings.TrimSpace(response)

	var selectedType string
	switch response {
	case "1":
		selectedType = "static"
		fmt.Printf("✅ Selected: static\n")
	case "2":
		selectedType = "single-image"
		fmt.Printf("✅ Selected: single-image\n")
	case "3", "":
		selectedType = "cloud-compose"
		fmt.Printf("✅ Selected: cloud-compose\n")
	default:
		selectedType = "cloud-compose"
		fmt.Printf("Invalid selection. Using cloud-compose as default.\n")
	}

	// Store the selected deployment type in context for use during generation
	context.Metadata["confirmed_deployment_type"] = selectedType

	return nil
}

// handleReadProjectFile reads and displays a project file
func (c *ChatInterface) handleReadProjectFile(ctx context.Context, args []string, context *ConversationContext) (*CommandResult, error) {
	if len(args) == 0 {
		return &CommandResult{
			Success: false,
			Message: "❌ Please specify a filename. Usage: /file <filename>",
		}, nil
	}

	filename := args[0]

	// Get current working directory (the user's project directory)
	cwd, err := os.Getwd()
	if err != nil {
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("❌ Failed to get current directory: %v", err),
		}, nil
	}

	// Build full file path
	filePath := filepath.Join(cwd, filename)

	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("❌ File not found: %s\n\n💡 **Tip**: Make sure you're in your project directory and the file exists.", filename),
		}, nil
	}

	// Read the file
	data, err := os.ReadFile(filePath)
	if err != nil {
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("❌ Failed to read file %s: %v", filename, err),
		}, nil
	}

	// Determine syntax highlighting based on file extension/name
	language := getSyntaxLanguage(filename)

	message := fmt.Sprintf("📄 **%s** (from %s)\n\n", filename, cwd)
	message += fmt.Sprintf("```%s\n", language)
	message += string(data)
	message += "\n```\n"

	// Add file info
	stat, _ := os.Stat(filePath)
	message += fmt.Sprintf("\n📊 **File Info**: %d bytes, modified %s",
		len(data), stat.ModTime().Format("2006-01-02 15:04:05"))

	return &CommandResult{
		Success: true,
		Message: message,
	}, nil
}

// getSyntaxLanguage determines the syntax highlighting language for a filename
func getSyntaxLanguage(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	name := strings.ToLower(filename)

	// Handle specific filenames first
	switch name {
	case "dockerfile", "dockerfile.dev", "dockerfile.prod":
		return "dockerfile"
	case "docker-compose.yaml", "docker-compose.yml":
		return "yaml"
	case "package.json", "composer.json":
		return "json"
	case "go.mod", "go.sum":
		return "go"
	case "requirements.txt", "setup.py", "pyproject.toml":
		return "python"
	case "makefile":
		return "makefile"
	case ".env", ".env.example", ".env.local", ".env.production":
		return "bash"
	}

	// Handle extensions
	switch ext {
	case ".js", ".mjs":
		return "javascript"
	case ".ts":
		return "typescript"
	case ".py":
		return "python"
	case ".go":
		return "go"
	case ".yaml", ".yml":
		return "yaml"
	case ".json":
		return "json"
	case ".toml":
		return "toml"
	case ".xml":
		return "xml"
	case ".sh", ".bash":
		return "bash"
	case ".sql":
		return "sql"
	case ".md":
		return "markdown"
	case ".html":
		return "html"
	case ".css":
		return "css"
	case ".java":
		return "java"
	case ".rb":
		return "ruby"
	case ".php":
		return "php"
	case ".rs":
		return "rust"
	case ".cpp", ".cc", ".cxx":
		return "cpp"
	case ".c":
		return "c"
	case ".cs":
		return "csharp"
	default:
		return ""
	}
}

// generateDeveloperFiles creates client configuration files using DeveloperMode
func (c *ChatInterface) generateDeveloperFiles(context *ConversationContext) ([]GeneratedFile, error) {
	// Use DeveloperMode for consistent file generation
	projectPath := "."
	if context.ProjectPath != "" {
		projectPath = context.ProjectPath
	}

	// Initialize metadata if it doesn't exist
	if context.Metadata == nil {
		context.Metadata = make(map[string]interface{})
	}

	// Get confirmed deployment type from context (if available)
	var deploymentType string
	if confirmedType, exists := context.Metadata["confirmed_deployment_type"]; exists {
		if typeStr, ok := confirmedType.(string); ok {
			deploymentType = typeStr
		}
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
		UseStreaming:   true,           // Enable streaming for better UX in chat mode
		DeploymentType: deploymentType, // Use the confirmed deployment type
	}

	// Capture the generated files by temporarily redirecting the DeveloperMode output
	// We'll use a custom approach to get the generated content without writing files
	return c.generateFilesUsingDeveloperMode(context, opts)
}

// generateFilesUsingDeveloperMode generates files using the modes package
func (c *ChatInterface) generateFilesUsingDeveloperMode(context *ConversationContext, opts *modes.SetupOptions) ([]GeneratedFile, error) {
	// Get or create project analysis
	var projectAnalysis *analysis.ProjectAnalysis
	if context.ProjectInfo != nil {
		projectAnalysis = context.ProjectInfo
	} else {
		// Add progress reporting for better UX during analysis
		fmt.Printf("🔄 Analyzing project for configuration generation...\n")
		progressReporter := analysis.NewStreamingProgressReporter(os.Stdout)
		c.analyzer.SetProgressReporter(progressReporter)

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

	// Start all file generations in parallel
	fmt.Printf("📄 Generating configuration files in parallel...\n")

	type fileGenResult struct {
		file GeneratedFile
		err  error
		name string
	}

	var wg sync.WaitGroup
	resultChan := make(chan fileGenResult, 3)

	// Generate client.yaml
	wg.Add(1)
	go func() {
		defer wg.Done()
		fmt.Printf("   📄 Generating client.yaml...\n")
		clientYaml, err := c.developerMode.GenerateClientYAMLWithLLM(opts, projectAnalysis)
		result := fileGenResult{name: "client.yaml", err: err}
		if err == nil {
			result.file = GeneratedFile{
				Path:        ".sc/stacks/" + projectName + "/client.yaml",
				Type:        "yaml",
				Description: "Simple Container client configuration",
				Generated:   true,
				Content:     clientYaml,
			}
		}
		resultChan <- result
	}()

	// Generate docker-compose.yaml
	if !opts.SkipCompose {
		wg.Add(1)
		go func() {
			defer wg.Done()
			fmt.Printf("   🐳 Generating docker-compose.yaml...\n")
			composeYaml, err := c.developerMode.GenerateComposeYAMLWithLLM(projectAnalysis)
			result := fileGenResult{name: "docker-compose.yaml", err: err}
			if err == nil {
				result.file = GeneratedFile{
					Path:        "docker-compose.yaml",
					Type:        "yaml",
					Description: "Local development environment",
					Generated:   true,
					Content:     composeYaml,
				}
			}
			resultChan <- result
		}()
	}

	// Generate Dockerfile
	if !opts.SkipDockerfile {
		wg.Add(1)
		go func() {
			defer wg.Done()
			fmt.Printf("   🐳 Generating Dockerfile...\n")
			dockerfile, err := c.developerMode.GenerateDockerfileWithLLM(projectAnalysis)
			result := fileGenResult{name: "Dockerfile", err: err}
			if err == nil {
				result.file = GeneratedFile{
					Path:        "Dockerfile",
					Type:        "dockerfile",
					Description: "Container image definition",
					Generated:   true,
					Content:     dockerfile,
				}
			}
			resultChan <- result
		}()
	}

	// Wait for all generations to complete and collect results
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Collect results as they complete
	for result := range resultChan {
		if result.err == nil {
			fmt.Printf("   ✅ %s generated\n", result.name)
			files = append(files, result.file)
		} else {
			fmt.Printf("   ⚠️ Failed to generate %s: %v\n", result.name, result.err)
		}
	}

	return files, nil
}

// generateDevOpsFiles creates infrastructure files for DevOps mode
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
    type: ecs-fargate
    config:
      credentials: "${auth:aws}"
      account: "${auth:aws.projectId}"
    
  api-service:
    type: ecs-fargate
    config:
      credentials: "${auth:aws}"
      account: "${auth:aws.projectId}"

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
	secretsContent := `# Simple Container secrets configuration
schemaVersion: 1.0

# Authentication for cloud providers
auth:
  aws:
    type: aws-token
    config:
      account: "123456789012"
      accessKey: "AKIA..."  # Replace with actual AWS access key
      secretAccessKey: "wJa..."  # Replace with actual AWS secret key
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
