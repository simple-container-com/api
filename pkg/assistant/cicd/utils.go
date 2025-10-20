package cicd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
// Follows SC's standard configuration reading patterns
func checkParentRepositoryConfig(stackName string) *ParentRepositoryInfo {
	// First, try to read SC configuration using standard profile system
	config, err := api.ReadConfigFile(".", "default")
	if err != nil {
		// Try other profile if default fails (following SC standard practice)
		profile := os.Getenv("SC_PROFILE")
		if profile != "" && profile != "default" {
			config, err = api.ReadConfigFile(".", profile)
			if err != nil {
				return nil
			}
		} else {
			return nil
		}
	}

	// Check if parent repository is configured
	if config.ParentRepository == "" {
		return nil
	}

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

// createEnhancedConfig converts server configuration to enhanced GitHub Actions config
func createEnhancedConfig(serverDesc *api.ServerDescriptor, stackName string, isParent bool) *github.EnhancedActionsCiCdConfig {
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

	// Choose templates based on repository type
	var defaultTemplates []string
	var defaultCustomActions map[string]string

	if isParentStack {
		// Parent repository workflows
		defaultTemplates = []string{"provision", "destroy-parent"}
		defaultCustomActions = map[string]string{
			"provision":      "simple-container-com/api/.github/actions/provision-parent-stack@main",
			"destroy-parent": "simple-container-com/api/.github/actions/destroy-parent-stack@main",
		}
	} else {
		// Client repository workflows
		defaultTemplates = []string{"deploy", "destroy"}
		defaultCustomActions = map[string]string{
			"deploy":  "simple-container-com/api/.github/actions/deploy-client-stack@main",
			"destroy": "simple-container-com/api/.github/actions/destroy-client-stack@main",
		}
	}

	// Use SC's standard conversion pattern to get strongly typed GitHub Actions configuration
	convertedConfig, err := api.ConvertConfig(&serverDesc.CiCd.Config, &github.GitHubActionsCiCdConfig{})
	if err != nil {
		// Fallback to default configuration
		return &github.EnhancedActionsCiCdConfig{
			Organization: github.OrganizationConfig{
				Name:          "simple-container-org",
				DefaultBranch: "main",
			},
			WorkflowGeneration: github.WorkflowGenerationConfig{
				Enabled:       true,
				Templates:     defaultTemplates,
				CustomActions: defaultCustomActions,
				SCVersion:     "latest",
			},
			Execution: github.ExecutionConfig{
				DefaultTimeout: "30",
			},
			Environments: map[string]github.EnvironmentConfig{
				"staging":    {Type: "staging", Runners: []string{"ubuntu-latest"}},
				"production": {Type: "production", Runners: []string{"ubuntu-latest"}},
			},
			Notifications: github.NotificationConfig{CCOnStart: false},
		}
	}

	// Extract the strongly typed configuration
	gitHubConfig, ok := convertedConfig.Config.(*github.GitHubActionsCiCdConfig)
	if !ok {
		// Fallback to default if type assertion fails
		return &github.EnhancedActionsCiCdConfig{
			Organization: github.OrganizationConfig{
				Name:          "simple-container-org",
				DefaultBranch: "main",
			},
			WorkflowGeneration: github.WorkflowGenerationConfig{
				Enabled:       true,
				Templates:     defaultTemplates,
				CustomActions: defaultCustomActions,
				SCVersion:     "latest",
			},
			Execution: github.ExecutionConfig{
				DefaultTimeout: "30",
			},
			Environments: map[string]github.EnvironmentConfig{
				"staging":    {Type: "staging", Runners: []string{"ubuntu-latest"}},
				"production": {Type: "production", Runners: []string{"ubuntu-latest"}},
			},
			Notifications: github.NotificationConfig{CCOnStart: false},
		}
	}

	// Convert to enhanced config with proper defaults
	config := &github.EnhancedActionsCiCdConfig{
		Organization: github.OrganizationConfig{
			Name:          gitHubConfig.Organization,
			DefaultBranch: "main",
		},
		WorkflowGeneration: github.WorkflowGenerationConfig{
			Enabled:       true,
			Templates:     defaultTemplates,     // Use repository type-specific templates
			CustomActions: defaultCustomActions, // Use repository type-specific actions
			SCVersion:     "latest",
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
			SlackWebhook:   gitHubConfig.Notifications.SlackWebhook,
			DiscordWebhook: gitHubConfig.Notifications.DiscordWebhook,
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
		runners := env.Runners
		if len(runners) == 0 {
			runners = []string{"ubuntu-latest"}
		} else {
			// Fix invalid runner names
			for i, runner := range runners {
				if runner == "ubuntu-22" {
					runners[i] = "ubuntu-latest"
				}
			}
		}

		config.Environments[name] = github.EnvironmentConfig{
			Type:        env.Type,
			Runners:     runners,
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
func autoDetectConfigFile(configFile, stackName string) (string, error) {
	if configFile != "" && configFile != "server.yaml" {
		return configFile, nil
	}

	// Try stack-specific server.yaml first
	stackDir := filepath.Join(".sc", "stacks", stackName)
	stackServerYaml := filepath.Join(stackDir, "server.yaml")
	if _, err := os.Stat(stackServerYaml); err == nil {
		return stackServerYaml, nil
	}

	// Fall back to root server.yaml
	if _, err := os.Stat("server.yaml"); err == nil {
		return "server.yaml", nil
	}

	return "", fmt.Errorf("no server.yaml found. Checked: %s, server.yaml", stackServerYaml)
}

// validateAndLoadServerConfig loads and validates server configuration
func validateAndLoadServerConfig(configFile string) (*api.ServerDescriptor, error) {
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
