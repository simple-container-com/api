package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/simple-container-com/api/pkg/assistant/analysis"
	"github.com/simple-container-com/api/pkg/assistant/embeddings"
	"github.com/simple-container-com/api/pkg/assistant/modes"
)

// UnifiedCommandHandler provides a shared layer for both MCP and chat interfaces
type UnifiedCommandHandler struct {
	embeddingsDB  *embeddings.Database
	analyzer      *analysis.ProjectAnalyzer
	developerMode *modes.DeveloperMode
}

// CommandResult represents the result of any command execution
type CommandResult struct {
	Success  bool                   `json:"success"`
	Message  string                 `json:"message"`
	Data     map[string]interface{} `json:"data,omitempty"`
	Error    string                 `json:"error,omitempty"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// NewUnifiedCommandHandler creates a new unified command handler
func NewUnifiedCommandHandler() (*UnifiedCommandHandler, error) {
	// Initialize embeddings database
	db, err := embeddings.LoadEmbeddedDatabase(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to load embeddings database: %w", err)
	}

	return &UnifiedCommandHandler{
		embeddingsDB:  db,
		analyzer:      analysis.NewProjectAnalyzer(),
		developerMode: modes.NewDeveloperMode(),
	}, nil
}

// SearchDocumentation searches Simple Container documentation
func (h *UnifiedCommandHandler) SearchDocumentation(ctx context.Context, query string, limit int) (*CommandResult, error) {
	results, err := embeddings.SearchDocumentation(h.embeddingsDB, query, limit)
	if err != nil {
		return &CommandResult{
			Success: false,
			Message: "Failed to search documentation",
			Error:   err.Error(),
		}, err
	}

	message := fmt.Sprintf("🔍 Found %d documentation results for '%s'", len(results), query)
	data := map[string]interface{}{
		"query":   query,
		"results": results,
		"count":   len(results),
	}

	return &CommandResult{
		Success: true,
		Message: message,
		Data:    data,
	}, nil
}

// GetProjectContext returns basic project information and Simple Container config status
func (h *UnifiedCommandHandler) GetProjectContext(ctx context.Context, path string) (*CommandResult, error) {
	if path == "" {
		path = "."
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return &CommandResult{
			Success: false,
			Message: "Failed to resolve project path",
			Error:   err.Error(),
		}, err
	}

	// Check if path exists
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("Project path does not exist: %s", absPath),
			Error:   "path_not_found",
		}, err
	}

	// Analyze project structure
	projectInfo, err := h.analyzer.AnalyzeProject(absPath)
	if err != nil {
		return &CommandResult{
			Success: false,
			Message: "Failed to analyze project",
			Error:   err.Error(),
		}, err
	}

	// Check for existing Simple Container configuration
	clientConfig := h.findClientYaml(absPath)
	serverConfig := h.findServerYaml(absPath)

	message := fmt.Sprintf("📁 Project: %s\n", projectInfo.Name)
	message += fmt.Sprintf("🗂️ Path: %s\n", absPath)

	if projectInfo.PrimaryStack != nil {
		message += fmt.Sprintf("💻 Primary Stack: %s (%s)\n",
			projectInfo.PrimaryStack.Language,
			projectInfo.PrimaryStack.Framework)
	}

	if clientConfig != "" {
		message += fmt.Sprintf("✅ Client config: %s\n", clientConfig)
	} else {
		message += "⚠️ No client.yaml found\n"
	}

	if serverConfig != "" {
		message += fmt.Sprintf("✅ Server config: %s\n", serverConfig)
	}

	data := map[string]interface{}{
		"project_info":      projectInfo,
		"absolute_path":     absPath,
		"client_config":     clientConfig,
		"server_config":     serverConfig,
		"has_client_config": clientConfig != "",
		"has_server_config": serverConfig != "",
	}

	return &CommandResult{
		Success: true,
		Message: message,
		Data:    data,
	}, nil
}

// AnalyzeProject performs detailed tech stack analysis
func (h *UnifiedCommandHandler) AnalyzeProject(ctx context.Context, path string, withLLM bool) (*CommandResult, error) {
	if path == "" {
		path = "."
	}

	projectInfo, err := h.analyzer.AnalyzeProject(path)
	if err != nil {
		return &CommandResult{
			Success: false,
			Message: "Failed to analyze project",
			Error:   err.Error(),
		}, err
	}

	message := fmt.Sprintf("🔍 Project Analysis: %s\n", projectInfo.Name)

	if projectInfo.PrimaryStack != nil {
		message += fmt.Sprintf("💻 Primary: %s %s\n",
			projectInfo.PrimaryStack.Language,
			projectInfo.PrimaryStack.Framework)
	}

	if len(projectInfo.TechStacks) > 1 {
		message += fmt.Sprintf("🔧 Additional stacks: %d\n", len(projectInfo.TechStacks)-1)
	}

	message += fmt.Sprintf("📄 Files analyzed: %d\n", len(projectInfo.Files))

	// Determine recommended deployment type based on analysis
	recommendedDeployment := "cloud-compose" // default
	if projectInfo.PrimaryStack != nil {
		switch projectInfo.PrimaryStack.Language {
		case "html", "css", "javascript":
			if len(projectInfo.Files) < 10 {
				recommendedDeployment = "static"
			}
		case "go", "python", "nodejs":
			if strings.Contains(strings.ToLower(projectInfo.Architecture), "lambda") ||
				strings.Contains(strings.ToLower(projectInfo.Architecture), "serverless") {
				recommendedDeployment = "single-image"
			}
		}
	}
	message += fmt.Sprintf("🎯 Recommended deployment: %s", recommendedDeployment)

	data := map[string]interface{}{
		"analysis":    projectInfo,
		"with_llm":    withLLM,
		"analyzed_at": time.Now(),
	}

	return &CommandResult{
		Success: true,
		Message: message,
		Data:    data,
	}, nil
}

// SetupSimpleContainer initializes Simple Container configuration
func (h *UnifiedCommandHandler) SetupSimpleContainer(ctx context.Context, path, environment, parent, deploymentType string, interactive bool) (*CommandResult, error) {
	if path == "" {
		path = "."
	}

	setupOptions := &modes.SetupOptions{
		Interactive:    interactive,
		Environment:    environment,
		Parent:         parent,
		DeploymentType: deploymentType,
		OutputDir:      path,
	}

	err := h.developerMode.Setup(ctx, setupOptions)
	if err != nil {
		return &CommandResult{
			Success: false,
			Message: "Failed to setup Simple Container configuration",
			Error:   err.Error(),
		}, err
	}

	// Detect generated files
	filesCreated := h.detectGeneratedFiles(path)

	message := "✅ Simple Container setup completed successfully!\n"
	message += fmt.Sprintf("📁 Project path: %s\n", path)
	message += fmt.Sprintf("🌍 Environment: %s\n", environment)
	if parent != "" {
		message += fmt.Sprintf("👨‍👩‍👧‍👦 Parent stack: %s\n", parent)
	}
	message += fmt.Sprintf("📄 Files created: %v", filesCreated)

	data := map[string]interface{}{
		"path":            path,
		"environment":     environment,
		"parent":          parent,
		"deployment_type": deploymentType,
		"interactive":     interactive,
		"files_created":   filesCreated,
		"setup_time":      time.Now(),
	}

	return &CommandResult{
		Success: true,
		Message: message,
		Data:    data,
	}, nil
}

// GetCurrentConfig reads and parses existing configuration files
func (h *UnifiedCommandHandler) GetCurrentConfig(ctx context.Context, configType, stackName string) (*CommandResult, error) {
	var filePath string
	var content map[string]interface{}

	switch configType {
	case "client":
		filePath = h.findClientYaml(".")
		if filePath == "" {
			return &CommandResult{
				Success: false,
				Message: "❌ No client.yaml found. Use setup_simple_container to create initial configuration.",
				Error:   "client_yaml_not_found",
			}, nil
		}

		yamlContent, err := h.readYamlFile(filePath)
		if err != nil {
			return &CommandResult{
				Success: false,
				Message: fmt.Sprintf("❌ Failed to read client.yaml: %v", err),
				Error:   err.Error(),
			}, err
		}
		content = yamlContent

	case "server":
		filePath = h.findServerYaml(".")
		if filePath == "" {
			return &CommandResult{
				Success: false,
				Message: "❌ No server.yaml found. This appears to be a client project, not a DevOps infrastructure project.",
				Error:   "server_yaml_not_found",
			}, nil
		}

		yamlContent, err := h.readYamlFile(filePath)
		if err != nil {
			return &CommandResult{
				Success: false,
				Message: fmt.Sprintf("❌ Failed to read server.yaml: %v", err),
				Error:   err.Error(),
			}, err
		}
		content = yamlContent

	default:
		return &CommandResult{
			Success: false,
			Message: "❌ Invalid config_type. Use 'client' or 'server'.",
			Error:   "invalid_config_type",
		}, fmt.Errorf("invalid config_type: %s", configType)
	}

	message := fmt.Sprintf("✅ Successfully read %s configuration\n", configType)
	message += fmt.Sprintf("📁 File: %s\n", filePath)

	data := map[string]interface{}{
		"config_type": configType,
		"file_path":   filePath,
		"content":     content,
		"stack_name":  stackName,
	}

	if configType == "client" {
		if stacks, ok := content["stacks"].(map[string]interface{}); ok {
			stackNames := h.getStackNames(stacks)
			message += fmt.Sprintf("📋 Found %d stacks: %v\n", len(stacks), stackNames)
			data["stack_names"] = stackNames
			data["stack_count"] = len(stacks)

			if stackName != "" {
				if stackConfig, exists := stacks[stackName]; exists {
					message += fmt.Sprintf("🎯 Focused on stack '%s'", stackName)
					data["focused_stack_config"] = stackConfig
				} else {
					message += fmt.Sprintf("⚠️ Stack '%s' not found. Available: %v", stackName, stackNames)
				}
			}
		}
	} else if configType == "server" {
		if resources, ok := content["resources"].(map[string]interface{}); ok {
			envs := make([]string, 0)
			for env := range resources {
				envs = append(envs, env)
			}
			message += fmt.Sprintf("🌍 Found %d environments: %v", len(envs), envs)
			data["environments"] = envs
			data["environment_count"] = len(envs)
		}
	}

	return &CommandResult{
		Success: true,
		Message: message,
		Data:    data,
	}, nil
}

// AddEnvironment adds a new environment/stack to client.yaml
func (h *UnifiedCommandHandler) AddEnvironment(ctx context.Context, stackName, deploymentType, parent, parentEnv string, config map[string]interface{}) (*CommandResult, error) {
	filePath := h.findClientYaml(".")
	if filePath == "" {
		return &CommandResult{
			Success: false,
			Message: "❌ No client.yaml found. Use setup_simple_container to create initial configuration.",
			Error:   "client_yaml_not_found",
		}, fmt.Errorf("client.yaml not found")
	}

	// Create backup
	backupPath, err := h.createBackup(filePath)
	if err != nil {
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("❌ Failed to create backup: %v", err),
			Error:   err.Error(),
		}, err
	}

	// Read current configuration
	content, err := h.readYamlFile(filePath)
	if err != nil {
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("❌ Failed to read client.yaml: %v", err),
			Error:   err.Error(),
		}, err
	}

	// Ensure stacks section exists
	stacks, ok := content["stacks"].(map[string]interface{})
	if !ok {
		stacks = make(map[string]interface{})
		content["stacks"] = stacks
	}

	// Check if stack already exists
	if _, exists := stacks[stackName]; exists {
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("⚠️ Stack '%s' already exists. Use modify_stack_config to update it.", stackName),
			Error:   "stack_already_exists",
			Data: map[string]interface{}{
				"backup_path": backupPath,
			},
		}, nil
	}

	// Create new stack configuration
	stackConfig := map[string]interface{}{
		"type":      deploymentType,
		"parent":    parent,
		"parentEnv": parentEnv,
		"config":    h.createDefaultStackConfig(deploymentType, config),
	}

	// Add the new stack
	stacks[stackName] = stackConfig

	// Write back to file
	err = h.writeYamlFile(filePath, content)
	if err != nil {
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("❌ Failed to write client.yaml: %v", err),
			Error:   err.Error(),
		}, err
	}

	message := fmt.Sprintf("✅ Successfully added '%s' environment to client.yaml\n", stackName)
	message += fmt.Sprintf("📁 File: %s\n", filePath)
	message += fmt.Sprintf("🎯 Type: %s\n", deploymentType)
	message += fmt.Sprintf("👨‍👩‍👧‍👦 Parent: %s -> %s\n", parent, parentEnv)
	message += fmt.Sprintf("💾 Backup: %s", backupPath)

	data := map[string]interface{}{
		"stack_name":   stackName,
		"file_path":    filePath,
		"config_added": stackConfig,
		"backup_path":  backupPath,
	}

	return &CommandResult{
		Success: true,
		Message: message,
		Data:    data,
	}, nil
}

// ModifyStackConfig modifies existing stack configuration in client.yaml
func (h *UnifiedCommandHandler) ModifyStackConfig(ctx context.Context, stackName string, changes map[string]interface{}) (*CommandResult, error) {
	filePath := h.findClientYaml(".")
	if filePath == "" {
		return &CommandResult{
			Success: false,
			Message: "❌ No client.yaml found. Use setup_simple_container to create initial configuration.",
			Error:   "client_yaml_not_found",
		}, fmt.Errorf("client.yaml not found")
	}

	// Create backup
	backupPath, err := h.createBackup(filePath)
	if err != nil {
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("❌ Failed to create backup: %v", err),
			Error:   err.Error(),
		}, err
	}

	// Read current configuration
	content, err := h.readYamlFile(filePath)
	if err != nil {
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("❌ Failed to read client.yaml: %v", err),
			Error:   err.Error(),
		}, err
	}

	// Find the stack
	stacks, ok := content["stacks"].(map[string]interface{})
	if !ok {
		return &CommandResult{
			Success: false,
			Message: "❌ No stacks found in client.yaml",
			Error:   "no_stacks_found",
		}, fmt.Errorf("no stacks section found")
	}

	stack, exists := stacks[stackName]
	if !exists {
		availableStacks := h.getStackNames(stacks)
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("❌ Stack '%s' not found. Available: %v", stackName, availableStacks),
			Error:   "stack_not_found",
			Data: map[string]interface{}{
				"available_stacks": availableStacks,
				"backup_path":      backupPath,
			},
		}, fmt.Errorf("stack not found: %s", stackName)
	}

	stackConfig, ok := stack.(map[string]interface{})
	if !ok {
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("❌ Invalid stack configuration for '%s'", stackName),
			Error:   "invalid_stack_config",
		}, fmt.Errorf("invalid stack config")
	}

	// Apply changes
	changesApplied := make(map[string]interface{})
	err = h.applyChangesToConfig(stackConfig, changes, "", changesApplied)
	if err != nil {
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("❌ Failed to apply changes: %v", err),
			Error:   err.Error(),
		}, err
	}

	// Write back to file
	err = h.writeYamlFile(filePath, content)
	if err != nil {
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("❌ Failed to write client.yaml: %v", err),
			Error:   err.Error(),
		}, err
	}

	message := fmt.Sprintf("✅ Successfully modified '%s' stack configuration\n", stackName)
	message += fmt.Sprintf("📁 File: %s\n", filePath)
	message += fmt.Sprintf("🔄 Changes applied: %+v\n", changesApplied)
	message += fmt.Sprintf("💾 Backup: %s", backupPath)

	data := map[string]interface{}{
		"stack_name":      stackName,
		"file_path":       filePath,
		"changes_applied": changesApplied,
		"backup_path":     backupPath,
	}

	return &CommandResult{
		Success: true,
		Message: message,
		Data:    data,
	}, nil
}

// AddResource adds a new resource to server.yaml
func (h *UnifiedCommandHandler) AddResource(ctx context.Context, resourceName, resourceType, environment string, config map[string]interface{}) (*CommandResult, error) {
	filePath := h.findServerYaml(".")
	if filePath == "" {
		return &CommandResult{
			Success: false,
			Message: "❌ No server.yaml found. This appears to be a client project, not a DevOps infrastructure project.",
			Error:   "server_yaml_not_found",
		}, fmt.Errorf("server.yaml not found")
	}

	// Create backup
	backupPath, err := h.createBackup(filePath)
	if err != nil {
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("❌ Failed to create backup: %v", err),
			Error:   err.Error(),
		}, err
	}

	// Read current configuration
	content, err := h.readYamlFile(filePath)
	if err != nil {
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("❌ Failed to read server.yaml: %v", err),
			Error:   err.Error(),
		}, err
	}

	// Ensure resources section exists
	resources, ok := content["resources"].(map[string]interface{})
	if !ok {
		resources = make(map[string]interface{})
		content["resources"] = resources
	}

	// Ensure environment section exists
	env, ok := resources[environment].(map[string]interface{})
	if !ok {
		env = make(map[string]interface{})
		resources[environment] = env
	}

	// Check if resource already exists
	if _, exists := env[resourceName]; exists {
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("⚠️ Resource '%s' already exists in '%s' environment", resourceName, environment),
			Error:   "resource_already_exists",
			Data: map[string]interface{}{
				"backup_path": backupPath,
			},
		}, nil
	}

	// Create resource configuration
	resourceConfig := map[string]interface{}{
		"type": resourceType,
	}

	// Add user-provided config
	for key, value := range config {
		resourceConfig[key] = value
	}

	// Add the new resource
	env[resourceName] = resourceConfig

	// Write back to file
	err = h.writeYamlFile(filePath, content)
	if err != nil {
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("❌ Failed to write server.yaml: %v", err),
			Error:   err.Error(),
		}, err
	}

	message := fmt.Sprintf("✅ Successfully added '%s' resource to '%s' environment\n", resourceName, environment)
	message += fmt.Sprintf("📁 File: %s\n", filePath)
	message += fmt.Sprintf("🗄️ Type: %s\n", resourceType)
	message += fmt.Sprintf("⚙️ Config: %+v\n", config)
	message += fmt.Sprintf("💾 Backup: %s", backupPath)

	data := map[string]interface{}{
		"resource_name": resourceName,
		"environment":   environment,
		"file_path":     filePath,
		"config_added":  resourceConfig,
		"backup_path":   backupPath,
	}

	return &CommandResult{
		Success: true,
		Message: message,
		Data:    data,
	}, nil
}

// Utility functions (moved from MCP server to shared location)

func (h *UnifiedCommandHandler) findClientYaml(basePath string) string {
	// Check current directory first
	if _, err := os.Stat(filepath.Join(basePath, "client.yaml")); err == nil {
		return filepath.Join(basePath, "client.yaml")
	}

	// Check in .sc/stacks subdirectories
	pattern := filepath.Join(basePath, ".sc/stacks/*/client.yaml")
	if entries, err := filepath.Glob(pattern); err == nil && len(entries) > 0 {
		return entries[0] // Return first match
	}

	return ""
}

func (h *UnifiedCommandHandler) findServerYaml(basePath string) string {
	// Check current directory first
	if _, err := os.Stat(filepath.Join(basePath, "server.yaml")); err == nil {
		return filepath.Join(basePath, "server.yaml")
	}

	return ""
}

func (h *UnifiedCommandHandler) readYamlFile(filePath string) (map[string]interface{}, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", filePath, err)
	}

	var content map[string]interface{}
	err = yaml.Unmarshal(data, &content)
	if err != nil {
		return nil, fmt.Errorf("failed to parse YAML in %s: %w", filePath, err)
	}

	return content, nil
}

func (h *UnifiedCommandHandler) writeYamlFile(filePath string, content map[string]interface{}) error {
	data, err := yaml.Marshal(content)
	if err != nil {
		return fmt.Errorf("failed to marshal YAML: %w", err)
	}

	err = os.WriteFile(filePath, data, 0o644)
	if err != nil {
		return fmt.Errorf("failed to write file %s: %w", filePath, err)
	}

	return nil
}

func (h *UnifiedCommandHandler) createBackup(filePath string) (string, error) {
	timestamp := time.Now().Format("20060102-150405")
	backupPath := filePath + ".backup." + timestamp

	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read original file: %w", err)
	}

	err = os.WriteFile(backupPath, data, 0o644)
	if err != nil {
		return "", fmt.Errorf("failed to create backup: %w", err)
	}

	return backupPath, nil
}

func (h *UnifiedCommandHandler) getStackNames(stacks map[string]interface{}) []string {
	names := make([]string, 0, len(stacks))
	for name := range stacks {
		names = append(names, name)
	}
	return names
}

func (h *UnifiedCommandHandler) createDefaultStackConfig(deploymentType string, additionalConfig map[string]interface{}) map[string]interface{} {
	config := make(map[string]interface{})

	switch deploymentType {
	case "static":
		config["bundleDir"] = "${git:root}/build"
		config["indexDocument"] = "index.html"
		config["errorDocument"] = "error.html"

	case "single-image":
		config["image"] = map[string]interface{}{
			"dockerfile": "${git:root}/Dockerfile",
		}
		config["timeout"] = 300

	case "cloud-compose":
		config["dockerComposeFile"] = "docker-compose.yaml"
		config["runs"] = []string{"app"}
		config["scale"] = map[string]interface{}{
			"min": 1,
			"max": 3,
		}
		config["env"] = map[string]interface{}{
			"NODE_ENV": "production",
		}
	}

	// Add additional config provided by user
	for key, value := range additionalConfig {
		config[key] = value
	}

	return config
}

func (h *UnifiedCommandHandler) applyChangesToConfig(config map[string]interface{}, changes map[string]interface{}, prefix string, changesApplied map[string]interface{}) error {
	for key, newValue := range changes {
		fullKey := key
		if prefix != "" {
			fullKey = prefix + "." + key
		}

		if strings.Contains(key, ".") {
			// Handle dot notation (e.g., "config.scale.max")
			parts := strings.Split(key, ".")
			current := config

			// Navigate to the nested object
			for i, part := range parts[:len(parts)-1] {
				if current[part] == nil {
					current[part] = make(map[string]interface{})
				}

				if nested, ok := current[part].(map[string]interface{}); ok {
					current = nested
				} else {
					return fmt.Errorf("cannot navigate to %s: %s is not an object", fullKey, strings.Join(parts[:i+1], "."))
				}
			}

			// Set the value
			finalKey := parts[len(parts)-1]
			oldValue := current[finalKey]
			current[finalKey] = newValue
			changesApplied[fullKey] = map[string]interface{}{
				"old": oldValue,
				"new": newValue,
			}
		} else {
			// Direct key
			oldValue := config[key]
			config[key] = newValue
			changesApplied[fullKey] = map[string]interface{}{
				"old": oldValue,
				"new": newValue,
			}
		}
	}

	return nil
}

func (h *UnifiedCommandHandler) detectGeneratedFiles(basePath string) []string {
	files := []string{}

	possibleFiles := []string{
		"client.yaml",
		"server.yaml",
		"docker-compose.yaml",
		"Dockerfile",
		".dockerignore",
	}

	for _, file := range possibleFiles {
		filePath := filepath.Join(basePath, file)
		if _, err := os.Stat(filePath); err == nil {
			files = append(files, file)
		}

		// Also check in .sc/stacks subdirectories for client.yaml
		if file == "client.yaml" {
			pattern := filepath.Join(basePath, ".sc/stacks/*/client.yaml")
			if entries, err := filepath.Glob(pattern); err == nil && len(entries) > 0 {
				for _, entry := range entries {
					relPath, _ := filepath.Rel(basePath, entry)
					files = append(files, relPath)
				}
			}
		}
	}

	return files
}
