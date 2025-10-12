package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds all configuration for GitHub Actions
type Config struct {
	// Core deployment inputs
	StackName   string `env:"STACK_NAME" required:"true"`
	Environment string `env:"ENVIRONMENT" required:"true"`
	SCConfig    string `env:"SC_CONFIG" required:"true"`

	// Simple Container configuration
	SCVersion     string `env:"SC_VERSION" default:"latest"`
	SCDeployFlags string `env:"SC_DEPLOY_FLAGS"`

	// Version management
	VersionSuffix   string `env:"VERSION_SUFFIX"`
	AppImageVersion string `env:"APP_IMAGE_VERSION"`

	// PR preview configuration
	PRPreview         bool   `env:"PR_PREVIEW" default:"false"`
	PreviewDomainBase string `env:"PREVIEW_DOMAIN_BASE" default:"preview.mycompany.com"`

	// Stack configuration
	StackYAMLConfig          string `env:"STACK_YAML_CONFIG"`
	StackYAMLConfigEncrypted bool   `env:"STACK_YAML_CONFIG_ENCRYPTED" default:"false"`

	// Validation
	ValidationCommand string `env:"VALIDATION_COMMAND"`

	// Notification configuration
	CCOnStart         bool   `env:"CC_ON_START" default:"true"`
	SlackWebhookURL   string `env:"SLACK_WEBHOOK_URL"`
	DiscordWebhookURL string `env:"DISCORD_WEBHOOK_URL"`

	// Runner configuration
	Runner string `env:"RUNNER" default:"ubuntu-latest"`

	// GitHub context (automatically available in GitHub Actions)
	GitHubToken       string `env:"GITHUB_TOKEN" required:"true"`
	GitHubRepository  string `env:"GITHUB_REPOSITORY" required:"true"`
	GitHubSHA         string `env:"GITHUB_SHA" required:"true"`
	GitHubRefName     string `env:"GITHUB_REF_NAME" required:"true"`
	GitHubActor       string `env:"GITHUB_ACTOR" required:"true"`
	GitHubRunID       string `env:"GITHUB_RUN_ID" required:"true"`
	GitHubRunNumber   string `env:"GITHUB_RUN_NUMBER" required:"true"`
	GitHubServerURL   string `env:"GITHUB_SERVER_URL" required:"true"`
	GitHubWorkspace   string `env:"GITHUB_WORKSPACE"`
	GitHubOutput      string `env:"GITHUB_OUTPUT"`
	GitHubStepSummary string `env:"GITHUB_STEP_SUMMARY"`

	// PR context for previews
	PRNumber  string `env:"PR_NUMBER"`
	PRHeadRef string `env:"PR_HEAD_REF"`
	PRHeadSHA string `env:"PR_HEAD_SHA"`
	PRBaseRef string `env:"PR_BASE_REF"`

	// Commit context
	CommitMessage string `env:"COMMIT_MESSAGE"`

	// Operational settings
	WaitTimeout time.Duration `env:"WAIT_TIMEOUT" default:"30m"`

	// Destroy-specific settings
	AutoConfirm         bool   `env:"AUTO_CONFIRM" default:"false"`
	SkipBackup          bool   `env:"SKIP_BACKUP" default:"false"`
	Confirmation        string `env:"CONFIRMATION"`       // For destroy-parent-stack
	TargetEnvironment   string `env:"TARGET_ENVIRONMENT"` // For destroy-parent-stack
	DestroyScope        string `env:"DESTROY_SCOPE" default:"environment-only"`
	SafetyMode          string `env:"SAFETY_MODE" default:"strict"`
	ForceDestroy        bool   `env:"FORCE_DESTROY" default:"false"`
	BackupBeforeDestroy bool   `env:"BACKUP_BEFORE_DESTROY" default:"true"`
	PreserveData        bool   `env:"PRESERVE_DATA" default:"true"`
	ExcludeResources    string `env:"EXCLUDE_RESOURCES"`

	// Provision-specific settings
	DryRun             bool `env:"DRY_RUN" default:"false"`
	NotifyOnCompletion bool `env:"NOTIFY_ON_COMPLETION" default:"true"`
}

// LoadFromEnvironment loads configuration from environment variables
func LoadFromEnvironment() (*Config, error) {
	cfg := &Config{}

	// Load all required and optional environment variables
	if err := loadEnvVars(cfg); err != nil {
		return nil, fmt.Errorf("failed to parse environment variables: %w", err)
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	return cfg, nil
}

// loadEnvVars loads environment variables into the config struct
func loadEnvVars(cfg *Config) error {
	// Core deployment inputs
	cfg.StackName = getEnvOrDefault("STACK_NAME", "")
	cfg.Environment = getEnvOrDefault("ENVIRONMENT", "")
	cfg.SCConfig = getEnvOrDefault("SC_CONFIG", "")

	// Simple Container configuration
	cfg.SCVersion = getEnvOrDefault("SC_VERSION", "latest")
	cfg.SCDeployFlags = getEnvOrDefault("SC_DEPLOY_FLAGS", "")

	// Version management
	cfg.VersionSuffix = getEnvOrDefault("VERSION_SUFFIX", "")
	cfg.AppImageVersion = getEnvOrDefault("APP_IMAGE_VERSION", "")

	// PR preview configuration
	cfg.PRPreview = parseBoolEnv("PR_PREVIEW", false)
	cfg.PreviewDomainBase = getEnvOrDefault("PREVIEW_DOMAIN_BASE", "preview.mycompany.com")

	// Stack configuration
	cfg.StackYAMLConfig = getEnvOrDefault("STACK_YAML_CONFIG", "")
	cfg.StackYAMLConfigEncrypted = parseBoolEnv("STACK_YAML_CONFIG_ENCRYPTED", false)

	// Validation
	cfg.ValidationCommand = getEnvOrDefault("VALIDATION_COMMAND", "")

	// Notification configuration
	cfg.CCOnStart = parseBoolEnv("CC_ON_START", true)
	cfg.SlackWebhookURL = getEnvOrDefault("SLACK_WEBHOOK_URL", "")
	cfg.DiscordWebhookURL = getEnvOrDefault("DISCORD_WEBHOOK_URL", "")

	// Runner configuration
	cfg.Runner = getEnvOrDefault("RUNNER", "ubuntu-latest")

	// GitHub context
	cfg.GitHubToken = getEnvOrDefault("GITHUB_TOKEN", "")
	cfg.GitHubRepository = getEnvOrDefault("GITHUB_REPOSITORY", "")
	cfg.GitHubSHA = getEnvOrDefault("GITHUB_SHA", "")
	cfg.GitHubRefName = getEnvOrDefault("GITHUB_REF_NAME", "")
	cfg.GitHubActor = getEnvOrDefault("GITHUB_ACTOR", "")
	cfg.GitHubRunID = getEnvOrDefault("GITHUB_RUN_ID", "")
	cfg.GitHubRunNumber = getEnvOrDefault("GITHUB_RUN_NUMBER", "")
	cfg.GitHubServerURL = getEnvOrDefault("GITHUB_SERVER_URL", "")
	cfg.GitHubWorkspace = getEnvOrDefault("GITHUB_WORKSPACE", "/workspace")
	cfg.GitHubOutput = getEnvOrDefault("GITHUB_OUTPUT", "")
	cfg.GitHubStepSummary = getEnvOrDefault("GITHUB_STEP_SUMMARY", "")

	// PR context
	cfg.PRNumber = getEnvOrDefault("PR_NUMBER", "")
	cfg.PRHeadRef = getEnvOrDefault("PR_HEAD_REF", "")
	cfg.PRHeadSHA = getEnvOrDefault("PR_HEAD_SHA", "")
	cfg.PRBaseRef = getEnvOrDefault("PR_BASE_REF", "")

	// Commit context
	cfg.CommitMessage = getEnvOrDefault("COMMIT_MESSAGE", "")

	// Operational settings
	var err error
	timeoutStr := getEnvOrDefault("WAIT_TIMEOUT", "30m")
	cfg.WaitTimeout, err = time.ParseDuration(timeoutStr)
	if err != nil {
		return fmt.Errorf("invalid WAIT_TIMEOUT format: %w", err)
	}

	// Destroy-specific settings
	cfg.AutoConfirm = parseBoolEnv("AUTO_CONFIRM", false)
	cfg.SkipBackup = parseBoolEnv("SKIP_BACKUP", false)
	cfg.Confirmation = getEnvOrDefault("CONFIRMATION", "")
	cfg.TargetEnvironment = getEnvOrDefault("TARGET_ENVIRONMENT", "")
	cfg.DestroyScope = getEnvOrDefault("DESTROY_SCOPE", "environment-only")
	cfg.SafetyMode = getEnvOrDefault("SAFETY_MODE", "strict")
	cfg.ForceDestroy = parseBoolEnv("FORCE_DESTROY", false)
	cfg.BackupBeforeDestroy = parseBoolEnv("BACKUP_BEFORE_DESTROY", true)
	cfg.PreserveData = parseBoolEnv("PRESERVE_DATA", true)
	cfg.ExcludeResources = getEnvOrDefault("EXCLUDE_RESOURCES", "")

	// Provision-specific settings
	cfg.DryRun = parseBoolEnv("DRY_RUN", false)
	cfg.NotifyOnCompletion = parseBoolEnv("NOTIFY_ON_COMPLETION", true)

	return nil
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	// Check required fields
	if c.StackName == "" {
		return fmt.Errorf("STACK_NAME is required")
	}
	if c.Environment == "" {
		return fmt.Errorf("ENVIRONMENT is required")
	}
	if c.SCConfig == "" {
		return fmt.Errorf("SC_CONFIG is required")
	}
	if c.GitHubToken == "" {
		return fmt.Errorf("GITHUB_TOKEN is required")
	}
	if c.GitHubRepository == "" {
		return fmt.Errorf("GITHUB_REPOSITORY is required")
	}
	if c.GitHubSHA == "" {
		return fmt.Errorf("GITHUB_SHA is required")
	}

	// Validate destroy parent stack specific requirements
	if c.Confirmation == "DESTROY-INFRASTRUCTURE" {
		if c.TargetEnvironment == "" {
			return fmt.Errorf("TARGET_ENVIRONMENT is required for infrastructure destruction")
		}

		validSafetyModes := map[string]bool{
			"strict":     true,
			"standard":   true,
			"permissive": true,
		}
		if !validSafetyModes[c.SafetyMode] {
			return fmt.Errorf("invalid SAFETY_MODE: %s, valid options: strict, standard, permissive", c.SafetyMode)
		}

		validDestroyScopes := map[string]bool{
			"environment-only": true,
			"shared-resources": true,
			"all":              true,
		}
		if !validDestroyScopes[c.DestroyScope] {
			return fmt.Errorf("invalid DESTROY_SCOPE: %s, valid options: environment-only, shared-resources, all", c.DestroyScope)
		}
	}

	return nil
}

// getEnvOrDefault gets an environment variable or returns default value
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// parseBoolEnv parses a boolean environment variable
func parseBoolEnv(key string, defaultValue bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}

	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return defaultValue
	}

	return parsed
}
