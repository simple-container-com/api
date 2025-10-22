package actions

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v2"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/git"
	"github.com/simple-container-com/api/pkg/api/secrets"
)

// cloneParentRepository clones the parent stack repository and copies stack configurations
// For parent stack operations, it reveals secrets in the current repository instead of cloning
func (e *Executor) cloneParentRepository(ctx context.Context) error {
	e.logger.Info(ctx, "📦 Setting up parent stack repository...")

	// Get SC config from environment
	scConfigYAML := os.Getenv("SC_CONFIG")
	if scConfigYAML == "" {
		e.logger.Info(ctx, "No SC_CONFIG found, skipping parent repository setup")
		return nil
	}

	var scConfig api.ConfigFile
	if err := yaml.Unmarshal([]byte(scConfigYAML), &scConfig); err != nil {
		return fmt.Errorf("failed to parse SC_CONFIG: %w", err)
	}

	// Detect if this is a parent stack operation
	actionType := os.Getenv("GITHUB_ACTION_TYPE")
	isParentOperation := strings.Contains(actionType, "parent")

	if scConfig.ParentRepository == "" && !isParentOperation {
		e.logger.Info(ctx, "No parent repository configured, skipping clone")
		return nil
	}

	// Handle parent stack operations - reveal secrets in current repository
	if isParentOperation && scConfig.ParentRepository == "" {
		e.logger.Info(ctx, "🏗️ Parent stack operation detected - revealing secrets in current repository")
		return e.revealCurrentRepositorySecrets(ctx, &scConfig)
	}

	// Extract and sanitize repository URL for logging
	repoURL := e.sanitizeRepoURL(scConfig.ParentRepository)
	e.logger.Info(ctx, "Cloning parent repository: %s", repoURL)

	// Set up SSH keys for git operations
	if err := e.setupSSHForGit(ctx, scConfig.PrivateKey); err != nil {
		return fmt.Errorf("failed to setup SSH for git: %w", err)
	}

	devopsDir := ".devops"

	// Remove existing directory if it exists
	if err := os.RemoveAll(devopsDir); err != nil {
		e.logger.Warn(ctx, "Failed to remove existing .devops directory: %v", err)
	}

	// Clone the repository
	e.logger.Info(ctx, "Executing git clone operation...")
	cmd := exec.Command("git", "clone", scConfig.ParentRepository, devopsDir)
	cmd.Env = append(os.Environ(), "GIT_SSH_COMMAND=ssh -o StrictHostKeyChecking=no")

	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to clone parent repository: %w (output: %s)", err, string(output))
	}

	e.logger.Info(ctx, "Successfully cloned parent repository")

	// Set up parent repository secrets with proper SC configuration
	secretsRevealed, err := e.setupParentRepositorySecrets(ctx, &scConfig, devopsDir)
	if err != nil {
		return fmt.Errorf("failed to setup parent repository secrets: %w", err)
	}

	// Copy .sc/stacks/* from parent repository to current workspace (including revealed secrets)
	parentStacksDir := filepath.Join(devopsDir, ".sc", "stacks")
	currentStacksDir := filepath.Join(".sc", "stacks")

	// Ensure current .sc/stacks directory exists
	if err := os.MkdirAll(currentStacksDir, 0o755); err != nil {
		return fmt.Errorf("failed to create .sc/stacks directory: %w", err)
	}

	// Copy stacks with awareness of secret revelation status
	if secretsRevealed {
		e.logger.Info(ctx, "📁 Copying parent stack configurations from %s (with revealed secrets)", parentStacksDir)
		if err := e.copyDirectory(parentStacksDir, currentStacksDir); err != nil {
			e.logger.Warn(ctx, "Failed to copy parent stack configurations: %v", err)
			return fmt.Errorf("failed to copy parent stack configurations: %w", err)
		}
		e.logger.Info(ctx, "✅ Successfully copied parent stack configurations with revealed secrets")
	} else {
		e.logger.Info(ctx, "📁 Copying parent stack configurations from %s (secrets may not be available)", parentStacksDir)
		if err := e.copyDirectory(parentStacksDir, currentStacksDir); err != nil {
			e.logger.Warn(ctx, "Failed to copy parent stack configurations: %v", err)
			return fmt.Errorf("failed to copy parent stack configurations: %w", err)
		}
		e.logger.Info(ctx, "✅ Successfully copied parent stack configurations (secrets not revealed)")
	}

	if secretsRevealed {
		e.logger.Info(ctx, "✅ Parent repository setup completed WITH secret revelation")
	} else {
		e.logger.Info(ctx, "✅ Parent repository setup completed (no secrets revealed - may be expected)")
	}

	return nil
}

// setupParentRepositorySecrets creates SC configuration in parent repository and reveals secrets there
// Returns true if secrets were successfully revealed, false otherwise
func (e *Executor) setupParentRepositorySecrets(ctx context.Context, scConfig *api.ConfigFile, devopsDir string) (bool, error) {
	e.logger.Info(ctx, "🔑 Setting up parent repository secrets...")

	// Save current working directory
	currentDir, err := os.Getwd()
	if err != nil {
		return false, fmt.Errorf("failed to get current directory: %w", err)
	}

	// Change to parent repository directory
	if err := os.Chdir(devopsDir); err != nil {
		return false, fmt.Errorf("failed to change to devops directory: %w", err)
	}

	// Ensure we return to original directory
	defer func() {
		if err := os.Chdir(currentDir); err != nil {
			e.logger.Error(ctx, "Failed to return to original directory: %v", err)
		}
	}()

	// Initialize parent repository configuration properly
	e.logger.Info(ctx, "🔧 Setting up parent repository configuration for secret revelation...")

	// Determine profile name from environment
	profile := os.Getenv("ENVIRONMENT")
	if profile == "" {
		profile = "default"
	}
	configFileName := fmt.Sprintf("cfg.%s.yaml", profile)
	configPath := fmt.Sprintf(".sc/%s", configFileName)

	// Check if profile-specific config exists, if not try to create it from template
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		if _, err := os.Stat(".sc/cfg.yaml.template"); err == nil {
			e.logger.Info(ctx, "📋 Found cfg.yaml.template, copying to %s", configFileName)

			// Copy template to profile-specific config
			templateContent, err := os.ReadFile(".sc/cfg.yaml.template")
			if err != nil {
				e.logger.Warn(ctx, "Failed to read cfg.yaml.template: %v", err)
			} else {
				err = os.WriteFile(configPath, templateContent, 0o644)
				if err != nil {
					e.logger.Warn(ctx, "Failed to create %s: %v", configFileName, err)
				} else {
					e.logger.Info(ctx, "✅ Created %s from template", configFileName)
				}
			}
		} else {
			e.logger.Info(ctx, "ℹ️  No cfg.yaml.template found in parent repository")
		}
	} else {
		e.logger.Info(ctx, "✅ %s already exists in parent repository", configFileName)
	}

	// Create cryptor using parent repository's configuration with Git context
	e.logger.Info(ctx, "🔧 Creating cryptor with Git context for parent repository...")

	// Initialize git repository context for the parent repository
	parentGitRepo, err := git.New(git.WithDetectRootDir())
	if err != nil {
		e.logger.Warn(ctx, "Failed to initialize Git context for parent repo: %v", err)
		// Try without Git context
		parentGitRepo = nil
	} else {
		e.logger.Info(ctx, "✅ Successfully initialized Git context for parent repository")
	}

	// Create cryptor with proper context using environment-specific profile
	parentCryptor, err := secrets.NewCryptor(
		".", // Parent repository root (current directory after chdir)
		secrets.WithProfile(profile),
		secrets.WithGitRepo(parentGitRepo),
	)
	if err != nil {
		e.logger.Warn(ctx, "Failed to create cryptor from parent repo config: %v", err)
		e.logger.Info(ctx, "🔍 Parent repository may not be properly configured for SC")
		return false, nil // No secrets revealed, but not an error
	}

	e.logger.Info(ctx, "✅ Created cryptor using parent repository configuration with Git context")

	// CRITICAL FIX: Configure cryptor exactly like 'sc secrets reveal' does
	e.logger.Info(ctx, "🔧 Configuring cryptor with profile config (SSH keys)...")
	if err := parentCryptor.ReadProfileConfig(); err != nil {
		e.logger.Warn(ctx, "Failed to read profile config: %v", err)
		e.logger.Info(ctx, "🔍 This is expected if parent repository has different configuration")
		return false, nil // No profile loaded, but not an error
	}
	e.logger.Info(ctx, "✅ Successfully loaded profile config (SSH keys configured)")

	e.logger.Info(ctx, "🔧 Loading secrets.yaml file into cryptor...")
	if err := parentCryptor.ReadSecretFiles(); err != nil {
		e.logger.Warn(ctx, "Failed to read secrets file: %v", err)
		e.logger.Info(ctx, "🔍 This is expected if parent repository has no secrets")
		return false, nil // No secrets loaded, but not an error
	}
	e.logger.Info(ctx, "✅ Successfully loaded secrets file into cryptor")

	// Reveal secrets in parent repository and verify success
	e.logger.Info(ctx, "🔍 Revealing secrets in parent repository...")
	secretsRevealed, err := e.revealAndVerifyParentSecrets(ctx, parentCryptor)
	if err != nil {
		return false, fmt.Errorf("failed to reveal parent repository secrets: %w", err)
	}

	return secretsRevealed, nil
}

// revealCurrentRepositorySecrets reveals secrets in the current repository for parent stack operations
func (e *Executor) revealCurrentRepositorySecrets(ctx context.Context, scConfig *api.ConfigFile) error {
	e.logger.Info(ctx, "🔑 Revealing secrets in current repository (parent stack operation)...")

	// Determine profile name from environment
	profile := os.Getenv("ENVIRONMENT")
	if profile == "" {
		profile = "default"
	}

	// SSH setup not needed - GitHub Actions already provides repository access
	e.logger.Info(ctx, "ℹ️  Using GitHub Actions repository access (no SSH setup needed)")

	// Initialize git repository context
	currentGitRepo, err := git.New(git.WithDetectRootDir())
	if err != nil {
		e.logger.Warn(ctx, "Failed to initialize Git context: %v", err)
		currentGitRepo = nil
	} else {
		e.logger.Info(ctx, "✅ Successfully initialized Git context")
	}

	// Create cryptor for current repository with keys from SC_CONFIG
	e.logger.Info(ctx, "🔧 Creating cryptor for current repository...")
	currentCryptor, err := secrets.NewCryptor(
		".", // Current repository root
		secrets.WithProfile(profile),
		secrets.WithGitRepo(currentGitRepo),
		secrets.WithPrivateKey(scConfig.PrivateKey),
		secrets.WithPublicKey(scConfig.PublicKey),
	)
	if err != nil {
		return fmt.Errorf("failed to create cryptor: %w", err)
	}

	e.logger.Info(ctx, "✅ Created cryptor for current repository")

	// Configure cryptor with profile config
	e.logger.Info(ctx, "🔧 Loading cryptor profile configuration...")
	if err := currentCryptor.ReadProfileConfig(); err != nil {
		e.logger.Warn(ctx, "Failed to read profile config: %v", err)
		// Continue anyway - cryptor already has keys from SC_CONFIG
	} else {
		e.logger.Info(ctx, "✅ Successfully loaded profile config")
	}

	// Load secrets.yaml file
	e.logger.Info(ctx, "🔧 Loading secrets.yaml file into cryptor...")
	if err := currentCryptor.ReadSecretFiles(); err != nil {
		e.logger.Warn(ctx, "Failed to read secrets file: %v", err)
		// Check if secrets.yaml exists
		if _, err := os.Stat(".sc/secrets.yaml"); os.IsNotExist(err) {
			e.logger.Info(ctx, "ℹ️  No .sc/secrets.yaml file found in current repository")
			e.logger.Info(ctx, "🔍 This may be expected if secrets are managed elsewhere")
			return nil // No secrets to reveal, but not an error
		}
		return fmt.Errorf("failed to load secrets: %w", err)
	}
	e.logger.Info(ctx, "✅ Successfully loaded secrets file")

	// Reveal secrets in current repository
	e.logger.Info(ctx, "🔍 Revealing secrets in current repository...")
	e.logger.Info(ctx, "🔧 Calling DecryptAll(true) - same as 'sc secrets reveal --force'")

	decryptErr := currentCryptor.DecryptAll(true) // forceReveal = true
	if decryptErr != nil {
		// Handle expected errors gracefully
		if strings.Contains(decryptErr.Error(), "public key is not configured") ||
			strings.Contains(decryptErr.Error(), "not found in secrets") {
			e.logger.Warn(ctx, "Secret decryption failed: %v", decryptErr)
			e.logger.Info(ctx, "🔍 Key mismatch detected - secrets encrypted with different keys than SC_CONFIG")
			return fmt.Errorf("secret decryption failed - key mismatch: %w", decryptErr)
		}
		return fmt.Errorf("unexpected decryption error: %w", decryptErr)
	}

	e.logger.Info(ctx, "✅ DecryptAll completed successfully - secrets revealed in current repository")
	e.logger.Info(ctx, "✅ Parent repository secret revelation completed successfully")
	return nil
}

// revealAndVerifyParentSecrets attempts to reveal secrets using the exact same approach as SC CLI
func (e *Executor) revealAndVerifyParentSecrets(ctx context.Context, parentCryptor secrets.Cryptor) (bool, error) {
	// Check if there are any encrypted secrets to reveal
	secretsFile := ".sc/secrets.yaml"
	if _, err := os.Stat(secretsFile); os.IsNotExist(err) {
		e.logger.Info(ctx, "ℹ️  No secrets.yaml file found in parent repository")
		return false, nil // No secrets to reveal, but not an error
	}

	e.logger.Info(ctx, "Found secrets.yaml in parent repository, attempting to reveal secrets...")
	e.logger.Info(ctx, "🔧 Calling DecryptAll(true) - same as 'sc secrets reveal --force'")

	// Use the same DecryptAll approach as the SC CLI
	decryptErr := parentCryptor.DecryptAll(true) // forceReveal = true

	if decryptErr != nil {
		// Handle expected errors gracefully
		if strings.Contains(decryptErr.Error(), "public key is not configured") ||
			strings.Contains(decryptErr.Error(), "not found in secrets") {
			e.logger.Warn(ctx, "Secret decryption failed: %v", decryptErr)
			e.logger.Info(ctx, "🔍 Key mismatch detected (expected in test environments)")
			e.logger.Info(ctx, "   - Parent repository secrets encrypted with different keys than SC_CONFIG")
			e.logger.Info(ctx, "   - In production, SC_CONFIG will contain matching keys")
			e.logger.Info(ctx, "ℹ️  No secrets found in parent repository or secrets don't match current keys")
			return false, nil // Expected in test environments
		}

		return false, fmt.Errorf("unexpected decryption error: %w", decryptErr)
	}

	// If DecryptAll succeeded, secrets were revealed successfully
	e.logger.Info(ctx, "✅ DecryptAll completed successfully - secrets revealed")
	return true, nil
}
