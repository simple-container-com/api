package sc

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/simple-container-com/api/pkg/githubactions/config"
	"github.com/simple-container-com/api/pkg/githubactions/utils/logging"
)

// Operations handles Simple Container CLI operations
type Operations struct {
	cfg    *config.Config
	logger logging.Logger
}

// DeployOptions specifies options for stack deployment
type DeployOptions struct {
	StackName    string
	Environment  string
	Version      string
	ImageVersion string
	Flags        string
	WorkDir      string
}

// PRPreviewOptions specifies options for PR preview configuration
type PRPreviewOptions struct {
	PRNumber    string
	DomainBase  string
	StackName   string
	Environment string
}

// CustomConfigOptions specifies options for custom YAML configuration
type CustomConfigOptions struct {
	YAMLConfig string
	Encrypted  bool
	StackName  string
}

// ValidationOptions specifies options for post-deployment validation
type ValidationOptions struct {
	Command     string
	StackName   string
	Environment string
	Version     string
	WorkDir     string
}

// FinalizeOptions specifies options for deployment finalization
type FinalizeOptions struct {
	Version     string
	StackName   string
	Environment string
	CreateTag   bool
	WorkDir     string
}

// NewOperations creates a new Simple Container operations instance
func NewOperations(cfg *config.Config, logger logging.Logger) *Operations {
	return &Operations{
		cfg:    cfg,
		logger: logger,
	}
}

// Setup initializes Simple Container configuration and environment
func (s *Operations) Setup(ctx context.Context) error {
	s.logger.Info("Setting up Simple Container environment")

	// Create SC configuration directory
	scDir := filepath.Join(s.cfg.GitHubWorkspace, ".sc")
	if err := os.MkdirAll(scDir, 0o755); err != nil {
		return fmt.Errorf("failed to create .sc directory: %w", err)
	}

	// Write SC configuration file
	configPath := filepath.Join(scDir, "cfg.default.yaml")
	if err := os.WriteFile(configPath, []byte(s.cfg.SCConfig), 0o600); err != nil {
		return fmt.Errorf("failed to write SC config: %w", err)
	}

	s.logger.Info("SC configuration written", "path", configPath)

	// Reveal secrets (this might fail if no secrets are configured)
	if err := s.revealSecrets(ctx); err != nil {
		s.logger.Warn("Failed to reveal secrets", "error", err)
		// Don't fail the setup for this, as not all stacks have secrets
	}

	// Setup DevOps repository access if needed
	if err := s.setupDevOpsRepository(ctx); err != nil {
		s.logger.Warn("DevOps repository setup failed", "error", err)
		// Don't fail the setup for this, as it might not be needed
	}

	return nil
}

// revealSecrets reveals secrets using SC CLI
func (s *Operations) revealSecrets(ctx context.Context) error {
	s.logger.Info("Revealing secrets")

	cmd := exec.CommandContext(ctx, "sc", "secrets", "reveal", "--force")
	cmd.Dir = s.cfg.GitHubWorkspace
	cmd.Env = s.getEnvironment()

	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("sc secrets reveal failed: %w, output: %s", err, output)
	}

	return nil
}

// setupDevOpsRepository sets up access to the DevOps repository if needed
func (s *Operations) setupDevOpsRepository(ctx context.Context) error {
	// This would typically involve reading SSH keys from SC secrets
	// and setting up access to a private DevOps repository
	// For now, we'll skip this as it's not always needed

	s.logger.Debug("DevOps repository setup skipped - not required for basic deployments")
	return nil
}

// Deploy deploys the specified stack
func (s *Operations) Deploy(ctx context.Context, opts *DeployOptions) error {
	s.logger.Info("Deploying stack",
		"stack", opts.StackName,
		"environment", opts.Environment,
		"version", opts.Version)

	// Prepare environment variables
	env := s.getEnvironment()
	env = append(env, fmt.Sprintf("VERSION=%s", opts.Version))

	if opts.ImageVersion != "" {
		env = append(env, fmt.Sprintf("IMAGE_VERSION=%s", opts.ImageVersion))
		s.logger.Info("Using custom image version", "image_version", opts.ImageVersion)
	}

	// Build deploy command
	args := []string{"deploy", "-s", opts.StackName, "-e", opts.Environment}

	// Add additional flags if provided
	if opts.Flags != "" {
		additionalArgs := s.parseFlags(opts.Flags)
		args = append(args, additionalArgs...)
	}

	// Execute deployment
	cmd := exec.CommandContext(ctx, "sc", args...)
	cmd.Dir = opts.WorkDir
	cmd.Env = env
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("sc deploy failed: %w", err)
	}

	s.logger.Info("Stack deployed successfully")
	return nil
}

// ConfigurePRPreview configures PR preview settings
func (s *Operations) ConfigurePRPreview(ctx context.Context, opts *PRPreviewOptions) error {
	s.logger.Info("Configuring PR preview",
		"pr_number", opts.PRNumber,
		"domain_base", opts.DomainBase)

	// Compute preview subdomain
	subdomain := fmt.Sprintf("pr%s-%s", opts.PRNumber, opts.DomainBase)

	// Path to client.yaml
	clientYamlPath := filepath.Join(s.cfg.GitHubWorkspace, ".sc", "stacks", opts.StackName, "client.yaml")

	// Create and execute script to append preview profile
	scriptContent := s.generatePreviewProfileScript(clientYamlPath, subdomain, opts.PRNumber)

	if err := s.executeScript(ctx, "configure-preview", scriptContent); err != nil {
		return fmt.Errorf("PR preview configuration failed: %w", err)
	}

	s.logger.Info("PR preview configured", "subdomain", subdomain)
	return nil
}

// ApplyCustomConfiguration applies custom YAML configuration
func (s *Operations) ApplyCustomConfiguration(ctx context.Context, opts *CustomConfigOptions) error {
	s.logger.Info("Applying custom YAML configuration", "encrypted", opts.Encrypted)

	clientYamlPath := filepath.Join(s.cfg.GitHubWorkspace, ".sc", "stacks", opts.StackName, "client.yaml")

	// Create and execute script to append custom configuration
	scriptContent := s.generateCustomConfigScript(clientYamlPath, opts.YAMLConfig, opts.Encrypted)

	if err := s.executeScript(ctx, "apply-custom-config", scriptContent); err != nil {
		return fmt.Errorf("custom configuration failed: %w", err)
	}

	s.logger.Info("Custom configuration applied successfully")
	return nil
}

// RunValidation runs post-deployment validation
func (s *Operations) RunValidation(ctx context.Context, opts *ValidationOptions) error {
	s.logger.Info("Running post-deployment validation")

	// Set up environment for validation
	env := s.getEnvironment()
	env = append(env,
		fmt.Sprintf("DEPLOYED_VERSION=%s", opts.Version),
		fmt.Sprintf("STACK_NAME=%s", opts.StackName),
		fmt.Sprintf("ENVIRONMENT=%s", opts.Environment),
	)

	// Execute validation command
	cmd := exec.CommandContext(ctx, "bash", "-c", opts.Command)
	cmd.Dir = opts.WorkDir
	cmd.Env = env
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("validation command failed: %w", err)
	}

	s.logger.Info("Validation completed successfully")
	return nil
}

// Finalize performs deployment finalization tasks
func (s *Operations) Finalize(ctx context.Context, opts *FinalizeOptions) error {
	s.logger.Info("Finalizing deployment")

	// Create release tag if requested
	if opts.CreateTag {
		tagName := fmt.Sprintf("v%s", opts.Version)
		message := fmt.Sprintf("Release %s for %s/%s", opts.Version, opts.StackName, opts.Environment)

		if err := s.createReleaseTag(ctx, opts.WorkDir, tagName, message); err != nil {
			s.logger.Warn("Failed to create release tag", "tag", tagName, "error", err)
			// Don't fail the entire process for tagging issues
		}
	}

	// Could add other finalization tasks here
	// - Cleanup temporary files
	// - Generate deployment report
	// - Update deployment status

	s.logger.Info("Finalization completed")
	return nil
}

// createReleaseTag creates a git tag for the release
func (s *Operations) createReleaseTag(ctx context.Context, workDir, tagName, message string) error {
	s.logger.Info("Creating release tag", "tag", tagName)

	// Create the tag
	cmd := exec.CommandContext(ctx, "git", "tag", "-a", tagName, "-m", message)
	cmd.Dir = workDir
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create tag: %w", err)
	}

	// Push the tag
	cmd = exec.CommandContext(ctx, "git", "push", "origin", tagName)
	cmd.Dir = workDir
	if err := cmd.Run(); err != nil {
		s.logger.Warn("Failed to push tag", "tag", tagName, "error", err)
		// Don't fail for push issues
	}

	return nil
}

// getEnvironment returns environment variables for SC CLI
func (s *Operations) getEnvironment() []string {
	env := os.Environ()
	env = append(env, fmt.Sprintf("SIMPLE_CONTAINER_CONFIG=%s", s.cfg.SCConfig))

	if s.cfg.SCVersion != "latest" {
		env = append(env, fmt.Sprintf("SIMPLE_CONTAINER_VERSION=%s", s.cfg.SCVersion))
	}

	return env
}

// parseFlags parses deployment flags string into arguments
func (s *Operations) parseFlags(flags string) []string {
	if flags == "" {
		return nil
	}

	// Simple parsing - split by spaces and handle quoted arguments
	var args []string
	parts := strings.Fields(flags)

	for _, part := range parts {
		// Remove quotes if present
		part = strings.Trim(part, `"'`)
		if part != "" {
			args = append(args, part)
		}
	}

	return args
}

// executeScript creates and executes a bash script
func (s *Operations) executeScript(ctx context.Context, name, content string) error {
	// Create temporary script file
	scriptPath := filepath.Join("/tmp", fmt.Sprintf("%s.sh", name))

	if err := os.WriteFile(scriptPath, []byte(content), 0o755); err != nil {
		return fmt.Errorf("failed to create script: %w", err)
	}

	defer os.Remove(scriptPath) // Clean up

	// Execute script
	cmd := exec.CommandContext(ctx, "bash", scriptPath)
	cmd.Dir = s.cfg.GitHubWorkspace
	cmd.Env = s.getEnvironment()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("script execution failed: %w", err)
	}

	return nil
}

// generatePreviewProfileScript generates a script to configure PR preview
func (s *Operations) generatePreviewProfileScript(yamlPath, subdomain, prNumber string) string {
	return fmt.Sprintf(`#!/bin/bash
set -euo pipefail

YAML_PATH="%s"
SUBDOMAIN="%s"
PR_NUMBER="%s"

# Append PR preview configuration to client.yaml
if [[ -f "$YAML_PATH" ]]; then
    echo "Appending PR preview configuration to $YAML_PATH"
    
    # Add preview configuration
    cat >> "$YAML_PATH" << EOF

# PR Preview Configuration (PR #$PR_NUMBER)
preview:
  domain: $SUBDOMAIN
  pr: $PR_NUMBER
EOF
    
    echo "PR preview configuration added successfully"
else
    echo "Warning: $YAML_PATH not found, skipping preview configuration"
fi
`, yamlPath, subdomain, prNumber)
}

// generateCustomConfigScript generates a script to apply custom configuration
func (s *Operations) generateCustomConfigScript(yamlPath, yamlConfig string, encrypted bool) string {
	decryptionStep := ""
	if encrypted {
		decryptionStep = `
    # Decrypt the YAML config using SC
    YAML_CONFIG=$(echo "$YAML_CONFIG" | sc decrypt)
`
	}

	return fmt.Sprintf(`#!/bin/bash
set -euo pipefail

YAML_PATH="%s"
YAML_CONFIG="%s"

%s

if [[ -n "$YAML_CONFIG" && -f "$YAML_PATH" ]]; then
    echo "Appending custom YAML configuration to $YAML_PATH"
    
    # Append custom configuration
    echo "" >> "$YAML_PATH"
    echo "# Custom Configuration" >> "$YAML_PATH"
    echo "$YAML_CONFIG" >> "$YAML_PATH"
    
    echo "Custom configuration applied successfully"
else
    echo "Skipping custom configuration (empty or file not found)"
fi
`, yamlPath, yamlConfig, decryptionStep)
}
