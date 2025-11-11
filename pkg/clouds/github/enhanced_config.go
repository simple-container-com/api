package github

import (
	"fmt"
	"time"
)

// Enhanced ActionsCiCdConfig for organizational workflow generation
type EnhancedActionsCiCdConfig struct {
	// Basic authentication (existing)
	AuthToken string `json:"auth-token" yaml:"auth-token"`

	// Organization settings
	Organization OrganizationConfig `json:"organization" yaml:"organization"`

	// Workflow generation settings
	WorkflowGeneration WorkflowGenerationConfig `json:"workflow-generation" yaml:"workflow-generation"`

	// Environment-specific deployment configurations
	Environments map[string]EnvironmentConfig `json:"environments" yaml:"environments"`

	// Notification settings
	Notifications NotificationConfig `json:"notifications" yaml:"notifications"`

	// Custom runners and execution settings
	Execution ExecutionConfig `json:"execution" yaml:"execution"`

	// Validation and testing
	Validation ValidationConfig `json:"validation" yaml:"validation"`
}

// OrganizationConfig defines organization-wide CI/CD policies
type OrganizationConfig struct {
	Name             string   `json:"name" yaml:"name"`
	DefaultRunner    string   `json:"default-runner" yaml:"default-runner"`
	RequiredSecrets  []string `json:"required-secrets" yaml:"required-secrets"`
	BranchProtection bool     `json:"branch-protection" yaml:"branch-protection"`
	Reviewers        []string `json:"reviewers" yaml:"reviewers"`
	DefaultBranch    string   `json:"default-branch" yaml:"default-branch"`
}

// WorkflowGenerationConfig controls workflow generation behavior
type WorkflowGenerationConfig struct {
	Enabled       bool              `json:"enabled" yaml:"enabled"`
	OutputPath    string            `json:"output-path" yaml:"output-path"`
	Templates     []string          `json:"templates" yaml:"templates"`
	AutoUpdate    bool              `json:"auto-update" yaml:"auto-update"`
	CustomActions map[string]string `json:"custom-actions" yaml:"custom-actions"`
	SCVersion     string            `json:"sc-version" yaml:"sc-version"`
}

// EnvironmentConfig defines environment-specific deployment settings
type EnvironmentConfig struct {
	Type          string            `json:"type" yaml:"type"`
	Runner        string            `json:"runner" yaml:"runner"`
	Protection    bool              `json:"protection" yaml:"protection"`
	Reviewers     []string          `json:"reviewers" yaml:"reviewers"`
	Secrets       []string          `json:"secrets" yaml:"secrets"`
	Variables     map[string]string `json:"variables" yaml:"variables"`
	DeployFlags   []string          `json:"deploy-flags" yaml:"deploy-flags"`
	AutoDeploy    bool              `json:"auto-deploy" yaml:"auto-deploy"`
	ValidationCmd string            `json:"validation-command" yaml:"validation-command"`
	PRPreview     PRPreviewConfig   `json:"pr-preview" yaml:"pr-preview"`
	Concurrency   ConcurrencyConfig `json:"concurrency" yaml:"concurrency"`
}

// PRPreviewConfig defines PR preview deployment settings
type PRPreviewConfig struct {
	Enabled      bool   `json:"enabled" yaml:"enabled"`
	DomainBase   string `json:"domain-base" yaml:"domain-base"`
	LabelTrigger string `json:"label-trigger" yaml:"label-trigger"`
	AutoCleanup  bool   `json:"auto-cleanup" yaml:"auto-cleanup"`
}

// NotificationConfig defines notification settings
type NotificationConfig struct {
	SlackWebhook   string            `json:"slack-webhook" yaml:"slack-webhook"`
	DiscordWebhook string            `json:"discord-webhook" yaml:"discord-webhook"`
	TelegramChatID string            `json:"telegram-chat-id" yaml:"telegram-chat-id"`
	TelegramToken  string            `json:"telegram-token" yaml:"telegram-token"`
	UserMappings   map[string]string `json:"user-mappings" yaml:"user-mappings"`
	CCOnStart      bool              `json:"cc-on-start" yaml:"cc-on-start"`
	Channels       map[string]string `json:"channels" yaml:"channels"`
}

// ExecutionConfig defines workflow execution settings
type ExecutionConfig struct {
	DefaultTimeout string                `json:"default-timeout" yaml:"default-timeout"`
	Concurrency    ConcurrencyConfig     `json:"concurrency" yaml:"concurrency"`
	RetryPolicy    RetryConfig           `json:"retry-policy" yaml:"retry-policy"`
	CustomRunners  map[string]string     `json:"custom-runners" yaml:"custom-runners"`
	Permissions    map[string]Permission `json:"permissions" yaml:"permissions"`
}

// ConcurrencyConfig defines concurrency control
type ConcurrencyConfig struct {
	Group            string `json:"group" yaml:"group"`
	CancelInProgress bool   `json:"cancel-in-progress" yaml:"cancel-in-progress"`
}

// RetryConfig defines retry behavior
type RetryConfig struct {
	MaxAttempts  int           `json:"max-attempts" yaml:"max-attempts"`
	BackoffDelay time.Duration `json:"backoff-delay" yaml:"backoff-delay"`
	RetryOn      []string      `json:"retry-on" yaml:"retry-on"`
}

// Permission defines GitHub workflow permissions
type Permission struct {
	Actions      string `json:"actions" yaml:"actions"`
	Contents     string `json:"contents" yaml:"contents"`
	Deployments  string `json:"deployments" yaml:"deployments"`
	PullRequests string `json:"pull-requests" yaml:"pull-requests"`
	Statuses     string `json:"statuses" yaml:"statuses"`
}

// ValidationConfig defines validation and testing settings
type ValidationConfig struct {
	Required     bool              `json:"required" yaml:"required"`
	Commands     map[string]string `json:"commands" yaml:"commands"`
	HealthChecks map[string]string `json:"health-checks" yaml:"health-checks"`
	TestSuites   []string          `json:"test-suites" yaml:"test-suites"`
	SonarQube    SonarConfig       `json:"sonarqube" yaml:"sonarqube"`
	Security     SecurityConfig    `json:"security" yaml:"security"`
}

// SonarConfig defines SonarQube integration
type SonarConfig struct {
	Enabled    bool   `json:"enabled" yaml:"enabled"`
	ProjectKey string `json:"project-key" yaml:"project-key"`
	URL        string `json:"url" yaml:"url"`
	Token      string `json:"token" yaml:"token"`
}

// SecurityConfig defines security scanning
type SecurityConfig struct {
	Enabled       bool     `json:"enabled" yaml:"enabled"`
	VulnScan      bool     `json:"vuln-scan" yaml:"vuln-scan"`
	LicenseScan   bool     `json:"license-scan" yaml:"license-scan"`
	SecretScan    bool     `json:"secret-scan" yaml:"secret-scan"`
	ExcludePaths  []string `json:"exclude-paths" yaml:"exclude-paths"`
	FailThreshold string   `json:"fail-threshold" yaml:"fail-threshold"`
}

// Default configuration values
func (c *EnhancedActionsCiCdConfig) SetDefaults() {
	if c.Organization.DefaultBranch == "" {
		c.Organization.DefaultBranch = "main"
	}

	if c.Organization.DefaultRunner == "" {
		c.Organization.DefaultRunner = "ubuntu-latest"
	}

	if c.WorkflowGeneration.OutputPath == "" {
		c.WorkflowGeneration.OutputPath = ".github/workflows/"
	}

	if len(c.WorkflowGeneration.Templates) == 0 {
		c.WorkflowGeneration.Templates = []string{"deploy", "destroy", "pr-preview"}
	}

	if c.WorkflowGeneration.SCVersion == "" {
		c.WorkflowGeneration.SCVersion = "latest" // Use latest by default, which maps to @main
	}

	if c.WorkflowGeneration.CustomActions == nil {
		// Use @main for latest version by default, but allow CalVer tags to be specified via SCVersion
		actionVersion := "@main"
		if c.WorkflowGeneration.SCVersion != "" && c.WorkflowGeneration.SCVersion != "latest" {
			actionVersion = "@" + c.WorkflowGeneration.SCVersion
		}

		c.WorkflowGeneration.CustomActions = map[string]string{
			"deploy":         "simple-container-com/api/.github/actions/deploy" + actionVersion,
			"provision":      "simple-container-com/api/.github/actions/provision" + actionVersion,
			"destroy-client": "simple-container-com/api/.github/actions/destroy" + actionVersion,
			"destroy-parent": "simple-container-com/api/.github/actions/destroy-parent" + actionVersion,
		}
	}

	if c.Execution.DefaultTimeout == "" {
		c.Execution.DefaultTimeout = "30m"
	}

	if c.Execution.Concurrency.Group == "" {
		c.Execution.Concurrency.Group = "${{ github.workflow }}-${{ github.ref }}"
	}

	// Set default permissions for security
	if c.Execution.Permissions == nil {
		c.Execution.Permissions = map[string]Permission{
			"default": {
				Actions:      "read",
				Contents:     "read",
				Deployments:  "write",
				PullRequests: "write",
				Statuses:     "write",
			},
		}
	}

	// Set default retry policy
	if c.Execution.RetryPolicy.MaxAttempts == 0 {
		c.Execution.RetryPolicy.MaxAttempts = 3
		c.Execution.RetryPolicy.BackoffDelay = 30 * time.Second
		c.Execution.RetryPolicy.RetryOn = []string{"network-error", "timeout"}
	}
}

// Validate ensures the configuration is valid
func (c *EnhancedActionsCiCdConfig) Validate() error {
	if c.AuthToken == "" {
		return fmt.Errorf("auth-token is required")
	}

	if c.Organization.Name == "" {
		return fmt.Errorf("organization.name is required")
	}

	// Validate environments
	for envName, env := range c.Environments {
		if env.Type == "" {
			return fmt.Errorf("environment %s: type is required", envName)
		}

		if env.Runner == "" {
			return fmt.Errorf("environment %s: runner is required", envName)
		}

		if env.Protection && len(env.Reviewers) == 0 {
			return fmt.Errorf("environment %s: protected environments require reviewers", envName)
		}
	}

	// Validate notification settings - notifications are optional
	// Both SlackWebhook and DiscordWebhook can be empty, this is acceptable

	return nil
}

// GetEnvironmentByType returns environments of a specific type
func (c *EnhancedActionsCiCdConfig) GetEnvironmentsByType(envType string) map[string]EnvironmentConfig {
	result := make(map[string]EnvironmentConfig)
	for name, env := range c.Environments {
		if env.Type == envType {
			result[name] = env
		}
	}
	return result
}

// GetProductionEnvironments returns all production environments
func (c *EnhancedActionsCiCdConfig) GetProductionEnvironments() map[string]EnvironmentConfig {
	return c.GetEnvironmentsByType("production")
}

// GetStagingEnvironments returns all staging environments
func (c *EnhancedActionsCiCdConfig) GetStagingEnvironments() map[string]EnvironmentConfig {
	return c.GetEnvironmentsByType("staging")
}

// GetPreviewEnvironments returns all preview environments
func (c *EnhancedActionsCiCdConfig) GetPreviewEnvironments() map[string]EnvironmentConfig {
	return c.GetEnvironmentsByType("preview")
}

// IsWorkflowGenerationEnabled checks if workflow generation is enabled
func (c *EnhancedActionsCiCdConfig) IsWorkflowGenerationEnabled() bool {
	return c.WorkflowGeneration.Enabled
}

// GetRequiredSecrets returns all required secrets across environments
func (c *EnhancedActionsCiCdConfig) GetRequiredSecrets() []string {
	secrets := make(map[string]bool)

	// Add organization-wide required secrets
	for _, secret := range c.Organization.RequiredSecrets {
		secrets[secret] = true
	}

	// Add environment-specific secrets
	for _, env := range c.Environments {
		for _, secret := range env.Secrets {
			secrets[secret] = true
		}
	}

	// Convert to slice
	var result []string
	for secret := range secrets {
		result = append(result, secret)
	}

	return result
}
