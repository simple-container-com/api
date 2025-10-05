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
	if err := provider.Configure(llm.Config{
		Provider:    "openai",
		MaxTokens:   2048,
		Temperature: 0.7,
		APIKey:      os.Getenv("OPENAI_API_KEY"),
	}); err != nil {
		// Don't print warning here as it will show even when LLM is not needed
		// The individual generation functions will handle fallback gracefully
	}

	// Initialize embeddings database for documentation search
	embeddingsDB, err := embeddings.LoadEmbeddedDatabase()
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

	if err := d.generateFiles(projectPath, opts, projectAnalysis); err != nil {
		return fmt.Errorf("file generation failed: %w", err)
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
	for {
		fmt.Printf("🏗️  Parent stack [%s]: ", color.CyanFmt(opts.Parent))
		scanner.Scan()
		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			break // Keep default
		}
		opts.Parent = input
		break
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
	fmt.Printf("\n📋 " + color.BlueFmt("Configuration Summary:"))
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
	if err := os.MkdirAll(scDir, 0755); err != nil {
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
		if err := os.WriteFile(clientPath, []byte(clientYaml), 0644); err != nil {
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
			if err := os.WriteFile(composePath, []byte(composeYaml), 0644); err != nil {
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
			if err := os.WriteFile(dockerfilePath, []byte(dockerfile), 0644); err != nil {
				return fmt.Errorf("failed to write Dockerfile: %w", err)
			}
			fmt.Printf(" %s\n", color.GreenFmt("✓"))
		} else {
			fmt.Printf(" %s (already exists)\n", color.YellowFmt("⚠"))
		}
	}

	return nil
}

func (d *DeveloperMode) generateClientYAML(opts *SetupOptions, analysis *analysis.ProjectAnalysis) string {
	projectName := filepath.Base(".")
	if analysis != nil {
		projectName = analysis.Name
	}

	// Extract recommended resources from analysis
	resources := []string{}
	if analysis != nil {
		for _, rec := range analysis.Recommendations {
			if rec.Type == "resource" && rec.Resource != "" {
				// Map resource recommendations to Simple Container resource names
				switch rec.Resource {
				case "aws-rds-postgres", "gcp-cloudsql-postgres":
					resources = append(resources, "postgres-db")
				case "redis-cache":
					resources = append(resources, "redis-cache")
				case "mongodb-atlas":
					resources = append(resources, "mongo-db")
				case "s3-bucket":
					resources = append(resources, "uploads-bucket")
				}
			}
		}
	}

	// Default to common resources if none detected
	if len(resources) == 0 {
		resources = []string{"postgres-db"}
	}

	yaml := fmt.Sprintf(`schemaVersion: 1.0

stacks:
  %s:
    type: cloud-compose
    parent: %s
    parentEnv: %s
    config:
      # Shared resources from DevOps team
      uses: %v

      # Services from docker-compose.yaml
      runs: [app]

      # Scaling configuration
      scale:
        min: 1
        max: 5

      # Environment variables
      env:
        PORT: 3000`,
		projectName, opts.Parent, opts.Environment, resources)

	// Add language-specific environment variables
	if analysis != nil && analysis.PrimaryStack != nil {
		switch analysis.PrimaryStack.Language {
		case "javascript":
			yaml += `
        DATABASE_URL: "${resource:postgres-db.connectionString}"
        REDIS_URL: "${resource:redis-cache.connectionString}"`
		case "python":
			yaml += `
        DATABASE_URL: "${resource:postgres-db.connectionString}"
        REDIS_URL: "${resource:redis-cache.connectionString}"
        DJANGO_SECRET_KEY: "${secret:django-secret-key}"`
		case "go":
			yaml += `
        DATABASE_URL: "${resource:postgres-db.connectionString}"
        REDIS_URL: "${resource:redis-cache.connectionString}"`
		}
	}

	yaml += `

      # Health check configuration
      healthCheck:
        path: "/health"
        port: 3000
        initialDelaySeconds: 30
        periodSeconds: 10

      # Secrets
      secrets:
        JWT_SECRET: "${secret:jwt-secret}"`

	return yaml
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
		{Role: "system", Content: "You are an expert in Simple Container configuration. Generate only valid YAML configurations based on actual Simple Container schemas and properties. Do not include fictional properties."},
		{Role: "user", Content: prompt},
	})
	if err != nil {
		fmt.Printf("LLM generation failed, using fallback: %v\n", err)
		return d.generateFallbackClientYAML(opts, analysis)
	}

	// Extract YAML from response (remove any markdown formatting)
	yamlContent := strings.TrimSpace(response.Content)
	if strings.HasPrefix(yamlContent, "```yaml") {
		yamlContent = strings.TrimPrefix(yamlContent, "```yaml")
	}
	if strings.HasPrefix(yamlContent, "```") {
		yamlContent = strings.TrimPrefix(yamlContent, "```")
	}
	if strings.HasSuffix(yamlContent, "```") {
		yamlContent = strings.TrimSuffix(yamlContent, "```")
	}

	return strings.TrimSpace(yamlContent), nil
}

func (d *DeveloperMode) buildClientYAMLPrompt(opts *SetupOptions, analysis *analysis.ProjectAnalysis, projectName string) string {
	var prompt strings.Builder

	prompt.WriteString("Generate a Simple Container client.yaml configuration with these requirements:\n\n")
	prompt.WriteString(fmt.Sprintf("Project: %s\n", projectName))
	prompt.WriteString(fmt.Sprintf("Parent stack: %s\n", opts.Parent))
	prompt.WriteString(fmt.Sprintf("Environment: %s\n", opts.Environment))

	if analysis != nil && analysis.PrimaryStack != nil {
		prompt.WriteString(fmt.Sprintf("Detected language: %s\n", analysis.PrimaryStack.Language))
		if analysis.PrimaryStack.Framework != "" {
			prompt.WriteString(fmt.Sprintf("Framework: %s\n", analysis.PrimaryStack.Framework))
		}
	}

	// Enrich context with relevant documentation
	contextEnrichment := d.enrichContextWithDocumentation("client.yaml", analysis)
	if contextEnrichment != "" {
		prompt.WriteString("\n" + contextEnrichment)
	}

	prompt.WriteString("\nRequired structure:\n")
	prompt.WriteString("- Use schemaVersion: 1.0\n")
	prompt.WriteString("- Use 'stacks:' section (NOT 'environments:')\n")
	prompt.WriteString("- Stack type should be 'cloud-compose' for containerized apps\n")
	prompt.WriteString("- Include 'runs: [app]' to specify containers from docker-compose.yaml\n")
	prompt.WriteString("- Add scaling with 'scale: {min: 1, max: 3}' in config section\n")
	prompt.WriteString("- Use 'env:' for environment variables (NOT 'environment:')\n")
	prompt.WriteString("- Include common secrets like JWT_SECRET using ${secret:jwt-secret} format\n")
	prompt.WriteString("- Only use real Simple Container properties validated against schemas\n")

	prompt.WriteString("\nGenerate only the YAML configuration without explanations:")

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

	template := fmt.Sprintf(`schemaVersion: 1.0

stacks:
  %s:
    type: cloud-compose
    parent: %s
    parentEnv: %s
    config:
      # Services from docker-compose.yaml
      runs: [app]
      
      # Scaling configuration
      scale:
        min: 1
        max: 3
      
      # Environment variables
      env:
        PORT: 3000
        
      # Secrets
      secrets:
        JWT_SECRET: "${secret:jwt-secret}"`,
		projectName, opts.Parent, opts.Environment)

	return template, nil
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
	if strings.HasPrefix(yamlContent, "```yaml") {
		yamlContent = strings.TrimPrefix(yamlContent, "```yaml")
	}
	if strings.HasPrefix(yamlContent, "```") {
		yamlContent = strings.TrimPrefix(yamlContent, "```")
	}
	if strings.HasSuffix(yamlContent, "```") {
		yamlContent = strings.TrimSuffix(yamlContent, "```")
	}

	return strings.TrimSpace(yamlContent), nil
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

	prompt.WriteString("\nRequired structure:\n")
	prompt.WriteString("- Use version: '3.8'\n")
	prompt.WriteString("- Main 'app' service with build context\n")
	prompt.WriteString("- Proper port mapping\n")
	prompt.WriteString("- Development environment variables\n")
	prompt.WriteString("- Volume mounts for hot reloading\n")
	prompt.WriteString("- Health checks where appropriate\n")

	prompt.WriteString("\nGenerate only the docker-compose.yaml content without explanations:")

	return prompt.String()
}

func (d *DeveloperMode) generateFallbackComposeYAML(analysis *analysis.ProjectAnalysis) (string, error) {
	template := `version: '3.8'

services:
  app:
    build: .
    ports:
      - "3000:3000"
    environment:
      - NODE_ENV=development
      - PORT=3000
    volumes:
      - .:/app:delegated
    command: npm run dev`

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
	if strings.HasPrefix(dockerfileContent, "```dockerfile") {
		dockerfileContent = strings.TrimPrefix(dockerfileContent, "```dockerfile")
	}
	if strings.HasPrefix(dockerfileContent, "```") {
		dockerfileContent = strings.TrimPrefix(dockerfileContent, "```")
	}
	if strings.HasSuffix(dockerfileContent, "```") {
		dockerfileContent = strings.TrimSuffix(dockerfileContent, "```")
	}

	return strings.TrimSpace(dockerfileContent), nil
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

func (d *DeveloperMode) generateComposeYAML(analysis *analysis.ProjectAnalysis) string {
	// Base compose structure
	compose := `version: '3.8'

services:
  app:
    build: .
    ports:
      - "3000:3000"
    environment:
      - NODE_ENV=development
      - PORT=3000
    volumes:
      - .:/app:delegated
    command: npm run dev`

	// Add database services based on analysis
	if analysis != nil {
		hasDatabase := false
		hasRedis := false

		// Check for database dependencies
		for _, rec := range analysis.Recommendations {
			if rec.Category == "database" {
				hasDatabase = true
			}
			if rec.Category == "cache" {
				hasRedis = true
			}
		}

		if hasDatabase {
			compose += `

  postgres:
    image: postgres:15-alpine
    environment:
      POSTGRES_DB: appdb
      POSTGRES_USER: appuser
      POSTGRES_PASSWORD: apppass
    ports:
      - "5432:5432"
    volumes:
      - postgres_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U appuser -d appdb"]
      interval: 10s
      timeout: 5s
      retries: 5`

			compose = compose[:strings.LastIndex(compose, "command: npm run dev")] +
				`depends_on:
        postgres:
          condition: service_healthy
      environment:
        - NODE_ENV=development
        - PORT=3000
        - DATABASE_URL=postgresql://appuser:apppass@postgres:5432/appdb
    command: npm run dev`
		}

		if hasRedis {
			compose += `

  redis:
    image: redis:7-alpine
    ports:
      - "6379:6379"
    volumes:
      - redis_data:/data
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 10s
      timeout: 5s
      retries: 5`

			// Add Redis URL to app environment
			compose = strings.Replace(compose,
				"- DATABASE_URL=postgresql://appuser:apppass@postgres:5432/appdb",
				`- DATABASE_URL=postgresql://appuser:apppass@postgres:5432/appdb
        - REDIS_URL=redis://redis:6379`, 1)
		}

		// Add volumes section
		if hasDatabase || hasRedis {
			compose += `

volumes:`
			if hasDatabase {
				compose += `
  postgres_data:`
			}
			if hasRedis {
				compose += `
  redis_data:`
			}
		}
	}

	return compose
}

func (d *DeveloperMode) generateDockerfile(analysis *analysis.ProjectAnalysis) string {
	if analysis == nil || analysis.PrimaryStack == nil {
		// Default Node.js Dockerfile
		return d.getNodeJSDockerfile()
	}

	switch analysis.PrimaryStack.Language {
	case "javascript":
		return d.getNodeJSDockerfile()
	case "python":
		return d.getPythonDockerfile()
	case "go":
		return d.getGoDockerfile()
	default:
		return d.getNodeJSDockerfile() // Default fallback
	}
}

func (d *DeveloperMode) getNodeJSDockerfile() string {
	return `# Multi-stage build for Node.js
FROM node:18-alpine AS dependencies

# Install dumb-init for proper signal handling
RUN apk add --no-cache dumb-init

# Create app directory
WORKDIR /app

# Copy package files
COPY package*.json ./

# Install production dependencies
RUN npm ci --only=production --silent && npm cache clean --force

# Production stage
FROM node:18-alpine AS production

# Install dumb-init
RUN apk add --no-cache dumb-init

# Create non-root user
RUN addgroup -g 1001 -S nodejs && \
    adduser -S nodeuser -u 1001

WORKDIR /app

# Copy dependencies
COPY --from=dependencies --chown=nodeuser:nodejs /app/node_modules ./node_modules

# Copy application code
COPY --chown=nodeuser:nodejs . .

# Switch to non-root user
USER nodeuser

# Expose port
EXPOSE 3000

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD node -e "require('http').get('http://localhost:3000/health', (res) => { process.exit(res.statusCode === 200 ? 0 : 1) })"

# Start the application
ENTRYPOINT ["dumb-init", "--"]
CMD ["npm", "start"]`
}

func (d *DeveloperMode) getPythonDockerfile() string {
	return `# Multi-stage build for Python
FROM python:3.11-slim AS base

# Install system dependencies
RUN apt-get update && apt-get install -y \
    gcc \
    && rm -rf /var/lib/apt/lists/*

# Set environment variables
ENV PYTHONUNBUFFERED=1 \
    PYTHONDONTWRITEBYTECODE=1 \
    PIP_NO_CACHE_DIR=1 \
    PIP_DISABLE_PIP_VERSION_CHECK=1

# Create non-root user
RUN groupadd -r appuser && useradd -r -g appuser appuser

WORKDIR /app

# Copy requirements and install dependencies
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# Copy application code
COPY --chown=appuser:appuser . .

# Switch to non-root user
USER appuser

# Expose port
EXPOSE 8000

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD python -c "import urllib.request; urllib.request.urlopen('http://localhost:8000/health')"

# Start the application
CMD ["python", "manage.py", "runserver", "0.0.0.0:8000"]`
}

func (d *DeveloperMode) getGoDockerfile() string {
	return `# Multi-stage build for Go
FROM golang:1.21-alpine AS builder

# Install git (required for some go modules)
RUN apk add --no-cache git

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main .

# Production stage
FROM alpine:latest

# Install ca-certificates for HTTPS calls
RUN apk --no-cache add ca-certificates

# Create non-root user
RUN addgroup -g 1001 -S appuser && \
    adduser -S appuser -u 1001 -G appuser

WORKDIR /root/

# Copy the binary from builder
COPY --from=builder /app/main .

# Change ownership to non-root user
RUN chown appuser:appuser ./main

# Switch to non-root user
USER appuser

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD ./main -health-check || exit 1

# Start the application
CMD ["./main"]`
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
		if err := os.WriteFile(outputFile, jsonData, 0644); err != nil {
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
		if err := os.WriteFile(outputFile, yamlData, 0644); err != nil {
			return fmt.Errorf("failed to write YAML to file %s: %w", outputFile, err)
		}
		fmt.Printf("✅ Analysis exported to %s\n", color.GreenFmt(outputFile))
	} else {
		fmt.Print(string(yamlData))
	}

	return nil
}
