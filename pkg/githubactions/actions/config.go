package actions

import (
	"context"
	"fmt"
	"os"

	"gopkg.in/yaml.v2"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/secrets"
	"github.com/simple-container-com/api/pkg/provisioner"
)

// createSCConfigFile creates the Simple Container configuration file from SC_CONFIG
func (e *Executor) createSCConfigFile(ctx context.Context, scConfig *api.ConfigFile) error {
	e.logger.Info(ctx, "Creating Simple Container configuration file...")

	// Ensure .sc directory exists
	if err := os.MkdirAll(".sc", 0o755); err != nil {
		return fmt.Errorf("failed to create .sc directory: %w", err)
	}

	// Use project name from SC_CONFIG - CRITICAL for state storage consistency
	projectName := scConfig.ProjectName
	if projectName == "" {
		return fmt.Errorf("projectName is required in SC_CONFIG to ensure state storage consistency - cannot proceed without it")
	}
	e.logger.Debug(ctx, "✅ Using project name from SC_CONFIG: %s", projectName)
	e.logger.Debug(ctx, "🔍 Parent Repository: %s", scConfig.ParentRepository)

	// Note: Parent repository URL is available for other operations if needed
	if scConfig.ParentRepository != "" {
		e.logger.Debug(ctx, "🔗 Parent repository configured: %s", scConfig.ParentRepository)
	} else {
		e.logger.Debug(ctx, "📁 No parent repository configured - standalone project")
	}

	e.logger.Debug(ctx, "🔍 FINAL PROJECT NAME: %s (this determines Pulumi stack reference)", projectName)

	// Create temporary SSH key files for SC CLI
	sshDir := os.Getenv("HOME") + "/.ssh"
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		return fmt.Errorf("failed to create SSH directory: %w", err)
	}

	privateKeyPath := sshDir + "/sc_github_actions"
	publicKeyPath := sshDir + "/sc_github_actions.pub"

	// Write SSH keys for SC CLI to use
	if err := os.WriteFile(privateKeyPath, []byte(scConfig.PrivateKey), 0o600); err != nil {
		return fmt.Errorf("failed to write private key: %w", err)
	}

	publicKeyContent := scConfig.PublicKey
	if publicKeyContent == "" {
		e.logger.Info(ctx, "No public key provided, using private key path only")
	} else {
		if err := os.WriteFile(publicKeyPath, []byte(publicKeyContent), 0o644); err != nil {
			return fmt.Errorf("failed to write public key: %w", err)
		}
	}

	// Create SC configuration
	configContent := fmt.Sprintf(`projectName: %s
privateKeyPath: %s
publicKeyPath: %s
`, projectName, privateKeyPath, publicKeyPath)

	// Write configuration file - use environment-specific profile name
	profile := os.Getenv("ENVIRONMENT")
	if profile == "" {
		profile = "default"
	}
	configPath := fmt.Sprintf(".sc/cfg.%s.yaml", profile)
	if err := os.WriteFile(configPath, []byte(configContent), 0o644); err != nil {
		return fmt.Errorf("failed to write SC config file: %w", err)
	}

	e.logger.Info(ctx, "Successfully created SC configuration: %s", configPath)
	e.logger.Info(ctx, "Project: %s, SSH keys: %s", projectName, privateKeyPath)

	return nil
}

// createSCConfigFromEnv creates SC configuration by parsing SC_CONFIG environment variable
// and reconfigures the provisioner's cryptor with the SSH keys
func (e *Executor) createSCConfigFromEnv(ctx context.Context) error {
	// Get SC config from environment
	scConfigYAML := os.Getenv("SC_CONFIG")
	if scConfigYAML == "" {
		return fmt.Errorf("SC_CONFIG environment variable not set")
	}

	// Parse SC_CONFIG YAML
	var scConfig api.ConfigFile
	if err := yaml.Unmarshal([]byte(scConfigYAML), &scConfig); err != nil {
		return fmt.Errorf("failed to parse SC_CONFIG: %w", err)
	}

	// Create SC configuration file
	if err := e.createSCConfigFile(ctx, &scConfig); err != nil {
		return fmt.Errorf("failed to create SC config file: %w", err)
	}

	// Reconfigure provisioner with SSH keys from SC_CONFIG
	if err := e.reconfigureProvisionerWithKeys(ctx, &scConfig); err != nil {
		e.logger.Warn(ctx, "Failed to reconfigure provisioner with SSH keys: %v", err)
		// Don't fail the entire operation, but log the warning
	}

	return nil
}

// reconfigureProvisionerWithKeys creates a new provisioner with SSH keys from SC_CONFIG
func (e *Executor) reconfigureProvisionerWithKeys(ctx context.Context, scConfig *api.ConfigFile) error {
	if scConfig.PrivateKey == "" || scConfig.PublicKey == "" {
		e.logger.Info(ctx, "No SSH keys in SC_CONFIG, keeping existing provisioner configuration")
		return nil
	}

	e.logger.Info(ctx, "Reconfiguring provisioner with SSH keys from SC_CONFIG...")

	// Get current working directory for cryptor
	workDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	// Create new cryptor with SSH keys from SC_CONFIG
	cryptor, err := secrets.NewCryptor(
		workDir,
		secrets.WithProfile(os.Getenv("ENVIRONMENT")),
		secrets.WithGitRepo(e.gitRepo),
		secrets.WithPrivateKey(scConfig.PrivateKey),
		secrets.WithPublicKey(scConfig.PublicKey),
	)
	if err != nil {
		return fmt.Errorf("failed to create cryptor with SSH keys: %w", err)
	}

	// Create and initialize new provisioner with configured cryptor
	newProvisioner, err := provisioner.New(
		provisioner.WithGitRepo(e.gitRepo),
		provisioner.WithLogger(e.logger),
		provisioner.WithCryptor(cryptor),
	)
	if err != nil {
		return fmt.Errorf("failed to create new provisioner: %w", err)
	}

	// Initialize the new provisioner with the same parameters as the original
	err = newProvisioner.Init(ctx, api.InitParams{
		ProjectName:         os.Getenv("STACK_NAME"),
		RootDir:             workDir,
		SkipInitialCommit:   true,
		SkipProfileCreation: true,
		Profile:             os.Getenv("ENVIRONMENT"),
	})
	if err != nil {
		return fmt.Errorf("failed to initialize new provisioner: %w", err)
	}

	// Replace existing provisioner
	e.provisioner = newProvisioner
	e.logger.Info(ctx, "✅ Successfully reconfigured provisioner with SSH keys")

	return nil
}
