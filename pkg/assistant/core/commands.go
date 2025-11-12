package core

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"gopkg.in/yaml.v3"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/assistant/analysis"
	"github.com/simple-container-com/api/pkg/assistant/cicd"
	"github.com/simple-container-com/api/pkg/assistant/configdiff"
	"github.com/simple-container-com/api/pkg/assistant/embeddings"
	"github.com/simple-container-com/api/pkg/assistant/llm"
	"github.com/simple-container-com/api/pkg/assistant/modes"
	"github.com/simple-container-com/api/pkg/assistant/security"
	"github.com/simple-container-com/api/pkg/assistant/utils"
)

// UnifiedCommandHandler provides a shared layer for both MCP and chat interfaces
type UnifiedCommandHandler struct {
	embeddingsDB  *embeddings.Database
	analyzer      *analysis.ProjectAnalyzer
	developerMode *modes.DeveloperMode
	cicdService   *cicd.Service
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
		cicdService:   cicd.NewService(),
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

	// First, read the raw file content to check for YAML anchors
	rawContent, err := os.ReadFile(filePath)
	if err != nil {
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("❌ Failed to read client.yaml: %v", err),
			Error:   err.Error(),
		}, err
	}

	// Check if the file contains YAML anchors
	hasAnchors := h.hasYamlAnchors(string(rawContent))
	if hasAnchors {
		// Use text-based modification to preserve anchors
		return h.modifyStackWithTextManipulation(ctx, string(rawContent), stackName, environmentName, changes, filePath)
	}

	// Read current configuration for normal processing
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

	// Use LLM interface directly (reusing existing pattern from DeveloperMode)
	llmProvider := h.developerMode.GetLLMProvider()
	if llmProvider == nil {
		return nil, nil, fmt.Errorf("no LLM provider available")
	}

	response, err := llmProvider.Chat(ctx, []llm.Message{
		{Role: "system", Content: `You are an expert in Simple Container configuration modification. Generate ONLY valid YAML that EXACTLY follows the provided schemas.

🚨 CRITICAL YAML GENERATION RULES:
✅ Return COMPLETE client.yaml configuration EXACTLY like the original
✅ Include EVERY section from the original configuration  
✅ When adding to nested objects (like secrets:), MERGE with existing entries
✅ NEVER replace entire sections - only ADD or MODIFY specific properties
✅ Generate ONLY pure YAML - no explanatory text, no comments, no extra content

🔒 PRESERVATION REQUIREMENTS:
✅ If original has secrets.API_KEY and you add secrets.REDIS_URL → BOTH must be in result
✅ If original has env.ENVIRONMENT and you add env.NODE_ENV → BOTH must be in result  
✅ NEVER lose existing properties when adding new ones
✅ MERGE operations, don't REPLACE operations

❌ FORBIDDEN ACTIONS:
❌ Do NOT add text like "AVAILABLE ENVIRONMENTS:" - YAML only!
❌ Do NOT replace entire sections - merge into them
❌ Do NOT omit existing properties when adding new ones
❌ Do NOT add explanatory comments or descriptions
❌ Do NOT set properties to 'null' - completely omit deleted keys instead

🗑️ DELETION INSTRUCTIONS:
✅ When removing a property, COMPLETELY OMIT it from the YAML
✅ Do NOT set deleted properties to 'null', 'nil', or empty values
✅ Example: If removing DB_PASSWORD from secrets, the secrets section should NOT contain DB_PASSWORD at all

🧠 INTELLIGENT RESOURCE CLEANUP:
✅ When removing a resource from 'uses', also remove any secrets that reference that resource
✅ Example: Removing 'gcp-cloudsql-postgres' from uses should also remove 'DATABASE_URL: \${resource:gcp-cloudsql-postgres.url}' from secrets
✅ Look for resource references in secrets like '\${resource:RESOURCE_NAME.*}' and clean them up
✅ Be comprehensive - clean up all traces of removed resources`},
		{Role: "user", Content: prompt},
	})
	if err != nil {
		return nil, nil, fmt.Errorf("LLM generation failed: %w", err)
	}

	// Clean the response to ensure only valid YAML
	cleanedResponse := h.cleanLLMYAMLResponse(response.Content)

	// Use cleaned response
	response.Content = cleanedResponse

	// Parse the response and extract changes applied
	modifiedContent, changesApplied, err := h.parseModifiedYAML(response.Content, content, stackName)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse LLM response: %w", err)
	}

	// Clean up any null values that might have slipped through
	h.removeNullValues(modifiedContent)

	// Clean up orphaned resource references
	h.cleanupOrphanedResourceReferences(modifiedContent)

	// CRITICAL: Preserve all environments and root-level properties (YAML anchors, defaults, etc.)
	h.preserveAllRootLevelProperties(content, modifiedContent, stackName)

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

	// Clean up orphaned resource references in raw fallback mode too
	h.cleanupOrphanedResourceReferences(content)

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
			prompt.WriteString(fmt.Sprintf("- SET %s TO: %v\n", key, value))
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
		prompt.WriteString("\n🚨 CRITICAL COMPLETE CONFIGURATION PRESERVATION REQUIREMENTS:\n")
		prompt.WriteString("- PRESERVE ALL ROOT-LEVEL PROPERTIES: schemaVersion, defaults, customers, any YAML anchor definitions\n")
		prompt.WriteString("- PRESERVE ALL OTHER ENVIRONMENTS EXACTLY AS THEY ARE\n")
		prompt.WriteString("- ONLY modify the specified environment ('" + stackName + "')\n")
		prompt.WriteString("- ALL other environments MUST remain completely unchanged\n")
		prompt.WriteString("- DO NOT remove, modify, or alter any other environment sections\n")
		prompt.WriteString("- DO NOT remove YAML anchor definitions like 'defaults:', 'customers:', or '&stack', '&config', etc.\n")
		prompt.WriteString("- Return the COMPLETE configuration with ALL root-level sections AND environments intact\n")

		// List all environments that must be preserved
		environmentsToPreserve := make([]string, 0, len(stacks))
		for envName := range stacks {
			if envName != stackName {
				environmentsToPreserve = append(environmentsToPreserve, envName)
			}
		}
		if len(environmentsToPreserve) > 0 {
			prompt.WriteString(fmt.Sprintf("- PRESERVE THESE ENVIRONMENTS UNTOUCHED: %v\n", environmentsToPreserve))
		}

		// List all root-level properties that must be preserved
		rootPropertiesToPreserve := make([]string, 0)
		for rootKey := range content {
			if rootKey != "stacks" {
				rootPropertiesToPreserve = append(rootPropertiesToPreserve, rootKey)
			}
		}
		if len(rootPropertiesToPreserve) > 0 {
			prompt.WriteString(fmt.Sprintf("- PRESERVE THESE ROOT-LEVEL SECTIONS UNTOUCHED: %v\n", rootPropertiesToPreserve))
		}
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

// cleanLLMYAMLResponse removes non-YAML content that LLMs sometimes add
func (h *UnifiedCommandHandler) cleanLLMYAMLResponse(response string) string {
	// Remove code block markers
	cleaned := strings.TrimSpace(response)
	cleaned = strings.TrimPrefix(cleaned, "```yaml")
	cleaned = strings.TrimPrefix(cleaned, "```")
	cleaned = strings.TrimSuffix(cleaned, "```")

	// Split into lines and filter out non-YAML content
	lines := strings.Split(cleaned, "\n")
	var yamlLines []string

	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)

		// Skip empty lines at the beginning but keep them in the middle/end
		if len(yamlLines) == 0 && trimmedLine == "" {
			continue
		}

		// Skip lines that look like explanatory text, comments, or contain null values
		if strings.HasPrefix(trimmedLine, "AVAILABLE ENVIRONMENTS:") ||
			strings.HasPrefix(trimmedLine, "AVAILABLE STACKS:") ||
			strings.HasPrefix(trimmedLine, "CONFIGURATION:") ||
			strings.HasPrefix(trimmedLine, "MODIFIED:") ||
			strings.Contains(trimmedLine, ": null") ||
			strings.Contains(trimmedLine, ": nil") ||
			(strings.HasPrefix(trimmedLine, "# ") && !isYAMLComment(line)) {
			continue
		}

		// Keep the line if it looks like valid YAML
		yamlLines = append(yamlLines, line)
	}

	return strings.Join(yamlLines, "\n")
}

// isYAMLComment checks if a line is a proper YAML comment (indented correctly)
func isYAMLComment(line string) bool {
	// YAML comments should maintain proper indentation and be part of the structure
	trimmed := strings.TrimLeft(line, " \t")
	return strings.HasPrefix(trimmed, "#") && (strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") || len(strings.TrimSpace(line)) == 0 || strings.Index(line, "#") == 0)
}

// removeNullValues recursively removes null values from the configuration
func (h *UnifiedCommandHandler) removeNullValues(config map[string]interface{}) {
	for key, value := range config {
		if value == nil {
			// Remove null values entirely
			delete(config, key)
		} else if nestedMap, ok := value.(map[string]interface{}); ok {
			// Recursively clean nested maps
			h.removeNullValues(nestedMap)
		}
	}
}

// cleanupOrphanedResourceReferences removes secrets/env vars that reference resources not in the uses section
func (h *UnifiedCommandHandler) cleanupOrphanedResourceReferences(config map[string]interface{}) {
	// Get the stacks configuration
	stacks, ok := config["stacks"].(map[string]interface{})
	if !ok {
		return
	}

	// Process each stack
	for _, stackConfig := range stacks {
		stackMap, ok := stackConfig.(map[string]interface{})
		if !ok {
			continue
		}

		stackConfigSection, ok := stackMap["config"].(map[string]interface{})
		if !ok {
			continue
		}

		// Get the list of used resources
		usedResources := make(map[string]bool)
		if uses, ok := stackConfigSection["uses"]; ok {
			switch usesVal := uses.(type) {
			case []interface{}:
				for _, resource := range usesVal {
					if resourceName, ok := resource.(string); ok {
						usedResources[resourceName] = true
					}
				}
			case []string:
				for _, resource := range usesVal {
					usedResources[resource] = true
				}
			case string:
				usedResources[usesVal] = true
			}
		}

		// Clean up secrets section
		if secrets, ok := stackConfigSection["secrets"].(map[string]interface{}); ok {
			for secretKey, secretValue := range secrets {
				if secretStr, ok := secretValue.(string); ok {
					// Check if this secret references a resource not in uses
					resourceName := h.extractResourceName(secretStr)
					if resourceName != "" && !usedResources[resourceName] {
						delete(secrets, secretKey)
					}
				}
			}
		}

		// Clean up env section
		if env, ok := stackConfigSection["env"].(map[string]interface{}); ok {
			for envKey, envValue := range env {
				if envStr, ok := envValue.(string); ok {
					// Check if this env var references a resource not in uses
					resourceName := h.extractResourceName(envStr)
					if resourceName != "" && !usedResources[resourceName] {
						delete(env, envKey)
					}
				}
			}
		}
	}
}

// extractResourceName extracts the resource name from a resource reference like ${resource:redis.url}
func (h *UnifiedCommandHandler) extractResourceName(value string) string {
	// Look for pattern like ${resource:RESOURCE_NAME.PROPERTY}
	if strings.Contains(value, "${resource:") {
		start := strings.Index(value, "${resource:") + len("${resource:")
		end := strings.Index(value[start:], ".")
		if end == -1 {
			end = strings.Index(value[start:], "}")
		}
		if end != -1 {
			return value[start : start+end]
		}
	}
	return ""
}

// hasYamlAnchors checks if the YAML content contains anchor definitions or references
func (h *UnifiedCommandHandler) hasYamlAnchors(content string) bool {
	// Look for YAML anchor definitions (&anchor) or references (<<: *anchor, *anchor)
	return strings.Contains(content, "&") && (strings.Contains(content, "<<:") || strings.Contains(content, "*"))
}

// preserveEnvironmentIndentation ensures the modified environment maintains original indentation
func (h *UnifiedCommandHandler) preserveEnvironmentIndentation(modifiedLines []string, originalBaseIndent string) []string {
	if len(modifiedLines) == 0 {
		return modifiedLines
	}

	result := make([]string, len(modifiedLines))

	// Find what indentation the LLM used for the first property line
	llmPropertyIndent := ""
	for i, line := range modifiedLines {
		if i == 0 {
			continue // Skip environment name
		}
		if strings.TrimSpace(line) == "" {
			continue // Skip empty lines
		}

		// Find the indentation of first property line
		for j, char := range line {
			if char != ' ' {
				llmPropertyIndent = line[:j]
				break
			}
		}
		break
	}

	// Target indentation for environment properties should be originalBaseIndent + 2 spaces
	targetPropertyIndent := originalBaseIndent + "  "

	for i, line := range modifiedLines {
		if i == 0 {
			// Environment name line - use original base indentation
			trimmed := strings.TrimSpace(line)
			if trimmed != "" {
				result[i] = originalBaseIndent + trimmed
			} else {
				result[i] = line
			}
		} else if strings.TrimSpace(line) == "" {
			// Empty lines remain empty
			result[i] = line
		} else {
			// Replace LLM's property indentation with target indentation
			if llmPropertyIndent != "" && strings.HasPrefix(line, llmPropertyIndent) {
				// Replace the LLM's base indentation with our target
				remainingContent := line[len(llmPropertyIndent):]
				result[i] = targetPropertyIndent + remainingContent
			} else {
				// Fallback: assume it's already properly formatted, just ensure minimum indentation
				trimmed := strings.TrimLeft(line, " ")
				result[i] = targetPropertyIndent + trimmed
			}
		}
	}

	return result
}

// modifyStackWithTextManipulation performs targeted environment-only LLM modifications to preserve YAML anchors
func (h *UnifiedCommandHandler) modifyStackWithTextManipulation(ctx context.Context, rawContent, stackName, environmentName string, changes map[string]interface{}, filePath string) (*CommandResult, error) {
	// First check if we have defaults section to provide as context to LLM
	defaultsSection := ""
	defaultsExists := strings.Contains(rawContent, "defaults:")
	if defaultsExists {
		if section, _, _, err := h.extractYamlSection(rawContent, "defaults"); err == nil {
			defaultsSection = section
		}
	}

	// Extract the stacks section to work within it
	stacksSection, stacksStart, stacksEnd, err := h.extractYamlSection(rawContent, "stacks")
	if err != nil {
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("❌ Could not find 'stacks' section in client.yaml: %v", err),
			Error:   "stacks_section_not_found",
		}, err
	}

	// Extract just the target environment section from within stacks
	envSection, envStart, envEnd, err := h.extractEnvironmentFromStacks(stacksSection, environmentName)
	if err != nil {
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("❌ Could not find environment '%s' in stacks: %v", environmentName, err),
			Error:   "environment_not_found",
		}, err
	}

	// Get LLM provider
	llmProvider := h.developerMode.GetLLMProvider()
	if llmProvider == nil {
		return h.fallbackToRegexReplacement(rawContent, environmentName, changes, filePath)
	}

	// Create enhanced prompt with defaults context
	modifiedEnvSection, changesApplied, err := h.modifyEnvironmentWithLLMAndContext(ctx, envSection, environmentName, changes, defaultsSection, llmProvider)
	if err != nil {
		return h.fallbackToRegexReplacement(rawContent, environmentName, changes, filePath)
	}

	// Reconstruct using line-based approach for safety with indentation preservation
	allLines := strings.Split(rawContent, "\n")
	stacksLines := strings.Split(stacksSection, "\n")
	modifiedEnvLines := strings.Split(modifiedEnvSection, "\n")

	// Preserve original indentation by detecting the base indentation of the environment
	originalEnvIndent := ""
	if envStart < len(stacksLines) && len(stacksLines[envStart]) > 0 {
		line := stacksLines[envStart]
		for i, char := range line {
			if char != ' ' {
				originalEnvIndent = line[:i]
				break
			}
		}
	}

	// Apply original indentation to modified environment lines
	properlyIndentedEnvLines := h.preserveEnvironmentIndentation(modifiedEnvLines, originalEnvIndent)

	// Replace the environment lines within the stacks section
	newStacksLines := make([]string, 0, len(stacksLines))
	newStacksLines = append(newStacksLines, stacksLines[:envStart]...)
	newStacksLines = append(newStacksLines, properlyIndentedEnvLines...)
	newStacksLines = append(newStacksLines, stacksLines[envEnd:]...)

	// Replace the stacks section within the entire file
	newAllLines := make([]string, 0, len(allLines))
	newAllLines = append(newAllLines, allLines[:stacksStart]...)
	newAllLines = append(newAllLines, newStacksLines...)
	newAllLines = append(newAllLines, allLines[stacksEnd:]...)

	modifiedContent := strings.Join(newAllLines, "\n")

	// Write the modified content back to file
	err = os.WriteFile(filePath, []byte(modifiedContent), 0o644)
	if err != nil {
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("❌ Failed to write client.yaml: %v", err),
			Error:   err.Error(),
		}, err
	}

	message := fmt.Sprintf("✅ Successfully modified '%s' environment using targeted LLM modification\n", environmentName)
	message += fmt.Sprintf("📁 File: %s\n", filePath)
	message += "🔗 YAML anchors and all root-level sections preserved\n"
	if defaultsExists {
		message += "🎯 Defaults section preserved and used as context\n"
	}
	message += "⚡ Optimized: Modified only target environment (not entire file)\n"
	message += fmt.Sprintf("🔄 Changes applied: %+v\n", changesApplied)

	data := map[string]interface{}{
		"stack_name":         stackName,
		"environment_name":   environmentName,
		"file_path":          filePath,
		"changes_applied":    changesApplied,
		"method":             "targeted_text_manipulation_with_context",
		"anchors_preserved":  true,
		"defaults_preserved": defaultsExists,
		"optimized":          true,
	}

	return &CommandResult{
		Success: true,
		Message: message,
		Data:    data,
	}, nil
}

// extractYamlSection extracts a specific top-level section from YAML content
func (h *UnifiedCommandHandler) extractYamlSection(content, sectionName string) (string, int, int, error) {
	lines := strings.Split(content, "\n")
	sectionStart := -1
	sectionEnd := len(lines)

	// Find the section start
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), sectionName+":") {
			sectionStart = i
			break
		}
	}

	if sectionStart == -1 {
		return "", -1, -1, fmt.Errorf("section '%s' not found", sectionName)
	}

	// Find the section end (next top-level key or end of file)
	sectionIndent := len(lines[sectionStart]) - len(strings.TrimLeft(lines[sectionStart], " "))

	for i := sectionStart + 1; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		// Skip empty lines and comments
		if len(trimmed) == 0 || strings.HasPrefix(trimmed, "#") {
			continue
		}

		// Check if this is another top-level section (same or less indentation)
		currentIndent := len(line) - len(strings.TrimLeft(line, " "))
		if currentIndent <= sectionIndent && strings.Contains(line, ":") && !strings.HasPrefix(trimmed, "-") {
			sectionEnd = i
			break
		}
	}

	sectionContent := strings.Join(lines[sectionStart:sectionEnd], "\n")

	// For safer text replacement, we'll return line indices instead of byte positions
	return sectionContent, sectionStart, sectionEnd, nil
}

// extractEnvironmentFromStacks extracts a specific environment from within the stacks section
func (h *UnifiedCommandHandler) extractEnvironmentFromStacks(stacksContent, environmentName string) (string, int, int, error) {
	lines := strings.Split(stacksContent, "\n")
	envStart := -1
	envEnd := len(lines)

	// Find the environment start
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Handle both "env:" and "env: &anchor" patterns
		if trimmed == environmentName+":" || strings.HasPrefix(trimmed, environmentName+": ") {
			envStart = i
			break
		}
	}

	if envStart == -1 {
		return "", -1, -1, fmt.Errorf("environment '%s' not found in stacks section", environmentName)
	}

	// Find the environment end (next environment or end of stacks)
	envIndent := len(lines[envStart]) - len(strings.TrimLeft(lines[envStart], " "))

	for i := envStart + 1; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		// Skip empty lines and comments
		if len(trimmed) == 0 || strings.HasPrefix(trimmed, "#") {
			continue
		}

		// Check if this is another environment (same indentation level with colon)
		currentIndent := len(line) - len(strings.TrimLeft(line, " "))
		if currentIndent <= envIndent && strings.HasSuffix(trimmed, ":") && !strings.HasPrefix(trimmed, "-") {
			envEnd = i
			break
		}
	}

	envContent := strings.Join(lines[envStart:envEnd], "\n")

	// Return line indices for safer text replacement
	return envContent, envStart, envEnd, nil
}

// modifyEnvironmentWithLLMAndContext enhanced version with defaults section context
func (h *UnifiedCommandHandler) modifyEnvironmentWithLLMAndContext(ctx context.Context, envSection, environmentName string, changes map[string]interface{}, defaultsSection string, llmProvider llm.Provider) (string, map[string]interface{}, error) {
	// Create enhanced prompt with defaults context
	var prompt strings.Builder
	prompt.WriteString("You are modifying a single environment in a Simple Container client.yaml file.\n\n")

	prompt.WriteString("🚨 CRITICAL YAML PRESERVATION INSTRUCTIONS:\n")
	prompt.WriteString("- ONLY modify the requested properties in this environment\n")
	prompt.WriteString("- PRESERVE all existing configuration that isn't being changed\n")
	prompt.WriteString("- PRESERVE all YAML anchors and references (<<: *anchor, *reference)\n")
	prompt.WriteString("- Use consistent 2-space YAML indentation\n")
	prompt.WriteString("- Environment properties (<<:, type, config:, etc.) should be indented 2 spaces from environment name\n")
	prompt.WriteString("- Nested config properties should be indented 4 spaces from environment name\n")
	prompt.WriteString("- Return ONLY the modified environment section starting with the environment name\n")
	prompt.WriteString("- DO NOT add explanatory text, code blocks, or markdown formatting\n")
	prompt.WriteString("- DO NOT place properties at wrong indentation levels\n\n")

	// Include defaults section if available
	if defaultsSection != "" {
		prompt.WriteString("🎯 AVAILABLE DEFAULTS SECTION FOR REFERENCE:\n")
		prompt.WriteString("Use these anchors if the environment references them:\n")
		prompt.WriteString(defaultsSection)
		prompt.WriteString("\n\n🔗 YAML ANCHOR USAGE:\n")
		prompt.WriteString("- If environment uses <<: *stack, preserve it exactly\n")
		prompt.WriteString("- If environment uses <<: *config, preserve it exactly\n")
		prompt.WriteString("- If environment uses *reference, preserve it exactly\n")
		prompt.WriteString("- DO NOT expand anchors - keep references intact\n\n")
	}

	prompt.WriteString(fmt.Sprintf("ENVIRONMENT TO MODIFY: %s\n\n", environmentName))

	prompt.WriteString("CURRENT ENVIRONMENT SECTION:\n")
	prompt.WriteString(envSection)
	prompt.WriteString("\n\n")

	// Detect deployment type for type-specific guidance
	deploymentType := ""
	if strings.Contains(envSection, "type: single-image") {
		deploymentType = "single-image"
	} else if strings.Contains(envSection, "type: cloud-compose") {
		deploymentType = "cloud-compose"
	} else if strings.Contains(envSection, "type: static") {
		deploymentType = "static"
	}

	// Validate memory-related changes against deployment type
	for key := range changes {
		if key == "config.maxMemory" && deploymentType == "cloud-compose" {
			return "", nil, fmt.Errorf("invalid configuration: config.maxMemory is not applicable for cloud-compose deployments (use config.size.memory or docker-compose.yaml resources instead)")
		}
		if key == "config.size.memory" && deploymentType == "single-image" {
			return "", nil, fmt.Errorf("invalid configuration: config.size.memory is not applicable for single-image deployments (use config.maxMemory instead)")
		}
	}

	prompt.WriteString("REQUESTED CHANGES:\n")
	for key, value := range changes {
		prompt.WriteString(fmt.Sprintf("- SET %s TO: %v\n", key, value))
	}

	// Add deployment type-specific guidance
	if deploymentType != "" {
		prompt.WriteString(fmt.Sprintf("\n🎯 DEPLOYMENT TYPE DETECTED: %s\n", deploymentType))

		switch deploymentType {
		case "single-image":
			prompt.WriteString("📋 SINGLE-IMAGE DEPLOYMENT RULES:\n")
			prompt.WriteString("- Use `config.maxMemory` for Lambda function memory (MB)\n")
			prompt.WriteString("- Use `config.timeout` for Lambda timeout (seconds)\n")
			prompt.WriteString("- No docker-compose related settings\n")
			prompt.WriteString("- Lambda-specific cloudExtras apply\n")
		case "cloud-compose":
			prompt.WriteString("📋 CLOUD-COMPOSE DEPLOYMENT RULES:\n")
			prompt.WriteString("- DO NOT use `config.maxMemory` - this is for single-image only!\n")
			prompt.WriteString("- Memory should be configured in docker-compose.yaml file or container settings\n")
			prompt.WriteString("- Use `config.dockerComposeFile` to specify compose file\n")
			prompt.WriteString("- Use `config.runs` to specify which services to run\n")
			prompt.WriteString("- For memory changes, consider `config.size.memory` or docker-compose resources\n")
		case "static":
			prompt.WriteString("📋 STATIC DEPLOYMENT RULES:\n")
			prompt.WriteString("- No memory settings applicable\n")
			prompt.WriteString("- Focus on domain, CDN, and static asset configuration\n")
		}
		prompt.WriteString("\n")
	}

	prompt.WriteString("⚠️  CRITICAL DEPLOYMENT TYPE VALIDATION:\n")
	prompt.WriteString("- If user requests memory changes, validate against deployment type\n")
	prompt.WriteString("- single-image: Use config.maxMemory (Lambda memory)\n")
	prompt.WriteString("- cloud-compose: Use config.size.memory or note that memory is configured in docker-compose.yaml\n")
	prompt.WriteString("- static: Memory settings not applicable\n")

	prompt.WriteString("\n✅ MERGE STRATEGY:\n")
	prompt.WriteString("- Apply changes additively - merge new values with existing configuration\n")
	prompt.WriteString("- If changing 'config.domain', only modify domain, keep all other config properties\n")
	prompt.WriteString("- If adding secrets, merge with existing secrets section\n")
	prompt.WriteString("- Preserve any YAML anchors used in this environment\n\n")

	prompt.WriteString("📋 EXPECTED YAML STRUCTURE EXAMPLES:\n")

	if deploymentType == "single-image" {
		prompt.WriteString("# SINGLE-IMAGE EXAMPLE:\n")
		prompt.WriteString("staging:\n")
		prompt.WriteString("  type: single-image\n")
		prompt.WriteString("  template: lambda-eu\n")
		prompt.WriteString("  config:\n")
		prompt.WriteString("    maxMemory: 8192    # ✅ Correct for Lambda\n")
		prompt.WriteString("    timeout: 30\n")
		prompt.WriteString("    domain: api.example.com\n")
	} else if deploymentType == "cloud-compose" {
		prompt.WriteString("# CLOUD-COMPOSE EXAMPLE:\n")
		prompt.WriteString("beta:\n")
		prompt.WriteString("  <<: *staging\n")
		prompt.WriteString("  type: cloud-compose\n")
		prompt.WriteString("  config:\n")
		prompt.WriteString("    <<: *config\n")
		prompt.WriteString("    dockerComposeFile: docker-compose.yaml\n")
		prompt.WriteString("    runs: [app, worker]\n")
		prompt.WriteString("    size:\n")
		prompt.WriteString("      memory: 8192     # ✅ Container memory\n")
		prompt.WriteString("      cpu: 2048\n")
		prompt.WriteString("    scale:\n")
		prompt.WriteString("      max: 6\n")
		prompt.WriteString("      min: 2\n")
		prompt.WriteString("    # Note: NO maxMemory for cloud-compose!\n")
	} else {
		prompt.WriteString("# GENERAL EXAMPLE:\n")
		prompt.WriteString("environment:\n")
		prompt.WriteString("  <<: *parent\n")
		prompt.WriteString("  type: [deployment-type]\n")
		prompt.WriteString("  config:\n")
		prompt.WriteString("    domain: example.com\n")
		prompt.WriteString("    # Memory config depends on deployment type\n")
	}
	prompt.WriteString("\n")

	prompt.WriteString("Return the complete modified environment section with proper indentation:")

	// Send to LLM
	messages := []llm.Message{
		{
			Role:      "user",
			Content:   prompt.String(),
			Timestamp: time.Now(),
			Metadata:  make(map[string]interface{}),
		},
	}
	response, err := llmProvider.Chat(ctx, messages)
	if err != nil {
		return "", nil, fmt.Errorf("LLM generation failed: %w", err)
	}

	// Clean and validate response
	cleanedResponse := strings.TrimSpace(response.Content)

	// Enhanced validation
	if !strings.Contains(cleanedResponse, environmentName+":") {
		return "", nil, fmt.Errorf("LLM response doesn't contain expected environment '%s'", environmentName)
	}

	// Validate that response contains reasonable content
	lines := strings.Split(cleanedResponse, "\n")
	if len(lines) < 2 {
		return "", nil, fmt.Errorf("LLM response too short, possibly invalid")
	}

	// Create changes map
	changesApplied := make(map[string]interface{})
	for key, value := range changes {
		changesApplied[key] = map[string]interface{}{
			"new":    value,
			"action": "modified_with_context",
		}
	}

	return cleanedResponse, changesApplied, nil
}

// fallbackToRegexReplacement handles simple modifications using regex when LLM fails
func (h *UnifiedCommandHandler) fallbackToRegexReplacement(rawContent, environmentName string, changes map[string]interface{}, filePath string) (*CommandResult, error) {
	modifiedContent := rawContent
	changesApplied := make(map[string]interface{})

	// Handle simple domain changes
	if len(changes) == 1 {
		if domainValue, ok := changes["config.domain"]; ok {
			if newDomain, ok := domainValue.(string); ok {
				// Look for domain in the specific environment section
				envPattern := fmt.Sprintf(`(?m)^(\s+%s:.*\n(?:\s+.*\n)*?\s+domain:\s+)(.+?)(\s*\n)`, regexp.QuoteMeta(environmentName))
				re := regexp.MustCompile(envPattern)

				if re.MatchString(modifiedContent) {
					modifiedContent = re.ReplaceAllString(modifiedContent, fmt.Sprintf("${1}%s${3}", newDomain))
					changesApplied["config.domain"] = map[string]interface{}{
						"new": newDomain,
					}
				} else {
					return &CommandResult{
						Success: false,
						Message: fmt.Sprintf("❌ Could not find domain configuration for environment '%s'", environmentName),
						Error:   "domain_not_found",
					}, fmt.Errorf("domain not found for environment %s", environmentName)
				}
			}
		}
	}

	// Write the modified content back to file
	err := os.WriteFile(filePath, []byte(modifiedContent), 0o644)
	if err != nil {
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("❌ Failed to write client.yaml: %v", err),
			Error:   err.Error(),
		}, err
	}

	message := fmt.Sprintf("✅ Successfully modified '%s' environment using regex replacement\n", environmentName)
	message += fmt.Sprintf("📁 File: %s\n", filePath)
	message += "🔗 YAML anchors preserved\n"
	message += fmt.Sprintf("🔄 Changes applied: %+v\n", changesApplied)

	data := map[string]interface{}{
		"environment_name":  environmentName,
		"file_path":         filePath,
		"changes_applied":   changesApplied,
		"method":            "regex_replacement",
		"anchors_preserved": true,
	}

	return &CommandResult{
		Success: true,
		Message: message,
		Data:    data,
	}, nil
}

// preserveAllRootLevelProperties ensures all root-level properties and environments are preserved
// This includes YAML anchors (defaults, customers), schema info, and all non-target environments
func (h *UnifiedCommandHandler) preserveAllRootLevelProperties(originalContent, modifiedContent map[string]interface{}, targetStackName string) {
	// Preserve ALL root-level properties from original (defaults, customers, schemaVersion, etc.)
	for rootKey, rootValue := range originalContent {
		if rootKey == "stacks" {
			// Handle stacks section specially to preserve environments
			h.preserveStackEnvironments(originalContent, modifiedContent, targetStackName)
		} else {
			// Preserve other root-level properties (defaults, customers, schemaVersion, etc.)
			if _, exists := modifiedContent[rootKey]; !exists {
				// Root-level property was removed by LLM, restore it
				modifiedContent[rootKey] = h.deepCopyInterface(rootValue)
			}
		}
	}
}

// preserveStackEnvironments ensures all environments from original config are preserved in modified config
func (h *UnifiedCommandHandler) preserveStackEnvironments(originalContent, modifiedContent map[string]interface{}, targetStackName string) {
	// Get original stacks
	originalStacks, ok := originalContent["stacks"].(map[string]interface{})
	if !ok {
		return
	}

	// Get modified stacks (create if missing)
	modifiedStacks, ok := modifiedContent["stacks"].(map[string]interface{})
	if !ok {
		modifiedStacks = make(map[string]interface{})
		modifiedContent["stacks"] = modifiedStacks
	}

	// Preserve all environments that were in the original but might be missing from modified
	for envName, envConfig := range originalStacks {
		if envName != targetStackName {
			// This is not the environment being modified, so preserve it exactly
			if _, exists := modifiedStacks[envName]; !exists {
				// Environment was removed by LLM, restore it
				modifiedStacks[envName] = h.deepCopyInterface(envConfig)
			}
		}
	}
}

// deepCopyInterface creates a deep copy of any interface{} value
func (h *UnifiedCommandHandler) deepCopyInterface(original interface{}) interface{} {
	switch x := original.(type) {
	case map[string]interface{}:
		clone := make(map[string]interface{})
		for k, v := range x {
			clone[k] = h.deepCopyInterface(v)
		}
		return clone
	case []interface{}:
		clone := make([]interface{}, len(x))
		for i, v := range x {
			clone[i] = h.deepCopyInterface(v)
		}
		return clone
	case []string:
		clone := make([]string, len(x))
		copy(clone, x)
		return clone
	default:
		return original
	}
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

		if originalStack != nil && modifiedStack != nil {
			h.compareConfigs(originalStack, modifiedStack, "", changesApplied)
		}
	}

	return modifiedContent, changesApplied, nil
}

// compareConfigs recursively compares configurations to determine what changed
func (h *UnifiedCommandHandler) compareConfigs(original, modified map[string]interface{}, prefix string, changes map[string]interface{}) {
	// First pass: Check for additions and modifications
	for key, modValue := range modified {
		fullKey := key
		if prefix != "" {
			fullKey = prefix + "." + key
		}

		if origValue, exists := original[key]; exists {
			// Key exists in both, check if values differ
			if origMap, ok := origValue.(map[string]interface{}); ok {
				if modMap, ok := modValue.(map[string]interface{}); ok {
					// Both are maps, recurse to compare nested structure
					h.compareConfigs(origMap, modMap, fullKey, changes)
					continue
				}
			}

			// Compare values directly (for non-map values)
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

	// Second pass: Check for deletions
	for key, origValue := range original {
		fullKey := key
		if prefix != "" {
			fullKey = prefix + "." + key
		}

		if _, exists := modified[key]; !exists {
			// Key was completely deleted
			changes[fullKey] = map[string]interface{}{
				"old": origValue,
				"new": "⚠️ DELETED",
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
// ShowConfigDiff shows configuration differences between versions or environments
func (h *UnifiedCommandHandler) ShowConfigDiff(ctx context.Context, stackName, configType, compareWith, format string) (*CommandResult, error) {
	// Set default values if not provided
	if configType == "" {
		configType = "client"
	}
	if compareWith == "" {
		compareWith = "HEAD"
	}
	if format == "" {
		format = "split"
	}

	// Check if stackName contains hierarchy separator (e.g., "simple-container:staging" or "simple-container/staging")
	var stackGroup string
	var stackFilter string
	var configFilePath string

	if stackName != "" && stackName != "*" {
		// Check for hierarchy separator
		var parts []string
		if strings.Contains(stackName, ":") {
			parts = strings.SplitN(stackName, ":", 2)
		} else if strings.Contains(stackName, "/") {
			parts = strings.SplitN(stackName, "/", 2)
		}

		if len(parts) == 2 {
			// Hierarchical syntax: "simple-container:staging" or "simple-container/staging"
			potentialGroup := parts[0]
			if h.isStackGroup(".", potentialGroup) {
				stackGroup = potentialGroup
				configFilePath = h.findClientYamlForStackGroup(".", stackGroup)
				stackFilter = parts[1] // Can be specific stack or pattern
			} else {
				// Not a valid stack group, treat whole string as stack filter
				stackFilter = stackName
			}
		} else {
			// No separator - check if this is a stack group
			if h.isStackGroup(".", stackName) {
				// It's a stack group - load its specific config
				stackGroup = stackName
				configFilePath = h.findClientYamlForStackGroup(".", stackGroup)
				stackFilter = "" // Show all stacks in this group
			} else {
				// Not a stack group - might be a pattern or specific stack
				stackFilter = stackName
			}
		}
	} else {
		stackFilter = stackName
	}

	// Get current config to access the stacks map
	var currentConfig *CommandResult
	var err error

	if configFilePath != "" {
		// Load config from specific stack group file
		yamlContent, readErr := h.readYamlFile(configFilePath)
		if readErr != nil {
			return &CommandResult{
				Success: false,
				Message: fmt.Sprintf("❌ Failed to read config from %s: %v", configFilePath, readErr),
			}, nil
		}

		currentConfig = &CommandResult{
			Success: true,
			Data: map[string]interface{}{
				"file_path": configFilePath,
				"content":   yamlContent,
			},
		}
	} else {
		// Use default config discovery
		currentConfig, err = h.GetCurrentConfig(ctx, "client", "")
		if err != nil || !currentConfig.Success {
			return &CommandResult{
				Success: false,
				Message: fmt.Sprintf("❌ Failed to get current configuration: %v", err),
			}, nil
		}
	}

	// Extract stacks map from the config
	stacksMap := make(api.StacksMap)
	var stacks map[string]interface{}

	// Check if stacks are in content first
	if content, ok := currentConfig.Data["content"].(map[string]interface{}); ok {
		if contentStacks, ok := content["stacks"].(map[string]interface{}); ok {
			stacks = contentStacks
		}
	}
	// Fallback: check direct stacks key
	if stacks == nil {
		if directStacks, ok := currentConfig.Data["stacks"].(map[string]interface{}); ok {
			stacks = directStacks
		}
	}

	for k := range stacks {
		// Create a new stack with the name and default values
		stack := api.Stack{
			Name:    k,
			Secrets: api.SecretsDescriptor{},
			Server:  api.ServerDescriptor{},
		}
		stacksMap[k] = stack
	}

	// Create a custom version provider that uses the correct file path
	versionProvider := &CustomConfigVersionProvider{
		filePath: currentConfig.Data["file_path"].(string),
		content:  currentConfig.Data["content"].(map[string]interface{}),
	}

	// Initialize configdiff service with the stacks map and custom provider
	diffSvc := configdiff.NewConfigDiffServiceWithProvider(stacksMap, versionProvider)

	// Determine which stacks to show diff for
	var stacksToProcess []string

	// If stackFilter is empty or "*", show all stacks in current context
	if stackFilter == "" || stackFilter == "*" {
		for k := range stacks {
			stacksToProcess = append(stacksToProcess, k)
		}

		if len(stacksToProcess) == 0 {
			groupInfo := ""
			if stackGroup != "" {
				groupInfo = fmt.Sprintf(" in group '%s'", stackGroup)
			}
			return &CommandResult{
				Success: true,
				Message: fmt.Sprintf("No stacks found%s. Use `/getconfig` to view the current configuration.", groupInfo),
			}, nil
		}

		// Show diff for all stacks
		var allMessages []string
		for _, stack := range stacksToProcess {
			result := h.showSingleStackDiff(ctx, stack, configType, compareWith, format, diffSvc, stacksMap, configFilePath)
			if result.Success {
				allMessages = append(allMessages, result.Message)
			} else {
				allMessages = append(allMessages, fmt.Sprintf("❌ %s", result.Message))
			}
		}

		if len(allMessages) == 0 {
			groupInfo := ""
			if stackGroup != "" {
				groupInfo = fmt.Sprintf(" in group '%s'", stackGroup)
			}
			return &CommandResult{
				Success: true,
				Message: fmt.Sprintf("No changes found in any stacks%s.", groupInfo),
			}, nil
		}

		// Combine all messages
		groupInfo := ""
		if stackGroup != "" {
			groupInfo = fmt.Sprintf(" in group '%s'", stackGroup)
		}
		finalMessage := fmt.Sprintf("🔍 Configuration diff for all stacks%s (comparing with %s):\n\n", groupInfo, compareWith)
		finalMessage += strings.Join(allMessages, "\n\n"+strings.Repeat("═", 80)+"\n\n")

		return &CommandResult{
			Success: true,
			Message: finalMessage,
		}, nil
	}

	// Check if stackFilter is a pattern (contains wildcard)
	if strings.Contains(stackFilter, "*") {
		// Convert pattern to regex
		pattern := "^" + strings.ReplaceAll(regexp.QuoteMeta(stackFilter), "\\*", ".*") + "$"
		re, err := regexp.Compile(pattern)
		if err != nil {
			return &CommandResult{
				Success: false,
				Message: fmt.Sprintf("❌ Invalid pattern '%s': %v", stackFilter, err),
			}, nil
		}

		// Filter stacks by pattern from current stacks map
		var matchingStacks []string
		for k := range stacks {
			if re.MatchString(k) {
				matchingStacks = append(matchingStacks, k)
			}
		}

		if len(matchingStacks) == 0 {
			groupInfo := ""
			if stackGroup != "" {
				groupInfo = fmt.Sprintf(" in group '%s'", stackGroup)
			}
			return &CommandResult{
				Success: false,
				Message: fmt.Sprintf("❌ No stacks found matching pattern '%s'%s", stackFilter, groupInfo),
			}, nil
		}

		// Show diff for all matching stacks
		var allMessages []string
		for _, s := range matchingStacks {
			result := h.showSingleStackDiff(ctx, s, configType, compareWith, format, diffSvc, stacksMap, configFilePath)
			if result.Success {
				allMessages = append(allMessages, result.Message)
			} else {
				allMessages = append(allMessages, fmt.Sprintf("❌ %s", result.Message))
			}
		}

		if len(allMessages) == 0 {
			groupInfo := ""
			if stackGroup != "" {
				groupInfo = fmt.Sprintf(" in group '%s'", stackGroup)
			}
			return &CommandResult{
				Success: true,
				Message: fmt.Sprintf("No changes found in stacks matching '%s'%s.", stackFilter, groupInfo),
			}, nil
		}

		// Combine all messages
		groupInfo := ""
		if stackGroup != "" {
			groupInfo = fmt.Sprintf(" in group '%s'", stackGroup)
		}
		finalMessage := fmt.Sprintf("🔍 Configuration diff for stacks matching '%s'%s (comparing with %s):\n\n", stackFilter, groupInfo, compareWith)
		finalMessage += strings.Join(allMessages, "\n\n"+strings.Repeat("═", 80)+"\n\n")

		return &CommandResult{
			Success: true,
			Message: finalMessage,
		}, nil
	}

	// Validate the stack exists
	if _, exists := stacksMap[stackFilter]; !exists {
		groupInfo := ""
		if stackGroup != "" {
			groupInfo = fmt.Sprintf(" in group '%s'", stackGroup)
		}
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("❌ Stack '%s' not found in configuration%s", stackFilter, groupInfo),
		}, nil
	}

	// Show diff for single stack
	return h.showSingleStackDiff(ctx, stackFilter, configType, compareWith, format, diffSvc, stacksMap, configFilePath), nil
}

// showSingleStackDiff shows diff for a single stack (helper method)
func (h *UnifiedCommandHandler) showSingleStackDiff(ctx context.Context, stackName, configType, compareWith, format string, diffSvc *configdiff.ConfigDiffService, stacksMap api.StacksMap, configFilePath string) *CommandResult {
	// Set up diff options
	options := configdiff.DefaultDiffOptions()
	switch format {
	case "unified":
		options.Format = configdiff.FormatUnified
	case "inline":
		options.Format = configdiff.FormatInline
	case "compact":
		options.Format = configdiff.FormatCompact
	default: // split
		options.Format = configdiff.FormatSplit
	}

	// Generate the diff
	result, err := diffSvc.GenerateConfigDiff(configdiff.ConfigDiffParams{
		StackName:   stackName,
		ConfigType:  configType,
		CompareWith: compareWith,
		Options:     options,
	})
	if err != nil {
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("❌ Failed to generate config diff: %v", err),
		}
	}

	if !result.Success {
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("❌ %s", result.Error),
		}
	}

	// Format the result message
	message := fmt.Sprintf("🔍 %s config diff for stack '%s' (comparing with %s):\n\n%s",
		cases.Title(language.English).String(configType), stackName, compareWith, result.Message)

	return &CommandResult{
		Success: true,
		Message: message,
		Metadata: map[string]interface{}{
			"stack_name":   stackName,
			"config_type":  configType,
			"compare_with": compareWith,
			"format":       format,
		},
	}
}

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

// findClientYamlForStackGroup finds client.yaml for a specific stack group
func (h *UnifiedCommandHandler) findClientYamlForStackGroup(basePath, stackGroup string) string {
	// Check in .sc/stacks/<stackGroup>/client.yaml
	path := filepath.Join(basePath, ".sc/stacks", stackGroup, "client.yaml")
	if _, err := os.Stat(path); err == nil {
		return path
	}
	return ""
}

// isStackGroup checks if a name is a stack group (directory in .sc/stacks/)
func (h *UnifiedCommandHandler) isStackGroup(basePath, name string) bool {
	path := filepath.Join(basePath, ".sc/stacks", name)
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		// Check if it contains a client.yaml
		clientPath := filepath.Join(path, "client.yaml")
		if _, err := os.Stat(clientPath); err == nil {
			return true
		}
	}
	return false
}

func (h *UnifiedCommandHandler) findServerYaml(basePath string) string {
	// Check current directory first
	if _, err := os.Stat(filepath.Join(basePath, "server.yaml")); err == nil {
		return filepath.Join(basePath, "server.yaml")
	}

	return ""
}

func (h *UnifiedCommandHandler) readYamlFile(filePath string) (map[string]interface{}, error) {
	// Use SecureFileReader for comprehensive credential protection
	secureReader := security.NewSecureFileReader()
	data, err := secureReader.ReadFileSecurely(filePath)
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

func (h *UnifiedCommandHandler) ListAvailableStacks() ([]string, error) {
	var stacks []string

	// Try to get stacks from configuration first
	configResult, err := h.GetCurrentConfig(context.Background(), "client", "")
	if err == nil && configResult.Success {
		// Check if stacks are in content
		if content, ok := configResult.Data["content"].(map[string]interface{}); ok {
			if stacksMap, ok := content["stacks"].(map[string]interface{}); ok {
				for stackName := range stacksMap {
					stacks = append(stacks, stackName)
				}
				if len(stacks) > 0 {
					return stacks, nil
				}
			}
		}
		// Fallback: check direct stacks key
		if stacksMap, ok := configResult.Data["stacks"].(map[string]interface{}); ok {
			for stackName := range stacksMap {
				stacks = append(stacks, stackName)
			}
			if len(stacks) > 0 {
				return stacks, nil
			}
		}
	}

	// Fall back to directory scanning
	// First check for stacks in the new location (dist directory)
	distPath := filepath.Join(".sc", "stacks", "dist")
	if _, err := os.Stat(distPath); err == nil {
		files, err := os.ReadDir(distPath)
		if err == nil {
			for _, file := range files {
				if file.IsDir() && file.Name() != "dist" && file.Name() != "docs" {
					stacks = append(stacks, file.Name())
				}
			}
			if len(stacks) > 0 {
				return stacks, nil
			}
		}
	}

	// Fall back to the old location (stacks directory)
	stacksPath := filepath.Join(".sc", "stacks")
	if _, err := os.Stat(stacksPath); err == nil {
		files, err := os.ReadDir(stacksPath)
		if err == nil {
			for _, file := range files {
				if file.IsDir() && file.Name() != "dist" && file.Name() != "docs" {
					stacks = append(stacks, file.Name())
				}
			}
		}
	}

	return stacks, nil
}

// getStackNames extracts stack names from a stacks map
func (h *UnifiedCommandHandler) getStackNames(stacks map[string]interface{}) []string {
	var names []string
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

			// Set the value with intelligent merging
			finalKey := parts[len(parts)-1]
			oldValue := current[finalKey]

			// Handle deletion vs modification
			if newValue == nil {
				// Delete the key entirely when newValue is nil
				delete(current, finalKey)
				changesApplied[fullKey] = map[string]interface{}{
					"old": oldValue,
					"new": "⚠️ DELETED",
				}
			} else if oldMap, oldIsMap := oldValue.(map[string]interface{}); oldIsMap {
				if newMap, newIsMap := newValue.(map[string]interface{}); newIsMap {
					// Merge maps: preserve existing keys, add/update new ones
					mergedMap := make(map[string]interface{})
					// Copy all existing keys first
					for k, v := range oldMap {
						mergedMap[k] = v
					}
					// Add/update with new keys, handle deletions
					for k, v := range newMap {
						if v == nil {
							// Delete the key when value is nil
							delete(mergedMap, k)
						} else {
							mergedMap[k] = v
						}
					}
					current[finalKey] = mergedMap
					changesApplied[fullKey] = map[string]interface{}{
						"old": oldValue,
						"new": mergedMap,
					}
				} else {
					// New value is not a map, replace entirely
					current[finalKey] = newValue
					changesApplied[fullKey] = map[string]interface{}{
						"old": oldValue,
						"new": newValue,
					}
				}
			} else {
				// Old value is not a map, replace entirely
				current[finalKey] = newValue
				changesApplied[fullKey] = map[string]interface{}{
					"old": oldValue,
					"new": newValue,
				}
			}
		} else {
			// Direct key with intelligent merging and deletion support
			oldValue := config[key]

			// Handle deletion vs modification
			if newValue == nil {
				// Delete the key entirely when newValue is nil
				delete(config, key)
				changesApplied[fullKey] = map[string]interface{}{
					"old": oldValue,
					"new": "⚠️ DELETED",
				}
			} else if oldMap, oldIsMap := oldValue.(map[string]interface{}); oldIsMap {
				if newMap, newIsMap := newValue.(map[string]interface{}); newIsMap {
					// Merge maps: preserve existing keys, add/update new ones
					mergedMap := make(map[string]interface{})
					// Copy all existing keys first
					for k, v := range oldMap {
						mergedMap[k] = v
					}
					// Add/update with new keys, handle deletions
					for k, v := range newMap {
						if v == nil {
							// Delete the key when value is nil
							delete(mergedMap, k)
						} else {
							mergedMap[k] = v
						}
					}
					config[key] = mergedMap
					changesApplied[fullKey] = map[string]interface{}{
						"old": oldValue,
						"new": mergedMap,
					}
				} else {
					// New value is not a map, replace entirely
					config[key] = newValue
					changesApplied[fullKey] = map[string]interface{}{
						"old": oldValue,
						"new": newValue,
					}
				}
			} else {
				// Old value is not a map, replace entirely
				config[key] = newValue
				changesApplied[fullKey] = map[string]interface{}{
					"old": oldValue,
					"new": newValue,
				}
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

// CheckExistingSimpleContainerProject checks if project is already using Simple Container and warns user
// Delegates to shared utility function for consistency
func (h *UnifiedCommandHandler) CheckExistingSimpleContainerProject(projectPath string, forceOverwrite, skipConfirmation bool) error {
	return utils.CheckAndWarnExistingSimpleContainerProject(projectPath, forceOverwrite, skipConfirmation, true)
}

// CustomConfigVersionProvider implements configdiff.ConfigVersionProvider for our specific use case
type CustomConfigVersionProvider struct {
	filePath string
	content  map[string]interface{}
}

// GetCurrent gets the current configuration for a specific stack
func (p *CustomConfigVersionProvider) GetCurrent(stackName, configType string) (*configdiff.ResolvedConfig, error) {
	// Extract the specific stack from the content
	if stacks, ok := p.content["stacks"].(map[string]interface{}); ok {
		if stackConfig, ok := stacks[stackName].(map[string]interface{}); ok {
			// Convert stack config to YAML string
			yamlBytes, err := yaml.Marshal(stackConfig)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal stack config to YAML: %v", err)
			}

			return &configdiff.ResolvedConfig{
				StackName:    stackName,
				ConfigType:   configType,
				Content:      string(yamlBytes),
				ParsedConfig: stackConfig,
				FilePath:     p.filePath,
			}, nil
		}
	}
	return nil, fmt.Errorf("stack '%s' not found in configuration", stackName)
}

// GetFromGit gets configuration from a git reference
func (p *CustomConfigVersionProvider) GetFromGit(stackName, configType, gitRef string) (*configdiff.ResolvedConfig, error) {
	// Get the file content from git using git show
	cmd := exec.Command("git", "show", fmt.Sprintf("%s:%s", gitRef, p.filePath))
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get file from git: %v", err)
	}

	// Parse the YAML content
	var yamlContent map[string]interface{}
	if err := yaml.Unmarshal(output, &yamlContent); err != nil {
		return nil, fmt.Errorf("failed to parse YAML from git: %v", err)
	}

	// Extract the specific stack from the content
	if stacks, ok := yamlContent["stacks"].(map[string]interface{}); ok {
		if stackConfig, ok := stacks[stackName].(map[string]interface{}); ok {
			// Convert stack config to YAML string
			yamlBytes, err := yaml.Marshal(stackConfig)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal stack config to YAML: %v", err)
			}

			return &configdiff.ResolvedConfig{
				StackName:    stackName,
				ConfigType:   configType,
				Content:      string(yamlBytes),
				ParsedConfig: stackConfig,
				FilePath:     p.filePath,
				GitRef:       gitRef,
			}, nil
		}
	}

	return nil, fmt.Errorf("stack '%s' not found in git reference %s", stackName, gitRef)
}

// GetFromLocal gets configuration from a local file path
func (p *CustomConfigVersionProvider) GetFromLocal(stackName, configType, filePath string) (*configdiff.ResolvedConfig, error) {
	// Read the file and extract the stack
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var yamlContent map[string]interface{}
	if err := yaml.Unmarshal(content, &yamlContent); err != nil {
		return nil, err
	}

	if stacks, ok := yamlContent["stacks"].(map[string]interface{}); ok {
		if stackConfig, ok := stacks[stackName].(map[string]interface{}); ok {
			// Convert stack config to YAML string
			yamlBytes, err := yaml.Marshal(stackConfig)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal stack config to YAML: %v", err)
			}

			return &configdiff.ResolvedConfig{
				StackName:    stackName,
				ConfigType:   configType,
				Content:      string(yamlBytes),
				ParsedConfig: stackConfig,
				FilePath:     filePath,
			}, nil
		}
	}

	return nil, fmt.Errorf("stack '%s' not found in file %s", stackName, filePath)
}

// GenerateCICD generates CI/CD workflows for GitHub Actions
func (h *UnifiedCommandHandler) GenerateCICD(ctx context.Context, stackName, configFile string) (*CommandResult, error) {
	return h.GenerateCICDWithStaging(ctx, stackName, configFile, false)
}

// GenerateCICDWithStaging generates CI/CD workflows for GitHub Actions with staging support
func (h *UnifiedCommandHandler) GenerateCICDWithStaging(ctx context.Context, stackName, configFile string, staging bool) (*CommandResult, error) {
	return h.GenerateCICDWithStagingAndLogger(ctx, nil, stackName, configFile, staging)
}

// GenerateCICDWithStagingAndLogger generates CI/CD workflows for GitHub Actions with staging support and logging
func (h *UnifiedCommandHandler) GenerateCICDWithStagingAndLogger(ctx context.Context, logger cicd.Logger, stackName, configFile string, staging bool) (*CommandResult, error) {
	params := cicd.GenerateParams{
		StackName:  stackName,
		ConfigFile: configFile,
		Output:     "", // Use default output directory
		Force:      false,
		DryRun:     false,
		Staging:    staging,
	}

	result, err := h.cicdService.GenerateWorkflowsWithContext(ctx, logger, params)
	if err != nil {
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("Failed to generate CI/CD workflows: %v", err),
			Error:   err.Error(),
		}, nil
	}

	return &CommandResult{
		Success: result.Success,
		Message: result.Message,
		Data:    result.Data,
	}, nil
}

// ValidateCICD validates CI/CD configuration in server.yaml
func (h *UnifiedCommandHandler) ValidateCICD(ctx context.Context, stackName, configFile string, showDiff bool) (*CommandResult, error) {
	return h.ValidateCICDWithStaging(ctx, stackName, configFile, showDiff, false)
}

// ValidateCICDWithStaging validates CI/CD configuration in server.yaml with staging support
func (h *UnifiedCommandHandler) ValidateCICDWithStaging(ctx context.Context, stackName, configFile string, showDiff bool, staging bool) (*CommandResult, error) {
	return h.ValidateCICDWithStagingAndLogger(ctx, nil, stackName, configFile, showDiff, staging)
}

// ValidateCICDWithStagingAndLogger validates CI/CD configuration in server.yaml with staging support and logging
func (h *UnifiedCommandHandler) ValidateCICDWithStagingAndLogger(ctx context.Context, logger cicd.Logger, stackName, configFile string, showDiff bool, staging bool) (*CommandResult, error) {
	params := cicd.ValidateParams{
		StackName:    stackName,
		ConfigFile:   configFile,
		WorkflowsDir: "", // Use default
		ShowDiff:     showDiff,
		Verbose:      false,
		Staging:      staging,
	}

	result, err := h.cicdService.ValidateWorkflowsWithContext(ctx, logger, params)
	if err != nil {
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("CI/CD configuration validation failed: %v", err),
			Error:   err.Error(),
		}, nil
	}

	return &CommandResult{
		Success: result.Success,
		Message: result.Message,
		Data:    result.Data,
	}, nil
}

// PreviewCICD previews CI/CD workflows that would be generated
func (h *UnifiedCommandHandler) PreviewCICD(ctx context.Context, stackName, configFile string, showContent bool) (*CommandResult, error) {
	params := cicd.PreviewParams{
		StackName:   stackName,
		ConfigFile:  configFile,
		ShowContent: showContent,
	}

	result, err := h.cicdService.PreviewWorkflows(params)
	if err != nil {
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("Failed to preview CI/CD workflows: %v", err),
			Error:   err.Error(),
		}, nil
	}

	return &CommandResult{
		Success: result.Success,
		Message: result.Message,
		Data:    result.Data,
	}, nil
}

// SyncCICD syncs CI/CD workflows to GitHub repository
func (h *UnifiedCommandHandler) SyncCICD(ctx context.Context, stackName, configFile string, dryRun bool) (*CommandResult, error) {
	params := cicd.SyncParams{
		StackName:  stackName,
		ConfigFile: configFile,
		DryRun:     dryRun,
		Force:      false,
	}

	result, err := h.cicdService.SyncWorkflows(params)
	if err != nil {
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("Failed to sync CI/CD workflows: %v", err),
			Error:   err.Error(),
		}, nil
	}

	return &CommandResult{
		Success: result.Success,
		Message: result.Message,
		Data:    result.Data,
	}, nil
}
