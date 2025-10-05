package modes

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/simple-container-com/api/pkg/api/logger/color"
	
	"github.com/simple-container-com/api/pkg/assistant/analysis"
	"github.com/simple-container-com/api/pkg/api/logger/color"
)

// DeveloperMode handles application-focused workflows
type DeveloperMode struct {
	analyzer  *analysis.ProjectAnalyzer
	generator *generation.FileGenerator
}

// NewDeveloperMode creates a new developer mode instance
func NewDeveloperMode() *DeveloperMode {
	return &DeveloperMode{
		analyzer:  analysis.NewProjectAnalyzer(),
		generator: generation.NewFileGenerator(),
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
func (d *DeveloperMode) Setup(ctx context.Context, opts SetupOptions) error {
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
		if err := d.interactiveSetup(&opts, projectAnalysis); err != nil {
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
func (d *DeveloperMode) Analyze(ctx context.Context, opts AnalyzeOptions) error {
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

	
	if analysis.Architecture != "" {
		fmt.Printf("   Architecture: %s\n", color.GreenFmt(analysis.Architecture))

	
	fmt.Printf("   Confidence:   %s\n", color.YellowFmt("%.0f%%", analysis.Confidence*100))

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
	// TODO: Implement interactive prompts

	
	// For now, just show what would be prompted
	fmt.Printf("   Target environment: %s\n", color.CyanFmt(opts.Environment))

	
	if analysis != nil && analysis.PrimaryStack != nil {
		fmt.Printf("   Detected language: %s\n", color.GreenFmt(analysis.PrimaryStack.Language))
		if analysis.PrimaryStack.Framework != "" {
			fmt.Printf("   Detected framework: %s\n", color.GreenFmt(analysis.PrimaryStack.Framework))
		}

	
	// TODO: Add actual prompts for user input

	
	return nil
}

func (d *DeveloperMode) generateFiles(projectPath string, opts SetupOptions, analysis *analysis.ProjectAnalysis) error {
	// Create .sc directory structure
	scDir := filepath.Join(projectPath, ".sc", "stacks", filepath.Base(projectPath))
	if err := os.MkdirAll(scDir, 0755); err != nil {
		return fmt.Errorf("failed to create .sc directory: %w", err)
	}

	// Generate client.yaml
	if !opts.SkipAnalysis {
		fmt.Printf("   📄 Generating client.yaml...")
		clientYaml := d.generateClientYAML(opts, analysis)
		clientPath := filepath.Join(scDir, "client.yaml")
		if err := os.WriteFile(clientPath, []byte(clientYaml), 0644); err != nil {
			return fmt.Errorf("failed to write client.yaml: %w", err)
		}
		fmt.Printf(" %s\n", color.GreenFmt("✓"))
	}

	// Generate docker-compose.yaml
	if !opts.SkipCompose {
		fmt.Printf("   📄 Generating docker-compose.yaml...")
		composePath := filepath.Join(projectPath, "docker-compose.yaml")
		if _, err := os.Stat(composePath); os.IsNotExist(err) {
			composeYaml := d.generateComposeYAML(analysis)
			if err := os.WriteFile(composePath, []byte(composeYaml), 0644); err != nil {
				return fmt.Errorf("failed to write docker-compose.yaml: %w", err)
			}
			fmt.Printf(" %s\n", color.GreenFmt("✓"))
		} else {
			fmt.Printf(" %s (already exists)\n", color.YellowFmt("⚠"))
		}
	}

	// Generate Dockerfile
	if !opts.SkipDockerfile {
		fmt.Printf("   📄 Generating Dockerfile...")
		dockerfilePath := filepath.Join(projectPath, "Dockerfile")
		if _, err := os.Stat(dockerfilePath); os.IsNotExist(err) {
			dockerfile := d.generateDockerfile(analysis)
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

func (d *DeveloperMode) generateClientYAML(opts SetupOptions, analysis *analysis.ProjectAnalysis) string {
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
				compose = strings.Replace(compose, 
					"- DATABASE_URL=postgresql://appuser:apppass@postgres:5432/appdb",
					`- DATABASE_URL=postgresql://appuser:apppass@postgres:5432/appdb
        - REDIS_URL=redis://redis:6379`, 1)
			}
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

func (d *DeveloperMode) printSetupSummary(opts SetupOptions, analysis *analysis.ProjectAnalysis) {

	
	fmt.Println("\n📁 Generated files:")
	fmt.Printf("   • client.yaml          - Simple Container configuration\n")
	if !opts.SkipCompose {
		fmt.Printf("   • docker-compose.yaml  - Local development environment\n")
	}
	if !opts.SkipDockerfile {
		fmt.Printf("   • Dockerfile           - Container image definition\n")

	
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
	// TODO: Implement JSON output
	fmt.Println("JSON output not yet implemented - will be available in Phase 2")
	return nil
}

	// TODO: Implement YAML output
	// TODO: Implement YAML output  
	fmt.Println("YAML output not yet implemented - will be available in Phase 2")
	return nil
}
