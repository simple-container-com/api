package git

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/simple-container-com/api/pkg/githubactions/config"
	"github.com/simple-container-com/api/pkg/githubactions/utils/logging"
)

// Operations handles Git operations for GitHub Actions
type Operations struct {
	cfg    *config.Config
	logger logging.Logger
}

// CloneOptions specifies options for repository cloning
type CloneOptions struct {
	Repository string
	Branch     string
	LFS        bool
	Depth      int
	WorkDir    string
}

// Metadata contains Git metadata extracted from the repository
type Metadata struct {
	Branch    string
	Author    string
	CommitSHA string
	Message   string
	BuildURL  string
}

// NewOperations creates a new Git operations instance
func NewOperations(cfg *config.Config, logger logging.Logger) *Operations {
	return &Operations{
		cfg:    cfg,
		logger: logger,
	}
}

// CloneRepository clones a repository with the specified options
func (g *Operations) CloneRepository(ctx context.Context, opts *CloneOptions) error {
	g.logger.Info("Cloning repository",
		"repo", opts.Repository,
		"branch", opts.Branch,
		"workdir", opts.WorkDir)

	// Ensure work directory exists
	if err := os.MkdirAll(opts.WorkDir, 0o755); err != nil {
		return fmt.Errorf("failed to create work directory: %w", err)
	}

	// Build git clone command
	args := []string{"clone"}

	if opts.Depth > 0 {
		args = append(args, "--depth", fmt.Sprintf("%d", opts.Depth))
	} else {
		// For GitHub Actions, we often need the full history for proper operations
		args = append(args, "--depth", "0")
	}

	// Use HTTPS with token authentication
	repoURL := fmt.Sprintf("https://x-access-token:%s@github.com/%s.git", g.cfg.GitHubToken, opts.Repository)
	args = append(args, repoURL, ".")

	// Execute git clone
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = opts.WorkDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git clone failed: %w", err)
	}

	// Configure Git user (required for some operations)
	if err := g.configureGitUser(ctx, opts.WorkDir); err != nil {
		g.logger.Warn("Failed to configure git user", "error", err)
	}

	// Switch to specific branch if needed (PR context)
	if opts.Branch != "" && opts.Branch != g.cfg.GitHubRefName {
		if err := g.checkoutBranch(ctx, opts.WorkDir, opts.Branch); err != nil {
			return fmt.Errorf("branch checkout failed: %w", err)
		}
	}

	// Pull LFS files if needed
	if opts.LFS {
		if err := g.pullLFS(ctx, opts.WorkDir); err != nil {
			g.logger.Warn("LFS pull failed", "error", err)
		}
	}

	g.logger.Info("Repository cloned successfully")
	return nil
}

// configureGitUser sets up git user configuration for commits
func (g *Operations) configureGitUser(ctx context.Context, workDir string) error {
	// Set up git user for any operations that might need it
	userEmail := fmt.Sprintf("%s@users.noreply.github.com", g.cfg.GitHubActor)
	userName := g.cfg.GitHubActor

	// Set user email
	cmd := exec.CommandContext(ctx, "git", "config", "user.email", userEmail)
	cmd.Dir = workDir
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to set git user email: %w", err)
	}

	// Set user name
	cmd = exec.CommandContext(ctx, "git", "config", "user.name", userName)
	cmd.Dir = workDir
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to set git user name: %w", err)
	}

	return nil
}

// checkoutBranch switches to a specific branch
func (g *Operations) checkoutBranch(ctx context.Context, workDir, branch string) error {
	g.logger.Info("Checking out branch", "branch", branch)

	// Fetch the branch
	cmd := exec.CommandContext(ctx, "git", "fetch", "origin", fmt.Sprintf("%s:%s", branch, branch))
	cmd.Dir = workDir
	if err := cmd.Run(); err != nil {
		g.logger.Debug("Branch fetch failed, trying direct checkout", "error", err)
	}

	// Checkout the branch
	cmd = exec.CommandContext(ctx, "git", "checkout", branch)
	cmd.Dir = workDir
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to checkout branch %s: %w", branch, err)
	}

	return nil
}

// pullLFS pulls Git LFS files
func (g *Operations) pullLFS(ctx context.Context, workDir string) error {
	g.logger.Info("Pulling Git LFS files")

	// Check if LFS is available
	if err := exec.CommandContext(ctx, "git", "lfs", "version").Run(); err != nil {
		return fmt.Errorf("git lfs not available: %w", err)
	}

	// Pull LFS files
	cmd := exec.CommandContext(ctx, "git", "lfs", "pull")
	cmd.Dir = workDir
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git lfs pull failed: %w", err)
	}

	return nil
}

// ExtractMetadata extracts Git metadata from the current context
func (g *Operations) ExtractMetadata(ctx context.Context) (*Metadata, error) {
	g.logger.Info("Extracting Git metadata")

	// Get commit message if available
	message := g.cfg.CommitMessage
	if message == "" {
		message = "GitHub Actions deployment"
	}

	// Clean up message (remove newlines)
	message = strings.ReplaceAll(message, "\n", " ")
	message = strings.TrimSpace(message)

	// Build metadata
	metadata := &Metadata{
		Branch:    g.cfg.GitHubRefName,
		Author:    g.cfg.GitHubActor,
		CommitSHA: g.cfg.GitHubSHA,
		Message:   message,
		BuildURL:  fmt.Sprintf("%s/%s/actions/runs/%s", g.cfg.GitHubServerURL, g.cfg.GitHubRepository, g.cfg.GitHubRunID),
	}

	g.logger.Info("Git metadata extracted",
		"branch", metadata.Branch,
		"author", metadata.Author,
		"commit", metadata.CommitSHA[:7])

	return metadata, nil
}

// CreateTag creates a git tag for the deployment
func (g *Operations) CreateTag(ctx context.Context, workDir, tagName, message string) error {
	g.logger.Info("Creating git tag", "tag", tagName)

	// Create the tag
	cmd := exec.CommandContext(ctx, "git", "tag", "-a", tagName, "-m", message)
	cmd.Dir = workDir
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create tag %s: %w", tagName, err)
	}

	// Push the tag
	cmd = exec.CommandContext(ctx, "git", "push", "origin", tagName)
	cmd.Dir = workDir
	if err := cmd.Run(); err != nil {
		g.logger.Warn("Failed to push tag", "tag", tagName, "error", err)
		// Don't fail the entire process for tag push failures
	}

	g.logger.Info("Git tag created successfully", "tag", tagName)
	return nil
}
