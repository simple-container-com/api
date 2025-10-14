package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"gopkg.in/yaml.v2"

	"github.com/simple-container-com/api/pkg/api/logger/color"
	"github.com/simple-container-com/api/pkg/assistant/analysis"
	"github.com/simple-container-com/api/pkg/assistant/modes"
	"github.com/simple-container-com/api/pkg/assistant/resources"
	"github.com/simple-container-com/api/pkg/assistant/security"
	"github.com/simple-container-com/api/pkg/assistant/utils"
)

// registerProjectCommands registers project analysis and configuration commands
func (c *ChatInterface) registerProjectCommands() {
	c.commands["analyze"] = &ChatCommand{
		Name:        "analyze",
		Description: "Analyze current project tech stack",
		Usage:       "/analyze [--full] [--force]",
		Handler:     c.handleAnalyze,
		Aliases:     []string{"a", "analyse"}, // Added British spelling
		Args: []CommandArg{
			{Name: "full", Type: "flag", Required: false, Description: "Run full analysis including resource detection (slower but comprehensive)"},
			{Name: "force", Type: "flag", Required: false, Description: "Force fresh analysis even if cache exists"},
		},
	}

	c.commands["setup"] = &ChatCommand{
		Name:        "setup",
		Description: "Generate configuration files for current project",
		Usage:       "/setup [--mode dev|devops]",
		Handler:     c.handleSetup,
		Aliases:     []string{"generate", "g"},
		Args: []CommandArg{
			{Name: "mode", Type: "string", Required: false, Description: "Setup mode", Default: "auto"},
		},
	}

	c.commands["config"] = &ChatCommand{
		Name:        "config",
		Description: "Display current Simple Container configuration with AI analysis",
		Usage:       "/config [--type client|server] [--stack <name>] [--explain]",
		Handler:     c.handleConfig,
		Aliases:     []string{"cfg"},
		Args: []CommandArg{
			{Name: "type", Type: "string", Required: false, Description: "Configuration type: 'client' or 'server'", Default: "client"},
			{Name: "stack", Type: "string", Required: false, Description: "Specific stack name (e.g. 'myapp', 'service-name')"},
			{Name: "explain", Type: "boolean", Required: false, Description: "Include AI-powered configuration analysis"},
		},
	}

	c.commands["context"] = &ChatCommand{
		Name:        "context",
		Description: "Get basic project context information",
		Usage:       "/context",
		Handler:     c.handleGetProjectContext,
		Args:        []CommandArg{},
	}

	c.commands["resources"] = &ChatCommand{
		Name:        "resources",
		Description: "List all supported Simple Container resources",
		Usage:       "/resources",
		Handler:     c.handleGetSupportedResources,
		Args:        []CommandArg{},
	}

	c.commands["file"] = &ChatCommand{
		Name:        "file",
		Description: "Read and display a project file (Dockerfile, docker-compose.yaml, etc.)",
		Usage:       "/file <filename>",
		Handler:     c.handleReadProjectFile,
		Aliases:     []string{"cat"},
		Args: []CommandArg{
			{Name: "filename", Type: "string", Required: true, Description: "File name to read (e.g., Dockerfile, docker-compose.yaml, package.json)"},
		},
	}

	c.commands["write"] = &ChatCommand{
		Name:        "write",
		Description: "Write content to a project file (create new or modify existing)",
		Usage:       "/write <filename> <content> [--lines start-end] [--append]",
		Handler:     c.handleWriteProjectFile,
		Aliases:     []string{"edit"},
		Args: []CommandArg{
			{Name: "filename", Type: "string", Required: true, Description: "File name to write (e.g., Dockerfile, docker-compose.yaml)"},
			{Name: "content", Type: "string", Required: true, Description: "Content to write to the file"},
			{Name: "lines", Type: "string", Required: false, Description: "Line range to replace (e.g., '10-20' or '5' for single line)"},
			{Name: "append", Type: "boolean", Required: false, Description: "Append content to end of file instead of replacing"},
		},
	}

	c.commands["show"] = &ChatCommand{
		Name:        "show",
		Description: "Show stack configuration (checks both client.yaml and server.yaml)",
		Usage:       "/show <stack_name> [--type client|server]",
		Handler:     c.handleShowStack,
		Aliases:     []string{"stack"},
		Args: []CommandArg{
			{Name: "stack_name", Type: "string", Required: true, Description: "Name of the stack to display (e.g., bewize, myapp)"},
			{Name: "type", Type: "string", Required: false, Description: "Configuration type: 'client' or 'server' (shows both if not specified)"},
		},
	}
}

// handleAnalyze analyzes the current project
func (c *ChatInterface) handleAnalyze(ctx context.Context, args []string, context *ConversationContext) (*CommandResult, error) {
	if context.ProjectPath == "" {
		return &CommandResult{
			Success: false,
			Message: "❌ No project directory set. Please navigate to your project directory.",
		}, nil
	}

	// Parse arguments to determine analysis mode
	fullAnalysis := false
	forceAnalysis := false

	for _, arg := range args {
		switch arg {
		case "--full", "-f":
			fullAnalysis = true
		case "--force":
			forceAnalysis = true
		}
	}

	// Check if cache exists and its completeness
	cacheExists := analysis.CacheExists(context.ProjectPath)
	hasResourcesInCache := analysis.HasResourcesInCache(context.ProjectPath)

	// Determine if we need to run full analysis despite cache
	needsFullAnalysis := forceAnalysis || (fullAnalysis && (!cacheExists || !hasResourcesInCache))

	if cacheExists && !forceAnalysis {
		if fullAnalysis && !hasResourcesInCache {
			fmt.Printf("📋 Found incomplete cached analysis (missing resources) for %s\n", color.YellowString(context.ProjectPath))
			fmt.Printf("🔍 Running full analysis to detect resources and environment variables...\n")
		} else {
			fmt.Printf("📋 Found cached analysis for %s\n", color.CyanString(context.ProjectPath))
		}
	} else {
		if fullAnalysis || needsFullAnalysis {
			fmt.Printf("🔍 Running full analysis at %s...\n", color.CyanString(context.ProjectPath))
		} else {
			fmt.Printf("🔍 Analyzing project at %s...\n", color.CyanString(context.ProjectPath))
		}
	}

	// Create analyzer and configure mode
	analyzer := analysis.NewProjectAnalyzer()

	// Set up progress reporting for better UX
	progressReporter := analysis.NewStreamingProgressReporter(os.Stdout)
	analyzer.SetProgressReporter(progressReporter)

	if forceAnalysis && fullAnalysis {
		analyzer.SetAnalysisMode(analysis.ForceFullMode)
	} else if forceAnalysis {
		analyzer.SetAnalysisMode(analysis.FullMode)
	} else if fullAnalysis {
		// If cache exists but lacks resources, use ForceFullMode to ensure we get everything
		if cacheExists && !hasResourcesInCache {
			analyzer.SetAnalysisMode(analysis.ForceFullMode)
		} else {
			analyzer.SetAnalysisMode(analysis.FullMode)
		}
	} else {
		analyzer.SetAnalysisMode(analysis.CachedMode)
	}

	result, err := analyzer.AnalyzeProject(context.ProjectPath)
	if err != nil {
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("❌ Analysis failed: %v", err),
		}, nil
	}

	// Update context with analysis results
	context.ProjectInfo = result

	// Format results for display
	message := "📊 Project Analysis Results:\n\n"

	if result.PrimaryStack != nil {
		message += fmt.Sprintf("🔧 **Primary Technology**: %s", result.PrimaryStack.Language)
		if result.PrimaryStack.Framework != "" {
			message += fmt.Sprintf(" (%s)", result.PrimaryStack.Framework)
		}
		message += "\n"
	}

	// Display comprehensive resource information
	if result.Resources != nil {
		resourceCount := 0

		// Environment Variables
		if len(result.Resources.EnvironmentVars) > 0 {
			if resourceCount == 0 {
				message += "\n📋 **Resources Detected**:\n"
			}
			message += fmt.Sprintf("  🌐 **Environment Variables**: %d found\n", len(result.Resources.EnvironmentVars))
			// Show first few environment variables as examples
			for i, envVar := range result.Resources.EnvironmentVars {
				if i >= 3 { // Limit to first 3 to avoid overwhelming output
					message += fmt.Sprintf("    • ... and %d more\n", len(result.Resources.EnvironmentVars)-3)
					break
				}
				message += fmt.Sprintf("    • %s\n", envVar.Name)
			}
			resourceCount++
		}

		// Databases
		if len(result.Resources.Databases) > 0 {
			if resourceCount == 0 {
				message += "\n📋 **Resources Detected**:\n"
			}
			message += fmt.Sprintf("  💾 **Databases**: %d found\n", len(result.Resources.Databases))
			for _, db := range result.Resources.Databases {
				message += fmt.Sprintf("    • %s (%s)\n", db.Name, db.Type)
			}
			resourceCount++
		}

		// External APIs
		if len(result.Resources.ExternalAPIs) > 0 {
			if resourceCount == 0 {
				message += "\n📋 **Resources Detected**:\n"
			}
			message += fmt.Sprintf("  🌐 **External APIs**: %d found\n", len(result.Resources.ExternalAPIs))
			for _, api := range result.Resources.ExternalAPIs {
				message += fmt.Sprintf("    • %s\n", api.Name)
			}
			resourceCount++
		}

		// Secrets
		if len(result.Resources.Secrets) > 0 {
			if resourceCount == 0 {
				message += "\n📋 **Resources Detected**:\n"
			}
			message += fmt.Sprintf("  🔐 **Secrets**: %d found\n", len(result.Resources.Secrets))
			for i, secret := range result.Resources.Secrets {
				if i >= 3 {
					message += fmt.Sprintf("    • ... and %d more\n", len(result.Resources.Secrets)-3)
					break
				}
				message += fmt.Sprintf("    • %s\n", secret.Name)
			}
			resourceCount++
		}

		// Storage
		if len(result.Resources.Storage) > 0 {
			if resourceCount == 0 {
				message += "\n📋 **Resources Detected**:\n"
			}
			message += fmt.Sprintf("  💽 **Storage**: %d found\n", len(result.Resources.Storage))
			for _, storage := range result.Resources.Storage {
				message += fmt.Sprintf("    • %s (%s)\n", storage.Name, storage.Type)
			}
			resourceCount++
		}

		// Queues
		if len(result.Resources.Queues) > 0 {
			if resourceCount == 0 {
				message += "\n📋 **Resources Detected**:\n"
			}
			message += fmt.Sprintf("  📨 **Queues**: %d found\n", len(result.Resources.Queues))
			for _, queue := range result.Resources.Queues {
				message += fmt.Sprintf("    • %s (%s)\n", queue.Name, queue.Type)
			}
			resourceCount++
		}

		if resourceCount == 0 {
			message += "\n📋 **Resources**: None detected (use `/analyze --full` for comprehensive resource detection)\n"
		}
	}

	if len(result.Recommendations) > 0 {
		message += "\n💡 **Deployment Recommendations**:\n"
		for _, rec := range result.Recommendations {
			message += fmt.Sprintf("  • %s: %s\n", rec.Type, rec.Description)
		}
	}

	return &CommandResult{
		Success: true,
		Message: message,
	}, nil
}

// handleSetup generates configuration files
func (c *ChatInterface) handleSetup(ctx context.Context, args []string, context *ConversationContext) (*CommandResult, error) {
	if context.ProjectInfo == nil {
		return &CommandResult{
			Success: false,
			Message: "Project not analyzed yet. Run /analyze first.",
		}, nil
	}

	// Check if project is already using Simple Container and warn the user
	if err := utils.CheckAndWarnExistingSimpleContainerProject(".", false, false, false); err != nil {
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("Setup cancelled: %v", err),
		}, nil
	}

	// Determine setup mode
	mode := context.Mode
	for _, arg := range args {
		if arg == "--mode" || strings.HasPrefix(arg, "--mode=") {
			if strings.Contains(arg, "=") {
				mode = strings.Split(arg, "=")[1]
			}
		} else if arg == "dev" || arg == "devops" {
			mode = arg
		}
	}

	var files []GeneratedFile
	var err error

	switch mode {
	case "dev", "developer":
		// Add deployment type confirmation for developer mode
		if err := c.confirmDeploymentTypeForChat(context); err != nil {
			return &CommandResult{
				Success: false,
				Message: fmt.Sprintf("Setup cancelled: %v", err),
			}, nil
		}
		files, err = c.generateDeveloperFiles(context)
	case "devops":
		files, err = c.generateDevOpsFiles(context)
	default:
		// Auto-detect based on project
		if context.ProjectInfo.PrimaryStack != nil {
			// Add deployment type confirmation for auto-detected developer mode
			if err := c.confirmDeploymentTypeForChat(context); err != nil {
				return &CommandResult{
					Success: false,
					Message: fmt.Sprintf("Setup cancelled: %v", err),
				}, nil
			}
			files, err = c.generateDeveloperFiles(context)
		} else {
			return &CommandResult{
				Success: false,
				Message: "Unable to determine setup mode. Specify --mode dev or --mode devops",
			}, nil
		}
	}

	if err != nil {
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("Setup failed: %v", err),
		}, nil
	}

	message := fmt.Sprintf("Generated %d configuration files for %s mode", len(files), mode)

	return &CommandResult{
		Success:  true,
		Message:  message,
		Files:    files,
		NextStep: "Review the generated files and run 'docker-compose up -d' from ${project:root} to test locally, then 'sc deploy -s ${project:name} -e staging' to deploy",
	}, nil
}

// handleConfig displays current Simple Container configuration
func (c *ChatInterface) handleConfig(ctx context.Context, args []string, context *ConversationContext) (*CommandResult, error) {
	// Parse arguments
	configType := "client"
	stackName := ""
	explain := false

	for i, arg := range args {
		switch {
		case strings.HasPrefix(arg, "--type="):
			configType = strings.TrimPrefix(arg, "--type=")
		case arg == "--type" && i+1 < len(args):
			configType = args[i+1]
		case strings.HasPrefix(arg, "--stack="):
			stackName = strings.TrimPrefix(arg, "--stack=")
		case arg == "--stack" && i+1 < len(args):
			stackName = args[i+1]
		case arg == "--explain":
			explain = true
		}
	}

	var filePath string
	switch configType {
	case "server":
		filePath = filepath.Join(".sc", "stacks", "infrastructure", "server.yaml")
	case "client":
		if stackName != "" {
			filePath = filepath.Join(".sc", "stacks", stackName, "client.yaml")
		} else {
			// Try to find client.yaml in current or subdirectories
			if _, err := os.Stat(filepath.Join(".sc", "stacks")); err == nil {
				entries, err := os.ReadDir(filepath.Join(".sc", "stacks"))
				if err == nil {
					for _, entry := range entries {
						if entry.IsDir() {
							clientPath := filepath.Join(".sc", "stacks", entry.Name(), "client.yaml")
							if _, err := os.Stat(clientPath); err == nil {
								filePath = clientPath
								break
							}
						}
					}
				}
			}
		}
	default:
		return &CommandResult{
			Success: false,
			Message: "❌ Invalid config type. Use 'client' or 'server'",
		}, nil
	}

	if filePath == "" {
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("❌ No %s configuration found", configType),
		}, nil
	}

	// Read the configuration file
	data, err := os.ReadFile(filePath)
	if err != nil {
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("❌ Failed to read %s: %v", filePath, err),
		}, nil
	}

	// Obfuscate credentials before exposing to LLM
	data = obfuscateCredentials(data, filePath)

	titleCaser := cases.Title(language.English)
	message := fmt.Sprintf("📋 **%s Configuration** (`%s`)\n\n", titleCaser.String(configType), filePath)
	message += "```yaml\n"
	message += string(data)
	message += "\n```\n"

	if explain && c.llm != nil {
		message += "\n🤖 **AI Analysis:**\n"

		// Parse YAML to provide analysis
		var config map[string]interface{}
		if err := yaml.Unmarshal(data, &config); err == nil {
			// Add basic analysis based on config content
			if stacks, ok := config["stacks"].(map[interface{}]interface{}); ok {
				message += fmt.Sprintf("• Found %d stack environments\n", len(stacks))
				for env := range stacks {
					message += fmt.Sprintf("• Environment: **%s**\n", env)
				}
			}
			if resources, ok := config["resources"].(map[interface{}]interface{}); ok {
				message += fmt.Sprintf("• Contains %d resource definitions\n", len(resources))
			}
		}
	}

	return &CommandResult{
		Success: true,
		Message: message,
	}, nil
}

// handleShowStack shows comprehensive stack configuration
func (c *ChatInterface) handleShowStack(ctx context.Context, args []string, context *ConversationContext) (*CommandResult, error) {
	if len(args) == 0 {
		return &CommandResult{
			Success: false,
			Message: "❌ Please specify a stack name. Usage: /show <stack_name>",
		}, nil
	}

	stackName := args[0]
	showType := "" // Show both by default

	// Parse arguments
	for i, arg := range args {
		switch {
		case strings.HasPrefix(arg, "--type="):
			showType = strings.TrimPrefix(arg, "--type=")
		case arg == "--type" && i+1 < len(args):
			showType = args[i+1]
		}
	}

	// Check for stack directory
	stackDir := filepath.Join(".sc", "stacks", stackName)
	if _, err := os.Stat(stackDir); os.IsNotExist(err) {
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("❌ Stack directory '%s' not found at %s", stackName, stackDir),
		}, nil
	}

	var message strings.Builder
	message.WriteString(fmt.Sprintf("📦 **Stack: %s**\n\n", stackName))

	foundConfigs := 0

	// Check for client.yaml
	clientPath := filepath.Join(stackDir, "client.yaml")
	if (showType == "" || showType == "client") && fileExists(clientPath) {
		data, err := os.ReadFile(clientPath)
		if err == nil {
			// Obfuscate credentials before exposing to LLM
			data = obfuscateCredentials(data, clientPath)
			message.WriteString(fmt.Sprintf("📋 **Client Configuration** (`%s`)\n\n", clientPath))
			message.WriteString("```yaml\n")
			message.WriteString(string(data))
			message.WriteString("\n```\n\n")
			foundConfigs++
		}
	}

	// Check for server.yaml
	serverPath := filepath.Join(stackDir, "server.yaml")
	if (showType == "" || showType == "server") && fileExists(serverPath) {
		data, err := os.ReadFile(serverPath)
		if err == nil {
			// Obfuscate credentials before exposing to LLM
			data = obfuscateCredentials(data, serverPath)
			if foundConfigs > 0 {
				message.WriteString("---\n\n")
			}
			message.WriteString(fmt.Sprintf("🖥️ **Server Configuration** (`%s`)\n\n", serverPath))
			message.WriteString("```yaml\n")
			message.WriteString(string(data))
			message.WriteString("\n```\n\n")
			foundConfigs++
		}
	}

	// Add summary of what exists
	if foundConfigs == 0 {
		message.WriteString("❌ **No configuration files found**\n\n")
		message.WriteString("📍 **Checked locations:**\n")
		message.WriteString(fmt.Sprintf("  • Client: %s ❌\n", clientPath))
		message.WriteString(fmt.Sprintf("  • Server: %s ❌\n", serverPath))
		message.WriteString("\n💡 **Tip**: Create configuration files using `/setup` command")
	} else {
		message.WriteString("📍 **Configuration status:**\n")
		if fileExists(clientPath) {
			message.WriteString(fmt.Sprintf("  • Client: %s ✅\n", clientPath))
		} else {
			message.WriteString(fmt.Sprintf("  • Client: %s ❌\n", clientPath))
		}
		if fileExists(serverPath) {
			message.WriteString(fmt.Sprintf("  • Server: %s ✅\n", serverPath))
		} else {
			message.WriteString(fmt.Sprintf("  • Server: %s ❌\n", serverPath))
		}
	}

	return &CommandResult{
		Success: foundConfigs > 0,
		Message: message.String(),
	}, nil
}

// fileExists checks if a file exists
func fileExists(filename string) bool {
	_, err := os.Stat(filename)
	return !os.IsNotExist(err)
}

// obfuscateCredentials masks sensitive values in file content before exposing to LLM
func obfuscateCredentials(content []byte, filePath string) []byte {
	// Get filename for context
	fileName := filepath.Base(filePath)

	// Check if this is a secrets file or contains sensitive content
	isSecretsFile := strings.Contains(fileName, "secrets") || strings.HasSuffix(fileName, "secrets.yaml") || strings.HasSuffix(fileName, "secrets.yml")

	contentStr := string(content)

	// For secrets.yaml files, apply comprehensive obfuscation
	if isSecretsFile {
		return []byte(obfuscateSecretsYAML(contentStr))
	}

	// For other files, apply general credential obfuscation
	return []byte(obfuscateGeneralCredentials(contentStr))
}

// obfuscateSecretsYAML specifically handles secrets.yaml files
func obfuscateSecretsYAML(content string) string {
	// Parse YAML to understand structure
	var data map[string]interface{}
	if err := yaml.Unmarshal([]byte(content), &data); err != nil {
		// If parsing fails, apply regex-based obfuscation as fallback
		return obfuscateGeneralCredentials(content)
	}

	// Obfuscate sensitive fields in structured way
	obfuscateYAMLValues(data)

	// Marshal back to YAML
	if obfuscatedBytes, err := yaml.Marshal(data); err == nil {
		return string(obfuscatedBytes)
	}

	// Fallback to regex if marshaling fails
	return obfuscateGeneralCredentials(content)
}

// obfuscateYAMLValues recursively obfuscates sensitive values in YAML structure
func obfuscateYAMLValues(data interface{}) {
	obfuscateYAMLValuesWithContext(data, "")
}

// obfuscateYAMLValuesWithContext recursively obfuscates sensitive values with section context
func obfuscateYAMLValuesWithContext(data interface{}, sectionPath string) {
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
					v[key] = obfuscateValue(strVal, key)
				} else {
					// If values section contains nested structure, obfuscate all string values
					obfuscateAllStringValues(value)
				}
			} else if isSensitiveKey(key) {
				if strVal, ok := value.(string); ok {
					v[key] = obfuscateValue(strVal, key)
				}
			} else {
				obfuscateYAMLValuesWithContext(value, newSectionPath)
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
					v[key] = obfuscateValue(strVal, keyStr)
				} else {
					// If values section contains nested structure, obfuscate all string values
					obfuscateAllStringValues(value)
				}
			} else if keyStr != "" && isSensitiveKey(keyStr) {
				if strVal, ok := value.(string); ok {
					v[key] = obfuscateValue(strVal, keyStr)
				}
			} else {
				obfuscateYAMLValuesWithContext(value, newSectionPath)
			}
		}
	case []interface{}:
		for _, item := range v {
			obfuscateYAMLValuesWithContext(item, sectionPath)
		}
	}
}

// obfuscateAllStringValues obfuscates all string values in a data structure (for values section)
func obfuscateAllStringValues(data interface{}) {
	switch v := data.(type) {
	case map[string]interface{}:
		for key, value := range v {
			if strVal, ok := value.(string); ok {
				v[key] = obfuscateValue(strVal, key)
			} else {
				obfuscateAllStringValues(value)
			}
		}
	case map[interface{}]interface{}:
		for key, value := range v {
			keyStr := ""
			if k, ok := key.(string); ok {
				keyStr = k
			}
			if strVal, ok := value.(string); ok {
				v[key] = obfuscateValue(strVal, keyStr)
			} else {
				obfuscateAllStringValues(value)
			}
		}
	case []interface{}:
		for _, item := range v {
			obfuscateAllStringValues(item)
		}
	}
}

// isSensitiveKey checks if a YAML key contains sensitive information
func isSensitiveKey(key string) bool {
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
func obfuscateValue(value, key string) string {
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
		return "sk-•••••••••••••••••••••••••••••••••••••••••••••••••••"
	case strings.HasPrefix(value, "xoxb-") || strings.HasPrefix(value, "xoxp-"):
		// Slack token pattern
		return strings.Split(value, "-")[0] + "-••••••••••••••••••••••••••••••••••"
	case strings.HasPrefix(value, "ghp_"):
		// GitHub token pattern
		return "ghp_••••••••••••••••••••••••••••••••••••••••"
	case strings.HasPrefix(value, "mongodb://") || strings.HasPrefix(value, "mongodb+srv://"):
		// MongoDB URI pattern - preserve structure but mask credentials
		return obfuscateURI(value)
	case strings.HasPrefix(value, "postgres://") || strings.HasPrefix(value, "postgresql://"):
		// PostgreSQL URI pattern
		return obfuscateURI(value)
	case strings.HasPrefix(value, "redis://"):
		// Redis URI pattern
		return obfuscateURI(value)
	case strings.HasPrefix(value, "-----BEGIN"):
		// Private key or certificate
		return obfuscateMultilineSecret(value)
	case strings.Contains(value, "\"private_key\"") || strings.Contains(value, "\"client_secret\""):
		// GCP service account JSON or similar embedded JSON credentials
		return obfuscateEmbeddedJSON(value)
	case strings.Contains(value, "apiVersion:") && strings.Contains(value, "clusters:"):
		// Kubernetes kubeconfig YAML
		return obfuscateEmbeddedYAML(value)
	case len(value) > 20 && isBase64Like(value):
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
func obfuscateURI(uri string) string {
	// Pattern to match URI with credentials: scheme://user:pass@host:port/path
	uriRegex := regexp.MustCompile(`^([^:]+://)[^:]+:[^@]+@(.+)$`)
	if matches := uriRegex.FindStringSubmatch(uri); len(matches) == 3 {
		return matches[1] + "••••••••:••••••••@" + matches[2]
	}

	// If no credentials found, just mask any embedded auth tokens
	return uri
}

// obfuscateMultilineSecret handles multi-line secrets like private keys
func obfuscateMultilineSecret(secret string) string {
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
func isBase64Like(s string) bool {
	base64Regex := regexp.MustCompile(`^[A-Za-z0-9+/]+=*$`)
	return base64Regex.MatchString(s) && len(s)%4 == 0
}

// obfuscateGeneralCredentials applies regex-based obfuscation for non-secrets files
func obfuscateGeneralCredentials(content string) string {
	// Common credential patterns to obfuscate
	patterns := map[string]string{
		// AWS Access Keys
		`AKIA[0-9A-Z]{16}`: "AKIA••••••••••••••••",
		// OpenAI API Keys
		`sk-[a-zA-Z0-9]{48}`: "sk-•••••••••••••••••••••••••••••••••••••••••••••••••••",
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
func obfuscateEmbeddedJSON(jsonStr string) string {
	// Try to parse as JSON
	var jsonData map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &jsonData); err != nil {
		// If parsing fails, apply general obfuscation
		return obfuscateGeneralCredentials(jsonStr)
	}

	// Obfuscate sensitive fields in the JSON
	obfuscateJSONCredentials(jsonData)

	// Marshal back to JSON
	if obfuscatedBytes, err := json.Marshal(jsonData); err == nil {
		return string(obfuscatedBytes)
	}

	// Fallback to general obfuscation
	return obfuscateGeneralCredentials(jsonStr)
}

// obfuscateJSONCredentials recursively obfuscates sensitive fields in JSON data
func obfuscateJSONCredentials(data interface{}) {
	switch v := data.(type) {
	case map[string]interface{}:
		for key, value := range v {
			if isSensitiveKey(key) {
				if strVal, ok := value.(string); ok {
					v[key] = obfuscateValue(strVal, key)
				}
			} else {
				obfuscateJSONCredentials(value)
			}
		}
	case []interface{}:
		for _, item := range v {
			obfuscateJSONCredentials(item)
		}
	}
}

// obfuscateEmbeddedYAML handles YAML structures embedded in credential values (e.g., kubeconfig)
func obfuscateEmbeddedYAML(yamlStr string) string {
	// Try to parse as YAML
	var yamlData interface{}
	if err := yaml.Unmarshal([]byte(yamlStr), &yamlData); err != nil {
		// If parsing fails, apply general obfuscation to sensitive-looking parts
		return obfuscateYAMLStringPatterns(yamlStr)
	}

	// Obfuscate sensitive fields in the YAML
	obfuscateEmbeddedYAMLValues(yamlData)

	// Marshal back to YAML
	if obfuscatedBytes, err := yaml.Marshal(yamlData); err == nil {
		return string(obfuscatedBytes)
	}

	// Fallback to pattern-based obfuscation
	return obfuscateYAMLStringPatterns(yamlStr)
}

// obfuscateEmbeddedYAMLValues recursively obfuscates sensitive fields in embedded YAML
func obfuscateEmbeddedYAMLValues(data interface{}) {
	switch v := data.(type) {
	case map[string]interface{}:
		for key, value := range v {
			if isSensitiveKey(key) {
				if strVal, ok := value.(string); ok {
					v[key] = obfuscateValue(strVal, key)
				}
			} else {
				obfuscateEmbeddedYAMLValues(value)
			}
		}
	case map[interface{}]interface{}:
		for key, value := range v {
			if keyStr, ok := key.(string); ok && isSensitiveKey(keyStr) {
				if strVal, ok := value.(string); ok {
					v[key] = obfuscateValue(strVal, keyStr)
				}
			} else {
				obfuscateEmbeddedYAMLValues(value)
			}
		}
	case []interface{}:
		for _, item := range v {
			obfuscateEmbeddedYAMLValues(item)
		}
	}
}

// obfuscateYAMLStringPatterns applies pattern-based obfuscation for YAML strings
func obfuscateYAMLStringPatterns(yamlStr string) string {
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
					return key + `: "` + obfuscateValue(value, key) + `"`
				}
				return key + `: ` + obfuscateValue(value, key)
			}
			return match
		})
	}

	return result
}

// handleGetProjectContext gets basic project context information using unified handler
func (c *ChatInterface) handleGetProjectContext(ctx context.Context, args []string, context *ConversationContext) (*CommandResult, error) {
	if c.commandHandler == nil {
		return &CommandResult{
			Success: false,
			Message: "❌ Command handler not available",
		}, nil
	}

	result, err := c.commandHandler.GetProjectContext(ctx, ".")
	if err != nil {
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("❌ Failed to get project context: %v", err),
		}, nil
	}

	return &CommandResult{
		Success: result.Success,
		Message: result.Message,
	}, nil
}

// handleGetSupportedResources lists all supported Simple Container resources
func (c *ChatInterface) handleGetSupportedResources(ctx context.Context, args []string, context *ConversationContext) (*CommandResult, error) {
	// Use the resources package for consistency
	supportedResources, err := resources.GetSupportedResourcesFromSchemas(ctx)
	if err != nil {
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("❌ Failed to get supported resources: %v", err),
		}, nil
	}

	message := "📚 **Supported Simple Container Resources:**\n\n"

	// Check if supportedResources has the expected structure
	if len(supportedResources.Providers) > 0 {
		for _, provider := range supportedResources.Providers {
			message += fmt.Sprintf("**%s** (%d resources):\n", provider.Name, len(provider.Resources))
			for _, resource := range provider.Resources {
				message += fmt.Sprintf("  • %s\n", resource)
			}
			message += "\n"
		}

		message += fmt.Sprintf("**Total**: %d resources across %d providers\n\n",
			len(supportedResources.Resources), len(supportedResources.Providers))
	} else {
		message += "No supported resources found or unable to load resource schemas.\n\n"
	}

	message += "Use `/addresource <name> <type> <environment>` to add resources to your infrastructure."

	return &CommandResult{
		Success: true,
		Message: message,
	}, nil
}

// confirmDeploymentTypeForChat handles deployment type confirmation in chat interface
func (c *ChatInterface) confirmDeploymentTypeForChat(context *ConversationContext) error {
	// Initialize metadata if it doesn't exist
	if context.Metadata == nil {
		context.Metadata = make(map[string]interface{})
	}

	// Skip confirmation if called via tool calling (stdin not available during streaming)
	if isToolCalling, ok := context.Metadata["is_tool_calling"].(bool); ok && isToolCalling {
		// Auto-detect and use deployment type without prompting
		detectedType := "cloud-compose" // Default fallback
		if context.ProjectInfo != nil && context.ProjectInfo.PrimaryStack != nil {
			lang := strings.ToLower(context.ProjectInfo.PrimaryStack.Language)
			if lang == "html" || lang == "javascript" || lang == "typescript" {
				detectedType = "static"
			} else if lang == "go" || lang == "python" || lang == "java" {
				detectedType = "single-image"
			}
		}
		context.Metadata["confirmed_deployment_type"] = detectedType
		return nil
	}

	// Determine deployment type using simple heuristic (since internal method is private)
	detectedType := "cloud-compose" // Default fallback
	if context.ProjectInfo != nil {
		// Simple heuristic based on project analysis
		if context.ProjectInfo.PrimaryStack != nil {
			lang := strings.ToLower(context.ProjectInfo.PrimaryStack.Language)
			if lang == "html" || lang == "javascript" || lang == "typescript" {
				// Check for static site indicators
				detectedType = "static"
			} else if lang == "go" || lang == "python" || lang == "java" {
				detectedType = "single-image"
			}
		}
	}

	// Display detected type with description
	fmt.Printf("🔍 Detected deployment type: %s\n", detectedType)

	switch detectedType {
	case "static":
		fmt.Printf("   📄 Static site deployment (HTML/CSS/JS files)\n")
		fmt.Printf("   💡 Best for: React, Vue, Angular, static sites\n")
	case "single-image":
		fmt.Printf("   🚀 Single container deployment (serverless/lambda style)\n")
		fmt.Printf("   💡 Best for: AWS Lambda, simple APIs, microservices\n")
	case "cloud-compose":
		fmt.Printf("   🐳 Multi-container deployment (docker-compose based)\n")
		fmt.Printf("   💡 Best for: Full-stack apps, databases, complex services\n")
	}

	// Use chat interface's ReadSimple for input (fixes Y/N prompt issue)
	response, err := c.inputHandler.ReadSimple("\n   Is this correct? [Y/n]: ")
	if err != nil {
		// If there's an error reading input, default to "yes" and store the detected type
		context.Metadata["confirmed_deployment_type"] = detectedType
		return nil
	}

	response = strings.ToLower(strings.TrimSpace(response))
	if response == "n" || response == "no" {
		// Let user choose the deployment type using chat interface
		return c.selectDeploymentTypeForChat(context)
	}

	// Store the confirmed deployment type in context for use during generation
	context.Metadata["confirmed_deployment_type"] = detectedType
	return nil
}

// selectDeploymentTypeForChat handles manual deployment type selection in chat interface
func (c *ChatInterface) selectDeploymentTypeForChat(context *ConversationContext) error {
	// Initialize metadata if it doesn't exist
	if context.Metadata == nil {
		context.Metadata = make(map[string]interface{})
	}
	fmt.Printf("\n📋 Available deployment types:\n")
	fmt.Printf("   1. static - Static site (HTML/CSS/JS files)\n")
	fmt.Printf("   2. single-image - Single container (serverless/lambda style)\n")
	fmt.Printf("   3. cloud-compose - Multi-container (docker-compose based)\n")

	// Use chat interface's ReadSimple for menu selection
	response, err := c.inputHandler.ReadSimple("\n   Select deployment type [1-3]: ")
	if err != nil {
		return fmt.Errorf("failed to read selection: %v", err)
	}

	response = strings.TrimSpace(response)

	var selectedType string
	switch response {
	case "1":
		selectedType = "static"
		fmt.Printf("✅ Selected: static\n")
	case "2":
		selectedType = "single-image"
		fmt.Printf("✅ Selected: single-image\n")
	case "3", "":
		selectedType = "cloud-compose"
		fmt.Printf("✅ Selected: cloud-compose\n")
	default:
		selectedType = "cloud-compose"
		fmt.Printf("Invalid selection. Using cloud-compose as default.\n")
	}

	// Store the selected deployment type in context for use during generation
	context.Metadata["confirmed_deployment_type"] = selectedType

	return nil
}

// handleReadProjectFile reads and displays a project file
func (c *ChatInterface) handleReadProjectFile(ctx context.Context, args []string, context *ConversationContext) (*CommandResult, error) {
	if len(args) == 0 {
		return &CommandResult{
			Success: false,
			Message: "❌ Please specify a filename. Usage: /file <filename>",
		}, nil
	}

	filename := args[0]

	// Get current working directory (the user's project directory)
	cwd, err := os.Getwd()
	if err != nil {
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("❌ Failed to get current directory: %v", err),
		}, nil
	}

	// Build full file path
	filePath := filepath.Join(cwd, filename)

	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("❌ File not found: %s\n\n💡 **Tip**: Make sure you're in your project directory and the file exists.", filename),
		}, nil
	}

	// Read the file securely with automatic credential obfuscation
	secureReader := security.NewSecureFileReader()
	data, err := secureReader.ReadFileSecurely(filePath)
	if err != nil {
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("❌ Failed to read file %s: %v", filename, err),
		}, nil
	}

	// Determine syntax highlighting based on file extension/name
	language := getSyntaxLanguage(filename)

	message := fmt.Sprintf("📄 **%s** (from %s)\n\n", filename, cwd)
	message += fmt.Sprintf("```%s\n", language)
	message += string(data)
	message += "\n```\n"

	// Add file info
	stat, _ := os.Stat(filePath)
	message += fmt.Sprintf("\n📊 **File Info**: %d bytes, modified %s",
		len(data), stat.ModTime().Format("2006-01-02 15:04:05"))

	return &CommandResult{
		Success: true,
		Message: message,
	}, nil
}

// getSyntaxLanguage determines the syntax highlighting language for a filename
func getSyntaxLanguage(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	name := strings.ToLower(filename)

	// Handle specific filenames first
	switch name {
	case "dockerfile", "dockerfile.dev", "dockerfile.prod":
		return "dockerfile"
	case "docker-compose.yaml", "docker-compose.yml":
		return "yaml"
	case "package.json", "composer.json":
		return "json"
	case "go.mod", "go.sum":
		return "go"
	case "requirements.txt", "setup.py", "pyproject.toml":
		return "python"
	case "makefile":
		return "makefile"
	case ".env", ".env.example", ".env.local", ".env.production":
		return "bash"
	}

	// Handle extensions
	switch ext {
	case ".js", ".mjs":
		return "javascript"
	case ".ts":
		return "typescript"
	case ".py":
		return "python"
	case ".go":
		return "go"
	case ".yaml", ".yml":
		return "yaml"
	case ".json":
		return "json"
	case ".toml":
		return "toml"
	case ".xml":
		return "xml"
	case ".sh", ".bash":
		return "bash"
	case ".sql":
		return "sql"
	case ".md":
		return "markdown"
	case ".html":
		return "html"
	case ".css":
		return "css"
	case ".java":
		return "java"
	case ".rb":
		return "ruby"
	case ".php":
		return "php"
	case ".rs":
		return "rust"
	case ".cpp", ".cc", ".cxx":
		return "cpp"
	case ".c":
		return "c"
	case ".cs":
		return "csharp"
	default:
		return ""
	}
}

// generateDeveloperFiles creates client configuration files using DeveloperMode
func (c *ChatInterface) generateDeveloperFiles(context *ConversationContext) ([]GeneratedFile, error) {
	// Use DeveloperMode for consistent file generation
	projectPath := "."
	if context.ProjectPath != "" {
		projectPath = context.ProjectPath
	}

	// Initialize metadata if it doesn't exist
	if context.Metadata == nil {
		context.Metadata = make(map[string]interface{})
	}

	// Get confirmed deployment type from context (if available)
	var deploymentType string
	if confirmedType, exists := context.Metadata["confirmed_deployment_type"]; exists {
		if typeStr, ok := confirmedType.(string); ok {
			deploymentType = typeStr
		}
	}

	// Create SetupOptions for DeveloperMode
	opts := &modes.SetupOptions{
		Interactive:    false, // Chat interface handles interactivity differently
		Environment:    "staging",
		Parent:         "infrastructure",
		SkipAnalysis:   false,
		SkipDockerfile: false,
		SkipCompose:    false,
		OutputDir:      projectPath,
		GenerateAll:    false,
		UseStreaming:   true,           // Enable streaming for better UX in chat mode
		DeploymentType: deploymentType, // Use the confirmed deployment type
	}

	// Capture the generated files by temporarily redirecting the DeveloperMode output
	// We'll use a custom approach to get the generated content without writing files
	return c.generateFilesUsingDeveloperMode(context, opts)
}

// generateFilesUsingDeveloperMode generates files using the modes package
func (c *ChatInterface) generateFilesUsingDeveloperMode(context *ConversationContext, opts *modes.SetupOptions) ([]GeneratedFile, error) {
	// Get or create project analysis
	var projectAnalysis *analysis.ProjectAnalysis
	if context.ProjectInfo != nil {
		projectAnalysis = context.ProjectInfo
	} else {
		// Add progress reporting for better UX during analysis
		fmt.Printf("🔄 Analyzing project for configuration generation...\n")
		progressReporter := analysis.NewStreamingProgressReporter(os.Stdout)
		c.analyzer.SetProgressReporter(progressReporter)

		var err error
		projectAnalysis, err = c.analyzer.AnalyzeProject(opts.OutputDir)
		if err != nil {
			// Use a basic fallback analysis
			projectAnalysis = &analysis.ProjectAnalysis{
				Name: "my-app",
				PrimaryStack: &analysis.TechStackInfo{
					Language: "javascript",
				},
			}
		}
	}

	// Determine proper project name for file paths
	projectName := projectAnalysis.Name
	if projectName == "." || projectName == "" {
		if absPath, err := filepath.Abs(opts.OutputDir); err == nil {
			projectName = filepath.Base(absPath)
		} else {
			projectName = "my-app"
		}
	}

	files := []GeneratedFile{}

	// Start all file generations in parallel
	fmt.Printf("📄 Generating configuration files in parallel...\n")

	type fileGenResult struct {
		file GeneratedFile
		err  error
		name string
	}

	var wg sync.WaitGroup
	resultChan := make(chan fileGenResult, 3)

	// Generate client.yaml
	wg.Add(1)
	go func() {
		defer wg.Done()
		fmt.Printf("   📄 Generating client.yaml...\n")
		clientYaml, err := c.developerMode.GenerateClientYAMLWithLLM(opts, projectAnalysis)
		result := fileGenResult{name: "client.yaml", err: err}
		if err == nil {
			result.file = GeneratedFile{
				Path:        ".sc/stacks/" + projectName + "/client.yaml",
				Type:        "yaml",
				Description: "Simple Container client configuration",
				Generated:   true,
				Content:     clientYaml,
			}
		}
		resultChan <- result
	}()

	// Generate docker-compose.yaml
	if !opts.SkipCompose {
		wg.Add(1)
		go func() {
			defer wg.Done()
			fmt.Printf("   🐳 Generating docker-compose.yaml...\n")
			composeYaml, err := c.developerMode.GenerateComposeYAMLWithLLM(projectAnalysis)
			result := fileGenResult{name: "docker-compose.yaml", err: err}
			if err == nil {
				result.file = GeneratedFile{
					Path:        "docker-compose.yaml",
					Type:        "yaml",
					Description: "Local development environment",
					Generated:   true,
					Content:     composeYaml,
				}
			}
			resultChan <- result
		}()
	}

	// Generate Dockerfile
	if !opts.SkipDockerfile {
		wg.Add(1)
		go func() {
			defer wg.Done()
			fmt.Printf("   🐳 Generating Dockerfile...\n")
			dockerfile, err := c.developerMode.GenerateDockerfileWithLLM(projectAnalysis)
			result := fileGenResult{name: "Dockerfile", err: err}
			if err == nil {
				result.file = GeneratedFile{
					Path:        "Dockerfile",
					Type:        "dockerfile",
					Description: "Container image definition",
					Generated:   true,
					Content:     dockerfile,
				}
			}
			resultChan <- result
		}()
	}

	// Wait for all generations to complete and collect results
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Collect results as they complete
	for result := range resultChan {
		if result.err == nil {
			fmt.Printf("   ✅ %s generated\n", result.name)
			files = append(files, result.file)
		} else {
			fmt.Printf("   ⚠️ Failed to generate %s: %v\n", result.name, result.err)
		}
	}

	return files, nil
}

// generateDevOpsFiles creates infrastructure files for DevOps mode
func (c *ChatInterface) generateDevOpsFiles(context *ConversationContext) ([]GeneratedFile, error) {
	// Use DevOps mode to generate infrastructure files
	files := []GeneratedFile{}

	// Generate server.yaml using proper schema structure
	serverContent := `schemaVersion: 1.0

# Provisioner configuration
provisioner:
  type: pulumi
  config:
    state-storage:
      type: s3-bucket
      config:
        bucketName: simple-container-state
        region: us-east-1
    secrets-provider:
      type: aws-kms
      config:
        keyId: "arn:aws:kms:us-east-1:123456789012:key/simple-container-secrets"

# Reusable templates for application teams
templates:
  web-app:
    type: ecs-fargate
    config:
      credentials: "${auth:aws}"
      account: "${auth:aws.projectId}"
    
  api-service:
    type: ecs-fargate
    config:
      credentials: "${auth:aws}"
      account: "${auth:aws.projectId}"

# Secrets management configuration
secrets:
  type: aws-kms
  config:
    keyId: "alias/simple-container"

# CI/CD integration
cicd:
  type: github-actions
  config:
    auth-token: "${secret:GITHUB_TOKEN}"

# Shared infrastructure resources
resources:
  # Domain registrar (optional)
  registrar:
    type: cloudflare
    config:
      credentials: "${secret:CLOUDFLARE_API_TOKEN}"
      accountId: "your-cloudflare-account-id"
      zoneName: "example.com"
  
  # Environment-specific resources
  resources:
    # Staging environment
    staging:
      template: web-app
      resources:
        # Database
        postgres-db:
          type: aws-rds-postgres
          config:
            name: myapp-staging-db
            instanceClass: db.t3.micro
            allocatedStorage: 20
            engineVersion: "15.4"
            username: dbadmin
            password: "${secret:staging-db-password}"
            databaseName: myapp
            
        # Cache
        redis-cache:
          type: aws-elasticache-redis
          config:
            name: myapp-staging-cache
            nodeType: cache.t3.micro
            numCacheNodes: 1
            
        # Storage
        uploads-bucket:
          type: s3-bucket
          config:
            name: myapp-staging-uploads
            allowOnlyHttps: true

    # Production environment
    production:
      template: web-app
      resources:
        # Database with high availability
        postgres-db:
          type: aws-rds-postgres
          config:
            name: myapp-prod-db
            instanceClass: db.r5.large
            allocatedStorage: 100
            multiAZ: true
            backupRetentionPeriod: 7
            engineVersion: "15.4"
            username: dbadmin
            password: "${secret:prod-db-password}"
            databaseName: myapp
            
        # Cache cluster
        redis-cache:
          type: aws-elasticache-redis
          config:
            name: myapp-prod-cache
            nodeType: cache.r5.large
            numCacheNodes: 3
            
        # Storage
        uploads-bucket:
          type: s3-bucket
          config:
            name: myapp-prod-uploads
            allowOnlyHttps: true

# Configuration variables
variables:
  app-prefix:
    type: string
    value: myapp`

	files = append(files, GeneratedFile{
		Path:        ".sc/stacks/infrastructure/server.yaml",
		Type:        "yaml",
		Description: "Infrastructure configuration",
		Generated:   true,
		Content:     serverContent,
	})

	// Generate secrets.yaml
	secretsContent := `# Simple Container secrets configuration
schemaVersion: 1.0

# Authentication for cloud providers
auth:
  aws:
    type: aws-token
    config:
      account: "123456789012"
      accessKey: "AKIA..."  # Replace with actual AWS access key
      secretAccessKey: "wJa..."  # Replace with actual AWS secret key
      region: us-east-1

# Secret values (managed with sc secrets add)
values:
  # Database passwords
  staging-db-password: "${STAGING_DB_PASSWORD}"
  prod-db-password: "${PROD_DB_PASSWORD}"
  
  # Cloud credentials  
  aws-access-key: "${AWS_ACCESS_KEY}"
  aws-secret-key: "${AWS_SECRET_KEY}"
  
  # Application secrets
  jwt-secret: "${JWT_SECRET}"`

	files = append(files, GeneratedFile{
		Path:        ".sc/stacks/infrastructure/secrets.yaml",
		Type:        "yaml",
		Description: "Authentication and secrets",
		Generated:   true,
		Content:     secretsContent,
	})

	return files, nil
}

// handleWriteProjectFile writes content to a project file with multiple modes
func (c *ChatInterface) handleWriteProjectFile(ctx context.Context, args []string, context *ConversationContext) (*CommandResult, error) {
	if len(args) < 2 {
		return &CommandResult{
			Success: false,
			Message: "❌ Please specify filename and content. Usage: /write <filename> <content> [--lines start-end] [--append]",
		}, nil
	}

	filename := args[0]
	content := args[1]

	// Parse optional flags
	var lineRange string
	var appendMode bool

	for i := 2; i < len(args); i++ {
		if args[i] == "--append" {
			appendMode = true
		} else if args[i] == "--lines" && i+1 < len(args) {
			lineRange = args[i+1]
			i++ // Skip the next arg since it's the line range value
		}
	}

	// Get current working directory (the user's project directory)
	cwd, err := os.Getwd()
	if err != nil {
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("❌ Failed to get current directory: %v", err),
		}, nil
	}

	// Build full file path and validate security
	filePath := filepath.Join(cwd, filename)

	// Basic security check - prevent writing outside project directory
	if !strings.HasPrefix(filepath.Clean(filePath), filepath.Clean(cwd)) {
		return &CommandResult{
			Success: false,
			Message: "❌ Security error: Cannot write files outside the project directory",
		}, nil
	}

	var finalContent []byte
	var mode string

	if lineRange != "" {
		// Line range replacement mode
		mode = "line-range replacement"
		finalContent, err = c.replaceFileLines(filePath, content, lineRange)
		if err != nil {
			return &CommandResult{
				Success: false,
				Message: fmt.Sprintf("❌ Failed to replace lines in %s: %v", filename, err),
			}, nil
		}
	} else if appendMode {
		// Append mode
		mode = "append"
		finalContent, err = c.appendToFile(filePath, content)
		if err != nil {
			return &CommandResult{
				Success: false,
				Message: fmt.Sprintf("❌ Failed to append to %s: %v", filename, err),
			}, nil
		}
	} else {
		// Full file replacement mode (default)
		mode = "full replacement"
		finalContent = []byte(content)
	}

	// Write the file
	err = os.WriteFile(filePath, finalContent, 0o644)
	if err != nil {
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("❌ Failed to write file %s: %v", filename, err),
		}, nil
	}

	// Determine syntax highlighting for preview
	language := getSyntaxLanguage(filename)

	// Create success message with preview
	message := fmt.Sprintf("✅ **File written successfully**: %s (%s)\n", filename, mode)
	message += fmt.Sprintf("📁 **Location**: %s\n\n", filePath)

	// Show a preview of the written content (first few lines)
	lines := strings.Split(string(finalContent), "\n")
	previewLines := 10
	if len(lines) > previewLines {
		message += fmt.Sprintf("📄 **Preview** (first %d lines):\n", previewLines)
		message += fmt.Sprintf("```%s\n", language)
		message += strings.Join(lines[:previewLines], "\n")
		message += "\n... (" + strconv.Itoa(len(lines)-previewLines) + " more lines)\n```"
	} else {
		message += fmt.Sprintf("📄 **Content**:\n```%s\n", language)
		message += string(finalContent)
		message += "\n```"
	}

	// Add file info
	stat, _ := os.Stat(filePath)
	message += fmt.Sprintf("\n📊 **File Info**: %d bytes written", len(finalContent))
	if stat != nil {
		message += fmt.Sprintf(", modified %s", stat.ModTime().Format("2006-01-02 15:04:05"))
	}

	return &CommandResult{
		Success: true,
		Message: message,
	}, nil
}

// replaceFileLines replaces specific lines in a file with new content
func (c *ChatInterface) replaceFileLines(filePath, newContent, lineRange string) ([]byte, error) {
	// Parse line range (e.g., "10-20" or "5")
	var startLine, endLine int
	var err error

	if strings.Contains(lineRange, "-") {
		parts := strings.Split(lineRange, "-")
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid line range format. Use 'start-end' or single line number")
		}
		startLine, err = strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil {
			return nil, fmt.Errorf("invalid start line number: %v", err)
		}
		endLine, err = strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil {
			return nil, fmt.Errorf("invalid end line number: %v", err)
		}
	} else {
		// Single line replacement
		startLine, err = strconv.Atoi(strings.TrimSpace(lineRange))
		if err != nil {
			return nil, fmt.Errorf("invalid line number: %v", err)
		}
		endLine = startLine
	}

	// Convert to 0-based indexing
	startLine--
	endLine--

	// Read existing file content
	var existingContent []byte
	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		existingContent, err = os.ReadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("failed to read existing file: %v", err)
		}
	}

	// Split into lines
	lines := strings.Split(string(existingContent), "\n")

	// Validate line range
	if startLine < 0 || startLine >= len(lines) {
		return nil, fmt.Errorf("start line %d is out of range (file has %d lines)", startLine+1, len(lines))
	}
	if endLine < 0 || endLine >= len(lines) {
		return nil, fmt.Errorf("end line %d is out of range (file has %d lines)", endLine+1, len(lines))
	}
	if startLine > endLine {
		return nil, fmt.Errorf("start line (%d) cannot be greater than end line (%d)", startLine+1, endLine+1)
	}

	// Replace the specified lines
	newLines := strings.Split(newContent, "\n")

	// Build the final content
	var result []string
	result = append(result, lines[:startLine]...) // Lines before replacement
	result = append(result, newLines...)          // New content
	result = append(result, lines[endLine+1:]...) // Lines after replacement

	return []byte(strings.Join(result, "\n")), nil
}

// appendToFile appends content to the end of a file
func (c *ChatInterface) appendToFile(filePath, content string) ([]byte, error) {
	var existingContent []byte

	// Read existing file if it exists
	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		var err error
		existingContent, err = os.ReadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("failed to read existing file: %v", err)
		}
	}

	// Ensure there's a newline before appending (if file has content)
	finalContent := string(existingContent)
	if len(existingContent) > 0 && !strings.HasSuffix(finalContent, "\n") {
		finalContent += "\n"
	}

	finalContent += content

	return []byte(finalContent), nil
}
