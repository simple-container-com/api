package analysis

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/simple-container-com/api/pkg/assistant/embeddings"
)

// LLMProvider interface for project analysis enhancement
type LLMProvider interface {
	GenerateResponse(ctx context.Context, prompt string) (string, error)
}

// ProjectAnalyzer orchestrates tech stack detection and generates recommendations
type ProjectAnalyzer struct {
	detectors         []TechStackDetector
	resourceDetectors []ResourceDetector
	llmProvider       LLMProvider
	embeddingsDB      *embeddings.Database
}

// NewProjectAnalyzer creates a new analyzer with default detectors
func NewProjectAnalyzer() *ProjectAnalyzer {
	// Initialize embeddings database for context enrichment
	embeddingsDB, _ := embeddings.LoadEmbeddedDatabase(context.Background())

	return &ProjectAnalyzer{
		detectors: []TechStackDetector{
			&NodeJSDetector{},
			&PythonDetector{},
			&GoDetector{},
			&DockerDetector{},
		},
		resourceDetectors: []ResourceDetector{
			&EnvironmentVariableDetector{},
			&SecretDetector{},
			&DatabaseDetector{},
			&QueueDetector{},
			&StorageDetector{},
			&ExternalAPIDetector{},
		},
		llmProvider:  nil, // Can be set later with SetLLMProvider
		embeddingsDB: embeddingsDB,
	}
}

// NewProjectAnalyzerWithEmbeddings creates an analyzer with existing embeddings DB (for reuse)
func NewProjectAnalyzerWithEmbeddings(embeddingsDB *embeddings.Database) *ProjectAnalyzer {
	return &ProjectAnalyzer{
		detectors: []TechStackDetector{
			&NodeJSDetector{},
			&PythonDetector{},
			&GoDetector{},
			&DockerDetector{},
		},
		resourceDetectors: []ResourceDetector{
			&EnvironmentVariableDetector{},
			&SecretDetector{},
			&DatabaseDetector{},
			&QueueDetector{},
			&StorageDetector{},
			&ExternalAPIDetector{},
		},
		llmProvider:  nil,
		embeddingsDB: embeddingsDB,
	}
}

// SetLLMProvider sets the LLM provider for enhanced analysis
func (pa *ProjectAnalyzer) SetLLMProvider(provider LLMProvider) {
	pa.llmProvider = provider
}

// AddDetector adds a custom detector
func (pa *ProjectAnalyzer) AddDetector(detector TechStackDetector) {
	pa.detectors = append(pa.detectors, detector)

	// Re-sort by priority
	sort.Slice(pa.detectors, func(i, j int) bool {
		return pa.detectors[i].Priority() > pa.detectors[j].Priority()
	})
}

// AddResourceDetector adds a custom resource detector
func (pa *ProjectAnalyzer) AddResourceDetector(detector ResourceDetector) {
	pa.resourceDetectors = append(pa.resourceDetectors, detector)

	// Re-sort by priority
	sort.Slice(pa.resourceDetectors, func(i, j int) bool {
		return pa.resourceDetectors[i].Priority() > pa.resourceDetectors[j].Priority()
	})
}

// AnalyzeProject performs comprehensive project analysis
func (pa *ProjectAnalyzer) AnalyzeProject(projectPath string) (*ProjectAnalysis, error) {
	// Validate project path
	if _, err := os.Stat(projectPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("project path does not exist: %s", projectPath)
	}

	analysis := &ProjectAnalysis{
		Path: projectPath,
		Name: filepath.Base(projectPath),
		Metadata: map[string]interface{}{
			"analyzed_at":      time.Now(),
			"analyzer_version": "1.0",
		},
	}

	// Run all detectors in parallel
	detectedStacks := pa.runTechStackDetectorsParallel(projectPath)

	if len(detectedStacks) == 0 {
		return nil, fmt.Errorf("no technology stacks detected in project")
	}

	analysis.TechStacks = detectedStacks

	// Determine primary stack (highest confidence)
	primaryStack := &detectedStacks[0]
	for i := 1; i < len(detectedStacks); i++ {
		if detectedStacks[i].Confidence > primaryStack.Confidence {
			primaryStack = &detectedStacks[i]
		}
	}
	analysis.PrimaryStack = primaryStack

	// Calculate overall confidence
	totalConfidence := float32(0)
	for _, stack := range detectedStacks {
		totalConfidence += stack.Confidence
	}
	analysis.Confidence = totalConfidence / float32(len(detectedStacks))

	// Detect architecture patterns
	analysis.Architecture = pa.detectArchitecture(detectedStacks, projectPath)

	// Generate recommendations (enhanced with LLM)
	analysis.Recommendations = pa.generateRecommendations(analysis)

	// Analyze file structure
	files, err := pa.analyzeFiles(projectPath)
	if err == nil {
		analysis.Files = files
	}

	// Run resource detectors in parallel
	resources, err := pa.analyzeResourcesParallel(projectPath)
	if err == nil {
		analysis.Resources = resources
	}

	// Analyze Git repository
	gitAnalyzer := NewGitAnalyzer(projectPath)
	if gitAnalysis, err := gitAnalyzer.AnalyzeGitRepository(); err == nil {
		analysis.Git = gitAnalysis
	}

	// Generate enhanced recommendations based on detected resources
	analysis.Recommendations = pa.generateEnhancedRecommendations(analysis)

	// Enhance analysis with LLM insights
	if pa.llmProvider != nil {
		if enhanced, err := pa.enhanceWithLLM(context.Background(), analysis); err == nil {
			analysis = enhanced
		}
	}

	return analysis, nil
}

// detectArchitecture determines the architectural pattern
func (pa *ProjectAnalyzer) detectArchitecture(stacks []TechStackInfo, projectPath string) string {
	// Check for microservice indicators
	if pa.hasMicroserviceIndicators(stacks, projectPath) {
		return "microservice"
	}

	// Check for monolith indicators
	if pa.hasMonolithIndicators(stacks, projectPath) {
		return "monolith"
	}

	// Check for serverless indicators
	if pa.hasServerlessIndicators(stacks, projectPath) {
		return "serverless"
	}

	// Check for static site indicators
	if pa.hasStaticSiteIndicators(stacks, projectPath) {
		return "static-site"
	}

	return "standard-web-app"
}

func (pa *ProjectAnalyzer) hasMicroserviceIndicators(stacks []TechStackInfo, projectPath string) bool {
	indicators := []string{
		"docker-compose.yml",
		"docker-compose.yaml",
		"kubernetes",
		"k8s",
		"microservice",
		"services",
	}

	// Check file names
	for _, indicator := range indicators {
		if _, err := os.Stat(filepath.Join(projectPath, indicator)); err == nil {
			return true
		}
	}

	// Check for multiple service directories
	entries, err := os.ReadDir(projectPath)
	if err != nil {
		return false
	}

	serviceCount := 0
	for _, entry := range entries {
		if entry.IsDir() && pa.looksLikeServiceDir(entry.Name()) {
			serviceCount++
		}
	}

	return serviceCount >= 2
}

func (pa *ProjectAnalyzer) hasMonolithIndicators(stacks []TechStackInfo, projectPath string) bool {
	// Large single-language projects tend to be monoliths
	for _, stack := range stacks {
		if len(stack.Dependencies) > 20 {
			return true
		}

		// Check for MVC frameworks
		mvcFrameworks := []string{"django", "rails", "spring", "laravel", "express"}
		for _, framework := range mvcFrameworks {
			if strings.Contains(strings.ToLower(stack.Framework), framework) {
				return true
			}
		}
	}

	return false
}

func (pa *ProjectAnalyzer) hasServerlessIndicators(stacks []TechStackInfo, projectPath string) bool {
	serverlessFiles := []string{
		"serverless.yml",
		"serverless.yaml",
		"template.yml",
		"template.yaml",
		"sam-template.yml",
		"netlify.toml",
		"vercel.json",
	}

	for _, file := range serverlessFiles {
		if _, err := os.Stat(filepath.Join(projectPath, file)); err == nil {
			return true
		}
	}

	// Check for serverless dependencies
	for _, stack := range stacks {
		for _, dep := range stack.Dependencies {
			serverlessDeps := []string{"serverless", "aws-lambda", "azure-functions", "functions-framework"}
			for _, serverlessDep := range serverlessDeps {
				if strings.Contains(strings.ToLower(dep.Name), serverlessDep) {
					return true
				}
			}
		}
	}

	return false
}

func (pa *ProjectAnalyzer) hasStaticSiteIndicators(stacks []TechStackInfo, projectPath string) bool {
	staticSiteFiles := []string{
		"index.html",
		"_config.yml",      // Jekyll
		"gatsby-config.js", // Gatsby
		"next.config.js",   // Next.js static export
		"nuxt.config.js",   // Nuxt.js static
		"hugo.toml",        // Hugo
		"hugo.yaml",
	}

	for _, file := range staticSiteFiles {
		if _, err := os.Stat(filepath.Join(projectPath, file)); err == nil {
			return true
		}
	}

	// Check for static site generators in dependencies
	for _, stack := range stacks {
		for _, dep := range stack.Dependencies {
			staticDeps := []string{"gatsby", "next", "nuxt", "hugo", "jekyll", "hexo"}
			for _, staticDep := range staticDeps {
				if strings.Contains(strings.ToLower(dep.Name), staticDep) {
					return true
				}
			}
		}
	}

	return false
}

func (pa *ProjectAnalyzer) looksLikeServiceDir(name string) bool {
	serviceIndicators := []string{
		"service", "api", "app", "backend", "frontend",
		"auth", "user", "payment", "notification", "gateway",
	}

	lowerName := strings.ToLower(name)
	for _, indicator := range serviceIndicators {
		if strings.Contains(lowerName, indicator) {
			return true
		}
	}

	return false
}

// generateRecommendations creates Simple Container configuration recommendations
func (pa *ProjectAnalyzer) generateRecommendations(analysis *ProjectAnalysis) []Recommendation {
	var recommendations []Recommendation

	if analysis.PrimaryStack == nil {
		return recommendations
	}

	primaryStack := analysis.PrimaryStack

	// Language-specific recommendations
	switch primaryStack.Language {
	case "javascript":
		recommendations = append(recommendations, pa.getNodeJSRecommendations(primaryStack)...)
	case "python":
		recommendations = append(recommendations, pa.getPythonRecommendations(primaryStack)...)
	case "go":
		recommendations = append(recommendations, pa.getGoRecommendations(primaryStack)...)
	}

	// Architecture-specific recommendations
	switch analysis.Architecture {
	case "microservice":
		recommendations = append(recommendations, pa.getMicroserviceRecommendations()...)
	case "serverless":
		recommendations = append(recommendations, pa.getServerlessRecommendations()...)
	case "static-site":
		recommendations = append(recommendations, pa.getStaticSiteRecommendations()...)
	}

	// Database recommendations
	recommendations = append(recommendations, pa.getDatabaseRecommendations(primaryStack)...)

	// General recommendations
	recommendations = append(recommendations, pa.getGeneralRecommendations()...)

	return recommendations
}

func (pa *ProjectAnalyzer) getNodeJSRecommendations(stack *TechStackInfo) []Recommendation {
	var recs []Recommendation

	// Dockerfile recommendation
	recs = append(recs, Recommendation{
		Type:        "configuration",
		Category:    "containerization",
		Priority:    "high",
		Title:       "Create Node.js Dockerfile",
		Description: fmt.Sprintf("Generate optimized Dockerfile for Node.js %s application", stack.Version),
		Action:      "generate_dockerfile",
		Code: `FROM node:18-alpine
WORKDIR /app
COPY package*.json ./
RUN npm ci --only=production
COPY . .
EXPOSE 3000
CMD ["npm", "start"]`,
	})

	// Framework-specific recommendations
	switch stack.Framework {
	case "express":
		recs = append(recs, Recommendation{
			Type:        "template",
			Category:    "deployment",
			Priority:    "high",
			Title:       "Express.js ECS Deployment",
			Description: "Use ECS Fargate template for Express.js applications",
			Template:    "aws-ecs-fargate",
			Action:      "use_template",
		})

	case "nextjs":
		recs = append(recs, Recommendation{
			Type:        "template",
			Category:    "deployment",
			Priority:    "high",
			Title:       "Next.js Static Site Deployment",
			Description: "Use static site template for Next.js applications",
			Template:    "static-site",
			Action:      "use_template",
		})
	}

	return recs
}

func (pa *ProjectAnalyzer) getPythonRecommendations(stack *TechStackInfo) []Recommendation {
	var recs []Recommendation

	// Dockerfile recommendation
	recs = append(recs, Recommendation{
		Type:        "configuration",
		Category:    "containerization",
		Priority:    "high",
		Title:       "Create Python Dockerfile",
		Description: "Generate optimized Dockerfile for Python application",
		Action:      "generate_dockerfile",
		Code: `FROM python:3.11-slim
WORKDIR /app
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt
COPY . .
EXPOSE 8000
CMD ["python", "app.py"]`,
	})

	// Framework-specific recommendations
	switch stack.Framework {
	case "django":
		recs = append(recs, Recommendation{
			Type:        "template",
			Category:    "deployment",
			Priority:    "high",
			Title:       "Django ECS Deployment",
			Description: "Use ECS Fargate template for Django applications",
			Template:    "aws-ecs-fargate",
			Action:      "use_template",
		})

	case "fastapi":
		recs = append(recs, Recommendation{
			Type:        "template",
			Category:    "deployment",
			Priority:    "high",
			Title:       "FastAPI ECS Deployment",
			Description: "Use ECS Fargate template for FastAPI applications",
			Template:    "aws-ecs-fargate",
			Action:      "use_template",
		})
	}

	return recs
}

func (pa *ProjectAnalyzer) getGoRecommendations(stack *TechStackInfo) []Recommendation {
	var recs []Recommendation

	// Dockerfile recommendation
	recs = append(recs, Recommendation{
		Type:        "configuration",
		Category:    "containerization",
		Priority:    "high",
		Title:       "Create Go Dockerfile",
		Description: "Generate multi-stage Dockerfile for Go application",
		Action:      "generate_dockerfile",
		Code: `FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o main .

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/main .
EXPOSE 8080
CMD ["./main"]`,
	})

	// Framework-specific recommendations
	switch stack.Framework {
	case "gin":
		recs = append(recs, Recommendation{
			Type:        "template",
			Category:    "deployment",
			Priority:    "high",
			Title:       "Gin API ECS Deployment",
			Description: "Use ECS Fargate template for Gin applications",
			Template:    "aws-ecs-fargate",
			Action:      "use_template",
		})
	}

	return recs
}

func (pa *ProjectAnalyzer) getMicroserviceRecommendations() []Recommendation {
	return []Recommendation{
		{
			Type:        "template",
			Category:    "orchestration",
			Priority:    "high",
			Title:       "Kubernetes Native Deployment",
			Description: "Use Kubernetes template for microservice architecture",
			Template:    "kubernetes-native",
			Action:      "use_template",
		},
		{
			Type:        "resource",
			Category:    "service-mesh",
			Priority:    "medium",
			Title:       "Service Mesh Setup",
			Description: "Consider service mesh for inter-service communication",
			Resource:    "service-mesh",
			Action:      "add_resource",
		},
	}
}

func (pa *ProjectAnalyzer) getServerlessRecommendations() []Recommendation {
	return []Recommendation{
		{
			Type:        "template",
			Category:    "serverless",
			Priority:    "high",
			Title:       "AWS Lambda Deployment",
			Description: "Use Lambda template for serverless applications",
			Template:    "aws-lambda",
			Action:      "use_template",
		},
	}
}

func (pa *ProjectAnalyzer) getStaticSiteRecommendations() []Recommendation {
	return []Recommendation{
		{
			Type:        "template",
			Category:    "hosting",
			Priority:    "high",
			Title:       "Static Site Deployment",
			Description: "Use static site template with CDN",
			Template:    "static-site",
			Action:      "use_template",
		},
		{
			Type:        "resource",
			Category:    "storage",
			Priority:    "high",
			Title:       "S3 Bucket for Static Assets",
			Description: "Add S3 bucket for static asset storage",
			Resource:    "s3-bucket",
			Action:      "add_resource",
		},
	}
}

func (pa *ProjectAnalyzer) getDatabaseRecommendations(stack *TechStackInfo) []Recommendation {
	var recs []Recommendation

	// Check for database dependencies
	dbDeps := map[string]string{
		"mongoose":   "mongodb",
		"mongodb":    "mongodb",
		"pg":         "postgresql",
		"postgresql": "postgresql",
		"mysql":      "mysql",
		"mysql2":     "mysql",
		"redis":      "redis",
		"ioredis":    "redis",
		"sqlite3":    "sqlite",
		"psycopg2":   "postgresql",
		"pymongo":    "mongodb",
		"sqlalchemy": "postgresql", // Default to PostgreSQL for SQLAlchemy
	}

	detectedDBs := make(map[string]bool)
	for _, dep := range stack.Dependencies {
		if db, exists := dbDeps[strings.ToLower(dep.Name)]; exists {
			if !detectedDBs[db] {
				detectedDBs[db] = true

				switch db {
				case "postgresql":
					recs = append(recs, Recommendation{
						Type:        "resource",
						Category:    "database",
						Priority:    "high",
						Title:       "PostgreSQL Database",
						Description: "Add PostgreSQL database resource",
						Resource:    "aws-rds-postgres",
						Action:      "add_resource",
					})
				case "mongodb":
					recs = append(recs, Recommendation{
						Type:        "resource",
						Category:    "database",
						Priority:    "high",
						Title:       "MongoDB Database",
						Description: "Add MongoDB Atlas database resource",
						Resource:    "mongodb-atlas",
						Action:      "add_resource",
					})
				case "mysql":
					recs = append(recs, Recommendation{
						Type:        "resource",
						Category:    "database",
						Priority:    "high",
						Title:       "MySQL Database",
						Description: "Add MySQL database resource",
						Resource:    "aws-rds-mysql",
						Action:      "add_resource",
					})
				case "redis":
					recs = append(recs, Recommendation{
						Type:        "resource",
						Category:    "cache",
						Priority:    "medium",
						Title:       "Redis Cache",
						Description: "Add Redis cache resource",
						Resource:    "redis-cache",
						Action:      "add_resource",
					})
				}
			}
		}
	}

	return recs
}

func (pa *ProjectAnalyzer) getGeneralRecommendations() []Recommendation {
	return []Recommendation{
		{
			Type:        "configuration",
			Category:    "setup",
			Priority:    "high",
			Title:       "Initialize Simple Container",
			Description: "Set up Simple Container configuration structure",
			Action:      "init_sc_structure",
		},
		{
			Type:        "configuration",
			Category:    "deployment",
			Priority:    "medium",
			Title:       "Create docker-compose.yaml",
			Description: "Generate docker-compose.yaml for local development",
			Action:      "generate_compose",
		},
		{
			Type:        "configuration",
			Category:    "optimization",
			Priority:    "low",
			Title:       "Add .dockerignore",
			Description: "Create .dockerignore to optimize build context",
			Action:      "create_dockerignore",
		},
	}
}

// analyzeFiles provides file-level analysis
func (pa *ProjectAnalyzer) analyzeFiles(projectPath string) ([]FileInfo, error) {
	var files []FileInfo

	err := filepath.WalkDir(projectPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		// Skip hidden directories and files
		if strings.HasPrefix(d.Name(), ".") && d.Name() != ".dockerignore" {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip common ignore directories
		if d.IsDir() {
			ignoreDirs := []string{"node_modules", "__pycache__", "vendor", "target", "build", "dist"}
			for _, ignore := range ignoreDirs {
				if d.Name() == ignore {
					return filepath.SkipDir
				}
			}
			return nil
		}

		// Analyze file
		relPath, _ := filepath.Rel(projectPath, path)
		info, err := d.Info()
		if err != nil {
			return nil
		}

		file := FileInfo{
			Path: relPath,
			Size: info.Size(),
			Type: pa.getFileType(d.Name()),
		}

		// Set language based on extension
		ext := strings.ToLower(filepath.Ext(d.Name()))
		switch ext {
		case ".js", ".mjs", ".jsx":
			file.Language = "javascript"
		case ".ts", ".tsx":
			file.Language = "typescript"
		case ".py":
			file.Language = "python"
		case ".go":
			file.Language = "go"
		case ".java":
			file.Language = "java"
		case ".rb":
			file.Language = "ruby"
		case ".php":
			file.Language = "php"
		case ".rs":
			file.Language = "rust"
		case ".cpp", ".cc", ".cxx":
			file.Language = "cpp"
		case ".c":
			file.Language = "c"
		case ".cs":
			file.Language = "csharp"
		}

		// Analyze complexity for source files
		if file.Type == "source" && file.Language != "" {
			complexityAnalyzer := NewComplexityAnalyzer()
			if complexity, err := complexityAnalyzer.AnalyzeFile(path, file.Language); err == nil {
				file.Complexity = complexity
			}
		}

		files = append(files, file)
		return nil
	})

	return files, err
}

func (pa *ProjectAnalyzer) getFileType(filename string) string {
	configFiles := map[string]bool{
		"package.json":        true,
		"requirements.txt":    true,
		"go.mod":              true,
		"Dockerfile":          true,
		"docker-compose.yml":  true,
		"docker-compose.yaml": true,
		".dockerignore":       true,
		"Makefile":            true,
		"makefile":            true,
	}

	buildFiles := map[string]bool{
		"webpack.config.js": true,
		"vite.config.js":    true,
		"rollup.config.js":  true,
		"tsconfig.json":     true,
		"babel.config.js":   true,
		"setup.py":          true,
		"pyproject.toml":    true,
		"Cargo.toml":        true,
		"build.gradle":      true,
		"pom.xml":           true,
	}

	docsFiles := map[string]bool{
		"README.md":    true,
		"readme.md":    true,
		"README.rst":   true,
		"LICENSE":      true,
		"CHANGELOG.md": true,
	}

	if configFiles[filename] {
		return "config"
	}
	if buildFiles[filename] {
		return "build"
	}
	if docsFiles[filename] {
		return "docs"
	}

	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".js", ".ts", ".py", ".go", ".java", ".rb", ".php", ".rs", ".cpp", ".c", ".cs":
		return "source"
	case ".json", ".yaml", ".yml", ".toml", ".ini", ".env":
		return "config"
	case ".md", ".rst", ".txt":
		return "docs"
	case ".html", ".css", ".scss", ".sass", ".less":
		return "web"
	case ".sql":
		return "database"
	case ".sh", ".bash", ".ps1", ".bat":
		return "script"
	default:
		return "other"
	}
}

// enhanceWithLLM uses LLM to provide deeper insights and better recommendations
func (pa *ProjectAnalyzer) enhanceWithLLM(ctx context.Context, analysis *ProjectAnalysis) (*ProjectAnalysis, error) {
	if pa.llmProvider == nil {
		return analysis, nil
	}

	// Prepare project context for LLM analysis
	projectSummary := pa.buildProjectSummary(analysis)

	// Get relevant documentation context
	var docContext string
	if pa.embeddingsDB != nil {
		if results, err := embeddings.SearchDocumentation(pa.embeddingsDB,
			fmt.Sprintf("%s %s architecture patterns", analysis.PrimaryStack.Language, analysis.PrimaryStack.Framework), 3); err == nil {
			for _, result := range results {
				docContext += result.Content + "\n"
			}
		}
	}

	// Build LLM prompt for enhanced analysis
	prompt := pa.buildAnalysisPrompt(projectSummary, docContext)

	// Get LLM insights
	response, err := pa.llmProvider.GenerateResponse(ctx, prompt)
	if err != nil {
		return analysis, err // Return original analysis if LLM fails
	}

	// Parse LLM response and enhance analysis
	enhanced := pa.parseAndEnhanceAnalysis(analysis, response)

	return enhanced, nil
}

// buildProjectSummary creates a concise summary for LLM analysis
func (pa *ProjectAnalyzer) buildProjectSummary(analysis *ProjectAnalysis) string {
	var summary strings.Builder

	summary.WriteString(fmt.Sprintf("Project: %s\n", analysis.Name))
	summary.WriteString(fmt.Sprintf("Path: %s\n", analysis.Path))
	summary.WriteString(fmt.Sprintf("Architecture: %s\n", analysis.Architecture))

	if analysis.PrimaryStack != nil {
		summary.WriteString(fmt.Sprintf("Primary Stack: %s %s (%.1f%% confidence)\n",
			analysis.PrimaryStack.Language, analysis.PrimaryStack.Framework, analysis.PrimaryStack.Confidence*100))
	}

	summary.WriteString(fmt.Sprintf("Tech Stacks: %d detected\n", len(analysis.TechStacks)))
	for _, stack := range analysis.TechStacks {
		summary.WriteString(fmt.Sprintf("- %s %s (%d dependencies)\n",
			stack.Language, stack.Framework, len(stack.Dependencies)))
	}

	summary.WriteString(fmt.Sprintf("Files: %d analyzed\n", len(analysis.Files)))

	// Add Git repository context
	if analysis.Git != nil && analysis.Git.IsGitRepo {
		summary.WriteString("\nGit Repository Context:\n")
		summary.WriteString(fmt.Sprintf("- Current Branch: %s\n", analysis.Git.Branch))

		if analysis.Git.CommitActivity != nil {
			summary.WriteString(fmt.Sprintf("- Total Commits: %d\n", analysis.Git.CommitActivity.TotalCommits))
			summary.WriteString(fmt.Sprintf("- Recent Activity: %d commits in last 30 days\n", analysis.Git.CommitActivity.RecentCommits))
			summary.WriteString(fmt.Sprintf("- Average: %.1f commits/week\n", analysis.Git.CommitActivity.AveragePerWeek))
		}

		if len(analysis.Git.Contributors) > 0 {
			summary.WriteString(fmt.Sprintf("- Contributors: %d (top: %s with %d commits)\n",
				len(analysis.Git.Contributors), analysis.Git.Contributors[0].Name, analysis.Git.Contributors[0].Commits))
		}

		if analysis.Git.ProjectAge > 0 {
			summary.WriteString(fmt.Sprintf("- Project Age: %d days\n", analysis.Git.ProjectAge))
		}

		if analysis.Git.HasCI {
			summary.WriteString(fmt.Sprintf("- CI/CD: %s\n", strings.Join(analysis.Git.CIPlatforms, ", ")))
		}

		if len(analysis.Git.Tags) > 0 {
			summary.WriteString(fmt.Sprintf("- Latest Tags: %s\n", strings.Join(analysis.Git.Tags[:min(3, len(analysis.Git.Tags))], ", ")))
		}
	}

	// Add resource context
	if analysis.Resources != nil {
		summary.WriteString("\nDetected Resources:\n")
		if len(analysis.Resources.Databases) > 0 {
			summary.WriteString(fmt.Sprintf("- Databases: %d detected\n", len(analysis.Resources.Databases)))
		}
		if len(analysis.Resources.ExternalAPIs) > 0 {
			summary.WriteString(fmt.Sprintf("- External APIs: %d detected\n", len(analysis.Resources.ExternalAPIs)))
		}
		if len(analysis.Resources.Secrets) > 0 {
			summary.WriteString(fmt.Sprintf("- Potential Secrets: %d detected\n", len(analysis.Resources.Secrets)))
		}
	}

	return summary.String()
}

// min helper function for integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// minFloat32 helper function for float32
func minFloat32(a, b float32) float32 {
	if a < b {
		return a
	}
	return b
}

// buildAnalysisPrompt creates the LLM prompt for enhanced analysis
func (pa *ProjectAnalyzer) buildAnalysisPrompt(projectSummary, docContext string) string {
	return fmt.Sprintf(`You are an expert software architect analyzing a project for Simple Container deployment recommendations.

PROJECT ANALYSIS:
%s

SIMPLE CONTAINER CONTEXT:
%s

Please provide enhanced analysis in the following areas:

1. ARCHITECTURE ASSESSMENT:
   - Validate the detected architecture pattern
   - Suggest improvements or alternative patterns
   - Identify potential scalability concerns

2. DEPLOYMENT RECOMMENDATIONS:
   - Recommend specific Simple Container deployment types (cloud-compose, single-image, etc.)
   - Suggest resource requirements and scaling strategies
   - Identify security considerations

3. OPTIMIZATION OPPORTUNITIES:
   - Performance optimization suggestions
   - Cost optimization recommendations
   - Best practices for the detected tech stack

4. SIMPLE CONTAINER INTEGRATION:
   - Specific resources that would benefit this project
   - Configuration recommendations
   - Migration strategy if applicable

Respond in JSON format with the following structure:
{
  "architecture_assessment": {
    "validated_pattern": "string",
    "confidence": 0.0-1.0,
    "improvements": ["string"],
    "concerns": ["string"]
  },
  "deployment_recommendations": {
    "recommended_type": "string",
    "resources": ["string"],
    "scaling_strategy": "string",
    "security_notes": ["string"]
  },
  "optimizations": {
    "performance": ["string"],
    "cost": ["string"],
    "best_practices": ["string"]
  },
  "simple_container_integration": {
    "recommended_resources": ["string"],
    "configuration_tips": ["string"],
    "migration_steps": ["string"]
  }
}`, projectSummary, docContext)
}

// parseAndEnhanceAnalysis parses LLM response and enhances the original analysis
func (pa *ProjectAnalyzer) parseAndEnhanceAnalysis(original *ProjectAnalysis, llmResponse string) *ProjectAnalysis {
	enhanced := *original // Copy original analysis

	// Try to parse JSON response
	var llmInsights struct {
		ArchitectureAssessment struct {
			ValidatedPattern string   `json:"validated_pattern"`
			Confidence       float32  `json:"confidence"`
			Improvements     []string `json:"improvements"`
			Concerns         []string `json:"concerns"`
		} `json:"architecture_assessment"`
		DeploymentRecommendations struct {
			RecommendedType string   `json:"recommended_type"`
			Resources       []string `json:"resources"`
			ScalingStrategy string   `json:"scaling_strategy"`
			SecurityNotes   []string `json:"security_notes"`
		} `json:"deployment_recommendations"`
		Optimizations struct {
			Performance   []string `json:"performance"`
			Cost          []string `json:"cost"`
			BestPractices []string `json:"best_practices"`
		} `json:"optimizations"`
		SimpleContainerIntegration struct {
			RecommendedResources []string `json:"recommended_resources"`
			ConfigurationTips    []string `json:"configuration_tips"`
			MigrationSteps       []string `json:"migration_steps"`
		} `json:"simple_container_integration"`
	}

	if err := json.Unmarshal([]byte(llmResponse), &llmInsights); err != nil {
		// If JSON parsing fails, add raw LLM response as metadata
		enhanced.Metadata["llm_insights"] = llmResponse
		return &enhanced
	}

	// Enhance architecture with LLM insights
	if llmInsights.ArchitectureAssessment.ValidatedPattern != "" {
		enhanced.Architecture = llmInsights.ArchitectureAssessment.ValidatedPattern
	}

	// Add LLM-generated recommendations
	llmRecommendations := []Recommendation{}

	// Architecture improvements
	for _, improvement := range llmInsights.ArchitectureAssessment.Improvements {
		llmRecommendations = append(llmRecommendations, Recommendation{
			Type:        "architecture",
			Category:    "improvement",
			Priority:    "medium",
			Title:       "Architecture Improvement",
			Description: improvement,
			Action:      "review_architecture",
		})
	}

	// Deployment recommendations
	if llmInsights.DeploymentRecommendations.RecommendedType != "" {
		llmRecommendations = append(llmRecommendations, Recommendation{
			Type:        "deployment",
			Category:    "configuration",
			Priority:    "high",
			Title:       "Recommended Deployment Type",
			Description: fmt.Sprintf("Use %s deployment type for optimal performance", llmInsights.DeploymentRecommendations.RecommendedType),
			Action:      "configure_deployment",
		})
	}

	// Resource recommendations
	for _, resource := range llmInsights.SimpleContainerIntegration.RecommendedResources {
		llmRecommendations = append(llmRecommendations, Recommendation{
			Type:        "resource",
			Category:    "infrastructure",
			Priority:    "medium",
			Title:       "Recommended Resource",
			Description: fmt.Sprintf("Consider adding %s resource for better functionality", resource),
			Action:      "add_resource",
		})
	}

	// Performance optimizations
	for _, optimization := range llmInsights.Optimizations.Performance {
		llmRecommendations = append(llmRecommendations, Recommendation{
			Type:        "optimization",
			Category:    "performance",
			Priority:    "medium",
			Title:       "Performance Optimization",
			Description: optimization,
			Action:      "optimize_performance",
		})
	}

	// Merge with existing recommendations
	enhanced.Recommendations = append(enhanced.Recommendations, llmRecommendations...)

	// Add LLM metadata
	enhanced.Metadata["llm_enhanced"] = true
	enhanced.Metadata["llm_confidence"] = llmInsights.ArchitectureAssessment.Confidence
	enhanced.Metadata["deployment_recommendation"] = llmInsights.DeploymentRecommendations.RecommendedType
	enhanced.Metadata["scaling_strategy"] = llmInsights.DeploymentRecommendations.ScalingStrategy

	return &enhanced
}

// generateEnhancedRecommendations generates recommendations based on all analysis including resources
func (pa *ProjectAnalyzer) generateEnhancedRecommendations(analysis *ProjectAnalysis) []Recommendation {
	var recommendations []Recommendation

	// Start with existing tech stack recommendations
	recommendations = pa.generateRecommendations(analysis)

	// Add resource-based recommendations if resources were detected
	if analysis.Resources != nil {
		recommendations = append(recommendations, pa.generateResourceRecommendations(analysis.Resources)...)
	}

	// Add Git-based recommendations
	if analysis.Git != nil {
		recommendations = append(recommendations, pa.generateGitRecommendations(analysis.Git)...)
	}

	// Add complexity-based recommendations
	if len(analysis.Files) > 0 {
		recommendations = append(recommendations, pa.generateComplexityRecommendations(analysis.Files)...)
	}

	return recommendations
}

// generateResourceRecommendations creates recommendations based on detected resources
func (pa *ProjectAnalyzer) generateResourceRecommendations(resources *ResourceAnalysis) []Recommendation {
	var recs []Recommendation

	// Environment variable recommendations
	if len(resources.EnvironmentVars) > 0 {
		recs = append(recs, Recommendation{
			Type:        "configuration",
			Category:    "secrets",
			Priority:    "high",
			Title:       "Environment Variables Management",
			Description: fmt.Sprintf("Detected %d environment variables. Consider using Simple Container secrets.yaml for sensitive values", len(resources.EnvironmentVars)),
			Action:      "setup_secrets",
		})

		// Check for database-related env vars
		dbEnvVars := 0
		for _, envVar := range resources.EnvironmentVars {
			if envVar.UsageType == "database_config" || envVar.UsageType == "secret" {
				dbEnvVars++
			}
		}

		if dbEnvVars > 0 {
			recs = append(recs, Recommendation{
				Type:        "configuration",
				Category:    "security",
				Priority:    "high",
				Title:       "Database Credentials Security",
				Description: fmt.Sprintf("Found %d database-related environment variables. Use Simple Container template placeholders like ${resource:db.url}", dbEnvVars),
				Action:      "secure_database_config",
			})
		}
	}

	// Secret recommendations
	if len(resources.Secrets) > 0 {
		recs = append(recs, Recommendation{
			Type:        "configuration",
			Category:    "security",
			Priority:    "critical",
			Title:       "Hardcoded Secrets Detected",
			Description: fmt.Sprintf("Found %d potential hardcoded secrets. Move these to Simple Container secrets.yaml immediately", len(resources.Secrets)),
			Action:      "move_secrets_to_vault",
		})
	}

	// Database recommendations
	for _, db := range resources.Databases {
		switch db.Type {
		case "postgresql":
			recs = append(recs, Recommendation{
				Type:        "resource",
				Category:    "database",
				Priority:    "high",
				Title:       "PostgreSQL Database Resource",
				Description: "Add PostgreSQL database resource for detected usage",
				Resource:    "aws-rds-postgres",
				Action:      "add_resource",
			})
		case "mongodb":
			recs = append(recs, Recommendation{
				Type:        "resource",
				Category:    "database",
				Priority:    "high",
				Title:       "MongoDB Database Resource",
				Description: "Add MongoDB Atlas database resource for detected usage",
				Resource:    "mongodb-atlas",
				Action:      "add_resource",
			})
		case "redis":
			recs = append(recs, Recommendation{
				Type:        "resource",
				Category:    "cache",
				Priority:    "medium",
				Title:       "Redis Cache Resource",
				Description: "Add Redis cache resource for detected usage",
				Resource:    "gcp-redis",
				Action:      "add_resource",
			})
		}
	}

	// Queue recommendations
	for _, queue := range resources.Queues {
		switch queue.Type {
		case "rabbitmq":
			recs = append(recs, Recommendation{
				Type:        "resource",
				Category:    "messaging",
				Priority:    "medium",
				Title:       "RabbitMQ Messaging Resource",
				Description: "Add RabbitMQ operator resource for detected messaging usage",
				Resource:    "kubernetes-helm-rabbitmq-operator",
				Action:      "add_resource",
			})
		case "aws_sqs":
			recs = append(recs, Recommendation{
				Type:        "configuration",
				Category:    "messaging",
				Priority:    "medium",
				Title:       "AWS SQS Configuration",
				Description: "Configure AWS SQS authentication with ${auth:aws}",
				Action:      "configure_aws_sqs",
			})
		}
	}

	// Storage recommendations
	for _, storage := range resources.Storage {
		switch storage.Type {
		case "s3":
			recs = append(recs, Recommendation{
				Type:        "resource",
				Category:    "storage",
				Priority:    "high",
				Title:       "S3 Bucket Resource",
				Description: "Add S3 bucket resource for detected storage usage",
				Resource:    "s3-bucket",
				Action:      "add_resource",
			})
		case "gcs":
			recs = append(recs, Recommendation{
				Type:        "resource",
				Category:    "storage",
				Priority:    "high",
				Title:       "Google Cloud Storage Resource",
				Description: "Add GCP bucket resource for detected storage usage",
				Resource:    "gcp-bucket",
				Action:      "add_resource",
			})
		case "file_upload":
			recs = append(recs, Recommendation{
				Type:        "resource",
				Category:    "storage",
				Priority:    "medium",
				Title:       "File Upload Storage",
				Description: "Consider adding S3 or GCP bucket for scalable file uploads",
				Resource:    "s3-bucket",
				Action:      "add_resource",
			})
		}
	}

	// External API recommendations
	paymentAPIs := 0
	emailAPIs := 0
	aiAPIs := 0

	for _, api := range resources.ExternalAPIs {
		switch api.Purpose {
		case "payment":
			paymentAPIs++
		case "email":
			emailAPIs++
		case "ai":
			aiAPIs++
		}
	}

	if paymentAPIs > 0 {
		recs = append(recs, Recommendation{
			Type:        "configuration",
			Category:    "integration",
			Priority:    "medium",
			Title:       "Payment Service Integration",
			Description: fmt.Sprintf("Detected %d payment service integrations. Ensure API keys are in secrets.yaml", paymentAPIs),
			Action:      "secure_payment_config",
		})
	}

	if emailAPIs > 0 {
		recs = append(recs, Recommendation{
			Type:        "configuration",
			Category:    "integration",
			Priority:    "medium",
			Title:       "Email Service Integration",
			Description: fmt.Sprintf("Detected %d email service integrations. Configure with Simple Container secrets", emailAPIs),
			Action:      "configure_email_service",
		})
	}

	if aiAPIs > 0 {
		recs = append(recs, Recommendation{
			Type:        "configuration",
			Category:    "integration",
			Priority:    "medium",
			Title:       "AI Service Integration",
			Description: fmt.Sprintf("Detected %d AI service integrations. Manage API keys securely", aiAPIs),
			Action:      "secure_ai_config",
		})
	}

	return recs
}

// generateGitRecommendations creates recommendations based on Git analysis
func (pa *ProjectAnalyzer) generateGitRecommendations(git *GitAnalysis) []Recommendation {
	var recs []Recommendation

	if !git.IsGitRepo {
		return recs
	}

	// CI/CD recommendations
	if !git.HasCI {
		recs = append(recs, Recommendation{
			Type:        "configuration",
			Category:    "ci_cd",
			Priority:    "medium",
			Title:       "Add CI/CD Pipeline",
			Description: "No CI/CD detected. Consider adding GitHub Actions or similar for automated builds and deployments",
			Action:      "setup_ci_cd",
		})
	} else {
		recs = append(recs, Recommendation{
			Type:        "optimization",
			Category:    "ci_cd",
			Priority:    "low",
			Title:       "CI/CD Integration",
			Description: fmt.Sprintf("Detected %s. Consider integrating Simple Container deployment into your CI/CD pipeline", strings.Join(git.CIPlatforms, ", ")),
			Action:      "integrate_sc_ci",
		})
	}

	// Activity-based recommendations
	if git.CommitActivity != nil {
		if git.CommitActivity.RecentCommits == 0 && git.ProjectAge > 30 {
			recs = append(recs, Recommendation{
				Type:        "maintenance",
				Category:    "project_health",
				Priority:    "medium",
				Title:       "Project Activity",
				Description: "No recent commits detected. Consider project maintenance or archival",
				Action:      "review_project_status",
			})
		} else if git.CommitActivity.AveragePerWeek > 20 {
			recs = append(recs, Recommendation{
				Type:        "optimization",
				Category:    "development",
				Priority:    "medium",
				Title:       "High Development Activity",
				Description: fmt.Sprintf("High commit frequency (%.1f/week). Consider automated testing and deployment strategies", git.CommitActivity.AveragePerWeek),
				Action:      "optimize_development_workflow",
			})
		}
	}

	// Versioning recommendations
	if len(git.Tags) == 0 && git.ProjectAge > 30 {
		recs = append(recs, Recommendation{
			Type:        "configuration",
			Category:    "versioning",
			Priority:    "medium",
			Title:       "Version Tagging",
			Description: "No version tags found. Consider implementing semantic versioning for better release management",
			Action:      "setup_versioning",
		})
	}

	// Collaboration recommendations
	if len(git.Contributors) == 1 {
		recs = append(recs, Recommendation{
			Type:        "configuration",
			Category:    "collaboration",
			Priority:    "low",
			Title:       "Single Contributor",
			Description: "Single contributor detected. Consider adding documentation and contribution guidelines for future collaborators",
			Action:      "improve_documentation",
		})
	} else if len(git.Contributors) > 5 {
		recs = append(recs, Recommendation{
			Type:        "configuration",
			Category:    "collaboration",
			Priority:    "medium",
			Title:       "Multi-Contributor Project",
			Description: fmt.Sprintf("%d contributors detected. Ensure proper access controls and deployment permissions in Simple Container", len(git.Contributors)),
			Action:      "setup_team_access",
		})
	}

	// File change pattern recommendations
	if git.FileChanges != nil {
		// Check for frequently changed config files
		for _, file := range git.FileChanges.MostChangedFiles {
			if file.Type == "config" && file.Changes > 10 {
				recs = append(recs, Recommendation{
					Type:        "configuration",
					Category:    "stability",
					Priority:    "medium",
					Title:       "Frequently Changed Configuration",
					Description: fmt.Sprintf("Configuration file '%s' changes frequently (%d times). Consider using Simple Container environment-specific configs", file.Path, file.Changes),
					Action:      "stabilize_config",
				})
				break // Only add one recommendation for this pattern
			}
		}
	}

	return recs
}

// generateComplexityRecommendations creates recommendations based on code complexity analysis
func (pa *ProjectAnalyzer) generateComplexityRecommendations(files []FileInfo) []Recommendation {
	var recs []Recommendation

	highComplexityFiles := 0
	veryHighComplexityFiles := 0
	totalSourceFiles := 0
	totalLOC := 0
	lowCommentFiles := 0

	// Analyze complexity patterns
	for _, file := range files {
		if file.Complexity == nil {
			continue
		}

		totalSourceFiles++
		totalLOC += file.Complexity.LinesOfCode

		switch file.Complexity.ComplexityLevel {
		case "high":
			highComplexityFiles++
		case "very_high":
			veryHighComplexityFiles++
		}

		if file.Complexity.CommentRatio < 0.1 {
			lowCommentFiles++
		}
	}

	if totalSourceFiles == 0 {
		return recs
	}

	// Very high complexity files recommendation
	if veryHighComplexityFiles > 0 {
		recs = append(recs, Recommendation{
			Type:        "optimization",
			Category:    "code_quality",
			Priority:    "high",
			Title:       "High Complexity Files Detected",
			Description: fmt.Sprintf("Found %d files with very high complexity. Consider refactoring for better maintainability and deployment reliability", veryHighComplexityFiles),
			Action:      "refactor_complex_code",
		})
	}

	// High complexity ratio recommendation
	complexityRatio := float32(highComplexityFiles+veryHighComplexityFiles) / float32(totalSourceFiles)
	if complexityRatio > 0.3 {
		recs = append(recs, Recommendation{
			Type:        "optimization",
			Category:    "architecture",
			Priority:    "medium",
			Title:       "Code Complexity Management",
			Description: fmt.Sprintf("%.1f%% of source files have high complexity. Consider microservice architecture or modular design patterns", complexityRatio*100),
			Action:      "consider_microservices",
		})
	}

	// Large codebase recommendation
	if totalLOC > 50000 {
		recs = append(recs, Recommendation{
			Type:        "optimization",
			Category:    "deployment",
			Priority:    "medium",
			Title:       "Large Codebase Optimization",
			Description: fmt.Sprintf("Large codebase detected (%d LOC). Consider multi-stage Docker builds and build caching for faster deployments", totalLOC),
			Action:      "optimize_build_process",
		})
	}

	// Low documentation recommendation
	lowCommentRatio := float32(lowCommentFiles) / float32(totalSourceFiles)
	if lowCommentRatio > 0.5 {
		recs = append(recs, Recommendation{
			Type:        "maintenance",
			Category:    "documentation",
			Priority:    "medium",
			Title:       "Improve Code Documentation",
			Description: fmt.Sprintf("%.1f%% of files have low comment coverage. Better documentation improves maintainability and team collaboration", lowCommentRatio*100),
			Action:      "improve_code_documentation",
		})
	}

	// Monolith vs microservice recommendation based on complexity
	if totalSourceFiles > 100 && complexityRatio > 0.4 {
		recs = append(recs, Recommendation{
			Type:        "architecture",
			Category:    "scalability",
			Priority:    "high",
			Title:       "Consider Microservice Architecture",
			Description: fmt.Sprintf("Large codebase (%d files) with high complexity suggests microservice architecture. This can improve deployment flexibility and team autonomy", totalSourceFiles),
			Action:      "evaluate_microservices",
		})
	}

	// Testing recommendation based on complexity
	if veryHighComplexityFiles > 0 {
		recs = append(recs, Recommendation{
			Type:        "configuration",
			Category:    "testing",
			Priority:    "high",
			Title:       "Automated Testing Strategy",
			Description: "High complexity files require comprehensive testing. Implement automated testing in your CI/CD pipeline before deployment",
			Action:      "implement_testing_strategy",
		})
	}

	return recs
}

// runTechStackDetectorsParallel runs tech stack detectors in parallel for better performance
func (pa *ProjectAnalyzer) runTechStackDetectorsParallel(projectPath string) []TechStackInfo {
	type result struct {
		stack *TechStackInfo
		err   error
	}

	resultChan := make(chan result, len(pa.detectors))
	var wg sync.WaitGroup

	// Start all detectors in parallel
	for _, detector := range pa.detectors {
		wg.Add(1)
		go func(d TechStackDetector) {
			defer wg.Done()
			stack, err := d.Detect(projectPath)
			resultChan <- result{stack: stack, err: err}
		}(detector)
	}

	// Wait for all detectors to complete
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Collect results
	var detectedStacks []TechStackInfo
	for res := range resultChan {
		if res.err == nil && res.stack != nil {
			detectedStacks = append(detectedStacks, *res.stack)
		}
	}

	// Sort by confidence (highest first)
	sort.Slice(detectedStacks, func(i, j int) bool {
		return detectedStacks[i].Confidence > detectedStacks[j].Confidence
	})

	return detectedStacks
}

// analyzeResourcesParallel runs resource detectors in parallel for better performance
func (pa *ProjectAnalyzer) analyzeResourcesParallel(projectPath string) (*ResourceAnalysis, error) {
	type result struct {
		analysis *ResourceAnalysis
		err      error
	}

	resultChan := make(chan result, len(pa.resourceDetectors))
	var wg sync.WaitGroup

	// Start all resource detectors in parallel
	for _, detector := range pa.resourceDetectors {
		wg.Add(1)
		go func(d ResourceDetector) {
			defer wg.Done()
			analysis, err := d.Detect(projectPath)
			resultChan <- result{analysis: analysis, err: err}
		}(detector)
	}

	// Wait for all detectors to complete
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Collect and merge results
	combined := &ResourceAnalysis{
		EnvironmentVars: []EnvironmentVariable{},
		Secrets:         []Secret{},
		Databases:       []Database{},
		Queues:          []Queue{},
		Storage:         []Storage{},
		ExternalAPIs:    []ExternalAPI{},
	}

	for res := range resultChan {
		if res.err != nil {
			continue // Skip failed detectors, continue with others
		}

		// Merge results
		combined.EnvironmentVars = append(combined.EnvironmentVars, res.analysis.EnvironmentVars...)
		combined.Secrets = append(combined.Secrets, res.analysis.Secrets...)
		combined.Databases = append(combined.Databases, res.analysis.Databases...)
		combined.Queues = append(combined.Queues, res.analysis.Queues...)
		combined.Storage = append(combined.Storage, res.analysis.Storage...)
		combined.ExternalAPIs = append(combined.ExternalAPIs, res.analysis.ExternalAPIs...)
	}

	return combined, nil
}
