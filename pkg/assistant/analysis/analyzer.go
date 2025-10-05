package analysis

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ProjectAnalyzer orchestrates tech stack detection and generates recommendations
type ProjectAnalyzer struct {
	detectors []TechStackDetector
}

// NewProjectAnalyzer creates a new analyzer with default detectors
func NewProjectAnalyzer() *ProjectAnalyzer {
	return &ProjectAnalyzer{
		detectors: []TechStackDetector{
			&NodeJSDetector{},
			&PythonDetector{},
			&GoDetector{},
			&DockerDetector{},
		},
	}
}

// AddDetector adds a custom detector
func (pa *ProjectAnalyzer) AddDetector(detector TechStackDetector) {
	pa.detectors = append(pa.detectors, detector)

	// Re-sort by priority
	sort.Slice(pa.detectors, func(i, j int) bool {
		return pa.detectors[i].Priority() > pa.detectors[j].Priority()
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

	// Run all detectors
	var detectedStacks []TechStackInfo
	for _, detector := range pa.detectors {
		if stack, err := detector.Detect(projectPath); err == nil {
			detectedStacks = append(detectedStacks, *stack)
		}
	}

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

	// Generate recommendations
	analysis.Recommendations = pa.generateRecommendations(analysis)

	// Analyze file structure
	files, err := pa.analyzeFiles(projectPath)
	if err == nil {
		analysis.Files = files
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
