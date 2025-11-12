package actions

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// setupSSHForGit sets up SSH keys for git operations
func (e *Executor) setupSSHForGit(ctx context.Context, privateKey string) error {
	// Validate private key format
	if privateKey == "" {
		return fmt.Errorf("private key is empty")
	}

	// Check if private key has proper format
	if !strings.Contains(privateKey, "BEGIN") || !strings.Contains(privateKey, "PRIVATE KEY") {
		return fmt.Errorf("private key does not appear to be in valid format (missing BEGIN/PRIVATE KEY markers)")
	}

	e.logger.Info(ctx, "🔑 Setting up SSH key authentication...")
	e.logger.Debug(ctx, "Private key length: %d characters", len(privateKey))

	// Get user home directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get user home directory: %w", err)
	}

	sshDir := filepath.Join(homeDir, ".ssh")
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		return fmt.Errorf("failed to create .ssh directory: %w", err)
	}

	// Generate unique key file name to avoid conflicts
	keyPath := filepath.Join(sshDir, fmt.Sprintf("github_actions_key_%d", time.Now().Unix()))
	sshConfigPath := filepath.Join(sshDir, "config")

	e.logger.Debug(ctx, "SSH key path: %s", keyPath)
	e.logger.Debug(ctx, "SSH config path: %s", sshConfigPath)

	// Note: SSH key file cleanup is disabled to allow git operations to use the key
	// The key will be cleaned up when the container terminates

	// Ensure private key ends with newline (required for SSH keys)
	keyContent := strings.TrimSpace(privateKey)
	if !strings.HasSuffix(keyContent, "\n") {
		keyContent += "\n"
	}

	// Write the private key
	if err := os.WriteFile(keyPath, []byte(keyContent), 0o600); err != nil {
		return fmt.Errorf("failed to write SSH private key: %w", err)
	}

	e.logger.Info(ctx, "✅ SSH private key written successfully")

	// Configure SSH to use this key for GitHub
	sshConfigContent := fmt.Sprintf(`Host github.com
	HostName github.com
	User git
	IdentityFile %s
	StrictHostKeyChecking no
	UserKnownHostsFile /dev/null
	LogLevel ERROR
	IdentitiesOnly yes
`, keyPath)

	if err := os.WriteFile(sshConfigPath, []byte(sshConfigContent), 0o644); err != nil {
		return fmt.Errorf("failed to write SSH config: %w", err)
	}

	e.logger.Info(ctx, "✅ SSH config written successfully")

	// Test SSH key by attempting to connect to GitHub (this will fail but show if key is recognized)
	e.logger.Info(ctx, "🧪 Testing SSH key authentication...")
	testCmd := exec.Command("ssh", "-T", "-F", sshConfigPath, "git@github.com")
	testCmd.Env = os.Environ()
	if output, err := testCmd.CombinedOutput(); err != nil {
		e.logger.Debug(ctx, "SSH test output: %s", string(output))
		// This is expected to fail, but we can check the error message
		if strings.Contains(string(output), "successfully authenticated") {
			e.logger.Info(ctx, "✅ SSH key authentication successful")
		} else if strings.Contains(string(output), "Permission denied (publickey)") {
			e.logger.Warn(ctx, "❌ SSH key authentication failed - key may not have access to repository")
		} else if strings.Contains(string(output), "Host key verification failed") {
			e.logger.Warn(ctx, "⚠️ SSH host key verification issue (but this should be handled by git)")
		} else {
			e.logger.Debug(ctx, "SSH test result: %s", string(output))
		}
	}

	return nil
}

// sanitizeRepoURL removes sensitive information from repository URLs for logging
func (e *Executor) sanitizeRepoURL(repoURL string) string {
	// Remove any embedded credentials from the URL for logging
	if strings.Contains(repoURL, "@") {
		// For SSH URLs like git@github.com:org/repo.git, this is safe
		// For HTTPS URLs with credentials, mask them
		if strings.HasPrefix(repoURL, "https://") {
			return "***@github.com/***"
		}
		return strings.ReplaceAll(repoURL, repoURL[:strings.Index(repoURL, "@")], "***")
	}
	return repoURL
}

// copyDirectory recursively copies a directory
func (e *Executor) copyDirectory(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Calculate relative path from src
		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		dstPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		}

		return e.copyFile(path, dstPath)
	})
}

// copyFile copies a single file
func (e *Executor) copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	// Ensure the destination directory exists
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	return err
}

// setGitHubOutputs sets GitHub Action outputs
func (e *Executor) setGitHubOutputs(outputs map[string]string) {
	outputFile := os.Getenv("GITHUB_OUTPUT")
	if outputFile == "" {
		// Just print to stdout for GitHub Actions to capture
		for key, value := range outputs {
			fmt.Printf("%s=%s\n", key, value)
		}
		return
	}

	// Write to the GitHub output file
	file, err := os.OpenFile(outputFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		// Fallback to stdout
		for key, value := range outputs {
			fmt.Printf("%s=%s\n", key, value)
		}
		return
	}
	defer file.Close()

	for key, value := range outputs {
		if _, err := file.WriteString(fmt.Sprintf("%s=%s\n", key, value)); err != nil {
			// Fallback to stdout for this output
			fmt.Printf("%s=%s\n", key, value)
		}
	}
}

// generateCalVerVersion generates a CalVer version in format YYYY.MM.DD-{commit_hash}
func (e *Executor) generateCalVerVersion(ctx context.Context) (string, error) {
	e.logger.Info(ctx, "📅 Generating CalVer version...")

	// Get current date in YYYY.MM.DD format
	now := time.Now()
	dateVersion := now.Format("2006.01.02")

	// Check if we're in a git repository and get working directory info
	wd, err := os.Getwd()
	if err != nil {
		e.logger.Warn(ctx, "⚠️  Failed to get working directory: %v", err)
	} else {
		e.logger.Debug(ctx, "📁 Working directory: %s", wd)
	}

	// Check if .git directory exists
	gitDir := ".git"
	if wd != "" {
		gitDir = filepath.Join(wd, ".git")
	}
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		e.logger.Warn(ctx, "⚠️  No .git directory found at %s", gitDir)
		// Fallback to date-only version when not in a git repository
		fallbackVersion := fmt.Sprintf("%s-nogit", dateVersion)
		e.logger.Info(ctx, "📅 Using fallback CalVer version (no git): %s", fallbackVersion)
		return fallbackVersion, nil
	}

	// Try to get commit hash from GitHub Actions environment variables first
	githubSha := os.Getenv("GITHUB_SHA")
	if githubSha != "" && len(githubSha) >= 7 {
		commitHash := githubSha[:7] // Take first 7 characters
		e.logger.Debug(ctx, "🔗 Using commit hash from GITHUB_SHA: %s", commitHash)
		version := fmt.Sprintf("%s-%s", dateVersion, commitHash)
		e.logger.Info(ctx, "✅ Generated CalVer version (from GITHUB_SHA): %s", version)
		return version, nil
	}

	// Get current commit hash (short form) using git command
	e.logger.Debug(ctx, "🔍 Executing: git rev-parse --short=7 HEAD")
	cmd := exec.Command("git", "rev-parse", "--short=7", "HEAD")
	cmd.Dir = wd                        // Ensure we're running in the correct directory
	output, err := cmd.CombinedOutput() // Use CombinedOutput to capture stderr too
	if err != nil {
		e.logger.Error(ctx, "❌ Git command failed: %v, output: %s", err, string(output))
		// Fallback to date-only version when git command fails
		fallbackVersion := fmt.Sprintf("%s-gitfail", dateVersion)
		e.logger.Warn(ctx, "⚠️  Using fallback CalVer version (git command failed): %s", fallbackVersion)
		return fallbackVersion, nil
	}

	commitHash := strings.TrimSpace(string(output))
	e.logger.Debug(ctx, "🔗 Raw git output: '%s'", string(output))
	e.logger.Debug(ctx, "🔗 Trimmed commit hash: '%s'", commitHash)

	if commitHash == "" {
		e.logger.Error(ctx, "❌ Git commit hash is empty after trimming")
		// Fallback to date-only version when commit hash is empty
		fallbackVersion := fmt.Sprintf("%s-nohash", dateVersion)
		e.logger.Warn(ctx, "⚠️  Using fallback CalVer version (empty commit hash): %s", fallbackVersion)
		return fallbackVersion, nil
	}

	// Generate CalVer version: YYYY.MM.DD-{commit_hash}
	version := fmt.Sprintf("%s-%s", dateVersion, commitHash)

	e.logger.Info(ctx, "✅ Generated CalVer version: %s", version)
	return version, nil
}

// tagRepository tags the current repository with the given version and pushes the tag
func (e *Executor) tagRepository(ctx context.Context, version string) error {
	e.logger.Info(ctx, "🏷️  Tagging repository with version: %s", version)

	// Create the git tag
	tagCmd := exec.Command("git", "tag", version)
	if output, err := tagCmd.CombinedOutput(); err != nil {
		// Check if tag already exists
		if strings.Contains(string(output), "already exists") {
			e.logger.Info(ctx, "ℹ️  Tag %s already exists, skipping tag creation", version)
		} else {
			return fmt.Errorf("failed to create git tag %s: %w, output: %s", version, err, string(output))
		}
	} else {
		e.logger.Info(ctx, "✅ Created git tag: %s", version)
	}

	// Push the tag to remote repository
	pushCmd := exec.Command("git", "push", "origin", version)
	if output, err := pushCmd.CombinedOutput(); err != nil {
		// Check if tag already exists on remote
		if strings.Contains(string(output), "already exists") {
			e.logger.Info(ctx, "ℹ️  Tag %s already exists on remote, skipping push", version)
		} else {
			return fmt.Errorf("failed to push git tag %s: %w, output: %s", version, err, string(output))
		}
	} else {
		e.logger.Info(ctx, "✅ Pushed git tag to remote: %s", version)
	}

	// Set GitHub Action output for the version
	e.setGitHubOutputs(map[string]string{
		"version": version,
	})

	return nil
}
