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
	"github.com/simple-container-com/api/pkg/assistant/embeddings"
	"github.com/simple-container-com/api/pkg/assistant/llm"
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
	_ = provider.Configure(llm.Config{
		Provider:    "openai",
		MaxTokens:   2048,
		Temperature: 0.7,
		APIKey:      os.Getenv("OPENAI_API_KEY"),
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
	GenerateAll    bool // Generate all files in one coordinated operation
	UseStreaming   bool // Use streaming LLM responses for better UX
	BackupExisting bool // Backup existing files before overwriting
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

	// Step 3: Generate Configuration Files
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

	// Perform analysis
	analysis, err := d.analyzer.AnalyzeProject(projectPath)
	if err != nil {
		return fmt.Errorf("analysis failed: %w", err)
	}

	// Display results based on format
	switch opts.Format {
	case "json":
		return d.outputAnalysisJSON(analysis, opts.Output)
	case "yaml":
		return d.outputAnalysisYAML(analysis, opts.Output)
	default:
		d.outputAnalysisTable(analysis, opts.Detailed)
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
			// Default to cloud-compose (already the default in templates)
			break
		}
		if input == "2" {
			// Note: We'd need to modify the template generation to support this
			fmt.Printf("   %s Static deployment selected\n", color.GreenFmt("✓"))
			break
		}
		if input == "3" {
			// Note: We'd need to modify the template generation to support this
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
	// Create .sc directory structure
	scDir := filepath.Join(projectPath, ".sc", "stacks", filepath.Base(projectPath))
	if err := os.MkdirAll(scDir, 0o755); err != nil {
		return fmt.Errorf("failed to create .sc directory: %w", err)
	}

	// Generate client.yaml using LLM
	if !opts.SkipAnalysis {
		fmt.Printf("   📄 Generating client.yaml...")
		clientYaml, err := d.GenerateClientYAMLWithLLM(opts, analysis)
		if err != nil {
			return fmt.Errorf("failed to generate client.yaml: %w", err)
		}
		clientPath := filepath.Join(scDir, "client.yaml")
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
			fmt.Printf(" %s (already exists)\n", color.YellowFmt("⚠"))
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
			fmt.Printf(" %s (already exists)\n", color.YellowFmt("⚠"))
		}
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
		BackupExisting:    opts.BackupExisting,
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
	if analysis != nil {
		projectName = analysis.Name
	}

	prompt := d.buildClientYAMLPrompt(opts, analysis, projectName)

	response, err := d.llm.Chat(context.Background(), []llm.Message{
		{Role: "system", Content: `You are an expert in Simple Container configuration. Generate ONLY valid YAML that EXACTLY follows the provided JSON schemas.

CRITICAL INSTRUCTIONS:
✅ Follow the JSON schemas EXACTLY - every property must match the schema structure
✅ Use ONLY properties defined in the schemas - no fictional or made-up properties
✅ client.yaml MUST have: schemaVersion, stacks section
✅ Each stack MUST have: type, parent, parentEnv, config
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
	if analysis != nil {
		projectName = analysis.Name
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
	prompt.WriteString("  " + projectName + ":\n")
	prompt.WriteString("    type: cloud-compose       # Valid types: cloud-compose, static, single-image\n")
	prompt.WriteString("    parent: " + opts.Parent + "\n")
	prompt.WriteString("    parentEnv: " + opts.Environment + "\n")
	prompt.WriteString("    config:\n")
	prompt.WriteString("      runs: [app]            # Container names from docker-compose.yaml\n")
	prompt.WriteString("      scale:\n")
	prompt.WriteString("        min: 1              # Must be in config section, NOT separate scaling block\n")
	prompt.WriteString("        max: 3\n")
	prompt.WriteString("      env:                  # Environment variables (NOT 'environment')\n")
	prompt.WriteString("        PORT: 3000\n")
	prompt.WriteString("      secrets:              # Secret references using ${secret:name} format\n")
	prompt.WriteString("        JWT_SECRET: \"${secret:jwt-secret}\"\n")

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

func (d *DeveloperMode) generateFallbackClientYAML(opts *SetupOptions, analysis *analysis.ProjectAnalysis) (string, error) {
	projectName := filepath.Base(".")
	if analysis != nil {
		projectName = analysis.Name
	}

	// Build language-specific environment variables based on project analysis
	envVars := d.buildLanguageSpecificEnvVars(analysis)
	secrets := d.buildLanguageSpecificSecrets(analysis)

	var template strings.Builder
	template.WriteString("schemaVersion: 1.0\n\n")
	template.WriteString("stacks:\n")
	template.WriteString(fmt.Sprintf("  %s:\n", projectName))
	template.WriteString("    type: cloud-compose\n")
	template.WriteString(fmt.Sprintf("    parent: %s\n", opts.Parent))
	template.WriteString(fmt.Sprintf("    parentEnv: %s\n", opts.Environment))
	template.WriteString("    config:\n")
	template.WriteString("      # Services from docker-compose.yaml\n")
	template.WriteString("      runs: [app]\n")
	template.WriteString("      \n")
	template.WriteString("      # Scaling configuration\n")
	template.WriteString("      scale:\n")
	template.WriteString("        min: 1\n")
	template.WriteString("        max: 3\n")
	template.WriteString("      \n")
	template.WriteString("      # Environment variables\n")
	template.WriteString("      env:\n")

	// Add language-specific environment variables
	for key, value := range envVars {
		template.WriteString(fmt.Sprintf("        %s: %s\n", key, value))
	}

	template.WriteString("        \n")
	template.WriteString("      # Secrets\n")
	template.WriteString("      secrets:\n")

	// Add language-specific secrets
	for key, value := range secrets {
		template.WriteString(fmt.Sprintf("        %s: \"%s\"\n", key, value))
	}

	return template.String(), nil
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
		if analysis.PrimaryStack.Framework == "express" {
			envVars["EXPRESS_SESSION_SECRET"] = "${secret:session-secret}"
		} else if analysis.PrimaryStack.Framework == "nextjs" {
			envVars["NEXTAUTH_URL"] = "https://myapp.com"
			envVars["NEXTAUTH_SECRET"] = "${secret:nextauth-secret}"
		}
	case "python":
		envVars["PYTHON_ENV"] = "production"
		envVars["PORT"] = "8000"
		if analysis.PrimaryStack.Framework == "django" {
			envVars["DJANGO_SETTINGS_MODULE"] = "myapp.settings.production"
			envVars["DJANGO_SECRET_KEY"] = "${secret:django-secret}"
		} else if analysis.PrimaryStack.Framework == "flask" {
			envVars["FLASK_ENV"] = "production"
			envVars["FLASK_SECRET_KEY"] = "${secret:flask-secret}"
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

	if analysis == nil || analysis.PrimaryStack == nil {
		return secrets
	}

	switch analysis.PrimaryStack.Language {
	case "javascript", "nodejs":
		if analysis.PrimaryStack.Framework == "nextjs" {
			secrets["NEXTAUTH_SECRET"] = "${secret:nextauth-secret}"
		}
		secrets["SESSION_SECRET"] = "${secret:session-secret}"
	case "python":
		if analysis.PrimaryStack.Framework == "django" {
			secrets["DJANGO_SECRET_KEY"] = "${secret:django-secret}"
		} else if analysis.PrimaryStack.Framework == "flask" {
			secrets["FLASK_SECRET_KEY"] = "${secret:flask-secret}"
		}
	case "go":
		secrets["API_SECRET"] = "${secret:api-secret}"
	}

	return secrets
}

func (d *DeveloperMode) GenerateComposeYAMLWithLLM(analysis *analysis.ProjectAnalysis) (string, error) {
	if d.llm == nil {
		return d.generateFallbackComposeYAML(analysis)
	}

	prompt := d.buildComposeYAMLPrompt(analysis)

	response, err := d.llm.Chat(context.Background(), []llm.Message{
		{Role: "system", Content: "You are an expert in Docker Compose configuration. Generate production-ready docker-compose.yaml files for local development."},
		{Role: "user", Content: prompt},
	})
	if err != nil {
		fmt.Printf("LLM generation failed, using fallback: %v\n", err)
		return d.generateFallbackComposeYAML(analysis)
	}

	// Extract YAML from response
	yamlContent := strings.TrimSpace(response.Content)
	yamlContent = strings.TrimPrefix(yamlContent, "```yaml")
	yamlContent = strings.TrimPrefix(yamlContent, "```yml")
	yamlContent = strings.TrimPrefix(yamlContent, "```")
	yamlContent = strings.TrimSuffix(yamlContent, "```")
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

	prompt.WriteString("Generate a docker-compose.yaml file for local development with these requirements:\n\n")

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

	prompt.WriteString("\n⚡ Generate ONLY the valid docker-compose.yaml content (no explanations, no markdown blocks):")

	return prompt.String()
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
		{Role: "system", Content: "You are an expert in Docker containerization. Generate production-ready, multi-stage Dockerfiles optimized for security and performance."},
		{Role: "user", Content: prompt},
	})
	if err != nil {
		fmt.Printf("LLM generation failed, using fallback: %v\n", err)
		return d.generateFallbackDockerfile(analysis)
	}

	// Extract Dockerfile content from response
	dockerfileContent := strings.TrimSpace(response.Content)
	dockerfileContent = strings.TrimPrefix(dockerfileContent, "```dockerfile")
	dockerfileContent = strings.TrimPrefix(dockerfileContent, "```")
	dockerfileContent = strings.TrimSuffix(dockerfileContent, "```")
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

	prompt.WriteString("\nGenerate only the Dockerfile content without explanations:")

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
	fmt.Println("\n📁 Generated files:")
	fmt.Printf("   • client.yaml          - Simple Container configuration\n")
	if !opts.SkipCompose {
		fmt.Printf("   • docker-compose.yaml  - Local development environment\n")
	}
	if !opts.SkipDockerfile {
		fmt.Printf("   • Dockerfile           - Container image definition\n")
	}

	fmt.Println("\n🚀 Next steps:")
	fmt.Printf("   1. Start local development: %s\n", color.CyanFmt("docker-compose up -d"))
	fmt.Printf("   2. Deploy to staging:       %s\n", color.CyanFmt("sc deploy -e staging"))

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
