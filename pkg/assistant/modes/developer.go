package modes

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/simple-container-com/api/pkg/api/logger/color"
	"github.com/simple-container-com/api/pkg/assistant/analysis"
	"github.com/simple-container-com/api/pkg/assistant/config"
	"github.com/simple-container-com/api/pkg/assistant/embeddings"
	"github.com/simple-container-com/api/pkg/assistant/llm"
	"github.com/simple-container-com/api/pkg/assistant/resources"
	"github.com/simple-container-com/api/pkg/assistant/validation"
)

// DeveloperMode handles application-focused workflows
type DeveloperMode struct {
	analyzer   *analysis.ProjectAnalyzer
	llm        llm.Provider
	embeddings *embeddings.Database
}

// NewDeveloperMode creates a new developer mode instance
func NewDeveloperMode() *DeveloperMode {
	// Initialize LLM provider (OpenAI by default)
	provider := llm.NewOpenAIProvider()

	// Get API key from config system first, then fallback to environment
	apiKey := getConfiguredAPIKey()

	_ = provider.Configure(llm.Config{
		Provider:    "openai",
		MaxTokens:   2048,
		Temperature: 0.7,
		APIKey:      apiKey,
	}) // Ignore configuration errors - fallback will be used

	// Initialize embeddings database for documentation search
	embeddingsDB, err := embeddings.LoadEmbeddedDatabase(context.Background())
	if err != nil {
		// Don't fail if embeddings can't be loaded - just use nil
		embeddingsDB = nil
	}

	return &DeveloperMode{
		analyzer:   analysis.NewProjectAnalyzer(),
		llm:        provider,
		embeddings: embeddingsDB,
	}
}

// NewDeveloperModeWithComponents creates a new developer mode with provided components (for reuse)
func NewDeveloperModeWithComponents(provider llm.Provider, embeddingsDB *embeddings.Database, analyzer *analysis.ProjectAnalyzer) *DeveloperMode {
	return &DeveloperMode{
		analyzer:   analyzer,
		llm:        provider,
		embeddings: embeddingsDB,
	}
}

// GetLLMProvider returns the LLM provider for use by other components
func (d *DeveloperMode) GetLLMProvider() llm.Provider {
	return d.llm
}

// SetupOptions for developer setup command
type SetupOptions struct {
	Interactive    bool
	Environment    string
	Parent         string
	SkipAnalysis   bool
	SkipDockerfile bool
	SkipCompose    bool
	Language       string
	Framework      string
	CloudProvider  string
	OutputDir      string

	// Multi-file generation options
	GenerateAll  bool // Generate all files in one coordinated operation
	UseStreaming bool // Use streaming LLM responses for better UX

	// Deployment type override (if user manually selected)
	DeploymentType string // Override detected deployment type: "static", "single-image", "cloud-compose"

	// Skip all confirmation prompts (useful for MCP/API usage)
	SkipConfirmation bool

	// Force overwrite existing files without prompting (useful for MCP/API usage)
	ForceOverwrite bool
}

// AnalyzeOptions for developer analyze command
type AnalyzeOptions struct {
	Detailed bool
	Path     string
	Output   string
	Format   string
}

// Setup generates application configuration files
func (d *DeveloperMode) Setup(ctx context.Context, opts *SetupOptions) error {
	projectPath := "."
	if opts.OutputDir != "" {
		projectPath = opts.OutputDir
	}

	fmt.Println(color.BlueFmt("🚀 Simple Container Developer Mode - Project Setup"))
	fmt.Printf("📂 Project path: %s\n", color.CyanFmt(projectPath))

	var projectAnalysis *analysis.ProjectAnalysis
	var err error

	// Step 1: Project Analysis (unless skipped)
	if !opts.SkipAnalysis {
		// Configure analyzer for setup mode (includes user confirmation for resources)
		d.analyzer.SetAnalysisMode(analysis.SetupMode)

		projectAnalysis, err = d.analyzer.AnalyzeProject(projectPath)
		if err != nil {
			if opts.Language != "" && opts.Framework != "" {
				fmt.Printf("⚠️  Auto-analysis failed, using manual specification: %s + %s\n",
					opts.Language, opts.Framework)
				projectAnalysis = d.createManualAnalysis(projectPath, opts.Language, opts.Framework)
			} else {
				return fmt.Errorf("project analysis failed: %w\nTry using --language and --framework flags", err)
			}
		} else {
			d.printAnalysisResults(projectAnalysis)
		}
	}

	// Step 2: Interactive Configuration (if enabled)
	if opts.Interactive {
		if err := d.interactiveSetup(opts, projectAnalysis); err != nil {
			return err
		}
	}

	// Step 3: Deployment Type Confirmation
	if !opts.Interactive {
		// For non-interactive mode, confirm deployment type
		if err := d.ConfirmDeploymentType(opts, projectAnalysis); err != nil {
			return err
		}
	} else {
		// Interactive mode already handled deployment type selection
		// Just show what was selected
		if opts.DeploymentType != "" {
			fmt.Printf("\n🔍 Using deployment type: %s\n", color.CyanString(opts.DeploymentType))
		}
	}

	// Step 4: Generate Configuration Files
	fmt.Println("\n📝 Generating configuration files...")

	// Check if coordinated multi-file generation is requested
	if opts.GenerateAll {
		if err := d.generateFilesCoordinated(ctx, projectPath, opts, projectAnalysis); err != nil {
			return fmt.Errorf("coordinated file generation failed: %w", err)
		}
	} else {
		if err := d.generateFiles(projectPath, opts, projectAnalysis); err != nil {
			return fmt.Errorf("file generation failed: %w", err)
		}
	}

	// Step 4: Success Summary
	d.printSetupSummary(opts, projectAnalysis)

	return nil
}

// Analyze performs detailed project analysis
func (d *DeveloperMode) Analyze(ctx context.Context, opts *AnalyzeOptions) error {
	projectPath := opts.Path
	if projectPath == "" {
		projectPath = "."
	}

	fmt.Println(color.BlueFmt("🔍 Simple Container Developer Mode - Project Analysis"))
	fmt.Printf("📂 Analyzing project: %s\n\n", color.CyanFmt(projectPath))

	// Configure analyzer based on detailed flag
	if opts.Detailed {
		d.analyzer.SetAnalysisMode(analysis.FullMode)
		fmt.Printf("📊 Running detailed analysis (including resource detection)...\n")
	} else {
		d.analyzer.SetAnalysisMode(analysis.CachedMode)
		fmt.Printf("⚡ Running quick analysis (cache-first)...\n")
	}

	// Perform analysis
	analysisResult, err := d.analyzer.AnalyzeProject(projectPath)
	if err != nil {
		return fmt.Errorf("analysis failed: %w", err)
	}

	// Display results based on format
	switch opts.Format {
	case "json":
		return d.outputAnalysisJSON(analysisResult, opts.Output)
	case "yaml":
		return d.outputAnalysisYAML(analysisResult, opts.Output)
	default:
		d.outputAnalysisTable(analysisResult, opts.Detailed)
	}

	return nil
}

// Helper methods

func (d *DeveloperMode) createManualAnalysis(projectPath, language, framework string) *analysis.ProjectAnalysis {
	return &analysis.ProjectAnalysis{
		Path: projectPath,
		Name: filepath.Base(projectPath),
		TechStacks: []analysis.TechStackInfo{
			{
				Language:   language,
				Framework:  framework,
				Confidence: 0.8, // Lower confidence for manual specification
				Evidence:   []string{"manually specified"},
			},
		},
		PrimaryStack: &analysis.TechStackInfo{
			Language:  language,
			Framework: framework,
		},
		Architecture: "standard-web-app",
		Recommendations: []analysis.Recommendation{
			{
				Type:        "configuration",
				Category:    "setup",
				Priority:    "high",
				Title:       "Manual Configuration",
				Description: fmt.Sprintf("Configure %s application with %s framework", language, framework),
			},
		},
	}
}

func (d *DeveloperMode) printAnalysisResults(analysis *analysis.ProjectAnalysis) {
	if analysis.PrimaryStack != nil {
		fmt.Printf("   Language:     %s\n", color.GreenFmt(analysis.PrimaryStack.Language))
		if analysis.PrimaryStack.Framework != "" {
			fmt.Printf("   Framework:    %s\n", color.GreenFmt(analysis.PrimaryStack.Framework))
		}
		if analysis.PrimaryStack.Version != "" {
			fmt.Printf("   Version:      %s\n", color.GreenFmt(analysis.PrimaryStack.Version))
		}
	}

	if analysis.Architecture != "" {
		fmt.Printf("   Architecture: %s\n", color.GreenFmt(analysis.Architecture))
	}

	fmt.Printf("   Confidence:   %s\n", color.YellowFmt(fmt.Sprintf("%.0f%%", analysis.Confidence*100)))

	// Show dependencies if detected
	if analysis.PrimaryStack != nil && len(analysis.PrimaryStack.Dependencies) > 0 {
		fmt.Println("\n📦 Dependencies:")
		for _, dep := range analysis.PrimaryStack.Dependencies {
			fmt.Printf("   ✅ %s %s\n", dep.Name, dep.Version)
		}
	}

	// Show recommendations
	if len(analysis.Recommendations) > 0 {
		fmt.Println("\n🎯 Recommendations:")
		for _, rec := range analysis.Recommendations {
			priority := rec.Priority
			switch rec.Priority {
			case "high":
				priority = color.RedFmt("high")
			case "medium":
				priority = color.YellowFmt("medium")
			case "low":
				priority = color.GrayFmt("low")
			}
			fmt.Printf("   🔹 %s (%s)\n", rec.Title, priority)
		}
	}
}

func (d *DeveloperMode) interactiveSetup(opts *SetupOptions, analysis *analysis.ProjectAnalysis) error {
	scanner := bufio.NewScanner(os.Stdin)

	fmt.Println("\n🔧 " + color.BlueFmt("Interactive Setup Configuration"))
	fmt.Println(strings.Repeat("─", 50))

	// Show current project analysis
	if analysis != nil && analysis.PrimaryStack != nil {
		fmt.Printf("🔍 Detected: %s", color.GreenFmt(analysis.PrimaryStack.Language))
		if analysis.PrimaryStack.Framework != "" {
			fmt.Printf(" with %s", color.GreenFmt(analysis.PrimaryStack.Framework))
		}
		fmt.Printf(" (%.0f%% confidence)\n\n", analysis.Confidence*100)
	}

	// 1. Confirm or change target environment
	for {
		fmt.Printf("🌍 Target environment [%s]: ", color.CyanFmt(opts.Environment))
		scanner.Scan()
		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			break // Keep default
		}
		if input == "staging" || input == "production" || input == "development" {
			opts.Environment = input
			break
		}
		fmt.Printf("   %s Please enter 'staging', 'production', or 'development'\n", color.YellowFmt("⚠"))
	}

	// 2. Confirm or change parent stack
	fmt.Printf("🏗️  Parent stack [%s]: ", color.CyanFmt(opts.Parent))
	scanner.Scan()
	input := strings.TrimSpace(scanner.Text())
	if input != "" {
		opts.Parent = input
	}

	// 3. Ask about stack type preferences
	fmt.Printf("\n📋 Configuration Options:\n")
	fmt.Printf("   1. %s - Full containerized application with scaling\n", color.GreenFmt("cloud-compose"))
	fmt.Printf("   2. %s - Static website hosting\n", color.GreenFmt("static"))
	fmt.Printf("   3. %s - Single container deployment\n", color.GreenFmt("single-image"))

	for {
		fmt.Printf("\nStack type [1]: ")
		scanner.Scan()
		input := strings.TrimSpace(scanner.Text())
		if input == "" || input == "1" {
			// Default to cloud-compose
			opts.DeploymentType = "cloud-compose"
			fmt.Printf("   %s Multi-container deployment selected\n", color.GreenFmt("✓"))
			break
		}
		if input == "2" {
			opts.DeploymentType = "static"
			fmt.Printf("   %s Static deployment selected\n", color.GreenFmt("✓"))
			break
		}
		if input == "3" {
			opts.DeploymentType = "single-image"
			fmt.Printf("   %s Single-image deployment selected\n", color.GreenFmt("✓"))
			break
		}
		fmt.Printf("   %s Please enter 1, 2, or 3\n", color.YellowFmt("⚠"))
	}

	// 4. Ask about scaling preferences
	fmt.Printf("\n📈 Scaling Configuration:\n")
	minInstances := 1
	maxInstances := 3

	for {
		fmt.Printf("Minimum instances [%d]: ", minInstances)
		scanner.Scan()
		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			break
		}
		if val, err := strconv.Atoi(input); err == nil && val >= 1 && val <= 10 {
			minInstances = val
			break
		}
		fmt.Printf("   %s Please enter a number between 1-10\n", color.YellowFmt("⚠"))
	}

	for {
		fmt.Printf("Maximum instances [%d]: ", maxInstances)
		scanner.Scan()
		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			break
		}
		if val, err := strconv.Atoi(input); err == nil && val >= minInstances && val <= 20 {
			maxInstances = val
			break
		}
		fmt.Printf("   %s Please enter a number between %d-20\n", color.YellowFmt("⚠"), minInstances)
	}

	// 5. Ask about additional services
	fmt.Printf("\n🔧 Additional Services:\n")
	includeDatabase := false
	includeRedis := false

	fmt.Printf("Include PostgreSQL database? [y/N]: ")
	scanner.Scan()
	if strings.ToLower(strings.TrimSpace(scanner.Text())) == "y" {
		includeDatabase = true
		fmt.Printf("   %s PostgreSQL will be included\n", color.GreenFmt("✓"))
	}

	fmt.Printf("Include Redis cache? [y/N]: ")
	scanner.Scan()
	if strings.ToLower(strings.TrimSpace(scanner.Text())) == "y" {
		includeRedis = true
		fmt.Printf("   %s Redis will be included\n", color.GreenFmt("✓"))
	}

	// 6. Summary
	fmt.Printf("%s", "\n📋 "+color.BlueFmt("Configuration Summary:"))
	fmt.Printf("\n   Environment: %s", color.CyanFmt(opts.Environment))
	fmt.Printf("\n   Parent: %s", color.CyanFmt(opts.Parent))
	fmt.Printf("\n   Scaling: %s-%s instances", color.YellowFmt(fmt.Sprintf("%d", minInstances)), color.YellowFmt(fmt.Sprintf("%d", maxInstances)))
	if includeDatabase {
		fmt.Printf("\n   Database: %s", color.GreenFmt("PostgreSQL"))
	}
	if includeRedis {
		fmt.Printf("\n   Cache: %s", color.GreenFmt("Redis"))
	}

	fmt.Printf("\n\nProceed with this configuration? [Y/n]: ")
	scanner.Scan()
	if strings.ToLower(strings.TrimSpace(scanner.Text())) == "n" {
		return fmt.Errorf("setup cancelled by user")
	}

	fmt.Printf("   %s Configuration confirmed!\n\n", color.GreenFmt("✓"))

	// Store the interactive choices (could extend SetupOptions to include these)
	// For now, the choices are just validated but the templates use defaults

	return nil
}

func (d *DeveloperMode) generateFiles(projectPath string, opts *SetupOptions, analysis *analysis.ProjectAnalysis) error {
	// Determine project name for directory structure
	projectName := filepath.Base(projectPath)
	if analysis != nil && analysis.Name != "" && analysis.Name != "." {
		projectName = analysis.Name
	}

	// Ensure we have a valid project name
	if projectName == "." || projectName == "" {
		// Use the current directory name as fallback
		if wd, err := os.Getwd(); err == nil {
			projectName = filepath.Base(wd)
		} else {
			projectName = "myapp"
		}
	}

	// Create .sc directory structure
	scDir := filepath.Join(projectPath, ".sc", "stacks", projectName)
	if err := os.MkdirAll(scDir, 0o755); err != nil {
		return fmt.Errorf("failed to create .sc directory: %w", err)
	}

	// Generate client.yaml using LLM - always generate, regardless of analysis skip
	fmt.Printf("   📄 Generating client.yaml...")
	clientPath := filepath.Join(scDir, "client.yaml")

	// Check if client.yaml already exists and prompt for confirmation
	if _, err := os.Stat(clientPath); err == nil {
		if !d.confirmOverwrite("client.yaml", opts.ForceOverwrite) {
			fmt.Printf(" %s (skipped)\n", color.YellowFmt("⚠"))
		} else {
			clientYaml, err := d.GenerateClientYAMLWithLLM(opts, analysis)
			if err != nil {
				return fmt.Errorf("failed to generate client.yaml: %w", err)
			}
			if err := os.WriteFile(clientPath, []byte(clientYaml), 0o644); err != nil {
				return fmt.Errorf("failed to write client.yaml: %w", err)
			}
			fmt.Printf(" %s\n", color.GreenFmt("✓"))
		}
	} else {
		clientYaml, err := d.GenerateClientYAMLWithLLM(opts, analysis)
		if err != nil {
			return fmt.Errorf("failed to generate client.yaml: %w", err)
		}
		if err := os.WriteFile(clientPath, []byte(clientYaml), 0o644); err != nil {
			return fmt.Errorf("failed to write client.yaml: %w", err)
		}
		fmt.Printf(" %s\n", color.GreenFmt("✓"))
	}

	// Generate docker-compose.yaml using LLM
	if !opts.SkipCompose {
		fmt.Printf("   📄 Generating docker-compose.yaml...")
		composePath := filepath.Join(projectPath, "docker-compose.yaml")
		if _, err := os.Stat(composePath); os.IsNotExist(err) {
			composeYaml, err := d.GenerateComposeYAMLWithLLM(analysis)
			if err != nil {
				return fmt.Errorf("failed to generate docker-compose.yaml: %w", err)
			}
			if err := os.WriteFile(composePath, []byte(composeYaml), 0o644); err != nil {
				return fmt.Errorf("failed to write docker-compose.yaml: %w", err)
			}
			fmt.Printf(" %s\n", color.GreenFmt("✓"))
		} else {
			// File exists, prompt for confirmation
			if !d.confirmOverwrite("docker-compose.yaml", opts.ForceOverwrite) {
				fmt.Printf(" %s (skipped)\n", color.YellowFmt("⚠"))
			} else {
				composeYaml, err := d.GenerateComposeYAMLWithLLM(analysis)
				if err != nil {
					return fmt.Errorf("failed to generate docker-compose.yaml: %w", err)
				}
				if err := os.WriteFile(composePath, []byte(composeYaml), 0o644); err != nil {
					return fmt.Errorf("failed to write docker-compose.yaml: %w", err)
				}
				fmt.Printf(" %s\n", color.GreenFmt("✓"))
			}
		}
	}

	// Generate Dockerfile using LLM
	if !opts.SkipDockerfile {
		fmt.Printf("   📄 Generating Dockerfile...")
		dockerfilePath := filepath.Join(projectPath, "Dockerfile")
		if _, err := os.Stat(dockerfilePath); os.IsNotExist(err) {
			dockerfile, err := d.GenerateDockerfileWithLLM(analysis)
			if err != nil {
				return fmt.Errorf("failed to generate Dockerfile: %w", err)
			}
			if err := os.WriteFile(dockerfilePath, []byte(dockerfile), 0o644); err != nil {
				return fmt.Errorf("failed to write Dockerfile: %w", err)
			}
			fmt.Printf(" %s\n", color.GreenFmt("✓"))
		} else {
			// File exists, prompt for confirmation
			if !d.confirmOverwrite("Dockerfile", opts.ForceOverwrite) {
				fmt.Printf(" %s (skipped)\n", color.YellowFmt("⚠"))
			} else {
				dockerfile, err := d.GenerateDockerfileWithLLM(analysis)
				if err != nil {
					return fmt.Errorf("failed to generate Dockerfile: %w", err)
				}
				if err := os.WriteFile(dockerfilePath, []byte(dockerfile), 0o644); err != nil {
					return fmt.Errorf("failed to write Dockerfile: %w", err)
				}
				fmt.Printf(" %s\n", color.GreenFmt("✓"))
			}
		}
	}

	return nil
}

// confirmOverwrite prompts the user to confirm overwriting an existing file
func (d *DeveloperMode) confirmOverwrite(filename string, forceOverwrite bool) bool {
	// Skip confirmation if force overwrite is enabled (for MCP/API usage)
	if forceOverwrite {
		return true
	}

	fmt.Printf("\n   ⚠️  %s already exists. Overwrite? [y/N]: ", color.YellowString(filename))

	var response string
	if _, err := fmt.Scanln(&response); err != nil {
		// If there's an error reading input, default to "no"
		return false
	}

	response = strings.ToLower(strings.TrimSpace(response))
	return response == "y" || response == "yes"
}

// ConfirmDeploymentType confirms the detected deployment type with the user
func (d *DeveloperMode) ConfirmDeploymentType(opts *SetupOptions, analysis *analysis.ProjectAnalysis) error {
	// Skip confirmation if requested (useful for MCP/API usage)
	if opts.SkipConfirmation {
		return nil
	}
	detectedType := d.determineDeploymentTypeWithOptions(analysis, opts)

	fmt.Printf("\n🔍 Detected deployment type: %s\n", color.CyanString(detectedType))

	// Show what this means
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

	fmt.Printf("\n   Is this correct? [Y/n]: ")

	var response string
	if _, err := fmt.Scanln(&response); err != nil {
		// If there's an error reading input, default to "yes"
		return nil
	}

	response = strings.ToLower(strings.TrimSpace(response))
	if response == "n" || response == "no" {
		// Let user choose the deployment type
		return d.selectDeploymentType(opts, analysis)
	}

	return nil
}

// selectDeploymentType allows user to manually select deployment type
func (d *DeveloperMode) selectDeploymentType(opts *SetupOptions, analysis *analysis.ProjectAnalysis) error {
	fmt.Println("\n📋 Available deployment types:")
	fmt.Printf("   1. %s - Static site (HTML/CSS/JS files)\n", color.CyanString("static"))
	fmt.Printf("   2. %s - Single container (serverless/lambda style)\n", color.CyanString("single-image"))
	fmt.Printf("   3. %s - Multi-container (docker-compose based)\n", color.CyanString("cloud-compose"))

	fmt.Printf("\n   Select deployment type [1-3]: ")

	var response string
	if _, err := fmt.Scanln(&response); err != nil {
		return fmt.Errorf("failed to read input: %w", err)
	}

	response = strings.TrimSpace(response)
	switch response {
	case "1":
		// Update options to reflect static deployment
		opts.DeploymentType = "static"
		fmt.Printf("✅ Selected: %s\n", color.GreenString("static"))
	case "2":
		// Update options to reflect single-image deployment
		opts.DeploymentType = "single-image"
		fmt.Printf("✅ Selected: %s\n", color.GreenString("single-image"))
	case "3":
		// Update options to reflect cloud-compose deployment
		opts.DeploymentType = "cloud-compose"
		fmt.Printf("✅ Selected: %s\n", color.GreenString("cloud-compose"))
	default:
		fmt.Printf("⚠️  Invalid selection, using detected type: %s\n", color.YellowString(d.determineDeploymentType(analysis)))
	}

	return nil
}

// generateFilesCoordinated generates multiple files using coordinated multi-file generation
func (d *DeveloperMode) generateFilesCoordinated(ctx context.Context, projectPath string, opts *SetupOptions, analysis *analysis.ProjectAnalysis) error {
	// Build multi-file generation request
	req := MultiFileGenerationRequest{
		ProjectPath:     projectPath,
		SetupOptions:    opts,
		ProjectAnalysis: analysis,

		// Determine which files to generate based on options
		GenerateDockerfile:    !opts.SkipDockerfile,
		GenerateDockerCompose: !opts.SkipCompose,
		GenerateClientYAML:    !opts.SkipAnalysis,
		GenerateServerYAML:    false, // Server YAML is typically generated by DevOps mode

		// Use options from SetupOptions
		UseStreaming:      opts.UseStreaming,
		ValidateGenerated: true, // Always validate coordinated generation

		ForceOverwrite: opts.ForceOverwrite,
	}

	// Execute coordinated generation
	result, err := d.GenerateMultipleFiles(ctx, req)
	if err != nil {
		return fmt.Errorf("coordinated generation failed: %w", err)
	}

	// Report results
	if !result.Success {
		return fmt.Errorf("coordinated generation was not successful: %v", result.Errors)
	}

	if len(result.Warnings) > 0 {
		fmt.Printf("\n⚠️  Generation completed with warnings:\n")
		for _, warning := range result.Warnings {
			fmt.Printf("   • %s\n", color.YellowFmt(warning))
		}
	}

	fmt.Printf("\n🎉 Coordinated generation completed successfully!\n")
	return nil
}

// LLM-based file generation functions
func (d *DeveloperMode) GenerateClientYAMLWithLLM(opts *SetupOptions, analysis *analysis.ProjectAnalysis) (string, error) {
	if d.llm == nil {
		return d.generateFallbackClientYAML(opts, analysis)
	}

	projectName := filepath.Base(".")
	if analysis != nil && analysis.Name != "" && analysis.Name != "." {
		projectName = analysis.Name
	}

	// Ensure we have a valid stack name
	if projectName == "." || projectName == "" {
		// Use the current directory name as fallback
		if wd, err := os.Getwd(); err == nil {
			projectName = filepath.Base(wd)
		} else {
			projectName = "myapp"
		}
	}

	prompt := d.buildClientYAMLPrompt(opts, analysis, projectName)

	response, err := d.llm.Chat(context.Background(), []llm.Message{
		{Role: "system", Content: `You are an expert in Simple Container configuration. Generate ONLY valid YAML that EXACTLY follows the provided JSON schemas.

CRITICAL INSTRUCTIONS:
✅ Follow the JSON schemas EXACTLY - every property must match the schema structure
✅ Use ONLY properties defined in the schemas - no fictional or made-up properties
✅ client.yaml MUST have: schemaVersion, stacks section
✅ Each stack MUST have: type, parent, parentEnv, config
✅ DEPLOYMENT TYPES: cloud-compose, static, single-image
✅ cloud-compose: Multi-container (dockerComposeFile, runs, env, secrets, scale, uses)
✅ static: Static websites (bundleDir, indexDocument, errorDocument)
✅ single-image: Single container (template, image.dockerfile, timeout, maxMemory)
✅ single-image MUST include image.dockerfile: ${git:root}/Dockerfile (REQUIRED)
✅ config section can contain: runs, env, secrets, scale, uses, dependencies
✅ scale uses: {min: number, max: number} structure only
✅ env: for environment variables (NOT environment)
✅ secrets: using ${secret:name} format for secret references

🚫 FORBIDDEN (will cause validation errors):
❌ environments section (use stacks only)
❌ scaling section (use scale in config)  
❌ version property (use schemaVersion)
❌ account property (server.yaml only)
❌ minCapacity/maxCapacity (use min/max in scale)
❌ bucketName in resources (use name)
❌ connectionString property (fictional)
❌ Connection strings in env: section (security risk)

✅ CORRECT SECURITY PATTERNS:
✅ ALWAYS include dockerComposeFile: docker-compose.yaml (REQUIRED)
✅ Optional: domain property for DNS routing (requires registrar in server.yaml)
✅ env: section for non-sensitive config (PORT, NODE_ENV, LOG_LEVEL)
✅ secrets: section for sensitive data (API keys, connection strings, tokens)
✅ Use ${resource:<name>.<property>} when consuming resources via 'uses:'
✅ Use ${secret:<name>} for manually defined secrets in parent stack

RESPONSE FORMAT: Generate ONLY the YAML content. No explanations, no markdown blocks, no additional text.`},
		{Role: "user", Content: prompt},
	})
	if err != nil {
		fmt.Printf("LLM generation failed, using fallback: %v\n", err)
		return d.generateFallbackClientYAML(opts, analysis)
	}

	// Extract YAML from response (remove any markdown formatting)
	yamlContent := strings.TrimSpace(response.Content)
	yamlContent = strings.TrimPrefix(yamlContent, "```yaml")
	yamlContent = strings.TrimPrefix(yamlContent, "```")
	yamlContent = strings.TrimSuffix(yamlContent, "```")
	yamlContent = strings.TrimSpace(yamlContent)

	// Validate generated YAML against schemas
	validator := validation.NewValidator()
	result := validator.ValidateClientYAML(context.Background(), yamlContent)

	if !result.Valid {
		fmt.Printf("⚠️  Generated client.yaml has validation errors:\n")
		for _, err := range result.Errors {
			fmt.Printf("   • %s\n", color.RedFmt(err))
		}
		fmt.Printf("   🔄 Using schema-compliant fallback template...\n")
		return d.generateFallbackClientYAML(opts, analysis)
	}

	if len(result.Warnings) > 0 {
		for _, warning := range result.Warnings {
			fmt.Printf("   ⚠️  %s\n", color.YellowFmt(warning))
		}
	}

	return yamlContent, nil
}

// GenerateClientYAMLWithStreamingLLM generates client.yaml with streaming progress feedback
func (d *DeveloperMode) GenerateClientYAMLWithStreamingLLM(opts *SetupOptions, analysis *analysis.ProjectAnalysis, progressCallback llm.StreamCallback) (string, error) {
	if d.llm == nil {
		return d.generateFallbackClientYAML(opts, analysis)
	}

	// Check if provider supports streaming
	caps := d.llm.GetCapabilities()
	if !caps.SupportsStreaming {
		// Fall back to regular generation
		return d.GenerateClientYAMLWithLLM(opts, analysis)
	}

	projectName := filepath.Base(".")
	if analysis != nil && analysis.Name != "" && analysis.Name != "." {
		projectName = analysis.Name
	}

	// Ensure we have a valid stack name
	if projectName == "." || projectName == "" {
		// Use the current directory name as fallback
		if wd, err := os.Getwd(); err == nil {
			projectName = filepath.Base(wd)
		} else {
			projectName = "myapp"
		}
	}

	prompt := d.buildClientYAMLPrompt(opts, analysis, projectName)

	fmt.Printf("🔄 Generating client.yaml with streaming...")

	response, err := d.llm.StreamChat(context.Background(), []llm.Message{
		{Role: "system", Content: `You are an expert in Simple Container configuration. Generate ONLY valid YAML that EXACTLY follows the provided JSON schemas.

CRITICAL INSTRUCTIONS:
✅ Follow the JSON schemas EXACTLY - every property must match the schema structure
✅ Use ONLY properties defined in the schemas - no fictional or made-up properties
✅ client.yaml MUST have: schemaVersion, stacks section
✅ Each stack MUST have: type, parent, parentEnv, config
✅ DEPLOYMENT TYPES: cloud-compose, static, single-image
✅ cloud-compose: Multi-container (dockerComposeFile, runs, env, secrets, scale, uses)
✅ static: Static websites (bundleDir, indexDocument, errorDocument)
✅ single-image: Single container (template, image.dockerfile, timeout, maxMemory)
✅ single-image MUST include image.dockerfile: ${git:root}/Dockerfile (REQUIRED)
✅ config section can contain: runs, env, secrets, scale, uses, dependencies
✅ scale uses: {min: number, max: number} structure only
✅ env: for environment variables (NOT environment)
✅ secrets: using ${secret:name} format for secret references

🚫 FORBIDDEN (will cause validation errors):
❌ environments section (use stacks only)
❌ scaling section (use scale in config)  
❌ version property (use schemaVersion)
❌ account property (server.yaml only)
❌ minCapacity/maxCapacity (use min/max in scale)
❌ bucketName in resources (use name)
❌ connectionString property (fictional)
❌ Connection strings in env: section (security risk)

✅ CORRECT PATTERNS:
- parent: Use format <parent-project>/<parent-stack-name> (e.g., mycompany/devops)
- parentEnv: Maps to environment in parent's server.yaml (staging, prod, etc.)
- template: Optional - only if overriding parent's default (must exist in server.yaml templates section)
- Stack naming: Use environment names directly (staging, prod) or custom names with parentEnv reference
- ALWAYS include dockerComposeFile: docker-compose.yaml (REQUIRED for cloud-compose - references ${project:root}/docker-compose.yaml)
- ALWAYS include image.dockerfile: ${git:root}/Dockerfile (REQUIRED for single-image deployments)
- Optional: domain property for DNS routing (requires registrar in server.yaml)
- env: section for non-sensitive config (PORT, NODE_ENV, LOG_LEVEL)
- secrets: section for sensitive data (API keys, connection strings, tokens)
- Use ${resource:name.url} when consuming resources via 'uses:'
- Use ${secret:name} for manually defined secrets in parent stack

RESPONSE FORMAT: Generate ONLY the YAML content. No explanations, no markdown blocks, no additional text.`},
		{Role: "user", Content: prompt},
	}, progressCallback)
	if err != nil {
		fmt.Printf("\nLLM streaming generation failed, using fallback: %v\n", err)
		return d.generateFallbackClientYAML(opts, analysis)
	}

	// Extract YAML content from response
	yamlContent := strings.TrimSpace(response.Content)
	yamlContent = strings.TrimPrefix(yamlContent, "```yaml")
	yamlContent = strings.TrimPrefix(yamlContent, "```")
	yamlContent = strings.TrimSuffix(yamlContent, "```")
	yamlContent = strings.TrimSpace(yamlContent)

	// Validate generated YAML against schemas
	validator := validation.NewValidator()
	result := validator.ValidateClientYAML(context.Background(), yamlContent)

	if !result.Valid {
		fmt.Printf("\n⚠️  Generated client.yaml has validation errors:\n")
		for _, err := range result.Errors {
			fmt.Printf("   • %s\n", color.RedFmt(err))
		}
		fmt.Printf("   🔄 Using schema-compliant fallback template...\n")
		return d.generateFallbackClientYAML(opts, analysis)
	}

	if len(result.Warnings) > 0 {
		for _, warning := range result.Warnings {
			fmt.Printf("   ⚠️  %s\n", color.YellowFmt(warning))
		}
	}

	fmt.Printf("\n✅ Generated schema-compliant client.yaml\n")
	return yamlContent, nil
}

func (d *DeveloperMode) buildClientYAMLPrompt(opts *SetupOptions, analysis *analysis.ProjectAnalysis, projectName string) string {
	var prompt strings.Builder

	prompt.WriteString("Generate a Simple Container client.yaml configuration using ONLY these validated properties:\n\n")
	prompt.WriteString(fmt.Sprintf("Project: %s\n", projectName))
	prompt.WriteString(fmt.Sprintf("Parent stack: %s\n", opts.Parent))
	prompt.WriteString(fmt.Sprintf("Environment: %s\n", opts.Environment))

	if analysis != nil && analysis.PrimaryStack != nil {
		prompt.WriteString(fmt.Sprintf("Detected language: %s\n", analysis.PrimaryStack.Language))
		if analysis.PrimaryStack.Framework != "" {
			prompt.WriteString(fmt.Sprintf("Framework: %s\n", analysis.PrimaryStack.Framework))
		}
	}

	// Add detected resources from comprehensive analysis
	if analysis != nil && analysis.Resources != nil {
		prompt.WriteString("\n🎯 DETECTED PROJECT RESOURCES (MUST include in configuration):\n")

		// Databases
		if len(analysis.Resources.Databases) > 0 {
			prompt.WriteString("Databases found:\n")
			for _, db := range analysis.Resources.Databases {
				prompt.WriteString(fmt.Sprintf("  • %s (%.0f%% confidence)\n", strings.ToUpper(db.Type), db.Confidence*100))
			}
		}

		// Storage systems
		if len(analysis.Resources.Storage) > 0 {
			prompt.WriteString("Storage systems found:\n")
			for _, storage := range analysis.Resources.Storage {
				prompt.WriteString(fmt.Sprintf("  • %s (%.0f%% confidence, purpose: %s)\n",
					strings.ToUpper(storage.Type), storage.Confidence*100, storage.Purpose))
			}
		}

		// Queue systems
		if len(analysis.Resources.Queues) > 0 {
			prompt.WriteString("Queue systems found:\n")
			for _, queue := range analysis.Resources.Queues {
				prompt.WriteString(fmt.Sprintf("  • %s (%.0f%% confidence)\n", strings.ToUpper(queue.Type), queue.Confidence*100))
			}
		}

		// External APIs
		if len(analysis.Resources.ExternalAPIs) > 0 {
			prompt.WriteString("External APIs found:\n")
			for _, api := range analysis.Resources.ExternalAPIs {
				prompt.WriteString(fmt.Sprintf("  • %s (%.0f%% confidence, purpose: %s)\n",
					api.Name, api.Confidence*100, api.Purpose))
			}
		}

		// Environment variables (show key ones)
		if len(analysis.Resources.EnvironmentVars) > 0 {
			prompt.WriteString(fmt.Sprintf("Environment variables: %d detected\n", len(analysis.Resources.EnvironmentVars)))
			// Show first few important ones
			count := 0
			for _, env := range analysis.Resources.EnvironmentVars {
				if count >= 5 { // Limit to avoid too much text
					break
				}
				if env.UsageType != "system" && env.UsageType != "development" {
					prompt.WriteString(fmt.Sprintf("  • %s (%s)\n", env.Name, env.UsageType))
					count++
				}
			}
		}

		// Secrets found
		if len(analysis.Resources.Secrets) > 0 {
			prompt.WriteString(fmt.Sprintf("Secrets: %d detected (API keys, tokens, etc.)\n", len(analysis.Resources.Secrets)))
		}
	}

	// Add JSON schema context for better generation
	validator := validation.NewValidator()
	if clientSchema, err := validator.GetClientYAMLSchema(context.Background()); err == nil {
		if schemaContent, err := json.MarshalIndent(clientSchema, "", "  "); err == nil {
			prompt.WriteString("\n📋 CLIENT.YAML JSON SCHEMA (follow this structure exactly):\n")
			prompt.WriteString("```json\n")
			prompt.WriteString(string(schemaContent))
			prompt.WriteString("\n```\n")
		}
	}

	if stackSchema, err := validator.GetStackConfigComposeSchema(context.Background()); err == nil {
		if schemaContent, err := json.MarshalIndent(stackSchema, "", "  "); err == nil {
			prompt.WriteString("\n📋 STACK CONFIG SCHEMA (for config section structure):\n")
			prompt.WriteString("```json\n")
			prompt.WriteString(string(schemaContent))
			prompt.WriteString("\n```\n")
		}
	}

	// Add validated example structure
	prompt.WriteString("\n✅ REQUIRED STRUCTURE EXAMPLE:\n")
	prompt.WriteString("schemaVersion: 1.0\n")
	prompt.WriteString("stacks:\n")
	prompt.WriteString("  " + opts.Environment + ":\n")
	// Determine deployment type for example
	deploymentType := d.determineDeploymentTypeWithOptions(analysis, opts)
	prompt.WriteString(fmt.Sprintf("    type: %s       # Valid types: cloud-compose, static, single-image\n", deploymentType))

	// Handle parent field properly - must be in format <parent-project>/<parent-stack-name>
	if opts.Parent != "" {
		// If parent contains a slash, use as-is. Otherwise, provide a proper example format.
		if strings.Contains(opts.Parent, "/") {
			prompt.WriteString("    parent: " + opts.Parent + "       # Format: <parent-project>/<parent-stack-name>\n")
		} else {
			prompt.WriteString("    parent: mycompany/" + opts.Parent + "     # Format: <parent-project>/<parent-stack-name>\n")
		}
	} else {
		// If no parent specified, use example format
		prompt.WriteString("    parent: mycompany/infrastructure     # Format: <parent-project>/<parent-stack-name>\n")
	}

	prompt.WriteString("    parentEnv: " + opts.Environment + "        # Environment in parent's server.yaml\n")
	prompt.WriteString("    config:\n")

	// Generate different example configs based on deployment type
	switch deploymentType {
	case "static":
		prompt.WriteString("      bundleDir: ${git:root}/build  # Directory containing static files\n")
		prompt.WriteString("      indexDocument: index.html     # Entry point for static site\n")
		prompt.WriteString("      errorDocument: error.html     # Custom error page\n")
		prompt.WriteString(fmt.Sprintf("      domain: %s.mycompany.com  # Optional: DNS domain (requires registrar in server.yaml)\n", projectName))
	case "single-image":
		prompt.WriteString("      template: lambda-us           # Optional: Override parent's default (must exist in server.yaml templates)\n")
		prompt.WriteString("      image:\n")
		prompt.WriteString("        dockerfile: ${git:root}/Dockerfile  # Path to Dockerfile\n")
		prompt.WriteString("      timeout: 120                 # Function timeout in seconds\n")
		prompt.WriteString("      maxMemory: 512               # Memory allocation in MB\n")

		// Add detected resources to uses section for single-image
		if analysis != nil && analysis.Resources != nil {
			resourceMatcher := resources.NewResourceMatcher()
			uses := []string{}

			// Add detected databases
			for _, db := range analysis.Resources.Databases {
				resourceType := resourceMatcher.GetBestResourceType(db.Type)
				uses = append(uses, resourceType)
			}

			// Add detected storage
			for _, storage := range analysis.Resources.Storage {
				resourceType := resourceMatcher.GetBestResourceType(storage.Type)
				uses = append(uses, resourceType)
			}

			// Add detected queues
			for _, queue := range analysis.Resources.Queues {
				resourceType := resourceMatcher.GetBestResourceType(queue.Type)
				// Avoid duplicates if Redis is used for both cache and pubsub
				if !contains(uses, resourceType) {
					uses = append(uses, resourceType)
				}
			}

			if len(uses) > 0 {
				prompt.WriteString(fmt.Sprintf("      uses: %s  # Based on detected resources\n", formatYAMLArray(uses)))
			}
		}

		prompt.WriteString("      env:\n")
		prompt.WriteString("        ENVIRONMENT: production\n")

		// Add environment variables from analysis
		if analysis != nil && analysis.Resources != nil && len(analysis.Resources.EnvironmentVars) > 0 {
			prompt.WriteString("        # Detected environment variables:\n")
			for _, env := range analysis.Resources.EnvironmentVars {
				if env.UsageType == "service_config" || env.UsageType == "api_config" {
					prompt.WriteString(fmt.Sprintf("        %s: \"${resource:service.%s}\"  # From analysis\n",
						env.Name, strings.ToLower(env.Name)))
				}
			}
		}

		prompt.WriteString("      secrets:\n")

		// Add secrets based on detected resources
		if analysis != nil && analysis.Resources != nil {
			resourceMatcher := resources.NewResourceMatcher()

			// Database connection secrets
			for _, db := range analysis.Resources.Databases {
				resourceType := resourceMatcher.GetBestResourceType(db.Type)
				switch strings.ToLower(db.Type) {
				case "mongodb", "mongo":
					prompt.WriteString(fmt.Sprintf("        MONGO_URI: \"${resource:%s.uri}\"  # MongoDB connection\n", resourceType))
				case "redis":
					prompt.WriteString(fmt.Sprintf("        REDIS_URL: \"${resource:%s.url}\"  # Redis connection\n", resourceType))
				case "postgresql", "postgres":
					prompt.WriteString(fmt.Sprintf("        DATABASE_URL: \"${resource:%s.url}\"  # PostgreSQL connection\n", resourceType))
				case "mysql":
					prompt.WriteString(fmt.Sprintf("        MYSQL_URL: \"${resource:%s.url}\"  # MySQL connection\n", resourceType))
				}
			}

			// Storage secrets
			for _, storage := range analysis.Resources.Storage {
				resourceType := resourceMatcher.GetBestResourceType(storage.Type)
				if strings.ToLower(storage.Type) == "s3" {
					prompt.WriteString(fmt.Sprintf("        S3_ACCESS_KEY: \"${resource:%s.accessKey}\"  # S3 credentials\n", resourceType))
					prompt.WriteString(fmt.Sprintf("        S3_SECRET_KEY: \"${resource:%s.secretKey}\"  # S3 credentials\n", resourceType))
				}
			}
		}

		prompt.WriteString("        API_KEY: \"${secret:api-key}\"  # Manual secrets\n")
	default: // cloud-compose
		prompt.WriteString("      dockerComposeFile: docker-compose.yaml  # REQUIRED: Reference to ${project:root}/docker-compose.yaml\n")
		prompt.WriteString("      runs: [app]            # Container names from ${project:root}/docker-compose.yaml\n")
		prompt.WriteString(fmt.Sprintf("      domain: %s.mycompany.com  # Optional: DNS domain (requires registrar in server.yaml)\n", projectName))
		prompt.WriteString("      scale:\n")
		prompt.WriteString("        min: 1              # Must be in config section, NOT separate scaling block\n")
		prompt.WriteString("        max: 3\n")

		// Add detected resources to uses section for cloud-compose
		if analysis != nil && analysis.Resources != nil {
			resourceMatcher := resources.NewResourceMatcher()
			uses := []string{}

			// Add detected databases
			for _, db := range analysis.Resources.Databases {
				resourceType := resourceMatcher.GetBestResourceType(db.Type)
				uses = append(uses, resourceType)
			}

			// Add detected storage
			for _, storage := range analysis.Resources.Storage {
				resourceType := resourceMatcher.GetBestResourceType(storage.Type)
				uses = append(uses, resourceType)
			}

			// Add detected queues
			for _, queue := range analysis.Resources.Queues {
				resourceType := resourceMatcher.GetBestResourceType(queue.Type)
				// Avoid duplicates if Redis is used for both cache and pubsub
				if !contains(uses, resourceType) {
					uses = append(uses, resourceType)
				}
			}

			if len(uses) > 0 {
				prompt.WriteString(fmt.Sprintf("      uses: %s  # Based on detected resources\n", formatYAMLArray(uses)))
			} else {
				prompt.WriteString("      uses: []  # No shared resources detected - add parent resources as needed\n")
			}
		} else {
			prompt.WriteString("      uses: []  # No resources detected - add parent resources as needed\n")
		}

		prompt.WriteString("      env:                  # Non-sensitive environment variables only\n")

		// Add detected tech stack port
		if analysis != nil && analysis.PrimaryStack != nil {
			switch analysis.PrimaryStack.Language {
			case "python":
				prompt.WriteString("        PORT: 8000\n")
			case "go":
				prompt.WriteString("        PORT: 8080\n")
			case "javascript":
				prompt.WriteString("        PORT: 3000\n")
			default:
				prompt.WriteString("        PORT: 3000\n")
			}
		} else {
			prompt.WriteString("        PORT: 3000\n")
		}

		if analysis != nil && analysis.PrimaryStack != nil {
			switch analysis.PrimaryStack.Language {
			case "javascript":
				prompt.WriteString("        NODE_ENV: production\n")
			case "python":
				prompt.WriteString("        PYTHON_ENV: production\n")
			case "go":
				prompt.WriteString("        GO_ENV: production\n")
			default:
				prompt.WriteString("        ENVIRONMENT: production\n")
			}
		} else {
			prompt.WriteString("        NODE_ENV: production\n")
		}

		// Add environment variables from analysis
		if analysis != nil && analysis.Resources != nil && len(analysis.Resources.EnvironmentVars) > 0 {
			prompt.WriteString("        # Detected environment variables:\n")
			for _, env := range analysis.Resources.EnvironmentVars {
				if env.UsageType == "service_config" || env.UsageType == "api_config" {
					prompt.WriteString(fmt.Sprintf("        %s: \"${resource:service.%s}\"  # From analysis\n",
						env.Name, strings.ToLower(env.Name)))
				}
			}
		}

		prompt.WriteString("        # Database connections use auto-injected environment variables:\n")

		// Show relevant connection info based on detected resources
		if analysis != nil && analysis.Resources != nil {
			for _, db := range analysis.Resources.Databases {
				switch strings.ToLower(db.Type) {
				case "postgresql", "postgres":
					prompt.WriteString("        # PostgreSQL: PGHOST, PGPORT, PGUSER, PGDATABASE, PGPASSWORD\n")
				case "redis":
					prompt.WriteString("        # Redis: REDIS_HOST, REDIS_PORT\n")
				case "mongodb":
					prompt.WriteString("        # MongoDB Atlas: MONGO_USER, MONGO_DATABASE, MONGO_PASSWORD, MONGO_URI\n")
				case "mysql":
					prompt.WriteString("        # MySQL: MYSQL_HOST, MYSQL_PORT, MYSQL_USER, MYSQL_DATABASE, MYSQL_PASSWORD\n")
				}
			}
		} else {
			prompt.WriteString("        # PostgreSQL: PGHOST, PGPORT, PGUSER, PGDATABASE, PGPASSWORD\n")
			prompt.WriteString("        # Redis: REDIS_HOST, REDIS_PORT\n")
			prompt.WriteString("        # MongoDB Atlas: MONGO_USER, MONGO_DATABASE, MONGO_PASSWORD, MONGO_URI\n")
		}

		prompt.WriteString("      secrets:              # Sensitive data: secrets vs resource consumption\n")

		// Add secrets based on detected resources
		if analysis != nil && analysis.Resources != nil {
			resourceMatcher := resources.NewResourceMatcher()

			// Database connection secrets
			for _, db := range analysis.Resources.Databases {
				resourceType := resourceMatcher.GetBestResourceType(db.Type)
				switch strings.ToLower(db.Type) {
				case "mongodb", "mongo":
					prompt.WriteString(fmt.Sprintf("        MONGO_URI: \"${resource:%s.uri}\"  # MongoDB connection\n", resourceType))
				case "redis":
					prompt.WriteString(fmt.Sprintf("        REDIS_URL: \"${resource:%s.url}\"  # Redis connection\n", resourceType))
				case "postgresql", "postgres":
					prompt.WriteString(fmt.Sprintf("        DATABASE_URL: \"${resource:%s.url}\"  # PostgreSQL connection\n", resourceType))
				case "mysql":
					prompt.WriteString(fmt.Sprintf("        MYSQL_URL: \"${resource:%s.url}\"  # MySQL connection\n", resourceType))
				}
			}

			// Storage secrets
			for _, storage := range analysis.Resources.Storage {
				resourceType := resourceMatcher.GetBestResourceType(storage.Type)
				if strings.ToLower(storage.Type) == "s3" {
					prompt.WriteString(fmt.Sprintf("        S3_ACCESS_KEY: \"${resource:%s.accessKey}\"  # S3 credentials\n", resourceType))
					prompt.WriteString(fmt.Sprintf("        S3_SECRET_KEY: \"${resource:%s.secretKey}\"  # S3 credentials\n", resourceType))
				}
			}
		} else {
			prompt.WriteString("        DATABASE_URL: \"${resource:aws-rds-postgres.url}\"  # From consumed resource\n")
			prompt.WriteString("        REDIS_URL: \"${resource:aws-elasticache.url}\"    # From consumed resource\n")
		}

		prompt.WriteString("        JWT_SECRET: \"${secret:jwt-secret}\"                    # Manual secrets from parent\n")
	}

	// Show multiple environments example
	prompt.WriteString("\n✅ MULTIPLE ENVIRONMENTS EXAMPLE:\n")
	prompt.WriteString("stacks:\n")
	prompt.WriteString("  staging:\n")
	prompt.WriteString("    type: " + deploymentType + "\n")
	prompt.WriteString("    parent: mycompany/devops\n")
	prompt.WriteString("    parentEnv: staging          # Maps to 'resources: staging:' in server.yaml\n")
	prompt.WriteString("    config: { ... }\n")
	prompt.WriteString("  prod:\n")
	prompt.WriteString("    type: " + deploymentType + "\n")
	prompt.WriteString("    parent: mycompany/devops\n")
	prompt.WriteString("    parentEnv: prod             # Maps to 'resources: prod:' in server.yaml\n")
	prompt.WriteString("    config: { ... }\n")
	prompt.WriteString("  custom-env:\n")
	prompt.WriteString("    type: " + deploymentType + "\n")
	prompt.WriteString("    parent: mycompany/devops\n")
	prompt.WriteString("    parentEnv: staging          # REQUIRED: Custom env name must reference actual server.yaml environment\n")
	prompt.WriteString("    config: { ... }\n")

	// Enrich context with validated examples
	contextEnrichment := d.enrichContextWithDocumentation("client.yaml", analysis)
	if contextEnrichment != "" {
		prompt.WriteString("\n📋 VALIDATED EXAMPLES:\n" + contextEnrichment)
	}

	prompt.WriteString("\n🚫 NEVER USE THESE (fictional properties eliminated in validation):\n")
	prompt.WriteString("- environments: section (use 'stacks:' only)\n")
	prompt.WriteString("- scaling: section (use 'scale:' in config)\n")
	prompt.WriteString("- version: property (use 'schemaVersion:')\n")
	prompt.WriteString("- account: property (DevOps server.yaml only)\n")
	prompt.WriteString("- minCapacity/maxCapacity (use min/max in scale)\n")
	prompt.WriteString("- ${secret:database-url} when using 'uses:' array (use ${resource:postgres-db.url} instead)\n")
	prompt.WriteString("- ${secret:redis-url} when using 'uses:' array (use ${resource:redis-cache.url} instead)\n")
	prompt.WriteString("- ${resource:name.connectionString} (fictional property - use .url instead)\n")
	prompt.WriteString("- Missing dockerComposeFile property (REQUIRED for cloud-compose stacks)\n")
	prompt.WriteString("\n✅ CORRECT PATTERNS:\n")
	prompt.WriteString("- parent: Use format <parent-project>/<parent-stack-name> (e.g., mycompany/devops)\n")
	prompt.WriteString("- parentEnv: Maps to environment in parent's server.yaml (staging, prod, etc.)\n")
	prompt.WriteString("- template: Optional - only if overriding parent's default (must exist in server.yaml templates section)\n")
	prompt.WriteString("- Stack naming: Use environment names directly (staging, prod) or custom names with parentEnv reference\n")
	prompt.WriteString("- ALWAYS include dockerComposeFile: docker-compose.yaml (REQUIRED for cloud-compose - references ${project:root}/docker-compose.yaml)\n")
	prompt.WriteString("- ALWAYS include image.dockerfile: ${git:root}/Dockerfile (REQUIRED for single-image deployments)\n")
	prompt.WriteString("- Optional: domain property for DNS routing (requires registrar in server.yaml)\n")
	prompt.WriteString("- Resource consumption: uses: [resource-name] + ${resource:name.url}\n")
	prompt.WriteString("- Manual secrets: Use ${secret:name} for parent-defined secrets only\n")
	prompt.WriteString("- Connection URLs: ${resource:postgres-db.url}, ${resource:redis-cache.url}\n")
	prompt.WriteString("- Auto-injected env vars: PGHOST, REDIS_HOST available from consumed resources\n")
	prompt.WriteString("- env: section for non-sensitive config (PORT, NODE_ENV, etc.)\n")
	prompt.WriteString("- secrets: section for sensitive data (API keys, connection strings, tokens)\n")

	prompt.WriteString("\n⚡ Generate ONLY the valid YAML (no explanations, no markdown):")

	return prompt.String()
}

// enrichContextWithDocumentation performs semantic search and enriches LLM context
func (d *DeveloperMode) enrichContextWithDocumentation(configType string, analysis *analysis.ProjectAnalysis) string {
	if d.embeddings == nil {
		return ""
	}

	var searchQueries []string
	var prompt strings.Builder

	// Build search queries based on context
	switch configType {
	case "client.yaml":
		searchQueries = []string{
			"client.yaml configuration example",
			"Simple Container stacks configuration",
			"cloud-compose stack type example",
		}

		// Add language-specific queries
		if analysis != nil && analysis.PrimaryStack != nil {
			searchQueries = append(searchQueries,
				fmt.Sprintf("%s client.yaml example", analysis.PrimaryStack.Language),
				fmt.Sprintf("%s Simple Container configuration", analysis.PrimaryStack.Language),
			)
			if analysis.PrimaryStack.Framework != "" {
				searchQueries = append(searchQueries,
					fmt.Sprintf("%s %s Simple Container example", analysis.PrimaryStack.Language, analysis.PrimaryStack.Framework),
				)
			}
		}

	case "docker-compose":
		searchQueries = []string{
			"docker-compose.yaml example",
			"Docker Compose best practices",
			"containerization patterns",
		}

		if analysis != nil && analysis.PrimaryStack != nil {
			searchQueries = append(searchQueries,
				fmt.Sprintf("%s docker-compose example", analysis.PrimaryStack.Language),
				fmt.Sprintf("%s containerization", analysis.PrimaryStack.Language),
			)
		}

	case "dockerfile":
		searchQueries = []string{
			"Dockerfile best practices",
			"multi-stage Dockerfile example",
			"container optimization",
		}

		if analysis != nil && analysis.PrimaryStack != nil {
			searchQueries = append(searchQueries,
				fmt.Sprintf("%s Dockerfile example", analysis.PrimaryStack.Language),
				fmt.Sprintf("%s container image", analysis.PrimaryStack.Language),
			)
		}
	}

	// Perform semantic search and collect relevant context
	var relevantDocs []string
	for _, query := range searchQueries {
		results, err := embeddings.SearchDocumentation(d.embeddings, query, 2) // Get top 2 results per query
		if err != nil {
			continue
		}

		for _, result := range results {
			if result.Similarity > 0.7 { // Only include highly relevant results
				// Use the Title field directly
				title := result.Title
				if title == "" {
					title = result.ID
				}

				// Truncate content to avoid overwhelming the LLM
				content := result.Content
				if len(content) > 300 {
					content = content[:300] + "..."
				}

				relevantDocs = append(relevantDocs, fmt.Sprintf("- %s: %s", title, content))
			}
		}

		// Limit total context to avoid overwhelming the LLM
		if len(relevantDocs) >= 5 {
			break
		}
	}

	// Format the enriched context
	if len(relevantDocs) > 0 {
		prompt.WriteString("Relevant documentation and examples:\n")
		for i, doc := range relevantDocs {
			if i >= 5 { // Limit to top 5 most relevant docs
				break
			}
			prompt.WriteString(doc)
			prompt.WriteString("\n")
		}
		prompt.WriteString("\n")
	}

	return prompt.String()
}

// determineDeploymentType detects the appropriate deployment type based on project analysis
func (d *DeveloperMode) determineDeploymentType(analysis *analysis.ProjectAnalysis) string {
	return d.determineDeploymentTypeWithOptions(analysis, nil)
}

// determineDeploymentTypeWithOptions detects deployment type with optional override from SetupOptions
func (d *DeveloperMode) determineDeploymentTypeWithOptions(analysis *analysis.ProjectAnalysis, opts *SetupOptions) string {
	// Check for manual override first
	if opts != nil && opts.DeploymentType != "" {
		return opts.DeploymentType
	}

	if analysis == nil {
		return "cloud-compose" // Default fallback
	}

	// Check for static site indicators
	staticIndicators := []string{"build", "dist", "public", "_site", "out"}
	for _, dir := range staticIndicators {
		if _, err := os.Stat(dir); err == nil {
			// Check if it contains web assets
			if d.containsStaticAssets(dir) {
				return "static"
			}
		}
	}

	// Check for single-image indicators (serverless/lambda patterns)
	if analysis.PrimaryStack != nil {
		// Check for AWS Lambda indicators
		if analysis.PrimaryStack.Language == "javascript" || analysis.PrimaryStack.Language == "python" || analysis.PrimaryStack.Language == "go" {
			// Look for handler files or serverless configurations
			serverlessFiles := []string{"handler.js", "lambda_function.py", "main.go", "serverless.yml", "template.yaml", "sam.yaml"}
			for _, file := range serverlessFiles {
				if _, err := os.Stat(file); err == nil {
					return "single-image"
				}
			}
		}
	}

	// Check for docker-compose.yaml (multi-container)
	if _, err := os.Stat("docker-compose.yaml"); err == nil {
		return "cloud-compose"
	}
	if _, err := os.Stat("docker-compose.yml"); err == nil {
		return "cloud-compose"
	}

	// Default to cloud-compose for containerized applications
	return "cloud-compose"
}

// containsStaticAssets checks if a directory contains typical static web assets
func (d *DeveloperMode) containsStaticAssets(dir string) bool {
	staticFiles := []string{"index.html", "index.htm", "main.js", "app.js", "style.css", "main.css"}
	for _, file := range staticFiles {
		if _, err := os.Stat(filepath.Join(dir, file)); err == nil {
			return true
		}
	}
	return false
}

func (d *DeveloperMode) generateFallbackClientYAML(opts *SetupOptions, analysis *analysis.ProjectAnalysis) (string, error) {
	// Determine deployment type based on project analysis
	deploymentType := d.determineDeploymentTypeWithOptions(analysis, opts)

	// Build language-specific environment variables based on project analysis
	envVars := d.buildLanguageSpecificEnvVars(analysis)
	secrets := d.buildLanguageSpecificSecrets(analysis)

	var template strings.Builder
	template.WriteString("schemaVersion: 1.0\n\n")
	template.WriteString("stacks:\n")
	template.WriteString(fmt.Sprintf("  %s:\n", opts.Environment))
	template.WriteString(fmt.Sprintf("    type: %s\n", deploymentType))
	// Handle parent field properly - must be in format <parent-project>/<parent-stack-name>
	if opts.Parent != "" {
		// If parent contains a slash, use as-is. Otherwise, provide a proper example format.
		if strings.Contains(opts.Parent, "/") {
			template.WriteString(fmt.Sprintf("    parent: %s\n", opts.Parent))
		} else {
			template.WriteString(fmt.Sprintf("    parent: mycompany/%s\n", opts.Parent))
		}
	} else {
		// If no parent specified, use example format
		template.WriteString("    parent: mycompany/infrastructure\n")
	}
	template.WriteString(fmt.Sprintf("    parentEnv: %s\n", opts.Environment))
	template.WriteString("    config:\n")

	// Generate different configurations based on deployment type
	switch deploymentType {
	case "static":
		d.generateStaticConfig(&template, analysis)
	case "single-image":
		d.generateSingleImageConfig(&template, analysis, envVars, secrets)
	case "cloud-compose":
		fallthrough
	default:
		d.generateCloudComposeConfig(&template, analysis, envVars, secrets)
	}

	return template.String(), nil
}

// generateStaticConfig generates configuration for static website deployments
func (d *DeveloperMode) generateStaticConfig(template *strings.Builder, analysis *analysis.ProjectAnalysis) {
	// Detect bundle directory
	bundleDir := "${git:root}/dist"
	staticDirs := []string{"build", "dist", "public", "_site", "out"}
	for _, dir := range staticDirs {
		if _, err := os.Stat(dir); err == nil {
			bundleDir = fmt.Sprintf("${git:root}/%s", dir)
			break
		}
	}

	template.WriteString("      # Static website configuration\n")
	template.WriteString(fmt.Sprintf("      bundleDir: %s\n", bundleDir))
	template.WriteString("      indexDocument: index.html\n")
	template.WriteString("      errorDocument: error.html\n")
	template.WriteString("      \n")
	template.WriteString("      # Optional: Custom domain (requires registrar in server.yaml)\n")
	if analysis != nil && analysis.Name != "" && analysis.Name != "." {
		template.WriteString(fmt.Sprintf("      # domain: %s.mycompany.com\n", analysis.Name))
	} else {
		template.WriteString("      # domain: mysite.mycompany.com\n")
	}
}

// generateSingleImageConfig generates configuration for single-image deployments (Lambda, Cloud Run)
func (d *DeveloperMode) generateSingleImageConfig(template *strings.Builder, analysis *analysis.ProjectAnalysis, envVars, secrets map[string]string) {
	template.WriteString("      # Single-image deployment (Lambda/Cloud Run)\n")
	template.WriteString("      template: lambda-us  # Optional: Override parent's default (must exist in server.yaml templates)\n")
	template.WriteString("      \n")
	template.WriteString("      # Container image configuration\n")
	template.WriteString("      image:\n")
	template.WriteString("        dockerfile: ${git:root}/Dockerfile\n")
	template.WriteString("      \n")
	template.WriteString("      # Function configuration\n")
	template.WriteString("      timeout: 120  # seconds\n")
	template.WriteString("      maxMemory: 512  # MB\n")

	if len(envVars) > 0 {
		template.WriteString("      \n")
		template.WriteString("      # Environment variables\n")
		template.WriteString("      env:\n")
		for key, value := range envVars {
			template.WriteString(fmt.Sprintf("        %s: %s\n", key, value))
		}
	}

	if len(secrets) > 0 {
		template.WriteString("      \n")
		template.WriteString("      # Secrets\n")
		template.WriteString("      secrets:\n")
		for key, value := range secrets {
			template.WriteString(fmt.Sprintf("        %s: \"%s\"\n", key, value))
		}
	}
}

// generateCloudComposeConfig generates configuration for multi-container deployments
func (d *DeveloperMode) generateCloudComposeConfig(template *strings.Builder, analysis *analysis.ProjectAnalysis, envVars, secrets map[string]string) {
	template.WriteString("      # Reference to ${project:root}/docker-compose.yaml (REQUIRED)\n")
	template.WriteString("      dockerComposeFile: docker-compose.yaml\n")
	template.WriteString("      \n")
	template.WriteString("      # Shared resources from DevOps team\n")
	template.WriteString("      uses: [postgres-db]\n")
	template.WriteString("      \n")
	template.WriteString("      # Services from ${project:root}/docker-compose.yaml\n")
	template.WriteString("      runs: [app]\n")
	template.WriteString("      \n")
	template.WriteString("      # Optional: DNS domain (only works if registrar configured in server.yaml)\n")
	if analysis != nil && analysis.Name != "" && analysis.Name != "." {
		template.WriteString(fmt.Sprintf("      # domain: %s.mycompany.com\n", analysis.Name))
	} else {
		template.WriteString("      # domain: myapp.mycompany.com\n")
	}
	template.WriteString("      \n")
	template.WriteString("      # Scaling configuration\n")
	template.WriteString("      scale:\n")
	template.WriteString("        min: 1\n")
	template.WriteString("        max: 5\n")

	if len(envVars) > 0 {
		template.WriteString("      \n")
		template.WriteString("      # Environment variables\n")
		template.WriteString("      env:\n")
		for key, value := range envVars {
			template.WriteString(fmt.Sprintf("        %s: %s\n", key, value))
		}
	}

	if len(secrets) > 0 {
		template.WriteString("      \n")
		template.WriteString("      # Secrets\n")
		template.WriteString("      secrets:\n")
		for key, value := range secrets {
			template.WriteString(fmt.Sprintf("        %s: \"%s\"\n", key, value))
		}
	}
}

// buildLanguageSpecificEnvVars creates environment variables based on detected language/framework
func (d *DeveloperMode) buildLanguageSpecificEnvVars(analysis *analysis.ProjectAnalysis) map[string]string {
	envVars := make(map[string]string)

	if analysis == nil || analysis.PrimaryStack == nil {
		// Default environment variables
		envVars["PORT"] = "3000"
		return envVars
	}

	switch analysis.PrimaryStack.Language {
	case "javascript", "nodejs":
		envVars["NODE_ENV"] = "production"
		envVars["PORT"] = "3000"
		if analysis.PrimaryStack.Framework == "nextjs" {
			envVars["NEXTAUTH_URL"] = "https://myapp.com" // Non-sensitive URL
		}
	case "python":
		envVars["PYTHON_ENV"] = "production"
		envVars["PORT"] = "8000"
		if analysis.PrimaryStack.Framework == "django" {
			envVars["DJANGO_SETTINGS_MODULE"] = "myapp.settings.production"
		} else if analysis.PrimaryStack.Framework == "flask" {
			envVars["FLASK_ENV"] = "production"
		} else if analysis.PrimaryStack.Framework == "fastapi" {
			envVars["FASTAPI_ENV"] = "production"
		}
	case "go":
		envVars["GO_ENV"] = "production"
		envVars["PORT"] = "8080"
		if analysis.PrimaryStack.Framework == "gin" {
			envVars["GIN_MODE"] = "release"
		}
	default:
		envVars["PORT"] = "3000"
	}

	return envVars
}

// buildLanguageSpecificSecrets creates secrets based on detected language/framework
func (d *DeveloperMode) buildLanguageSpecificSecrets(analysis *analysis.ProjectAnalysis) map[string]string {
	secrets := make(map[string]string)

	// Common secrets for all applications
	secrets["JWT_SECRET"] = "${secret:jwt-secret}"

	// Database connection strings from consumed resources
	secrets["DATABASE_URL"] = "${resource:postgres-db.url}"
	secrets["REDIS_URL"] = "${resource:redis-cache.url}"

	// Common API keys and tokens
	secrets["API_KEY"] = "${secret:api-key}"

	if analysis == nil || analysis.PrimaryStack == nil {
		return secrets
	}

	switch analysis.PrimaryStack.Language {
	case "javascript", "nodejs":
		secrets["SESSION_SECRET"] = "${secret:session-secret}"
		if analysis.PrimaryStack.Framework == "nextjs" {
			secrets["NEXTAUTH_SECRET"] = "${secret:nextauth-secret}"
		} else if analysis.PrimaryStack.Framework == "nestjs" {
			secrets["NEST_JWT_SECRET"] = "${secret:nest-jwt-secret}"
		}
		// MongoDB connection from consumed resource
		secrets["MONGODB_URI"] = "${resource:mongo-db.uri}"
	case "python":
		if analysis.PrimaryStack.Framework == "django" {
			secrets["DJANGO_SECRET_KEY"] = "${secret:django-secret}"
			secrets["DATABASE_URL"] = "${resource:postgres-db.url}" // Keep consistent with resource consumption
		} else if analysis.PrimaryStack.Framework == "flask" {
			secrets["FLASK_SECRET_KEY"] = "${secret:flask-secret}"
		} else if analysis.PrimaryStack.Framework == "fastapi" {
			secrets["SECRET_KEY"] = "${secret:fastapi-secret}"
		}
	case "go":
		secrets["API_SECRET"] = "${secret:api-secret}"
		secrets["JWT_SIGNING_KEY"] = "${secret:jwt-signing-key}"
	}

	return secrets
}

func (d *DeveloperMode) GenerateComposeYAMLWithLLM(analysis *analysis.ProjectAnalysis) (string, error) {
	if d.llm == nil {
		return d.generateFallbackComposeYAML(analysis)
	}

	prompt := d.buildComposeYAMLPrompt(analysis)

	response, err := d.llm.Chat(context.Background(), []llm.Message{
		{Role: "system", Content: "You are an expert in Docker Compose configuration. You MUST respond with ONLY valid YAML content - no explanations, no markdown blocks, no additional text. Return only the raw docker-compose.yaml file content that can be directly written to a file."},
		{Role: "user", Content: prompt},
	})
	if err != nil {
		fmt.Printf("LLM generation failed, using fallback: %v\n", err)
		return d.generateFallbackComposeYAML(analysis)
	}

	// Extract YAML from response with more robust parsing
	yamlContent := strings.TrimSpace(response.Content)

	// Remove any markdown code blocks
	yamlContent = strings.TrimPrefix(yamlContent, "```yaml")
	yamlContent = strings.TrimPrefix(yamlContent, "```yml")
	yamlContent = strings.TrimPrefix(yamlContent, "```")
	yamlContent = strings.TrimSuffix(yamlContent, "```")

	// If LLM still includes explanations, try to extract just the YAML part
	lines := strings.Split(yamlContent, "\n")
	var yamlLines []string
	foundVersion := false

	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)

		// Start collecting from the line that starts with "version:"
		if strings.HasPrefix(trimmedLine, "version:") {
			foundVersion = true
		}

		// If we found the version line, collect all lines until we hit explanatory text
		if foundVersion {
			// Stop if we encounter typical explanatory text patterns
			if strings.Contains(strings.ToLower(trimmedLine), "this file") ||
				strings.Contains(strings.ToLower(trimmedLine), "defines") ||
				strings.Contains(strings.ToLower(trimmedLine), "service named") ||
				(strings.Contains(trimmedLine, "```") && len(strings.TrimSpace(strings.ReplaceAll(trimmedLine, "`", ""))) == 0) {
				break
			}
			yamlLines = append(yamlLines, line)
		}
	}

	// If we found version-based content, use that; otherwise use the original
	if foundVersion && len(yamlLines) > 0 {
		yamlContent = strings.Join(yamlLines, "\n")
	}

	yamlContent = strings.TrimSpace(yamlContent)

	// Validate docker-compose content for best practices
	if !d.validateComposeContent(yamlContent) {
		fmt.Printf("⚠️  Generated docker-compose.yaml doesn't meet best practices, using fallback...\n")
		return d.generateFallbackComposeYAML(analysis)
	}

	return yamlContent, nil
}

func (d *DeveloperMode) buildComposeYAMLPrompt(analysis *analysis.ProjectAnalysis) string {
	var prompt strings.Builder

	prompt.WriteString("Generate a ${project:root}/docker-compose.yaml file for local development with these requirements:\n\n")

	if analysis != nil && analysis.PrimaryStack != nil {
		prompt.WriteString(fmt.Sprintf("Technology: %s", analysis.PrimaryStack.Language))
		if analysis.PrimaryStack.Framework != "" {
			prompt.WriteString(fmt.Sprintf(" with %s framework", analysis.PrimaryStack.Framework))
		}
		prompt.WriteString("\n")

		// Add language-specific recommendations
		switch analysis.PrimaryStack.Language {
		case "javascript":
			prompt.WriteString("- Use Node.js base setup with npm/yarn\n")
			prompt.WriteString("- Include volume mounts for hot reloading\n")
			prompt.WriteString("- Default port 3000\n")
		case "python":
			prompt.WriteString("- Use Python setup with pip requirements\n")
			prompt.WriteString("- Include volume mounts for development\n")
			prompt.WriteString("- Default port 8000\n")
		case "go":
			prompt.WriteString("- Use Go build setup\n")
			prompt.WriteString("- Include volume mounts for development\n")
			prompt.WriteString("- Default port 8080\n")
		}
	}

	// Add detected resources for docker-compose generation
	if analysis != nil && analysis.Resources != nil {
		prompt.WriteString("\n🎯 DETECTED PROJECT RESOURCES (MUST include as services):\n")

		// Databases - add as services
		if len(analysis.Resources.Databases) > 0 {
			prompt.WriteString("Databases to add as services:\n")
			for _, db := range analysis.Resources.Databases {
				switch strings.ToLower(db.Type) {
				case "mongodb":
					prompt.WriteString("  • MongoDB service (use mongo:latest image)\n")
				case "redis":
					prompt.WriteString("  • Redis service (use redis:latest image)\n")
				case "postgresql", "postgres":
					prompt.WriteString("  • PostgreSQL service (use postgres:latest image)\n")
				case "mysql":
					prompt.WriteString("  • MySQL service (use mysql:latest image)\n")
				default:
					prompt.WriteString(fmt.Sprintf("  • %s service (find appropriate Docker image)\n", strings.ToUpper(db.Type)))
				}
			}
		}

		// Storage systems - add volumes or services if needed
		if len(analysis.Resources.Storage) > 0 {
			prompt.WriteString("Storage systems detected:\n")
			for _, storage := range analysis.Resources.Storage {
				switch strings.ToLower(storage.Type) {
				case "s3":
					prompt.WriteString("  • S3 storage (use environment variables for AWS credentials)\n")
				case "gcs":
					prompt.WriteString("  • Google Cloud Storage (use environment variables for GCS credentials)\n")
				case "azure":
					prompt.WriteString("  • Azure Blob Storage (use environment variables for Azure credentials)\n")
				default:
					prompt.WriteString(fmt.Sprintf("  • %s storage (configure via environment variables)\n", strings.ToUpper(storage.Type)))
				}
			}
		}

		// Queue systems - add as services
		if len(analysis.Resources.Queues) > 0 {
			prompt.WriteString("Queue systems to add as services:\n")
			for _, queue := range analysis.Resources.Queues {
				switch strings.ToLower(queue.Type) {
				case "rabbitmq":
					prompt.WriteString("  • RabbitMQ service (use rabbitmq:management image)\n")
				case "kafka":
					prompt.WriteString("  • Apache Kafka service (use confluentinc/cp-kafka image)\n")
				case "redis":
					prompt.WriteString("  • Redis Pub/Sub (same as Redis database service)\n")
				default:
					prompt.WriteString(fmt.Sprintf("  • %s queue service\n", strings.ToUpper(queue.Type)))
				}
			}
		}

		// Environment variables
		if len(analysis.Resources.EnvironmentVars) > 0 {
			prompt.WriteString(fmt.Sprintf("Environment variables: %d detected - configure these in your app service\n", len(analysis.Resources.EnvironmentVars)))
		}
	}

	// Enrich context with relevant documentation
	contextEnrichment := d.enrichContextWithDocumentation("docker-compose", analysis)
	if contextEnrichment != "" {
		prompt.WriteString("\n" + contextEnrichment)
	}

	prompt.WriteString("\n🏷️ REQUIRED SIMPLE CONTAINER LABELS:\n")
	prompt.WriteString("For the MAIN SERVICE (ingress container):\n")
	prompt.WriteString("  labels:\n")
	prompt.WriteString("    \"simple-container.com/ingress\": \"true\"  # Marks this as the main ingress service\n")
	prompt.WriteString("    \"simple-container.com/ingress/port\": \"3000\"  # Optional: specify ingress port\n")
	prompt.WriteString("    \"simple-container.com/healthcheck/path\": \"/health\"  # Optional: health check endpoint\n")
	prompt.WriteString("    \"simple-container.com/healthcheck/port\": \"3000\"  # Optional: health check port\n")

	prompt.WriteString("\nFor VOLUMES (create separate volumes block):\n")
	prompt.WriteString("  volumes:\n")
	prompt.WriteString("    app_data:  # Example volume name\n")
	prompt.WriteString("      labels:\n")
	prompt.WriteString("        \"simple-container.com/volume-size\": \"10Gi\"  # Volume size specification\n")
	prompt.WriteString("        \"simple-container.com/volume-storage-class\": \"gp3\"  # Optional: storage class\n")
	prompt.WriteString("        \"simple-container.com/volume-access-modes\": \"ReadWriteOnce\"  # Optional: access mode\n")

	prompt.WriteString("\n📋 REQUIRED STRUCTURE:\n")
	prompt.WriteString("- Use version: '3.8' or higher\n")
	prompt.WriteString("- Main 'app' service with build context and Simple Container labels\n")
	prompt.WriteString("- Create separate volumes block for ALL required volumes with labels\n")
	prompt.WriteString("- Proper port mapping with ingress labels\n")
	prompt.WriteString("- Environment variables for configuration\n")
	prompt.WriteString("- Volume mounts using the defined volumes\n")
	prompt.WriteString("- Restart policies (restart: unless-stopped recommended)\n")
	prompt.WriteString("- Include networks block if multiple services need communication\n")

	prompt.WriteString("\n🚨 CRITICAL: Return ONLY the docker-compose.yaml file content below.\n")
	prompt.WriteString("❌ DO NOT include any explanations, descriptions, or comments about the file\n")
	prompt.WriteString("❌ DO NOT wrap the content in markdown code blocks (```yaml```)\n")
	prompt.WriteString("❌ DO NOT add any text before or after the YAML content\n")
	prompt.WriteString("✅ Return ONLY the raw YAML that starts with 'version:' and can be directly written to docker-compose.yaml\n\n")

	return prompt.String()
}

// Helper functions for resource detection

// contains checks if a slice contains a string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// formatYAMLArray formats a string slice as a YAML array
func formatYAMLArray(items []string) string {
	if len(items) == 0 {
		return "[]"
	}
	if len(items) == 1 {
		return "[" + items[0] + "]"
	}

	var result strings.Builder
	result.WriteString("[")
	for i, item := range items {
		if i > 0 {
			result.WriteString(", ")
		}
		result.WriteString(item)
	}
	result.WriteString("]")
	return result.String()
}

func (d *DeveloperMode) generateFallbackComposeYAML(analysis *analysis.ProjectAnalysis) (string, error) {
	// Build language-specific template
	port := "3000"
	if analysis != nil && analysis.PrimaryStack != nil {
		switch analysis.PrimaryStack.Language {
		case "python":
			port = "8000"
		case "go":
			port = "8080"
		default:
			port = "3000"
		}
	}

	template := fmt.Sprintf(`version: '3.8'

services:
  app:
    build: .
    labels:
      "simple-container.com/ingress": "true"
      "simple-container.com/ingress/port": "%s"
      "simple-container.com/healthcheck/path": "/health"
      "simple-container.com/healthcheck/port": "%s"
    ports:
      - "%s:%s"
    environment:
      - NODE_ENV=development
      - PORT=%s
    volumes:
      - .:/app:delegated
      - app_data:/data
    restart: unless-stopped
    networks:
      - app_network

volumes:
  app_data:
    labels:
      "simple-container.com/volume-size": "10Gi"
      "simple-container.com/volume-storage-class": "gp3"
      "simple-container.com/volume-access-modes": "ReadWriteOnce"

networks:
  app_network:
    driver: bridge`, port, port, port, port, port)

	return template, nil
}

func (d *DeveloperMode) GenerateDockerfileWithLLM(analysis *analysis.ProjectAnalysis) (string, error) {
	if d.llm == nil {
		return d.generateFallbackDockerfile(analysis)
	}

	prompt := d.buildDockerfilePrompt(analysis)

	response, err := d.llm.Chat(context.Background(), []llm.Message{
		{Role: "system", Content: "You are an expert in Docker containerization. You MUST respond with ONLY valid Dockerfile content - no explanations, no markdown blocks, no additional text. Return only the raw Dockerfile content that can be directly written to a Dockerfile."},
		{Role: "user", Content: prompt},
	})
	if err != nil {
		fmt.Printf("LLM generation failed, using fallback: %v\n", err)
		return d.generateFallbackDockerfile(analysis)
	}

	// Extract Dockerfile content from response with more robust parsing
	dockerfileContent := strings.TrimSpace(response.Content)

	// Remove any markdown code blocks
	dockerfileContent = strings.TrimPrefix(dockerfileContent, "```dockerfile")
	dockerfileContent = strings.TrimPrefix(dockerfileContent, "```")
	dockerfileContent = strings.TrimSuffix(dockerfileContent, "```")

	// If LLM still includes explanations, try to extract just the Dockerfile part
	lines := strings.Split(dockerfileContent, "\n")
	var dockerfileLines []string
	foundFrom := false

	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)

		// Start collecting from the line that starts with "FROM"
		if strings.HasPrefix(strings.ToUpper(trimmedLine), "FROM ") {
			foundFrom = true
		}

		// If we found the FROM line, collect all lines until we hit explanatory text
		if foundFrom {
			// Stop if we encounter typical explanatory text patterns
			if strings.Contains(strings.ToLower(trimmedLine), "this dockerfile") ||
				strings.Contains(strings.ToLower(trimmedLine), "creates") ||
				strings.Contains(strings.ToLower(trimmedLine), "builds") ||
				(strings.Contains(trimmedLine, "```") && len(strings.TrimSpace(strings.ReplaceAll(trimmedLine, "`", ""))) == 0) {
				break
			}
			dockerfileLines = append(dockerfileLines, line)
		}
	}

	// If we found FROM-based content, use that; otherwise use the original
	if foundFrom && len(dockerfileLines) > 0 {
		dockerfileContent = strings.Join(dockerfileLines, "\n")
	}

	dockerfileContent = strings.TrimSpace(dockerfileContent)

	// Validate Dockerfile content for best practices
	if !d.validateDockerfileContent(dockerfileContent) {
		fmt.Printf("⚠️  Generated Dockerfile doesn't meet security standards, using fallback...\n")
		return d.generateFallbackDockerfile(analysis)
	}

	return dockerfileContent, nil
}

func (d *DeveloperMode) buildDockerfilePrompt(analysis *analysis.ProjectAnalysis) string {
	var prompt strings.Builder

	prompt.WriteString("Generate an optimized, production-ready Dockerfile with these requirements:\n\n")

	if analysis != nil && analysis.PrimaryStack != nil {
		prompt.WriteString(fmt.Sprintf("Technology: %s", analysis.PrimaryStack.Language))
		if analysis.PrimaryStack.Framework != "" {
			prompt.WriteString(fmt.Sprintf(" with %s framework", analysis.PrimaryStack.Framework))
		}
		prompt.WriteString("\n")

		// Add language-specific requirements
		switch analysis.PrimaryStack.Language {
		case "javascript":
			prompt.WriteString("- Use Node.js 18-alpine or newer\n")
			prompt.WriteString("- Multi-stage build with dependencies\n")
			prompt.WriteString("- npm ci for production dependencies\n")
			prompt.WriteString("- Non-root user for security\n")
		case "python":
			prompt.WriteString("- Use Python 3.11-slim\n")
			prompt.WriteString("- Multi-stage build pattern\n")
			prompt.WriteString("- pip install with requirements.txt\n")
			prompt.WriteString("- Non-root user for security\n")
		case "go":
			prompt.WriteString("- Multi-stage build with golang:alpine builder\n")
			prompt.WriteString("- Final stage with scratch or alpine\n")
			prompt.WriteString("- Static binary compilation\n")
			prompt.WriteString("- Minimal attack surface\n")
		}
	}

	// Enrich context with relevant documentation
	contextEnrichment := d.enrichContextWithDocumentation("dockerfile", analysis)
	if contextEnrichment != "" {
		prompt.WriteString("\n" + contextEnrichment)
	}

	prompt.WriteString("\nRequired features:\n")
	prompt.WriteString("- Multi-stage build for optimization\n")
	prompt.WriteString("- Security best practices (non-root user)\n")
	prompt.WriteString("- Proper layer caching\n")
	prompt.WriteString("- Health checks\n")
	prompt.WriteString("- Appropriate EXPOSE directive\n")
	prompt.WriteString("- Optimized for container registry\n")

	prompt.WriteString("\n🚨 CRITICAL: Return ONLY the Dockerfile content below.\n")
	prompt.WriteString("❌ DO NOT include any explanations, descriptions, or comments about the file\n")
	prompt.WriteString("❌ DO NOT wrap the content in markdown code blocks (```dockerfile```)\n")
	prompt.WriteString("❌ DO NOT add any text before or after the Dockerfile content\n")
	prompt.WriteString("✅ Return ONLY the raw Dockerfile that starts with 'FROM' and can be directly written to Dockerfile\n\n")

	return prompt.String()
}

func (d *DeveloperMode) generateFallbackDockerfile(analysis *analysis.ProjectAnalysis) (string, error) {
	template := `FROM node:18-alpine

WORKDIR /app

COPY package*.json ./
RUN npm ci --only=production

COPY . .

EXPOSE 3000

CMD ["npm", "start"]`

	return template, nil
}

func (d *DeveloperMode) printSetupSummary(opts *SetupOptions, analysis *analysis.ProjectAnalysis) {
	// Determine project name for the deploy command
	projectName := filepath.Base(".")
	if analysis != nil && analysis.Name != "" && analysis.Name != "." {
		projectName = analysis.Name
	}

	// Ensure we have a valid project name
	if projectName == "." || projectName == "" {
		// Use the current directory name as fallback
		if wd, err := os.Getwd(); err == nil {
			projectName = filepath.Base(wd)
		} else {
			projectName = "myapp"
		}
	}

	fmt.Println("\n📁 Generated files:")
	fmt.Printf("   • client.yaml                    - Simple Container configuration\n")
	if !opts.SkipCompose {
		fmt.Printf("   • ${project:root}/docker-compose.yaml  - Local development environment\n")
	}
	if !opts.SkipDockerfile {
		fmt.Printf("   • ${project:root}/Dockerfile            - Container image definition\n")
	}

	fmt.Println("\n🚀 Next steps:")
	fmt.Printf("   1. Start local development: %s\n", color.CyanFmt("docker-compose up -d"))
	fmt.Printf("   2. Deploy to %s:       %s\n", opts.Environment, color.CyanFmt(fmt.Sprintf("sc deploy -s %s -e %s", projectName, opts.Environment)))

	if analysis != nil && len(analysis.Recommendations) > 0 {
		fmt.Println("\n💡 Recommendations:")
		for i, rec := range analysis.Recommendations {
			if i >= 3 { // Show only first 3 recommendations
				break
			}
			fmt.Printf("   • %s\n", rec.Description)
		}
	}
}

// Output methods for different formats

func (d *DeveloperMode) outputAnalysisTable(analysis *analysis.ProjectAnalysis, detailed bool) {
	// Basic analysis output
	fmt.Println("📊 Technology Stack:")
	if analysis.PrimaryStack != nil {
		fmt.Printf("   Language:     %s\n", color.GreenFmt(analysis.PrimaryStack.Language))
		if analysis.PrimaryStack.Framework != "" {
			fmt.Printf("   Framework:    %s\n", color.GreenFmt(analysis.PrimaryStack.Framework))
		}
		if analysis.PrimaryStack.Version != "" {
			fmt.Printf("   Version:      %s\n", color.GreenFmt(analysis.PrimaryStack.Version))
		}
		fmt.Printf("   Confidence:   %s\n", color.YellowFmt("%.0f%%", analysis.PrimaryStack.Confidence*100))
	}

	if analysis.Architecture != "" {
		fmt.Printf("   Architecture: %s\n", color.GreenFmt(analysis.Architecture))
	}

	// Detailed output
	if detailed {
		if len(analysis.TechStacks) > 1 {
			fmt.Println("\n📋 All Detected Stacks:")
			for i, stack := range analysis.TechStacks {
				fmt.Printf("   %d. %s", i+1, stack.Language)
				if stack.Framework != "" {
					fmt.Printf(" (%s)", stack.Framework)
				}
				fmt.Printf(" - %.0f%% confidence\n", stack.Confidence*100)
			}
		}

		if analysis.PrimaryStack != nil && len(analysis.PrimaryStack.Dependencies) > 0 {
			fmt.Println("\n📦 Dependencies:")
			for _, dep := range analysis.PrimaryStack.Dependencies {
				fmt.Printf("   • %s %s (%s)\n", dep.Name, dep.Version, dep.Type)
			}
		}

		if len(analysis.Files) > 0 {
			fmt.Printf("\n📄 Project Files (%d analyzed):\n", len(analysis.Files))
			languageCounts := make(map[string]int)
			for _, file := range analysis.Files {
				if file.Language != "" {
					languageCounts[file.Language]++
				}
			}
			for lang, count := range languageCounts {
				fmt.Printf("   • %s: %d files\n", lang, count)
			}
		}
	}

	if len(analysis.Recommendations) > 0 {
		fmt.Println("\n🎯 Recommendations:")
		for _, rec := range analysis.Recommendations {
			priority := rec.Priority
			switch rec.Priority {
			case "high":
				priority = color.RedFmt("HIGH")
			case "medium":
				priority = color.YellowFmt("MED")
			case "low":
				priority = color.GrayFmt("LOW")
			}
			fmt.Printf("   • [%s] %s\n", priority, rec.Title)
			if detailed {
				fmt.Printf("     %s\n", rec.Description)
			}
		}
	}
}

func (d *DeveloperMode) outputAnalysisJSON(analysis *analysis.ProjectAnalysis, outputFile string) error {
	jsonData, err := json.MarshalIndent(analysis, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal analysis to JSON: %w", err)
	}

	if outputFile != "" {
		if err := os.WriteFile(outputFile, jsonData, 0o644); err != nil {
			return fmt.Errorf("failed to write JSON to file %s: %w", outputFile, err)
		}
		fmt.Printf("✅ Analysis exported to %s\n", color.GreenFmt(outputFile))
	} else {
		fmt.Println(string(jsonData))
	}

	return nil
}

func (d *DeveloperMode) outputAnalysisYAML(analysis *analysis.ProjectAnalysis, outputFile string) error {
	yamlData, err := yaml.Marshal(analysis)
	if err != nil {
		return fmt.Errorf("failed to marshal analysis to YAML: %w", err)
	}

	if outputFile != "" {
		if err := os.WriteFile(outputFile, yamlData, 0o644); err != nil {
			return fmt.Errorf("failed to write YAML to file %s: %w", outputFile, err)
		}
		fmt.Printf("✅ Analysis exported to %s\n", color.GreenFmt(outputFile))
	} else {
		fmt.Print(string(yamlData))
	}

	return nil
}

// validateDockerfileContent checks Dockerfile for security best practices
func (d *DeveloperMode) validateDockerfileContent(content string) bool {
	lines := strings.Split(content, "\n")

	var hasNonRootUser bool
	var hasMultiStage bool
	var hasSecurityPractices bool

	for _, line := range lines {
		line = strings.TrimSpace(line)
		lineLower := strings.ToLower(line)

		// Check for non-root user
		if strings.HasPrefix(lineLower, "user ") && !strings.Contains(lineLower, "user 0") && !strings.Contains(lineLower, "user root") {
			hasNonRootUser = true
		}

		// Check for multi-stage build
		if strings.HasPrefix(lineLower, "from ") && strings.Contains(lineLower, " as ") {
			hasMultiStage = true
		}

		// Check for security practices
		if strings.Contains(lineLower, "apk add --no-cache") ||
			strings.Contains(lineLower, "apt-get update") ||
			strings.Contains(lineLower, "npm ci") ||
			strings.Contains(lineLower, "pip install --no-cache-dir") {
			hasSecurityPractices = true
		}
	}

	// Basic validation - at least one security practice should be present
	return hasNonRootUser || hasMultiStage || hasSecurityPractices
}

// validateComposeContent checks docker-compose.yaml for best practices and Simple Container labels
func (d *DeveloperMode) validateComposeContent(content string) bool {
	// Basic validation - check for required sections
	requiredSections := []string{
		"version:",
		"services:",
	}

	for _, section := range requiredSections {
		if !strings.Contains(content, section) {
			return false
		}
	}

	// Check for Simple Container ingress label (critical for deployment)
	hasIngressLabel := strings.Contains(content, "simple-container.com/ingress")

	// Check for security and operational practices
	hasSecurityPractices := false
	if strings.Contains(content, "restart:") ||
		strings.Contains(content, "healthcheck:") ||
		strings.Contains(content, "environment:") {
		hasSecurityPractices = true
	}

	// Pass validation if it has ingress label and basic security practices
	// Volume labels are optional but recommended
	return hasIngressLabel && hasSecurityPractices
}

// getConfiguredAPIKey retrieves the OpenAI API key from configuration system first, then environment
func getConfiguredAPIKey() string {
	// Try to load from configuration system first
	cfg, err := config.Load()
	if err == nil {
		if providerCfg, exists := cfg.GetProviderConfig(config.ProviderOpenAI); exists && providerCfg.APIKey != "" {
			return providerCfg.APIKey
		}
	}

	// Fallback to environment variable
	return os.Getenv("OPENAI_API_KEY")
}
