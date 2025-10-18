package actions

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// setupSSHForGit sets up SSH keys for git operations
func (e *Executor) setupSSHForGit(ctx context.Context, privateKey string) error {
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

	// Ensure cleanup happens even if function returns early
	var keyFileCreated bool
	defer func() {
		if keyFileCreated {
			os.Remove(keyPath)
		}
	}()

	// Write the private key
	if err := os.WriteFile(keyPath, []byte(privateKey), 0o600); err != nil {
		return fmt.Errorf("failed to write SSH private key: %w", err)
	}
	keyFileCreated = true

	// Configure SSH to use this key for GitHub
	sshConfigContent := fmt.Sprintf(`
Host github.com
	HostName github.com
	User git
	IdentityFile %s
	StrictHostKeyChecking no
`, keyPath)

	if err := os.WriteFile(sshConfigPath, []byte(sshConfigContent), 0o644); err != nil {
		return fmt.Errorf("failed to write SSH config: %w", err)
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
