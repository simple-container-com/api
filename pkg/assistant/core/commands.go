package core

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/simple-container-com/api/pkg/assistant/analysis"
	"github.com/simple-container-com/api/pkg/assistant/embeddings"
	"github.com/simple-container-com/api/pkg/assistant/llm"
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
	// Initialize embeddings database (optional - only needed for documentation search)
	db, err := embeddings.LoadEmbeddedDatabase(context.Background())
	if err != nil {
		// Log warning but continue without embeddings database
		fmt.Printf("Warning: Failed to load embeddings database: %v\n", err)
		db = nil
	}

	return &UnifiedCommandHandler{
		embeddingsDB:  db,
		analyzer:      analysis.NewProjectAnalyzer(),
		developerMode: modes.NewDeveloperMode(),
	}, nil
}

// SearchDocumentation searches Simple Container documentation
func (h *UnifiedCommandHandler) SearchDocumentation(ctx context.Context, query string, limit int) (*CommandResult, error) {
	if h.embeddingsDB == nil {
		return &CommandResult{
			Success: false,
			Message: "❌ Documentation search is not available - embeddings database not loaded",
			Error:   "embeddings database not initialized",
		}, fmt.Errorf("embeddings database not initialized")
	}

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

// AddEnvironment adds a new environment/stack to client.yaml using LLM when available
func (h *UnifiedCommandHandler) AddEnvironment(ctx context.Context, stackName, deploymentType, parent, parentEnv string, config map[string]interface{}) (*CommandResult, error) {
	filePath := h.findClientYaml(".")
	if filePath == "" {
		return &CommandResult{
			Success: false,
			Message: "❌ No client.yaml found. Use setup_simple_container to create initial configuration.",
			Error:   "client_yaml_not_found",
		}, fmt.Errorf("client.yaml not found")
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
			Data:    map[string]interface{}{},
		}, nil
	}

	// Try LLM-enhanced environment addition first, fallback to raw manipulation
	modifiedContent, stackConfig, err := h.addEnvironmentWithLLM(ctx, content, stackName, deploymentType, parent, parentEnv, config)
	if err != nil {
		// Fallback to raw YAML manipulation
		return h.addEnvironmentRaw(ctx, content, stackName, deploymentType, parent, parentEnv, config, filePath)
	}

	// Write back to file
	err = h.writeYamlFile(filePath, modifiedContent)
	if err != nil {
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("❌ Failed to write client.yaml: %v", err),
			Error:   err.Error(),
		}, err
	}

	message := fmt.Sprintf("✅ Successfully added '%s' environment using LLM\n", stackName)
	message += fmt.Sprintf("📁 File: %s\n", filePath)
	message += fmt.Sprintf("🎯 Type: %s\n", deploymentType)
	message += fmt.Sprintf("👨‍👩‍👧‍👦 Parent: %s -> %s\n", parent, parentEnv)

	data := map[string]interface{}{
		"stack_name":   stackName,
		"file_path":    filePath,
		"config_added": stackConfig,
		"method":       "llm_enhanced",
	}

	return &CommandResult{
		Success: true,
		Message: message,
		Data:    data,
	}, nil
}

// ModifyStackConfig modifies existing stack environment configuration in client.yaml using LLM when available
func (h *UnifiedCommandHandler) ModifyStackConfig(ctx context.Context, stackName, environmentName string, changes map[string]interface{}) (*CommandResult, error) {
	// Find the client.yaml file in the specific stack directory
	stackDir := filepath.Join(".sc", "stacks", stackName)
	filePath := filepath.Join(stackDir, "client.yaml")

	// Check if the file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("❌ Stack directory '%s' or client.yaml not found at %s. Use setup_simple_container to create initial configuration.", stackName, filePath),
			Error:   "client_yaml_not_found",
		}, fmt.Errorf("client.yaml not found at %s", filePath)
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

	_, exists := stacks[environmentName]
	if !exists {
		availableEnvironments := h.getStackNames(stacks)
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("❌ Environment '%s' not found in stack '%s'. Available environments: %v", environmentName, stackName, availableEnvironments),
			Error:   "environment_not_found",
			Data: map[string]interface{}{
				"available_environments": availableEnvironments,
			},
		}, fmt.Errorf("environment not found: %s", environmentName)
	}

	// Try LLM-enhanced modification first, fallback to raw YAML manipulation
	modifiedContent, changesApplied, err := h.modifyStackWithLLM(ctx, content, environmentName, changes)
	if err != nil {
		// Fallback to raw YAML manipulation
		return h.modifyStackRaw(ctx, content, environmentName, changes, filePath)
	}

	// Optional: Debug file writes for troubleshooting
	// debugYAML, _ := yaml.Marshal(modifiedContent)
	// fmt.Printf("DEBUG: Writing to file %s\n", filePath)

	// Write back to file
	err = h.writeYamlFile(filePath, modifiedContent)
	if err != nil {
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("❌ Failed to write client.yaml: %v", err),
			Error:   err.Error(),
		}, err
	}

	message := fmt.Sprintf("✅ Successfully modified '%s' stack configuration using LLM\n", stackName)
	message += fmt.Sprintf("📁 File: %s\n", filePath)
	message += fmt.Sprintf("🔄 Changes applied: %+v\n", changesApplied)

	data := map[string]interface{}{
		"stack_name":      stackName,
		"file_path":       filePath,
		"changes_applied": changesApplied,
		"method":          "llm_enhanced",
	}

	return &CommandResult{
		Success: true,
		Message: message,
		Data:    data,
	}, nil
}

// modifyStackWithLLM uses LLM with enriched context to modify stack configuration
func (h *UnifiedCommandHandler) modifyStackWithLLM(ctx context.Context, content map[string]interface{}, stackName string, changes map[string]interface{}) (map[string]interface{}, map[string]interface{}, error) {
	// Check if DeveloperMode has LLM capability
	if h.developerMode == nil {
		return nil, nil, fmt.Errorf("no LLM available")
	}

	// Get project analysis for context
	projectAnalysis, err := h.analyzer.AnalyzeProject(".")
	if err != nil {
		projectAnalysis = &analysis.ProjectAnalysis{Name: "unknown"}
	}

	// Build enriched prompt using existing functions (DRY principle)
	prompt := h.buildStackModificationPrompt(content, stackName, changes, projectAnalysis)

	// Optional: Debug output for troubleshooting
	// fmt.Printf("DEBUG: ModifyStack - stackName: %s, changes: %+v\n", stackName, changes)

	// Use LLM interface directly (reusing existing pattern from DeveloperMode)
	llmProvider := h.developerMode.GetLLMProvider()
	if llmProvider == nil {
		return nil, nil, fmt.Errorf("no LLM provider available")
	}

	response, err := llmProvider.Chat(ctx, []llm.Message{
		{Role: "system", Content: `You are an expert in Simple Container configuration modification. Generate ONLY valid YAML that EXACTLY follows the provided schemas.

CRITICAL INSTRUCTIONS:
✅ Follow the JSON schemas EXACTLY - every property must match the schema structure
✅ Use ONLY properties defined in the schemas - no fictional or made-up properties
✅ Return complete, valid client.yaml configuration
✅ When you see "REMOVE ENTIRE SECTION", DELETE that section completely from the YAML
✅ When you see "SET X TO: Y (remove any keys not listed here)", include ONLY the keys listed in Y
✅ DO NOT add fictional keys, services, or environment variables
✅ DO NOT keep keys that were explicitly meant to be removed
✅ EXACTLY follow removal instructions - if something should be deleted, DELETE IT`},
		{Role: "user", Content: prompt},
	})
	if err != nil {
		return nil, nil, fmt.Errorf("LLM generation failed: %w", err)
	}

	// Optional: Debug LLM response for troubleshooting
	// fmt.Printf("DEBUG: LLM Response (%d chars)\n", len(response.Content))

	// Parse the response and extract changes applied
	modifiedContent, changesApplied, err := h.parseModifiedYAML(response.Content, content, stackName)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse LLM response: %w", err)
	}

	// Optional: Debug changes detection for troubleshooting
	// fmt.Printf("DEBUG: Changes detected: %+v\n", changesApplied)

	return modifiedContent, changesApplied, nil
}

// modifyStackRaw provides fallback raw YAML manipulation (original logic)
func (h *UnifiedCommandHandler) modifyStackRaw(ctx context.Context, content map[string]interface{}, stackName string, changes map[string]interface{}, filePath string) (*CommandResult, error) {
	stacks := content["stacks"].(map[string]interface{})
	stack := stacks[stackName]
	stackConfig := stack.(map[string]interface{})

	// Apply changes using raw manipulation
	changesApplied := make(map[string]interface{})
	err := h.applyChangesToConfig(stackConfig, changes, "", changesApplied)
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

	message := fmt.Sprintf("✅ Successfully modified '%s' stack configuration (fallback mode)\n", stackName)
	message += fmt.Sprintf("📁 File: %s\n", filePath)
	message += fmt.Sprintf("🔄 Changes applied: %+v\n", changesApplied)

	data := map[string]interface{}{
		"stack_name":      stackName,
		"file_path":       filePath,
		"changes_applied": changesApplied,
		"method":          "raw_yaml",
	}

	return &CommandResult{
		Success: true,
		Message: message,
		Data:    data,
	}, nil
}

// buildStackModificationPrompt creates an enriched prompt for stack modification using existing patterns
func (h *UnifiedCommandHandler) buildStackModificationPrompt(content map[string]interface{}, stackName string, changes map[string]interface{}, analysis *analysis.ProjectAnalysis) string {
	var prompt strings.Builder

	// Enrich with documentation context using existing embeddings
	embeddingContext := h.enrichContextWithDocumentation("client.yaml modification", analysis)

	prompt.WriteString("You are an expert Simple Container configuration modifier. Modify the existing client.yaml stack configuration intelligently.\n\n")

	if embeddingContext != "" {
		prompt.WriteString("RELEVANT DOCUMENTATION CONTEXT:\n")
		prompt.WriteString(embeddingContext)
		prompt.WriteString("\n\n")
	}

	prompt.WriteString("CURRENT CONFIGURATION:\n")
	currentYAML, _ := yaml.Marshal(content)
	prompt.WriteString(string(currentYAML))
	prompt.WriteString("\n")

	// Analyze available environments
	if stacks, ok := content["stacks"].(map[string]interface{}); ok {
		environments := make([]string, 0, len(stacks))
		for envName := range stacks {
			environments = append(environments, envName)
		}

		prompt.WriteString(fmt.Sprintf("AVAILABLE ENVIRONMENTS: %v\n", environments))
		if len(environments) > 1 {
			prompt.WriteString("⚠️ MULTIPLE ENVIRONMENTS DETECTED - Make sure you're modifying the correct environment!\n")
		}
		prompt.WriteString("\n")
	}

	prompt.WriteString(fmt.Sprintf("ENVIRONMENT TO MODIFY: %s\n\n", stackName))

	prompt.WriteString("REQUESTED CHANGES:\n")
	for key, value := range changes {
		if value == nil || fmt.Sprintf("%v", value) == "" {
			prompt.WriteString(fmt.Sprintf("- REMOVE ENTIRE SECTION: %s (delete this section completely)\n", key))
		} else {
			prompt.WriteString(fmt.Sprintf("- SET %s TO: %v (remove any keys not listed here)\n", key, value))
		}
	}

	prompt.WriteString("\nINSTRUCTIONS:\n")
	prompt.WriteString("- Apply the requested changes intelligently to the specified stack\n")
	prompt.WriteString("- Maintain all existing configuration that doesn't conflict\n")
	prompt.WriteString("- Use ONLY valid Simple Container properties from the documentation context\n")
	prompt.WriteString("- Handle dot notation (config.scale.max) by updating nested properties\n")

	prompt.WriteString("\nCRITICAL MULTIPLE ENVIRONMENTS HANDLING:\n")
	if stacks, ok := content["stacks"].(map[string]interface{}); ok && len(stacks) > 1 {
		prompt.WriteString("- ⚠️ MULTIPLE ENVIRONMENTS EXIST - User must specify which environment to modify\n")
		prompt.WriteString("- If user request doesn't specify environment, STOP and ask: 'Which environment would you like to modify?'\n")
		prompt.WriteString("- List available environments and ask user to choose before proceeding\n")
		prompt.WriteString("- DO NOT assume or guess which environment the user wants to modify\n")
	} else {
		prompt.WriteString("- Single environment detected - proceed with modifications\n")
	}

	prompt.WriteString("\nCRITICAL ENVIRONMENT VARIABLES SEMANTIC UNDERSTANDING:\n")
	prompt.WriteString("- BOTH 'env:' and 'secrets:' sections contain ENVIRONMENT VARIABLES\n")
	prompt.WriteString("- 'env:' = non-sensitive environment variables (plain text)\n")
	prompt.WriteString("- 'secrets:' = sensitive environment variables (handled securely at deploy)\n")
	prompt.WriteString("- When user asks to 'remove environment variables for X', remove from BOTH env: AND secrets: sections\n")
	prompt.WriteString("- When user asks to 'remove database env vars', remove database-related entries from BOTH sections\n")

	prompt.WriteString("\nCRITICAL LAMBDA CONFIGURATION UNDERSTANDING:\n")
	prompt.WriteString("⚠️ MEMORY vs SCALING ARE COMPLETELY DIFFERENT CONCEPTS:\n")
	prompt.WriteString("- 'maxMemory' = Lambda function memory allocation in MB (e.g., 512, 1024, 2048)\n")
	prompt.WriteString("- 'timeout' = Lambda function timeout in seconds\n")
	prompt.WriteString("- 'scale.max' = Container scaling maximum instances (NOT MEMORY!)\n")
	prompt.WriteString("\n🚨 CRITICAL MAPPING RULES:\n")
	prompt.WriteString("- 'increase memory' → MODIFY 'maxMemory' field (NOT scale.max!)\n")
	prompt.WriteString("- 'extend max memory' → MODIFY 'maxMemory' field (NOT scale.max!)\n")
	prompt.WriteString("- 'memory to 1024' → SET 'maxMemory: 1024' (NOT scale.max!)\n")
	prompt.WriteString("- 'scaling' or 'instances' → MODIFY 'scale.max' field\n")
	prompt.WriteString("\n❌ WRONG EXAMPLE:\n")
	prompt.WriteString("User: 'increase memory to 1024' → DO NOT CREATE: scale: {max: 1024}\n")
	prompt.WriteString("✅ CORRECT EXAMPLE:\n")
	prompt.WriteString("User: 'increase memory to 1024' → CREATE: maxMemory: 1024\n")
	prompt.WriteString("\n🔒 NEVER CONFUSE MEMORY ALLOCATION WITH SCALING CONFIGURATION!\n")

	prompt.WriteString("\nCRITICAL REMOVAL/DELETION INSTRUCTIONS:\n")
	prompt.WriteString("- When a change shows empty value (e.g., 'config.uses:' or 'config.env:'), REMOVE that entire section\n")
	prompt.WriteString("- When asked to 'remove resources', DELETE the uses: section entirely\n")
	prompt.WriteString("- When asked to 'remove environment variables', consider BOTH env: and secrets: sections\n")
	prompt.WriteString("- DO NOT add fictional resources or environment variables that weren't in the original\n")
	prompt.WriteString("- DO NOT hallucinate new services like 'additional-service-1' or fake env vars like 'DATABASE_URL'\n")
	prompt.WriteString("- ONLY modify what exists in the CURRENT CONFIGURATION provided above\n")

	prompt.WriteString("\nCRITICAL CONFIGURATION PRESERVATION:\n")
	prompt.WriteString("- PRESERVE ALL existing configuration that is not being modified\n")
	prompt.WriteString("- DO NOT remove existing properties like 'env:', 'secrets:', 'template:', 'timeout:', etc.\n")
	prompt.WriteString("- ONLY modify the specific properties requested by the user\n")
	prompt.WriteString("- Keep the complete original structure and add/modify only requested changes\n")
	prompt.WriteString("- If original has 'maxMemory: 512' and user wants 1024, change ONLY that value\n")

	prompt.WriteString("\n- Return the complete modified client.yaml configuration\n")
	prompt.WriteString("- Ensure proper YAML formatting and schema compliance\n\n")

	return prompt.String()
}

// enrichContextWithDocumentation reuses existing enrichment logic from DeveloperMode
func (h *UnifiedCommandHandler) enrichContextWithDocumentation(configType string, analysis *analysis.ProjectAnalysis) string {
	if h.embeddingsDB == nil {
		return ""
	}

	// Use embeddings to find relevant documentation
	queries := []string{
		fmt.Sprintf("%s modification examples", configType),
		"Simple Container client.yaml configuration",
		"stack configuration best practices",
	}

	if analysis != nil && analysis.PrimaryStack != nil {
		queries = append(queries, fmt.Sprintf("%s Simple Container configuration", analysis.PrimaryStack.Language))
	}

	var contextBuilder strings.Builder
	for _, query := range queries {
		results, err := embeddings.SearchDocumentation(h.embeddingsDB, query, 2)
		if err != nil {
			continue
		}

		for _, result := range results {
			if result.Score > 0.7 { // Only include highly relevant results
				// Truncate to avoid overwhelming the LLM
				content := result.Content
				if len(content) > 300 {
					content = content[:300] + "..."
				}
				contextBuilder.WriteString(fmt.Sprintf("• %s\n", content))
			}
		}
	}

	return contextBuilder.String()
}

// parseModifiedYAML extracts the modified configuration and determines what changed
func (h *UnifiedCommandHandler) parseModifiedYAML(response string, originalContent map[string]interface{}, stackName string) (map[string]interface{}, map[string]interface{}, error) {
	// Clean the response (remove code blocks if present)
	yamlContent := strings.TrimSpace(response)
	yamlContent = strings.TrimPrefix(yamlContent, "```yaml")
	yamlContent = strings.TrimPrefix(yamlContent, "```")
	yamlContent = strings.TrimSuffix(yamlContent, "```")

	// Parse the modified YAML
	var modifiedContent map[string]interface{}
	err := yaml.Unmarshal([]byte(yamlContent), &modifiedContent)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid YAML in LLM response: %w", err)
	}

	// Compare and determine what changed
	changesApplied := make(map[string]interface{})

	// Get original and modified stack configurations for comparison
	originalStacks, _ := originalContent["stacks"].(map[string]interface{})
	modifiedStacks, _ := modifiedContent["stacks"].(map[string]interface{})

	if originalStacks != nil && modifiedStacks != nil {
		originalStack, _ := originalStacks[stackName].(map[string]interface{})
		modifiedStack, _ := modifiedStacks[stackName].(map[string]interface{})

		// Optional: Debug stack comparison for troubleshooting
		// fmt.Printf("DEBUG: Comparing stacks for %s\n", stackName)

		if originalStack != nil && modifiedStack != nil {
			h.compareConfigs(originalStack, modifiedStack, "", changesApplied)
		}
	}

	return modifiedContent, changesApplied, nil
}

// compareConfigs recursively compares configurations to determine what changed
func (h *UnifiedCommandHandler) compareConfigs(original, modified map[string]interface{}, prefix string, changes map[string]interface{}) {
	for key, modValue := range modified {
		fullKey := key
		if prefix != "" {
			fullKey = prefix + "." + key
		}

		if origValue, exists := original[key]; exists {
			// Key exists in both, check if values differ
			if origMap, ok := origValue.(map[string]interface{}); ok {
				if modMap, ok := modValue.(map[string]interface{}); ok {
					// Both are maps, recurse
					h.compareConfigs(origMap, modMap, fullKey, changes)
					continue
				}
			}

			// Compare values directly
			if fmt.Sprintf("%v", origValue) != fmt.Sprintf("%v", modValue) {
				changes[fullKey] = map[string]interface{}{
					"old": origValue,
					"new": modValue,
				}
			}
		} else {
			// New key added
			changes[fullKey] = map[string]interface{}{
				"old": nil,
				"new": modValue,
			}
		}
	}
}

// addEnvironmentWithLLM uses LLM with enriched context to add new environment/stack
func (h *UnifiedCommandHandler) addEnvironmentWithLLM(ctx context.Context, content map[string]interface{}, stackName, deploymentType, parent, parentEnv string, config map[string]interface{}) (map[string]interface{}, map[string]interface{}, error) {
	// Check if DeveloperMode has LLM capability
	if h.developerMode == nil {
		return nil, nil, fmt.Errorf("no LLM available")
	}

	// Get project analysis for context
	projectAnalysis, err := h.analyzer.AnalyzeProject(".")
	if err != nil {
		projectAnalysis = &analysis.ProjectAnalysis{Name: "unknown"}
	}

	// Build enriched prompt using existing functions (DRY principle)
	prompt := h.buildEnvironmentAdditionPrompt(content, stackName, deploymentType, parent, parentEnv, config, projectAnalysis)

	// Use LLM interface directly (reusing existing pattern from DeveloperMode)
	llmProvider := h.developerMode.GetLLMProvider()
	if llmProvider == nil {
		return nil, nil, fmt.Errorf("no LLM provider available")
	}

	response, err := llmProvider.Chat(ctx, []llm.Message{
		{Role: "system", Content: `You are an expert in Simple Container configuration. Generate ONLY valid YAML that EXACTLY follows the provided schemas.

CRITICAL INSTRUCTIONS:
✅ Follow the JSON schemas EXACTLY - every property must match the schema structure
✅ Use ONLY properties defined in the schemas - no fictional or made-up properties
✅ Return complete, valid client.yaml configuration with the new environment added
✅ Maintain all existing configuration while adding the new stack intelligently`},
		{Role: "user", Content: prompt},
	})
	if err != nil {
		return nil, nil, fmt.Errorf("LLM generation failed: %w", err)
	}

	// Parse the response and extract the added stack config
	modifiedContent, stackConfig, err := h.parseAddedEnvironment(response.Content, content, stackName)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse LLM response: %w", err)
	}

	return modifiedContent, stackConfig, nil
}

// addEnvironmentRaw provides fallback raw YAML manipulation (original logic)
func (h *UnifiedCommandHandler) addEnvironmentRaw(ctx context.Context, content map[string]interface{}, stackName, deploymentType, parent, parentEnv string, config map[string]interface{}, filePath string) (*CommandResult, error) {
	stacks := content["stacks"].(map[string]interface{})

	// Create new stack configuration using raw manipulation
	stackConfig := map[string]interface{}{
		"type":      deploymentType,
		"parent":    parent,
		"parentEnv": parentEnv,
		"config":    h.createDefaultStackConfig(deploymentType, config),
	}

	// Add the new stack
	stacks[stackName] = stackConfig

	// Write back to file
	err := h.writeYamlFile(filePath, content)
	if err != nil {
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("❌ Failed to write client.yaml: %v", err),
			Error:   err.Error(),
		}, err
	}

	message := fmt.Sprintf("✅ Successfully added '%s' environment (fallback mode)\n", stackName)
	message += fmt.Sprintf("📁 File: %s\n", filePath)
	message += fmt.Sprintf("🎯 Type: %s\n", deploymentType)
	message += fmt.Sprintf("👨‍👩‍👧‍👦 Parent: %s -> %s\n", parent, parentEnv)

	data := map[string]interface{}{
		"stack_name":   stackName,
		"file_path":    filePath,
		"config_added": stackConfig,
		"method":       "raw_yaml",
	}

	return &CommandResult{
		Success: true,
		Message: message,
		Data:    data,
	}, nil
}

// buildEnvironmentAdditionPrompt creates an enriched prompt for environment addition using existing patterns
func (h *UnifiedCommandHandler) buildEnvironmentAdditionPrompt(content map[string]interface{}, stackName, deploymentType, parent, parentEnv string, config map[string]interface{}, analysis *analysis.ProjectAnalysis) string {
	var prompt strings.Builder

	// Enrich with documentation context using existing embeddings
	embeddingContext := h.enrichContextWithDocumentation("client.yaml environment addition", analysis)

	prompt.WriteString("You are an expert Simple Container configuration creator. Add a new environment/stack to the existing client.yaml configuration intelligently.\n\n")

	if embeddingContext != "" {
		prompt.WriteString("RELEVANT DOCUMENTATION CONTEXT:\n")
		prompt.WriteString(embeddingContext)
		prompt.WriteString("\n\n")
	}

	prompt.WriteString("CURRENT CONFIGURATION:\n")
	currentYAML, _ := yaml.Marshal(content)
	prompt.WriteString(string(currentYAML))
	prompt.WriteString("\n")

	prompt.WriteString("NEW ENVIRONMENT TO ADD:\n")
	prompt.WriteString(fmt.Sprintf("- Stack Name: %s\n", stackName))
	prompt.WriteString(fmt.Sprintf("- Deployment Type: %s\n", deploymentType))
	prompt.WriteString(fmt.Sprintf("- Parent Stack: %s\n", parent))
	prompt.WriteString(fmt.Sprintf("- Parent Environment: %s\n", parentEnv))

	if len(config) > 0 {
		prompt.WriteString("- Additional Config:\n")
		for key, value := range config {
			prompt.WriteString(fmt.Sprintf("  %s: %v\n", key, value))
		}
	}

	prompt.WriteString("\nINSTRUCTIONS:\n")
	prompt.WriteString("- Add the new environment/stack to the existing configuration intelligently\n")
	prompt.WriteString("- Use appropriate defaults for the deployment type based on documentation context\n")
	prompt.WriteString("- Use ONLY valid Simple Container properties from the documentation context\n")
	prompt.WriteString("- Maintain all existing stacks and configuration\n")
	prompt.WriteString("- Ensure proper YAML formatting and schema compliance\n")
	prompt.WriteString("- Return the complete modified client.yaml configuration\n\n")

	return prompt.String()
}

// parseAddedEnvironment extracts the added environment configuration from LLM response
func (h *UnifiedCommandHandler) parseAddedEnvironment(response string, originalContent map[string]interface{}, stackName string) (map[string]interface{}, map[string]interface{}, error) {
	// Clean the response (remove code blocks if present)
	yamlContent := strings.TrimSpace(response)
	yamlContent = strings.TrimPrefix(yamlContent, "```yaml")
	yamlContent = strings.TrimPrefix(yamlContent, "```")
	yamlContent = strings.TrimSuffix(yamlContent, "```")

	// Parse the modified YAML
	var modifiedContent map[string]interface{}
	err := yaml.Unmarshal([]byte(yamlContent), &modifiedContent)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid YAML in LLM response: %w", err)
	}

	// Extract the added stack configuration
	modifiedStacks, ok := modifiedContent["stacks"].(map[string]interface{})
	if !ok {
		return nil, nil, fmt.Errorf("no stacks section in LLM response")
	}

	stackConfig, ok := modifiedStacks[stackName].(map[string]interface{})
	if !ok {
		return nil, nil, fmt.Errorf("new stack '%s' not found in LLM response", stackName)
	}

	return modifiedContent, stackConfig, nil
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
			Data:    map[string]interface{}{},
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

	data := map[string]interface{}{
		"resource_name": resourceName,
		"environment":   environment,
		"file_path":     filePath,
		"config_added":  resourceConfig,
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

	// Obfuscate credentials before parsing - this protects against secrets exposure during YAML processing
	data = h.obfuscateCredentials(data, filePath)

	var content map[string]interface{}
	err = yaml.Unmarshal(data, &content)
	if err != nil {
		return nil, fmt.Errorf("failed to parse YAML in %s: %w", filePath, err)
	}

	return content, nil
}

// obfuscateCredentials masks sensitive values in file content before exposing to LLM
func (h *UnifiedCommandHandler) obfuscateCredentials(content []byte, filePath string) []byte {
	// Get filename for context
	fileName := filepath.Base(filePath)

	// Check if this is a secrets file or contains sensitive content
	isSecretsFile := strings.Contains(fileName, "secrets") || strings.HasSuffix(fileName, "secrets.yaml") || strings.HasSuffix(fileName, "secrets.yml")

	contentStr := string(content)

	// For secrets.yaml files, apply comprehensive obfuscation
	if isSecretsFile {
		return []byte(h.obfuscateSecretsYAML(contentStr))
	}

	// For other files, apply general credential obfuscation
	return []byte(h.obfuscateGeneralCredentials(contentStr))
}

// obfuscateSecretsYAML specifically handles secrets.yaml files
func (h *UnifiedCommandHandler) obfuscateSecretsYAML(content string) string {
	// Parse YAML to understand structure
	var data map[string]interface{}
	if err := yaml.Unmarshal([]byte(content), &data); err != nil {
		// If parsing fails, apply regex-based obfuscation as fallback
		return h.obfuscateGeneralCredentials(content)
	}

	// Obfuscate sensitive fields in structured way
	h.obfuscateYAMLValues(data)

	// Marshal back to YAML
	if obfuscatedBytes, err := yaml.Marshal(data); err == nil {
		return string(obfuscatedBytes)
	}

	// Fallback to regex if marshaling fails
	return h.obfuscateGeneralCredentials(content)
}

// obfuscateYAMLValues recursively obfuscates sensitive values in YAML structure
func (h *UnifiedCommandHandler) obfuscateYAMLValues(data interface{}) {
	h.obfuscateYAMLValuesWithContext(data, "")
}

// obfuscateYAMLValuesWithContext recursively obfuscates sensitive values with section context
func (h *UnifiedCommandHandler) obfuscateYAMLValuesWithContext(data interface{}, sectionPath string) {
	switch v := data.(type) {
	case map[string]interface{}:
		for key, value := range v {
			newSectionPath := key
			if sectionPath != "" {
				newSectionPath = sectionPath + "." + key
			}

			// Special handling for secrets.yaml 'values' section - obfuscate ALL values
			if sectionPath == "values" || key == "values" && sectionPath == "" {
				if strVal, ok := value.(string); ok {
					v[key] = h.obfuscateValue(strVal, key)
				} else {
					// If values section contains nested structure, obfuscate all string values
					h.obfuscateAllStringValues(value)
				}
			} else if h.isSensitiveKey(key) {
				if strVal, ok := value.(string); ok {
					v[key] = h.obfuscateValue(strVal, key)
				}
			} else {
				h.obfuscateYAMLValuesWithContext(value, newSectionPath)
			}
		}
	case map[interface{}]interface{}:
		for key, value := range v {
			keyStr := ""
			if k, ok := key.(string); ok {
				keyStr = k
			}

			newSectionPath := keyStr
			if sectionPath != "" {
				newSectionPath = sectionPath + "." + keyStr
			}

			// Special handling for secrets.yaml 'values' section - obfuscate ALL values
			if sectionPath == "values" || keyStr == "values" && sectionPath == "" {
				if strVal, ok := value.(string); ok {
					v[key] = h.obfuscateValue(strVal, keyStr)
				} else {
					// If values section contains nested structure, obfuscate all string values
					h.obfuscateAllStringValues(value)
				}
			} else if keyStr != "" && h.isSensitiveKey(keyStr) {
				if strVal, ok := value.(string); ok {
					v[key] = h.obfuscateValue(strVal, keyStr)
				}
			} else {
				h.obfuscateYAMLValuesWithContext(value, newSectionPath)
			}
		}
	case []interface{}:
		for _, item := range v {
			h.obfuscateYAMLValuesWithContext(item, sectionPath)
		}
	}
}

// obfuscateAllStringValues obfuscates all string values in a data structure (for values section)
func (h *UnifiedCommandHandler) obfuscateAllStringValues(data interface{}) {
	switch v := data.(type) {
	case map[string]interface{}:
		for key, value := range v {
			if strVal, ok := value.(string); ok {
				v[key] = h.obfuscateValue(strVal, key)
			} else {
				h.obfuscateAllStringValues(value)
			}
		}
	case map[interface{}]interface{}:
		for key, value := range v {
			keyStr := ""
			if k, ok := key.(string); ok {
				keyStr = k
			}
			if strVal, ok := value.(string); ok {
				v[key] = h.obfuscateValue(strVal, keyStr)
			} else {
				h.obfuscateAllStringValues(value)
			}
		}
	case []interface{}:
		for _, item := range v {
			h.obfuscateAllStringValues(item)
		}
	}
}

// isSensitiveKey checks if a YAML key contains sensitive information
func (h *UnifiedCommandHandler) isSensitiveKey(key string) bool {
	key = strings.ToLower(key)

	// Common sensitive key patterns
	sensitiveKeys := []string{
		"password", "passwd", "pwd",
		"secret", "secretkey", "secretaccesskey",
		"token", "apikey", "api_key", "accesskey", "access_key",
		"private_key", "privatekey", "private_key_id",
		"credentials", "auth", "authentication",
		"cert", "certificate", "key", "pem",
		"webhook", "webhookurl", "webhook_url",
		"dsn", "database_url", "connection_string", "connectionstring",
		"mongodb_uri", "mongo_uri", "redis_url", "postgres_url",
		"jwt_secret", "jwtsecret", "session_secret",
		// Kubernetes-specific fields
		"kubeconfig", "client-key", "client-key-data", "client-certificate-data",
		"certificate-authority-data", "client-cert", "client-cert-data",
		"user-key", "user-cert", "ca-cert", "ca-key",
		// GCP-specific fields
		"service_account_key", "client_secret", "refresh_token",
	}

	for _, sensitive := range sensitiveKeys {
		if strings.Contains(key, sensitive) {
			return true
		}
	}

	return false
}

// obfuscateValue masks a sensitive value while preserving its type/format context
func (h *UnifiedCommandHandler) obfuscateValue(value, key string) string {
	if value == "" {
		return value
	}

	// Preserve placeholder patterns (${secret:...}, ${env:...}, etc.)
	if strings.HasPrefix(value, "${") && strings.HasSuffix(value, "}") {
		return value
	}

	// Determine obfuscation pattern based on value characteristics
	switch {
	case strings.HasPrefix(value, "AKIA"):
		// AWS Access Key pattern
		return "AKIA••••••••••••••••"
	case strings.HasPrefix(value, "sk-"):
		// OpenAI API key pattern
		return "sk-•••••••••••••••••••••••••••••••••••••••••••••••••"
	case strings.HasPrefix(value, "xoxb-") || strings.HasPrefix(value, "xoxp-"):
		// Slack token pattern
		return strings.Split(value, "-")[0] + "-••••••••••••••••••••••••••••••••••"
	case strings.HasPrefix(value, "ghp_"):
		// GitHub token pattern
		return "ghp_••••••••••••••••••••••••••••••••••••••••"
	case strings.HasPrefix(value, "mongodb://") || strings.HasPrefix(value, "mongodb+srv://"):
		// MongoDB URI pattern - preserve structure but mask credentials
		return h.obfuscateURI(value)
	case strings.HasPrefix(value, "postgres://") || strings.HasPrefix(value, "postgresql://"):
		// PostgreSQL URI pattern
		return h.obfuscateURI(value)
	case strings.HasPrefix(value, "redis://"):
		// Redis URI pattern
		return h.obfuscateURI(value)
	case strings.HasPrefix(value, "-----BEGIN"):
		// Private key or certificate
		return h.obfuscateMultilineSecret(value)
	case strings.Contains(value, "\"private_key\"") || strings.Contains(value, "\"client_secret\""):
		// GCP service account JSON or similar embedded JSON credentials
		return h.obfuscateEmbeddedJSON(value)
	case strings.Contains(value, "apiVersion:") && strings.Contains(value, "clusters:"):
		// Kubernetes kubeconfig YAML
		return h.obfuscateEmbeddedYAML(value)
	case len(value) > 20 && h.isBase64Like(value):
		// Long base64-like string (certificates, tokens)
		return value[:8] + "••••••••••••••••••••••••••••••••••••••••••••" + value[len(value)-4:]
	case len(value) > 10:
		// Generic long secret
		if len(value) <= 20 {
			return "••••••••••••••••••••"
		}
		return value[:4] + "••••••••••••••••••••••••••••••••" + value[len(value)-2:]
	default:
		// Short secrets
		return "••••••••"
	}
}

// obfuscateURI masks credentials in database/service URIs while preserving structure
func (h *UnifiedCommandHandler) obfuscateURI(uri string) string {
	// Pattern to match URI with credentials: scheme://user:pass@host:port/path
	uriRegex := regexp.MustCompile(`^([^:]+://)[^:]+:[^@]+@(.+)$`)
	if matches := uriRegex.FindStringSubmatch(uri); len(matches) == 3 {
		return matches[1] + "••••••••:••••••••@" + matches[2]
	}

	// If no credentials found, just mask any embedded auth tokens
	return uri
}

// obfuscateMultilineSecret handles multi-line secrets like private keys
func (h *UnifiedCommandHandler) obfuscateMultilineSecret(secret string) string {
	lines := strings.Split(secret, "\n")
	if len(lines) < 3 {
		return "••••••••••••••••••••••••••••••••"
	}

	// Preserve header and footer, mask content
	result := []string{lines[0]}
	for i := 1; i < len(lines)-1; i++ {
		if strings.TrimSpace(lines[i]) != "" {
			result = append(result, "••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••")
		} else {
			result = append(result, lines[i])
		}
	}
	if len(lines) > 1 {
		result = append(result, lines[len(lines)-1])
	}

	return strings.Join(result, "\n")
}

// isBase64Like checks if a string looks like base64 encoding
func (h *UnifiedCommandHandler) isBase64Like(s string) bool {
	base64Regex := regexp.MustCompile(`^[A-Za-z0-9+/]+=*$`)
	return base64Regex.MatchString(s) && len(s)%4 == 0
}

// obfuscateGeneralCredentials applies regex-based obfuscation for non-secrets files
func (h *UnifiedCommandHandler) obfuscateGeneralCredentials(content string) string {
	// Common credential patterns to obfuscate
	patterns := map[string]string{
		// AWS Access Keys
		`AKIA[0-9A-Z]{16}`: "AKIA••••••••••••••••",
		// OpenAI API Keys
		`sk-[a-zA-Z0-9]{48}`: "sk-•••••••••••••••••••••••••••••••••••••••••••••••••",
		// GitHub Tokens
		`ghp_[a-zA-Z0-9]{36}`: "ghp_••••••••••••••••••••••••••••••••••••••••",
		// JWT Tokens (long base64 strings)
		`eyJ[a-zA-Z0-9+/]{50,}[=]*`: "eyJ••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••",
		// Long hex strings (32+ chars)
		`[a-fA-F0-9]{32,}`: func(match string) string {
			return match[:8] + "••••••••••••••••••••••••••••••••••••••••••••" + match[len(match)-4:]
		}("placeholder"),
	}

	result := content
	for pattern, replacement := range patterns {
		regex := regexp.MustCompile(pattern)
		if pattern == `[a-fA-F0-9]{32,}` {
			// Special handling for hex patterns to preserve prefix/suffix
			result = regex.ReplaceAllStringFunc(result, func(match string) string {
				if len(match) > 32 {
					return match[:8] + "••••••••••••••••••••••••••••••••••••••••••••" + match[len(match)-4:]
				}
				return "••••••••••••••••••••••••••••••••"
			})
		} else {
			result = regex.ReplaceAllString(result, replacement)
		}
	}

	return result
}

// obfuscateEmbeddedJSON handles JSON structures embedded in credential values (e.g., GCP service accounts)
func (h *UnifiedCommandHandler) obfuscateEmbeddedJSON(jsonStr string) string {
	// Try to parse as JSON
	var jsonData map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &jsonData); err != nil {
		// If parsing fails, apply general obfuscation
		return h.obfuscateGeneralCredentials(jsonStr)
	}

	// Obfuscate sensitive fields in the JSON
	h.obfuscateJSONCredentials(jsonData)

	// Marshal back to JSON
	if obfuscatedBytes, err := json.Marshal(jsonData); err == nil {
		return string(obfuscatedBytes)
	}

	// Fallback to general obfuscation
	return h.obfuscateGeneralCredentials(jsonStr)
}

// obfuscateJSONCredentials recursively obfuscates sensitive fields in JSON data
func (h *UnifiedCommandHandler) obfuscateJSONCredentials(data interface{}) {
	switch v := data.(type) {
	case map[string]interface{}:
		for key, value := range v {
			if h.isSensitiveKey(key) {
				if strVal, ok := value.(string); ok {
					v[key] = h.obfuscateValue(strVal, key)
				}
			} else {
				h.obfuscateJSONCredentials(value)
			}
		}
	case []interface{}:
		for _, item := range v {
			h.obfuscateJSONCredentials(item)
		}
	}
}

// obfuscateEmbeddedYAML handles YAML structures embedded in credential values (e.g., kubeconfig)
func (h *UnifiedCommandHandler) obfuscateEmbeddedYAML(yamlStr string) string {
	// Try to parse as YAML
	var yamlData interface{}
	if err := yaml.Unmarshal([]byte(yamlStr), &yamlData); err != nil {
		// If parsing fails, apply general obfuscation to sensitive-looking parts
		return h.obfuscateYAMLStringPatterns(yamlStr)
	}

	// Obfuscate sensitive fields in the YAML
	h.obfuscateEmbeddedYAMLValues(yamlData)

	// Marshal back to YAML
	if obfuscatedBytes, err := yaml.Marshal(yamlData); err == nil {
		return string(obfuscatedBytes)
	}

	// Fallback to pattern-based obfuscation
	return h.obfuscateYAMLStringPatterns(yamlStr)
}

// obfuscateEmbeddedYAMLValues recursively obfuscates sensitive fields in embedded YAML
func (h *UnifiedCommandHandler) obfuscateEmbeddedYAMLValues(data interface{}) {
	switch v := data.(type) {
	case map[string]interface{}:
		for key, value := range v {
			if h.isSensitiveKey(key) {
				if strVal, ok := value.(string); ok {
					v[key] = h.obfuscateValue(strVal, key)
				}
			} else {
				h.obfuscateEmbeddedYAMLValues(value)
			}
		}
	case map[interface{}]interface{}:
		for key, value := range v {
			if keyStr, ok := key.(string); ok && h.isSensitiveKey(keyStr) {
				if strVal, ok := value.(string); ok {
					v[key] = h.obfuscateValue(strVal, keyStr)
				}
			} else {
				h.obfuscateEmbeddedYAMLValues(value)
			}
		}
	case []interface{}:
		for _, item := range v {
			h.obfuscateEmbeddedYAMLValues(item)
		}
	}
}

// obfuscateYAMLStringPatterns applies pattern-based obfuscation for YAML strings
func (h *UnifiedCommandHandler) obfuscateYAMLStringPatterns(yamlStr string) string {
	// Define patterns for sensitive YAML keys and their typical values
	patterns := map[string]*regexp.Regexp{
		// Kubernetes certificate data (base64)
		`(client-key-data|client-certificate-data|certificate-authority-data):\s*([A-Za-z0-9+/=]{20,})`: regexp.MustCompile(`(client-key-data|client-certificate-data|certificate-authority-data):\s*([A-Za-z0-9+/=]{20,})`),
		// Private keys in YAML
		`(private_key|private-key):\s*"([^"]+)"`: regexp.MustCompile(`(private_key|private-key):\s*"([^"]+)"`),
		// JWT tokens and other long tokens
		`(token):\s*([A-Za-z0-9._-]{20,})`: regexp.MustCompile(`(token):\s*([A-Za-z0-9._-]{20,})`),
	}

	result := yamlStr
	for _, pattern := range patterns {
		result = pattern.ReplaceAllStringFunc(result, func(match string) string {
			// Extract the key-value structure and obfuscate the value part
			parts := strings.SplitN(match, ":", 2)
			if len(parts) == 2 {
				key := strings.TrimSpace(parts[0])
				value := strings.TrimSpace(parts[1])
				// Remove quotes if present
				if strings.HasPrefix(value, `"`) && strings.HasSuffix(value, `"`) {
					value = value[1 : len(value)-1]
					return key + `: "` + h.obfuscateValue(value, key) + `"`
				}
				return key + `: ` + h.obfuscateValue(value, key)
			}
			return match
		})
	}

	return result
}

func (h *UnifiedCommandHandler) writeYamlFile(filePath string, content map[string]interface{}) error {
	// Check if this is a client.yaml file that needs special formatting
	if strings.HasSuffix(filePath, "client.yaml") {
		return h.writeClientYamlWithOrdering(filePath, content)
	}

	// Use standard YAML marshaling for other files
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

// writeClientYamlWithOrdering writes client.yaml with proper field ordering and formatting
func (h *UnifiedCommandHandler) writeClientYamlWithOrdering(filePath string, content map[string]interface{}) error {
	var output strings.Builder

	// Write top-level fields first (if they exist)
	if schemaVersion, ok := content["schemaVersion"]; ok {
		output.WriteString(fmt.Sprintf("schemaVersion: %v\n", schemaVersion))
		output.WriteString("\n")
	}

	// Write stacks section with proper ordering
	if stacks, ok := content["stacks"].(map[string]interface{}); ok {
		output.WriteString("stacks:\n")

		for stackName, stackConfig := range stacks {
			if stackConfigMap, ok := stackConfig.(map[string]interface{}); ok {
				output.WriteString(fmt.Sprintf("  %s:\n", stackName))
				h.writeStackConfigOrdered(&output, stackConfigMap, "    ")
			}
		}
	}

	// Write other top-level sections (variables, etc.)
	for key, value := range content {
		if key != "schemaVersion" && key != "stacks" {
			output.WriteString(fmt.Sprintf("\n%s:\n", key))
			h.writeYamlValue(&output, value, "  ")
		}
	}

	err := os.WriteFile(filePath, []byte(output.String()), 0o644)
	if err != nil {
		return fmt.Errorf("failed to write file %s: %w", filePath, err)
	}

	return nil
}

// writeStackConfigOrdered writes stack configuration with proper field ordering
func (h *UnifiedCommandHandler) writeStackConfigOrdered(output *strings.Builder, stackConfig map[string]interface{}, indent string) {
	// Define the desired order of fields
	orderedFields := []string{"parent", "parentEnv", "type", "runs", "uses", "dependencies", "config"}

	// Write fields in the specified order
	for _, field := range orderedFields {
		if value, exists := stackConfig[field]; exists {
			output.WriteString(fmt.Sprintf("%s%s: ", indent, field))
			h.writeYamlValue(output, value, indent+"  ")
		}
	}

	// Write any remaining fields that weren't in the ordered list
	for field, value := range stackConfig {
		found := false
		for _, orderedField := range orderedFields {
			if field == orderedField {
				found = true
				break
			}
		}
		if !found {
			output.WriteString(fmt.Sprintf("%s%s: ", indent, field))
			h.writeYamlValue(output, value, indent+"  ")
		}
	}
}

// writeYamlValue writes a YAML value with proper formatting and indentation
func (h *UnifiedCommandHandler) writeYamlValue(output *strings.Builder, value interface{}, indent string) {
	switch v := value.(type) {
	case map[string]interface{}:
		output.WriteString("\n")
		for key, subValue := range v {
			output.WriteString(fmt.Sprintf("%s%s: ", indent, key))
			h.writeYamlValue(output, subValue, indent+"  ")
		}
	case []interface{}:
		output.WriteString("\n")
		for _, item := range v {
			output.WriteString(fmt.Sprintf("%s- ", indent))
			if itemMap, ok := item.(map[string]interface{}); ok {
				// Handle array of objects
				output.WriteString("\n")
				for key, subValue := range itemMap {
					output.WriteString(fmt.Sprintf("%s  %s: ", indent, key))
					h.writeYamlValue(output, subValue, indent+"    ")
				}
			} else {
				// Handle simple array items
				output.WriteString(fmt.Sprintf("%v\n", item))
			}
		}
	case []string:
		if len(v) == 0 {
			output.WriteString("[]\n")
		} else if len(v) == 1 {
			output.WriteString(fmt.Sprintf("%s\n", v[0]))
		} else {
			output.WriteString("\n")
			for _, item := range v {
				output.WriteString(fmt.Sprintf("%s- %s\n", indent, item))
			}
		}
	case string:
		output.WriteString(fmt.Sprintf("%s\n", v))
	case nil:
		output.WriteString("null\n")
	default:
		output.WriteString(fmt.Sprintf("%v\n", v))
	}
}
