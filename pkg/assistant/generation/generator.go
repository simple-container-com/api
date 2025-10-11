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

// NewFileGeneratorWithMode creates a file generator with existing DeveloperMode (for reuse)
func NewFileGeneratorWithMode(devMode *modes.DeveloperMode) *FileGenerator {
	return &FileGenerator{
		devMode: devMode,
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
	// Convert GenerateOptions to SetupOptions for DeveloperMode
	setupOpts := &modes.SetupOptions{
		Parent:      opts.Parent,
		Environment: opts.Environment,
		OutputDir:   opts.ProjectPath,
	}

	// Use the improved LLM-based generation from DeveloperMode
	content, err := g.devMode.GenerateClientYAMLWithLLM(setupOpts, analysis)
	if err != nil {
		return "", fmt.Errorf("failed to generate client.yaml: %w", err)
	}
	return content, nil
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
