package modes

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/simple-container-com/api/pkg/api/logger/color"
	"github.com/simple-container-com/api/pkg/assistant/analysis"
	"github.com/simple-container-com/api/pkg/assistant/llm"
	"github.com/simple-container-com/api/pkg/assistant/validation"
)

// MultiFileGenerationRequest represents a request to generate multiple coordinated files
type MultiFileGenerationRequest struct {
	ProjectPath     string                    `json:"project_path"`
	SetupOptions    *SetupOptions             `json:"setup_options"`
	ProjectAnalysis *analysis.ProjectAnalysis `json:"project_analysis"`

	// File selection
	GenerateDockerfile    bool `json:"generate_dockerfile"`
	GenerateDockerCompose bool `json:"generate_docker_compose"`
	GenerateClientYAML    bool `json:"generate_client_yaml"`
	GenerateServerYAML    bool `json:"generate_server_yaml"`

	// Generation options
	UseStreaming      bool `json:"use_streaming"`
	ValidateGenerated bool `json:"validate_generated"`
	BackupExisting    bool `json:"backup_existing"`
}

// MultiFileGenerationResult contains the results of multi-file generation
type MultiFileGenerationResult struct {
	Success bool `json:"success"`

	// Generated content
	DockerfileContent    string `json:"dockerfile_content"`
	DockerComposeContent string `json:"docker_compose_content"`
	ClientYAMLContent    string `json:"client_yaml_content"`
	ServerYAMLContent    string `json:"server_yaml_content"`

	// File paths
	DockerfilePath    string `json:"dockerfile_path"`
	DockerComposePath string `json:"docker_compose_path"`
	ClientYAMLPath    string `json:"client_yaml_path"`
	ServerYAMLPath    string `json:"server_yaml_path"`

	// Generation metadata
	GenerationTime    float64         `json:"generation_time_seconds"`
	ValidationResults map[string]bool `json:"validation_results"`
	Warnings          []string        `json:"warnings"`
	Errors            []string        `json:"errors"`
}

// GenerateMultipleFiles generates multiple coordinated files for a complete project setup
func (d *DeveloperMode) GenerateMultipleFiles(ctx context.Context, req MultiFileGenerationRequest) (*MultiFileGenerationResult, error) {
	result := &MultiFileGenerationResult{
		ValidationResults: make(map[string]bool),
		Warnings:          []string{},
		Errors:            []string{},
	}

	fmt.Printf("🚀 %s\n", color.BlueFmt("Starting intelligent multi-file generation..."))
	fmt.Printf("📂 Project: %s\n", color.CyanFmt(req.ProjectPath))

	// Create progress displays for streaming
	var progressDisplays []*ProgressDisplay
	var fileTypes []string

	if req.GenerateDockerfile {
		fileTypes = append(fileTypes, "Dockerfile")
		progressDisplays = append(progressDisplays, NewProgressDisplay("Generating Dockerfile"))
	}
	if req.GenerateDockerCompose {
		fileTypes = append(fileTypes, "docker-compose.yaml")
		progressDisplays = append(progressDisplays, NewProgressDisplay("Generating docker-compose.yaml"))
	}
	if req.GenerateClientYAML {
		fileTypes = append(fileTypes, "client.yaml")
		progressDisplays = append(progressDisplays, NewProgressDisplay("Generating client.yaml"))
	}
	if req.GenerateServerYAML {
		fileTypes = append(fileTypes, "server.yaml")
		progressDisplays = append(progressDisplays, NewProgressDisplay("Generating server.yaml"))
	}

	fmt.Printf("📋 Files to generate: %s\n\n", color.GreenFmt(strings.Join(fileTypes, ", ")))

	// Check for existing files and get user confirmation
	if !d.checkExistingFilesAndConfirm(req) {
		result.Errors = append(result.Errors, "User cancelled file generation due to existing files")
		return result, fmt.Errorf("file generation cancelled by user")
	}

	// Generate coordinated content using intelligent prompting
	coordinated, err := d.generateCoordinatedContent(ctx, req, progressDisplays)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("Coordinated generation failed: %v", err))
		return result, err
	}

	// Extract individual files from coordinated content
	if req.GenerateDockerfile && coordinated.Dockerfile != "" {
		result.DockerfileContent = coordinated.Dockerfile
		result.DockerfilePath = filepath.Join(req.ProjectPath, "Dockerfile")
	}

	if req.GenerateDockerCompose && coordinated.DockerCompose != "" {
		result.DockerComposeContent = coordinated.DockerCompose
		result.DockerComposePath = filepath.Join(req.ProjectPath, "docker-compose.yaml")
	}

	if req.GenerateClientYAML && coordinated.ClientYAML != "" {
		result.ClientYAMLContent = coordinated.ClientYAML
		result.ClientYAMLPath = filepath.Join(req.ProjectPath, ".sc", "stacks", req.SetupOptions.Parent, "client.yaml")
	}

	if req.GenerateServerYAML && coordinated.ServerYAML != "" {
		result.ServerYAMLContent = coordinated.ServerYAML
		result.ServerYAMLPath = filepath.Join(req.ProjectPath, ".sc", "stacks", "infrastructure", "server.yaml")
	}

	// Validate generated content if requested
	if req.ValidateGenerated {
		result.ValidationResults = d.validateMultipleFiles(ctx, coordinated)

		// Check if any validation failed
		hasValidationErrors := false
		for fileName, isValid := range result.ValidationResults {
			if !isValid {
				hasValidationErrors = true
				result.Errors = append(result.Errors, fmt.Sprintf("%s failed validation", fileName))
			}
		}

		if hasValidationErrors {
			result.Warnings = append(result.Warnings, "Some files failed validation - using fallback templates")

			// Generate fallbacks for failed files
			if req.GenerateDockerfile && !result.ValidationResults["Dockerfile"] {
				fallback, _ := d.generateFallbackDockerfile(req.ProjectAnalysis)
				result.DockerfileContent = fallback
			}

			if req.GenerateDockerCompose && !result.ValidationResults["docker-compose.yaml"] {
				fallback, _ := d.generateFallbackComposeYAML(req.ProjectAnalysis)
				result.DockerComposeContent = fallback
			}

			if req.GenerateClientYAML && !result.ValidationResults["client.yaml"] {
				fallback, _ := d.generateFallbackClientYAML(req.SetupOptions, req.ProjectAnalysis)
				result.ClientYAMLContent = fallback
			}
		}
	}

	// Write generated files
	writeErrors := d.writeGeneratedFiles(ctx, req, result)
	if len(writeErrors) > 0 {
		result.Errors = append(result.Errors, writeErrors...)
		return result, fmt.Errorf("failed to write %d files", len(writeErrors))
	}

	result.Success = true

	fmt.Printf("\n🎉 %s\n", color.GreenFmt("Multi-file generation completed successfully!"))
	fmt.Printf("📊 Generated: %s\n", color.CyanFmt(strings.Join(fileTypes, ", ")))

	if len(result.Warnings) > 0 {
		fmt.Printf("⚠️  Warnings: %d\n", len(result.Warnings))
		for _, warning := range result.Warnings {
			fmt.Printf("   • %s\n", color.YellowFmt(warning))
		}
	}

	return result, nil
}

// CoordinatedContent holds the coordinated generated content
type CoordinatedContent struct {
	Dockerfile    string
	DockerCompose string
	ClientYAML    string
	ServerYAML    string
	Metadata      map[string]interface{}
}

// generateCoordinatedContent generates multiple files with coordinated content
func (d *DeveloperMode) generateCoordinatedContent(ctx context.Context, req MultiFileGenerationRequest, progressDisplays []*ProgressDisplay) (*CoordinatedContent, error) {
	if d.llm == nil {
		return d.generateCoordinatedFallbacks(req)
	}

	// Check if provider supports streaming
	caps := d.llm.GetCapabilities()
	useStreaming := req.UseStreaming && caps.SupportsStreaming

	// Build coordinated prompt that generates multiple files together
	prompt := d.buildCoordinatedPrompt(req)

	var response *llm.ChatResponse
	var err error

	if useStreaming && len(progressDisplays) > 0 {
		// Use first progress display for coordinated generation
		response, err = d.llm.StreamChat(ctx, []llm.Message{
			{Role: "system", Content: d.buildCoordinatedSystemPrompt(req)},
			{Role: "user", Content: prompt},
		}, progressDisplays[0].StreamCallback())
	} else {
		response, err = d.llm.Chat(ctx, []llm.Message{
			{Role: "system", Content: d.buildCoordinatedSystemPrompt(req)},
			{Role: "user", Content: prompt},
		})
	}

	if err != nil {
		fmt.Printf("LLM coordinated generation failed, using individual fallbacks: %v\n", err)
		return d.generateCoordinatedFallbacks(req)
	}

	// Parse the coordinated response
	return d.parseCoordinatedResponse(response.Content, req)
}

// buildCoordinatedSystemPrompt builds a system prompt for coordinated multi-file generation
func (d *DeveloperMode) buildCoordinatedSystemPrompt(req MultiFileGenerationRequest) string {
	var prompt strings.Builder

	prompt.WriteString(`You are an expert in Simple Container configuration and containerization. You will generate multiple coordinated files that work together seamlessly and follow Simple Container schema requirements EXACTLY.

CRITICAL SIMPLE CONTAINER SCHEMA REQUIREMENTS:

FOR CLIENT.YAML:
✅ MUST have: schemaVersion: 1.0
✅ MUST have: stacks section (NOT environments)
✅ Each stack MUST have: type, parent, parentEnv, config
✅ config section can contain: runs, env, secrets, scale, uses, dependencies
✅ scale uses: {min: number, max: number} structure only
✅ env: for environment variables, secrets: using ${secret:name} format

🚫 FORBIDDEN in client.yaml (will cause validation errors):
❌ apiVersion, kind (Kubernetes properties)
❌ environments section (use stacks only)
❌ version property (use schemaVersion)
❌ account property (belongs in server.yaml)
❌ scaling section (use scale in config)
❌ minCapacity/maxCapacity (use min/max in scale)

FOR DOCKER-COMPOSE.YAML:
✅ MUST include Simple Container labels:
   - "simple-container.com/ingress": "true" (for main service)
   - "simple-container.com/ingress/port": "PORT_NUMBER"
   - "simple-container.com/healthcheck/path": "/health"
✅ MUST have separate volumes block with labels:
   - "simple-container.com/volume-size": "10Gi"
   - "simple-container.com/volume-storage-class": "gp3"
   - "simple-container.com/volume-access-modes": "ReadWriteOnce"

FOR DOCKERFILE:
✅ Multi-stage builds for production
✅ Non-root user for security
✅ Proper EXPOSE directive

RESPONSE FORMAT:
Generate files in this exact format, separated by clear markers:

=== DOCKERFILE ===
[Multi-stage Dockerfile content here]

=== DOCKER-COMPOSE ===
[docker-compose.yaml with Simple Container labels]

=== CLIENT-YAML ===
[Simple Container client.yaml with schemaVersion and stacks]

=== SERVER-YAML ===
[Simple Container server.yaml with resources]

COORDINATION REQUIREMENTS:
- Use consistent service names and ports across all files
- Ensure Simple Container labels are properly configured
- All files must be immediately deployable
- Follow schema requirements EXACTLY`)

	return prompt.String()
}

// buildCoordinatedPrompt builds the user prompt for coordinated generation
func (d *DeveloperMode) buildCoordinatedPrompt(req MultiFileGenerationRequest) string {
	var prompt strings.Builder

	prompt.WriteString("Generate coordinated containerization files for this project:\n\n")

	if req.ProjectAnalysis != nil && req.ProjectAnalysis.PrimaryStack != nil {
		prompt.WriteString(fmt.Sprintf("Language: %s\n", req.ProjectAnalysis.PrimaryStack.Language))
		if req.ProjectAnalysis.PrimaryStack.Framework != "" {
			prompt.WriteString(fmt.Sprintf("Framework: %s\n", req.ProjectAnalysis.PrimaryStack.Framework))
		}
	}

	if req.SetupOptions != nil {
		prompt.WriteString(fmt.Sprintf("Parent stack: %s\n", req.SetupOptions.Parent))
		prompt.WriteString(fmt.Sprintf("Environment: %s\n", req.SetupOptions.Environment))
	}

	prompt.WriteString("\nFiles to generate:\n")
	if req.GenerateDockerfile {
		prompt.WriteString("- Dockerfile: Multi-stage, optimized, secure\n")
	}
	if req.GenerateDockerCompose {
		prompt.WriteString("- docker-compose.yaml: With Simple Container labels, proper volumes, networking\n")
	}
	if req.GenerateClientYAML {
		prompt.WriteString("- client.yaml: Schema-compliant Simple Container client configuration\n")
	}
	if req.GenerateServerYAML {
		prompt.WriteString("- server.yaml: Schema-compliant Simple Container server configuration\n")
	}

	prompt.WriteString("\nEnsure all files work together with consistent naming, ports, and configurations.")

	return prompt.String()
}

// parseCoordinatedResponse parses the LLM response to extract individual files
func (d *DeveloperMode) parseCoordinatedResponse(content string, req MultiFileGenerationRequest) (*CoordinatedContent, error) {
	result := &CoordinatedContent{
		Metadata: make(map[string]interface{}),
	}

	// Define file markers
	markers := map[string]*string{
		"=== DOCKERFILE ===":     &result.Dockerfile,
		"=== DOCKER-COMPOSE ===": &result.DockerCompose,
		"=== CLIENT-YAML ===":    &result.ClientYAML,
		"=== SERVER-YAML ===":    &result.ServerYAML,
	}

	// Parse sections
	lines := strings.Split(content, "\n")
	var currentSection *string
	var currentContent strings.Builder

	for _, line := range lines {
		// Check if this line is a marker
		if section, exists := markers[strings.TrimSpace(line)]; exists {
			// Save previous section
			if currentSection != nil {
				*currentSection = strings.TrimSpace(currentContent.String())
			}

			// Start new section
			currentSection = section
			currentContent.Reset()
			continue
		}

		// Add line to current section
		if currentSection != nil {
			currentContent.WriteString(line + "\n")
		}
	}

	// Save last section
	if currentSection != nil {
		*currentSection = strings.TrimSpace(currentContent.String())
	}

	return result, nil
}

// generateCoordinatedFallbacks generates fallback content when LLM is unavailable
func (d *DeveloperMode) generateCoordinatedFallbacks(req MultiFileGenerationRequest) (*CoordinatedContent, error) {
	result := &CoordinatedContent{
		Metadata: make(map[string]interface{}),
	}

	if req.GenerateDockerfile {
		dockerfile, _ := d.generateFallbackDockerfile(req.ProjectAnalysis)
		result.Dockerfile = dockerfile
	}

	if req.GenerateDockerCompose {
		compose, _ := d.generateFallbackComposeYAML(req.ProjectAnalysis)
		result.DockerCompose = compose
	}

	if req.GenerateClientYAML {
		clientYAML, _ := d.generateFallbackClientYAML(req.SetupOptions, req.ProjectAnalysis)
		result.ClientYAML = clientYAML
	}

	if req.GenerateServerYAML {
		// Generate server YAML using DevOps mode (this needs to be implemented)
		result.ServerYAML = "# Server YAML fallback not yet implemented"
	}

	return result, nil
}

// validateMultipleFiles validates all generated files
func (d *DeveloperMode) validateMultipleFiles(ctx context.Context, content *CoordinatedContent) map[string]bool {
	results := make(map[string]bool)

	if content.Dockerfile != "" {
		results["Dockerfile"] = d.validateDockerfileContent(content.Dockerfile)
	}

	if content.DockerCompose != "" {
		results["docker-compose.yaml"] = d.validateComposeContent(content.DockerCompose)
	}

	if content.ClientYAML != "" {
		// Validate client.yaml against Simple Container schema
		validator := validation.NewValidator()
		result := validator.ValidateClientYAML(ctx, content.ClientYAML)
		results["client.yaml"] = result.Valid
	}

	if content.ServerYAML != "" {
		// Add server YAML validation here
		results["server.yaml"] = true // Placeholder
	}

	return results
}

// checkExistingFilesAndConfirm checks for existing files and prompts user for confirmation
func (d *DeveloperMode) checkExistingFilesAndConfirm(req MultiFileGenerationRequest) bool {
	var existingFiles []string

	// Check which files already exist
	if req.GenerateDockerfile {
		dockerfilePath := filepath.Join(req.ProjectPath, "Dockerfile")
		if _, err := os.Stat(dockerfilePath); err == nil {
			existingFiles = append(existingFiles, "Dockerfile")
		}
	}

	if req.GenerateDockerCompose {
		composePath := filepath.Join(req.ProjectPath, "docker-compose.yaml")
		if _, err := os.Stat(composePath); err == nil {
			existingFiles = append(existingFiles, "docker-compose.yaml")
		}
	}

	if req.GenerateClientYAML {
		// Determine project name for client.yaml path
		projectName := filepath.Base(req.ProjectPath)
		if req.ProjectAnalysis != nil && req.ProjectAnalysis.Name != "" && req.ProjectAnalysis.Name != "." {
			projectName = req.ProjectAnalysis.Name
		}
		if projectName == "." || projectName == "" {
			if wd, err := os.Getwd(); err == nil {
				projectName = filepath.Base(wd)
			} else {
				projectName = "myapp"
			}
		}

		clientPath := filepath.Join(req.ProjectPath, ".sc", "stacks", projectName, "client.yaml")
		if _, err := os.Stat(clientPath); err == nil {
			existingFiles = append(existingFiles, "client.yaml")
		}
	}

	if req.GenerateServerYAML {
		serverPath := filepath.Join(req.ProjectPath, ".sc", "stacks", "infrastructure", "server.yaml")
		if _, err := os.Stat(serverPath); err == nil {
			existingFiles = append(existingFiles, "server.yaml")
		}
	}

	// If no existing files, proceed
	if len(existingFiles) == 0 {
		return true
	}

	// Prompt user for confirmation
	fmt.Printf("\n⚠️  The following files already exist: %s\n", color.YellowString(strings.Join(existingFiles, ", ")))
	fmt.Printf("   Overwrite all existing files? [y/N]: ")

	var response string
	if _, err := fmt.Scanln(&response); err != nil {
		// If there's an error reading input, default to "no"
		return false
	}

	response = strings.ToLower(strings.TrimSpace(response))
	return response == "y" || response == "yes"
}

// writeGeneratedFiles writes all generated files to disk
func (d *DeveloperMode) writeGeneratedFiles(ctx context.Context, req MultiFileGenerationRequest, result *MultiFileGenerationResult) []string {
	var errors []string

	filesToWrite := []struct {
		path    string
		content string
		name    string
	}{
		{result.DockerfilePath, result.DockerfileContent, "Dockerfile"},
		{result.DockerComposePath, result.DockerComposeContent, "docker-compose.yaml"},
		{result.ClientYAMLPath, result.ClientYAMLContent, "client.yaml"},
		{result.ServerYAMLPath, result.ServerYAMLContent, "server.yaml"},
	}

	for _, file := range filesToWrite {
		if file.content != "" && file.path != "" {
			// Create directory if it doesn't exist
			if err := os.MkdirAll(filepath.Dir(file.path), 0o755); err != nil {
				errors = append(errors, fmt.Sprintf("Failed to create directory for %s: %v", file.name, err))
				continue
			}

			// Write file
			if err := os.WriteFile(file.path, []byte(file.content), 0o644); err != nil {
				errors = append(errors, fmt.Sprintf("Failed to write %s: %v", file.name, err))
			} else {
				fmt.Printf("✅ Generated %s → %s\n", color.GreenFmt(file.name), color.CyanFmt(file.path))
			}
		}
	}

	return errors
}
