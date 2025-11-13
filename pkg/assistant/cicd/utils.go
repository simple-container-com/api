package cicd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v2"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/github"
)

// ParentRepositoryInfo holds information about parent repository configuration
type ParentRepositoryInfo struct {
	IsParent        bool
	ParentRepoURL   string
	ParentStackPath string
	HasParentConfig bool
}

// getParentRepositoryInfo determines if this is a parent repository and gathers parent configuration
// This function is only called when --parent flag is NOT used, to determine if this is a client stack
// that needs parent repository information for workflow generation
func getParentRepositoryInfo(serverDesc *api.ServerDescriptor, stackName string) *ParentRepositoryInfo {
	info := &ParentRepositoryInfo{}

	// This stack is a client stack, check if it has parent repository configuration
	parentConfig := checkParentRepositoryConfig(stackName)
	if parentConfig != nil {
		info.ParentRepoURL = parentConfig.ParentRepoURL
		info.ParentStackPath = parentConfig.ParentStackPath
		info.HasParentConfig = parentConfig.HasParentConfig
		info.IsParent = false // This is a client stack, not a parent
		return info
	}

	// No parent configuration found, this is likely a standalone stack
	info.IsParent = false
	return info
}

// checkParentRepositoryConfig reads SC configuration to find parent repository information
// Follows SC's standard configuration reading patterns, checking both SC_CONFIG env var and local files
func checkParentRepositoryConfig(stackName string) *ParentRepositoryInfo {
	return checkParentRepositoryConfigWithLogging(context.Background(), nil, stackName)
}

// checkParentRepositoryConfigWithLogging reads SC configuration with debug logging
func checkParentRepositoryConfigWithLogging(ctx context.Context, logger Logger, stackName string) *ParentRepositoryInfo {
	// Use logger if available
	logDebug := func(format string, args ...interface{}) {
		if logger != nil {
			logger.Debug(ctx, format, args...)
		}
	}

	logDebug("checkParentRepositoryConfig called for stack: %s", stackName)

	// First, try to read from SC_CONFIG environment variable (GitHub Actions scenario)
	logDebug("Trying to read from SC_CONFIG environment variable")
	config, err := readConfigFromSCConfigEnv()
	if err != nil {
		logDebug("SC_CONFIG failed: %v, falling back to local config files", err)
		// Fall back to local configuration files
		logDebug("Trying to read local config file with default profile")
		config, err = api.ReadConfigFile(".", "default")
		if err != nil {
			logDebug("Default profile failed: %v, trying SC_PROFILE", err)
			// Try other profile if default fails (following SC standard practice)
			profile := os.Getenv("SC_PROFILE")
			if profile != "" && profile != "default" {
				logDebug("Trying profile: %s", profile)
				config, err = api.ReadConfigFile(".", profile)
				if err != nil {
					logDebug("Profile %s failed: %v", profile, err)
					return nil
				}
			} else {
				logDebug("No SC_PROFILE set, no configuration found")
				return nil
			}
		}
	}

	// Check if parent repository is configured
	logDebug("Checking if parent repository is configured: '%s'", config.ParentRepository)
	if config.ParentRepository == "" {
		logDebug("No parent repository configured")
		return nil
	}

	logDebug("Found parent repository: %s", config.ParentRepository)
	info := &ParentRepositoryInfo{
		ParentRepoURL:   config.ParentRepository,
		HasParentConfig: true,
	}

	// Read client.yaml for the specific stack to get parent stack path
	clientPath := filepath.Join(".sc", "stacks", stackName, "client.yaml")
	if _, err := os.Stat(clientPath); err == nil {
		var clientConfig api.ClientDescriptor
		if readConfig, err := api.ReadDescriptor(clientPath, &clientConfig); err == nil {
			// Look for parent configuration in the stack's environments
			for envStackName, stackConfig := range readConfig.Stacks {
				// Match stack name (handle environment variants like "stack-staging")
				if strings.HasPrefix(envStackName, stackName) || envStackName == stackName {
					if stackConfig.ParentStack != "" {
						info.ParentStackPath = stackConfig.ParentStack
						break
					}
				}
			}
		}
	}

	// Check if parent stack is available locally or needs to be fetched from git
	if info.ParentStackPath != "" {
		// Try to find parent stack locally first
		localParentPath := filepath.Join(".sc", "stacks", info.ParentStackPath, "server.yaml")
		if _, err := os.Stat(localParentPath); err == nil {
			// Parent stack is available locally
			return info
		}

		// Parent stack not available locally, will need to be fetched from git
		// This is handled by the workflow generation process
	}

	return info
}

// getConcurrencyGroup returns appropriate concurrency group based on repository type
func getConcurrencyGroup(isParent bool, stackName string) string {
	if isParent {
		return "provision-" + stackName + "-${{ github.ref }}"
	}
	return "deploy-" + stackName + "-${{ github.ref }}"
}

// createDefaultEnhancedConfig creates a default configuration with logging
func createDefaultEnhancedConfig(defaultBranch, defaultRunner string, defaultTemplates []string, defaultCustomActions map[string]string, scVersion string, reason string, isParentStack bool, stackName string) *github.EnhancedActionsCiCdConfig {
	fmt.Printf("⚠️  WARNING: Using default CI/CD configuration for stack '%s' (reason: %s)\n", stackName, reason)
	fmt.Printf("   → Using default runner: %s\n", defaultRunner)
	fmt.Printf("   → Parent stack: %t\n", isParentStack)

	return &github.EnhancedActionsCiCdConfig{
		Organization: github.OrganizationConfig{
			Name:          "simple-container-org",
			DefaultBranch: defaultBranch,
			DefaultRunner: defaultRunner,
		},
		WorkflowGeneration: github.WorkflowGenerationConfig{
			Enabled:       true,
			Templates:     defaultTemplates,
			CustomActions: defaultCustomActions,
			SCVersion:     scVersion,
		},
		Execution: github.ExecutionConfig{
			DefaultTimeout: "30",
			Concurrency: github.ConcurrencyConfig{
				Group:            getConcurrencyGroup(isParentStack, stackName),
				CancelInProgress: false,
			},
		},
		Environments: map[string]github.EnvironmentConfig{
			"staging":    {Type: "staging", Runner: "ubuntu-latest"},
			"production": {Type: "production", Runner: "ubuntu-latest"},
		},
		Notifications: github.NotificationConfig{CCOnStart: false},
	}
}

// createEnhancedConfig converts server configuration to enhanced GitHub Actions config
func createEnhancedConfig(serverDesc *api.ServerDescriptor, stackName string, isParent bool, isStaging bool) *github.EnhancedActionsCiCdConfig {
	// Determine if this is a parent stack based on explicit flag or configuration
	var isParentStack bool
	if isParent {
		// User explicitly specified --parent flag
		isParentStack = true
	} else {
		// Check if this is a client stack that should use client workflows
		parentInfo := getParentRepositoryInfo(serverDesc, stackName)
		isParentStack = parentInfo.IsParent
	}

	// Determine SC version and action version based on staging flag
	scVersion := "latest" // Default to latest (which maps to @main)
	actionVersion := "@main"
	if isStaging {
		scVersion = "staging" // Use staging branch for SC actions
		actionVersion = "@staging"
	}

	// Choose templates based on repository type
	var defaultTemplates []string
	var defaultCustomActions map[string]string

	if isParentStack {
		// Parent repository workflows
		defaultTemplates = []string{"provision", "destroy-parent"}
		defaultCustomActions = map[string]string{
			"provision":      "simple-container-com/api/.github/actions/provision-parent-stack" + actionVersion,
			"destroy-parent": "simple-container-com/api/.github/actions/destroy-parent-stack" + actionVersion,
			"cancel-stack":   "simple-container-com/api/.github/actions/cancel-stack" + actionVersion,
		}
	} else {
		// Client repository workflows
		defaultTemplates = []string{"deploy", "destroy"}
		defaultCustomActions = map[string]string{
			"deploy":       "simple-container-com/api/.github/actions/deploy-client-stack" + actionVersion,
			"destroy":      "simple-container-com/api/.github/actions/destroy-client-stack" + actionVersion,
			"cancel-stack": "simple-container-com/api/.github/actions/cancel-stack" + actionVersion,
		}
	}

	// Default branch remains main for workflow triggers
	defaultBranch := "main"

	// Use SC's standard conversion pattern to get strongly typed GitHub Actions configuration
	convertedConfig, err := api.ConvertConfig(&serverDesc.CiCd.Config, &github.GitHubActionsCiCdConfig{})
	if err != nil {
		// Fallback to default configuration with logging
		return createDefaultEnhancedConfig(defaultBranch, "ubuntu-latest", defaultTemplates, defaultCustomActions, scVersion, fmt.Sprintf("config conversion failed: %v", err), isParentStack, stackName)
	}

	// Extract the strongly typed configuration
	gitHubConfig, ok := convertedConfig.Config.(*github.GitHubActionsCiCdConfig)
	if !ok {
		// Fallback to default if type assertion fails
		return createDefaultEnhancedConfig(defaultBranch, "ubuntu-latest", defaultTemplates, defaultCustomActions, scVersion, "type assertion failed - invalid configuration format", isParentStack, stackName)
	}

	// Successfully parsed server.yaml configuration
	fmt.Printf("✅ Using CI/CD configuration from server.yaml for stack '%s'\n", stackName)
	fmt.Printf("   → Organization: %s\n", gitHubConfig.Organization)
	fmt.Printf("   → Environments: %d configured\n", len(gitHubConfig.Environments))
	fmt.Printf("   → Parent stack: %t\n", isParentStack)

	// Extract default runner from environments for auxiliary jobs
	var defaultRunner string
	for _, env := range gitHubConfig.Environments {
		if env.Runner != "" {
			defaultRunner = env.Runner
			break // Use runner from first environment
		}
	}
	if defaultRunner == "" {
		defaultRunner = "ubuntu-latest"
	}

	// Convert to enhanced config with proper defaults
	config := &github.EnhancedActionsCiCdConfig{
		Organization: github.OrganizationConfig{
			Name:          gitHubConfig.Organization,
			DefaultBranch: defaultBranch,
			DefaultRunner: defaultRunner,
		},
		WorkflowGeneration: github.WorkflowGenerationConfig{
			Enabled:       true,
			Templates:     defaultTemplates,     // Use repository type-specific templates
			CustomActions: defaultCustomActions, // Use repository type-specific actions
			SCVersion:     scVersion,            // Use staging or latest based on flag
		},
		Execution: github.ExecutionConfig{
			DefaultTimeout: "30",
			Concurrency: github.ConcurrencyConfig{
				Group:            getConcurrencyGroup(isParentStack, stackName),
				CancelInProgress: false,
			},
		},
		Environments: make(map[string]github.EnvironmentConfig),
		Notifications: github.NotificationConfig{
			SlackWebhook:   gitHubConfig.Notifications.Slack.WebhookURL,
			DiscordWebhook: gitHubConfig.Notifications.Discord.WebhookURL,
			TelegramChatID: gitHubConfig.Notifications.Telegram.ChatID,
			TelegramToken:  gitHubConfig.Notifications.Telegram.BotToken,
			CCOnStart:      false,
		},
	}

	// Override with user-provided config if available, but respect explicit --parent flag
	// When --parent flag is used, the explicit user intent takes precedence over server.yaml templates
	if len(gitHubConfig.WorkflowGeneration.Templates) > 0 && !isParentStack {
		// Only override templates if this is not a parent stack (either explicit --parent or detected)
		// This ensures "sc cicd generate --parent" always generates parent workflows
		config.WorkflowGeneration.Templates = gitHubConfig.WorkflowGeneration.Templates
	}
	if len(gitHubConfig.WorkflowGeneration.CustomActions) > 0 {
		for key, value := range gitHubConfig.WorkflowGeneration.CustomActions {
			config.WorkflowGeneration.CustomActions[key] = value
		}
	}
	if gitHubConfig.WorkflowGeneration.SCVersion != "" {
		config.WorkflowGeneration.SCVersion = gitHubConfig.WorkflowGeneration.SCVersion
	}

	// Convert environments with proper defaults and validation
	for name, env := range gitHubConfig.Environments {
		// Validate and fix runner names
		runner := env.Runner
		if runner == "" {
			runner = "ubuntu-latest"
		} else {
			// Fix invalid runner names
			if runner == "ubuntu-22" {
				runner = "ubuntu-latest"
			}
		}

		config.Environments[name] = github.EnvironmentConfig{
			Type:        env.Type,
			Runner:      runner,
			Variables:   env.Variables,
			Protection:  env.Protection,
			Reviewers:   env.Reviewers,
			Secrets:     env.Secrets,
			DeployFlags: env.DeployFlags,
			AutoDeploy:  env.AutoDeploy,
		}
	}

	// The default environment selection is handled by the WorkflowGenerator
	// in the getDefaultEnvironment() function

	return config
}

// getRequiredSecrets returns the list of required secrets for the configuration
func getRequiredSecrets(config *github.EnhancedActionsCiCdConfig) []string {
	// Simple Container uses unified secrets management:
	// - Only SC_CONFIG is required as a GitHub repository secret
	// - All other secrets (notifications, webhooks, tokens) are managed in .sc/stacks/<stack>/secrets.yaml
	// - This approach eliminates the need to manage dozens of individual repository secrets
	return []string{"SC_CONFIG"}
}

// processStackName handles stack name validation and defaulting
func processStackName(stackName string) string {
	if stackName == "" {
		return "default-stack"
	}
	return stackName
}

// autoDetectConfigFile detects server.yaml file location based on stack name
// Returns empty string if no server.yaml is found but parent repository config is available
func autoDetectConfigFile(configFile, stackName string) (string, error) {
	return autoDetectConfigFileWithLogging(context.Background(), nil, configFile, stackName)
}

// autoDetectConfigFileWithLogging detects server.yaml file location with debug logging
func autoDetectConfigFileWithLogging(ctx context.Context, logger Logger, configFile, stackName string) (string, error) {
	// Use logger if available
	logDebug := func(format string, args ...interface{}) {
		if logger != nil {
			logger.Debug(ctx, format, args...)
		}
	}

	logDebug("autoDetectConfigFile called with configFile='%s', stackName='%s'", configFile, stackName)

	if configFile != "" && configFile != "server.yaml" {
		logDebug("Using provided config file: %s", configFile)
		return configFile, nil
	}

	// Try stack-specific server.yaml first
	stackDir := filepath.Join(".sc", "stacks", stackName)
	stackServerYaml := filepath.Join(stackDir, "server.yaml")
	logDebug("Checking stack-specific server.yaml: %s", stackServerYaml)
	if _, err := os.Stat(stackServerYaml); err == nil {
		logDebug("Found stack-specific server.yaml: %s", stackServerYaml)
		return stackServerYaml, nil
	}

	// Fall back to root server.yaml
	logDebug("Checking root server.yaml")
	if _, err := os.Stat("server.yaml"); err == nil {
		logDebug("Found root server.yaml")
		return "server.yaml", nil
	}

	// Check if parent repository configuration is available as fallback
	logDebug("Checking parent repository configuration for stack: %s", stackName)
	parentConfig := checkParentRepositoryConfigWithLogging(ctx, logger, stackName)
	if parentConfig != nil && parentConfig.HasParentConfig {
		logDebug("Found parent repository configuration, using parent config")
		// Return empty string to indicate parent repository configuration should be used
		return "", nil
	}

	logDebug("No configuration found anywhere")
	return "", fmt.Errorf("no server.yaml found and no parent repository configuration available. Checked: %s, server.yaml, .sc/cfg.default.yaml", stackServerYaml)
}

// validateAndLoadServerConfig loads and validates server configuration
// If configFile is empty, attempts to create configuration from parent repository info
func validateAndLoadServerConfig(configFile string) (*api.ServerDescriptor, error) {
	// Handle parent repository configuration when no server.yaml is available
	if configFile == "" {
		return createServerDescriptorFromParentConfig()
	}

	serverDesc, err := readServerConfig(configFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read server configuration: %w", err)
	}

	// Validate CI/CD configuration exists
	if serverDesc.CiCd.Type == "" {
		return nil, fmt.Errorf(`no CI/CD configuration found in %s

To add GitHub Actions CI/CD support, add the following to your server.yaml:

cicd:
  type: github-actions
  config:
    organization: "your-org-name"
    environments:
      staging:
        type: staging
        auto-deploy: true
        runners: ["ubuntu-latest"]
      production:
        type: production
        protection: true
        auto-deploy: false
        runners: ["ubuntu-latest"]
    notifications:
      slack: "${secret:slack-webhook-url}"
    workflow-generation:
      enabled: true`, configFile)
	}

	// Validate that the CI/CD type is supported
	if serverDesc.CiCd.Type != "github-actions" {
		return nil, fmt.Errorf("unsupported CI/CD type '%s'. Only 'github-actions' is currently supported", serverDesc.CiCd.Type)
	}

	return serverDesc, nil
}

// readServerConfig reads the server configuration file
func readServerConfig(configFile string) (*api.ServerDescriptor, error) {
	// Use SC's internal API to read server configuration
	serverDesc, err := api.ReadServerDescriptor(configFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read server configuration: %w", err)
	}
	return serverDesc, nil
}

// previewGeneration shows what workflows would be generated
func (s *Service) previewGeneration(config *github.EnhancedActionsCiCdConfig, stackName, outputDir string) (*Result, error) {
	var message strings.Builder
	message.WriteString("🔍 **CI/CD Workflow Preview**\n\n")
	message.WriteString(fmt.Sprintf("📋 **Stack**: %s\n", stackName))
	message.WriteString(fmt.Sprintf("🏢 **Organization**: %s\n", config.Organization.Name))
	message.WriteString(fmt.Sprintf("📁 **Output Directory**: %s\n\n", outputDir))

	message.WriteString("**Workflows to be generated:**\n")

	// List workflows based on templates
	for _, template := range config.WorkflowGeneration.Templates {
		workflowFile := fmt.Sprintf("%s-%s.yml", template, stackName)
		message.WriteString(fmt.Sprintf("- %s\n", workflowFile))
	}

	message.WriteString(fmt.Sprintf("\n**Environments**: %s\n", strings.Join(getEnvironmentNames(config.Environments), ", ")))

	requiredSecrets := getRequiredSecrets(config)
	message.WriteString(fmt.Sprintf("**Required Secrets**: %s\n", strings.Join(requiredSecrets, ", ")))

	return &Result{
		Success: true,
		Message: message.String(),
		Data: map[string]interface{}{
			"stack_name":       stackName,
			"organization":     config.Organization.Name,
			"output_dir":       outputDir,
			"templates":        config.WorkflowGeneration.Templates,
			"environments":     getEnvironmentNames(config.Environments),
			"required_secrets": requiredSecrets,
		},
	}, nil
}

// checkExistingWorkflows checks for existing workflow files
func (s *Service) checkExistingWorkflows(config *github.EnhancedActionsCiCdConfig, stackName, outputDir string) []string {
	var existingFiles []string

	for _, template := range config.WorkflowGeneration.Templates {
		workflowFile := fmt.Sprintf("%s-%s.yml", template, stackName)
		filePath := filepath.Join(outputDir, workflowFile)
		if _, err := os.Stat(filePath); err == nil {
			existingFiles = append(existingFiles, workflowFile)
		}
	}

	return existingFiles
}

// getEnvironmentNames extracts environment names from configuration
func getEnvironmentNames(environments map[string]github.EnvironmentConfig) []string {
	var names []string
	for name := range environments {
		names = append(names, name)
	}
	return names
}

// createServerDescriptorFromParentConfig reads the actual server.yaml from parent repository
// instead of creating synthetic configuration
func createServerDescriptorFromParentConfig() (*api.ServerDescriptor, error) {
	// First, try to read from SC_CONFIG environment variable (GitHub Actions scenario)
	config, err := readConfigFromSCConfigEnv()
	if err != nil {
		// Fall back to local configuration files
		config, err = api.ReadConfigFile(".", "default")
		if err != nil {
			// Try other profile if default fails
			profile := os.Getenv("SC_PROFILE")
			if profile != "" && profile != "default" {
				config, err = api.ReadConfigFile(".", profile)
				if err != nil {
					return nil, fmt.Errorf("no server.yaml found and no parent repository configuration available in SC_CONFIG, .sc/cfg.default.yaml or profile '%s'", profile)
				}
			} else {
				return nil, fmt.Errorf("no server.yaml found and no parent repository configuration available in SC_CONFIG or .sc/cfg.default.yaml")
			}
		}
	}

	// Check if parent repository is configured
	if config.ParentRepository == "" {
		return nil, fmt.Errorf("no server.yaml found and no parentRepository configured in SC_CONFIG or .sc/cfg.default.yaml")
	}

	// Clone parent repository and read actual server.yaml configuration
	serverDesc, err := cloneParentRepositoryAndReadServerConfig(config.ParentRepository, config.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to read CI/CD configuration from parent repository: %w", err)
	}

	return serverDesc, nil
}

// readConfigFromSCConfigEnv reads SC configuration from SC_CONFIG environment variable
// Returns error if SC_CONFIG is not set or contains invalid YAML
func readConfigFromSCConfigEnv() (*api.ConfigFile, error) {
	// Get SC config from environment
	scConfigYAML := os.Getenv("SC_CONFIG")
	if scConfigYAML == "" {
		return nil, fmt.Errorf("SC_CONFIG environment variable not set")
	}

	// Parse SC_CONFIG YAML
	var scConfig api.ConfigFile
	if err := yaml.Unmarshal([]byte(scConfigYAML), &scConfig); err != nil {
		return nil, fmt.Errorf("failed to parse SC_CONFIG: %w", err)
	}

	return &scConfig, nil
}

// cloneParentRepositoryAndReadServerConfig clones the parent repository and reads server.yaml
// Reuses logic similar to parent repository operations in GitHub Actions
func cloneParentRepositoryAndReadServerConfig(parentRepoURL, privateKey string) (*api.ServerDescriptor, error) {
	devopsDir := ".devops-cicd-temp"

	// Remove existing directory if it exists
	if err := os.RemoveAll(devopsDir); err != nil {
		return nil, fmt.Errorf("failed to remove existing temp directory: %w", err)
	}

	// Ensure cleanup happens
	defer func() {
		if err := os.RemoveAll(devopsDir); err != nil {
			// Log warning but don't fail
			fmt.Printf("Warning: failed to cleanup temp directory %s: %v\n", devopsDir, err)
		}
	}()

	// Set up SSH for git operations if private key is available
	if privateKey != "" {
		if err := setupTempSSHForGit(privateKey); err != nil {
			return nil, fmt.Errorf("failed to setup SSH for git: %w", err)
		}
	}

	// Clone the repository (use git command for simplicity)
	if err := executeGitClone(parentRepoURL, devopsDir); err != nil {
		return nil, fmt.Errorf("failed to clone parent repository: %w", err)
	}

	// Read server.yaml from the cloned parent repository
	// First try root server.yaml
	parentServerYaml := filepath.Join(devopsDir, "server.yaml")
	if _, err := os.Stat(parentServerYaml); os.IsNotExist(err) {
		// If not found in root, look for server.yaml files in .sc/stacks/* directories
		stacksDir := filepath.Join(devopsDir, ".sc", "stacks")
		if _, scErr := os.Stat(stacksDir); scErr == nil {
			if scEntries, scListErr := os.ReadDir(stacksDir); scListErr == nil {
				for _, scEntry := range scEntries {
					if scEntry.IsDir() {
						stackServerYaml := filepath.Join(stacksDir, scEntry.Name(), "server.yaml")
						if _, stackErr := os.Stat(stackServerYaml); stackErr == nil {
							// Found server.yaml in a stack directory, use it
							parentServerYaml = stackServerYaml
							break
						}
					}
				}
			}
		}

		// If still not found, return error
		if _, finalErr := os.Stat(parentServerYaml); os.IsNotExist(finalErr) {
			return nil, fmt.Errorf("no server.yaml found in parent repository root or .sc/stacks/* directories")
		}
	}

	// Use SC's internal API to read server configuration
	serverDesc, err := api.ReadServerDescriptor(parentServerYaml)
	if err != nil {
		return nil, fmt.Errorf("failed to read server.yaml from parent repository: %w", err)
	}

	// Validate that CI/CD configuration exists in parent repository
	if serverDesc.CiCd.Type == "" {
		return nil, fmt.Errorf("no CI/CD configuration found in parent repository's server.yaml")
	}

	if serverDesc.CiCd.Type != "github-actions" {
		return nil, fmt.Errorf("unsupported CI/CD type '%s' in parent repository. Only 'github-actions' is currently supported", serverDesc.CiCd.Type)
	}

	return serverDesc, nil
}

// setupTempSSHForGit sets up SSH configuration for git operations with temp files
func setupTempSSHForGit(privateKey string) error {
	// Create temporary SSH directory
	sshDir := filepath.Join(os.TempDir(), "ssh-cicd-temp")
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		return fmt.Errorf("failed to create SSH directory: %w", err)
	}

	// Write private key to temporary file
	keyFile := filepath.Join(sshDir, "id_rsa")
	if err := os.WriteFile(keyFile, []byte(privateKey), 0o600); err != nil {
		return fmt.Errorf("failed to write SSH key: %w", err)
	}

	// Set GIT_SSH_COMMAND environment variable
	gitSSHCommand := fmt.Sprintf("ssh -i %s -o StrictHostKeyChecking=no", keyFile)
	if err := os.Setenv("GIT_SSH_COMMAND", gitSSHCommand); err != nil {
		return fmt.Errorf("failed to set GIT_SSH_COMMAND: %w", err)
	}

	return nil
}

// executeGitClone executes git clone command using os/exec
// Reuses the same approach as parent repository operations in GitHub Actions
func executeGitClone(repoURL, destDir string) error {
	// Create git clone command - use shallow clone for faster performance
	cmd := exec.Command("git", "clone", "--depth", "1", repoURL, destDir)

	// Set environment to use SSH configuration if available
	cmd.Env = append(os.Environ(), "GIT_SSH_COMMAND=ssh -o StrictHostKeyChecking=no")

	// Execute git clone
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git clone failed: %w (output: %s)", err, string(output))
	}

	return nil
}
