package generation

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/simple-container-com/api/pkg/assistant/analysis"
	"github.com/simple-container-com/api/pkg/assistant/modes"
)

// FileGenerator handles configuration file generation
type FileGenerator struct {
	devMode *modes.DeveloperMode
}

// NewFileGenerator creates a new file generator instance
func NewFileGenerator() *FileGenerator {
	return &FileGenerator{
		devMode: modes.NewDeveloperMode(),
	}
}

// GenerateOptions contains options for file generation
type GenerateOptions struct {
	ProjectPath   string
	ProjectName   string
	Environment   string
	Parent        string
	CloudProvider string
	OutputDir     string
	SkipExisting  bool
}

// GenerateClientYAML generates Simple Container client configuration
func (g *FileGenerator) GenerateClientYAML(analysis *analysis.ProjectAnalysis, opts GenerateOptions) (string, error) {
	projectName := opts.ProjectName
	if projectName == "" {
		projectName = analysis.Name
	}

	// Extract resources from analysis recommendations
	resources := g.extractRecommendedResources(analysis)

	// Generate environment variables based on detected tech stack
	envVars := g.generateEnvironmentVariables(analysis)

	clientYaml := fmt.Sprintf(`schemaVersion: 1.0

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
      env:%s
        
      # Health check configuration  
      healthCheck:
        path: "/health"
        port: 3000
        initialDelaySeconds: 30
        periodSeconds: 10
        
      # Secrets
      secrets:
        JWT_SECRET: "${secret:jwt-secret}"`,
		projectName, opts.Parent, opts.Environment, resources, envVars)

	return clientYaml, nil
}

// GenerateDockerCompose generates docker-compose.yaml for local development
func (g *FileGenerator) GenerateDockerCompose(analysis *analysis.ProjectAnalysis, opts GenerateOptions) (string, error) {
	// Use the LLM-based generation from DeveloperMode
	content, err := g.devMode.GenerateComposeYAMLWithLLM(analysis)
	if err != nil {
		return "", fmt.Errorf("failed to generate docker-compose.yaml: %w", err)
	}
	return content, nil
}

// GenerateDockerfile generates optimized Dockerfile based on tech stack
func (g *FileGenerator) GenerateDockerfile(analysis *analysis.ProjectAnalysis, opts GenerateOptions) (string, error) {
	// Use the LLM-based generation from DeveloperMode
	content, err := g.devMode.GenerateDockerfileWithLLM(analysis)
	if err != nil {
		return "", fmt.Errorf("failed to generate Dockerfile: %w", err)
	}
	return content, nil
}

// Helper methods

func (g *FileGenerator) extractRecommendedResources(analysis *analysis.ProjectAnalysis) []string {
	resources := []string{}

	if analysis == nil {
		return []string{"postgres-db"} // Default
	}

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

	// Default to postgres if no resources detected
	if len(resources) == 0 {
		resources = []string{"postgres-db"}
	}

	return resources
}

func (g *FileGenerator) generateEnvironmentVariables(analysis *analysis.ProjectAnalysis) string {
	if analysis == nil || analysis.PrimaryStack == nil {
		return `
        NODE_ENV: production
        PORT: 3000
        DATABASE_URL: "${resource:postgres-db.connectionString}"`
	}

	switch analysis.PrimaryStack.Language {
	case "javascript":
		return `
        NODE_ENV: production
        PORT: 3000
        DATABASE_URL: "${resource:postgres-db.connectionString}"
        REDIS_URL: "${resource:redis-cache.connectionString}"`
	case "python":
		return `
        DJANGO_SETTINGS_MODULE: app.settings.production
        DATABASE_URL: "${resource:postgres-db.connectionString}"
        REDIS_URL: "${resource:redis-cache.connectionString}"
        DJANGO_SECRET_KEY: "${secret:django-secret-key}"`
	case "go":
		return `
        GO_ENV: production
        PORT: 8080
        DATABASE_URL: "${resource:postgres-db.connectionString}"
        REDIS_URL: "${resource:redis-cache.connectionString}"`
	default:
		return `
        NODE_ENV: production
        PORT: 3000
        DATABASE_URL: "${resource:postgres-db.connectionString}"`
	}
}

// WriteFile writes content to a file, creating directories as needed
func (g *FileGenerator) WriteFile(path, content string, overwrite bool) error {
	// Check if file exists and overwrite flag
	if _, err := os.Stat(path); err == nil && !overwrite {
		return fmt.Errorf("file already exists: %s (use overwrite flag to replace)", path)
	}

	// Create directory if needed
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Write file
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}
