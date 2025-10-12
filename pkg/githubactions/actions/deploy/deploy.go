package deploy

import (
	"context"
	"fmt"
	"time"

	"github.com/simple-container-com/api/pkg/githubactions/common/git"
	"github.com/simple-container-com/api/pkg/githubactions/common/notifications"
	"github.com/simple-container-com/api/pkg/githubactions/common/sc"
	"github.com/simple-container-com/api/pkg/githubactions/common/version"
	"github.com/simple-container-com/api/pkg/githubactions/config"
	"github.com/simple-container-com/api/pkg/githubactions/utils/logging"
)

// Execute performs the deploy client stack action
func Execute(ctx context.Context, cfg *config.Config, logger logging.Logger) error {
	logger.Info("Starting Simple Container client stack deployment",
		"stack", cfg.StackName,
		"environment", cfg.Environment,
		"repository", cfg.GitHubRepository,
		"pr_preview", cfg.PRPreview)

	startTime := time.Now()

	// Initialize components
	gitOps := git.NewOperations(cfg, logger)
	versionGen := version.NewGenerator(cfg, logger)
	scOps := sc.NewOperations(cfg, logger)
	notifier := notifications.NewManager(cfg, logger)

	// Phase 1: Setup and Preparation
	logger.Info("Phase 1: Setup and Preparation")

	// Generate deployment version
	deployVersion, err := versionGen.GenerateCalVer(ctx)
	if err != nil {
		return fmt.Errorf("version generation failed: %w", err)
	}
	logger.Info("Generated deployment version", "version", deployVersion)

	// Extract build metadata
	metadata, err := gitOps.ExtractMetadata(ctx)
	if err != nil {
		return fmt.Errorf("metadata extraction failed: %w", err)
	}
	logger.Info("Extracted build metadata",
		"branch", metadata.Branch,
		"author", metadata.Author,
		"commit", metadata.CommitSHA[:7])

	// Phase 2: Repository Operations
	logger.Info("Phase 2: Repository Operations")

	cloneOpts := &git.CloneOptions{
		Repository: cfg.GitHubRepository,
		Branch:     cfg.PRHeadRef, // Will be empty for non-PR deployments
		LFS:        true,
		Depth:      0, // Full clone for proper git operations
		WorkDir:    cfg.GitHubWorkspace,
	}

	if err := gitOps.CloneRepository(ctx, cloneOpts); err != nil {
		return fmt.Errorf("repository clone failed: %w", err)
	}

	// Phase 3: Simple Container Setup
	logger.Info("Phase 3: Simple Container Setup")

	if err := scOps.Setup(ctx); err != nil {
		return fmt.Errorf("Simple Container setup failed: %w", err)
	}

	// Phase 4: PR Preview Configuration (if applicable)
	if cfg.PRPreview {
		logger.Info("Phase 4: PR Preview Configuration", "pr_number", cfg.PRNumber)

		if cfg.PRNumber == "" {
			return fmt.Errorf("PR preview enabled but PR_NUMBER not available")
		}

		previewOpts := &sc.PRPreviewOptions{
			PRNumber:    cfg.PRNumber,
			DomainBase:  cfg.PreviewDomainBase,
			StackName:   cfg.StackName,
			Environment: cfg.Environment,
		}

		if err := scOps.ConfigurePRPreview(ctx, previewOpts); err != nil {
			return fmt.Errorf("PR preview configuration failed: %w", err)
		}
	}

	// Phase 5: Custom Configuration (if provided)
	if cfg.StackYAMLConfig != "" {
		logger.Info("Phase 5: Applying custom YAML configuration")

		configOpts := &sc.CustomConfigOptions{
			YAMLConfig: cfg.StackYAMLConfig,
			Encrypted:  cfg.StackYAMLConfigEncrypted,
			StackName:  cfg.StackName,
		}

		if err := scOps.ApplyCustomConfiguration(ctx, configOpts); err != nil {
			return fmt.Errorf("custom configuration failed: %w", err)
		}
	}

	// Phase 6: Send Start Notification
	logger.Info("Phase 6: Sending start notification")

	if err := notifier.SendNotification(ctx, notifications.StatusStarted, metadata, deployVersion, time.Since(startTime)); err != nil {
		logger.Warn("Failed to send start notification", "error", err)
	}

	// Phase 7: Stack Deployment
	logger.Info("Phase 7: Stack Deployment")

	deployOpts := &sc.DeployOptions{
		StackName:    cfg.StackName,
		Environment:  cfg.Environment,
		Version:      deployVersion,
		ImageVersion: cfg.AppImageVersion,
		Flags:        cfg.SCDeployFlags,
		WorkDir:      cfg.GitHubWorkspace,
	}

	if err := scOps.Deploy(ctx, deployOpts); err != nil {
		// Send failure notification
		notifyErr := notifier.SendNotification(ctx, notifications.StatusFailure, metadata, deployVersion, time.Since(startTime))
		if notifyErr != nil {
			logger.Warn("Failed to send failure notification", "error", notifyErr)
		}
		return fmt.Errorf("stack deployment failed: %w", err)
	}

	// Phase 8: Validation (if provided)
	if cfg.ValidationCommand != "" {
		logger.Info("Phase 8: Post-deployment validation")

		validationOpts := &sc.ValidationOptions{
			Command:     cfg.ValidationCommand,
			StackName:   cfg.StackName,
			Environment: cfg.Environment,
			Version:     deployVersion,
			WorkDir:     cfg.GitHubWorkspace,
		}

		if err := scOps.RunValidation(ctx, validationOpts); err != nil {
			// Send failure notification
			notifyErr := notifier.SendNotification(ctx, notifications.StatusFailure, metadata, deployVersion, time.Since(startTime))
			if notifyErr != nil {
				logger.Warn("Failed to send failure notification", "error", notifyErr)
			}
			return fmt.Errorf("validation failed: %w", err)
		}
	}

	// Phase 9: Finalization
	logger.Info("Phase 9: Finalization")

	finalizeOpts := &sc.FinalizeOptions{
		Version:     deployVersion,
		StackName:   cfg.StackName,
		Environment: cfg.Environment,
		CreateTag:   !cfg.PRPreview, // Only create tags for non-preview deployments
		WorkDir:     cfg.GitHubWorkspace,
	}

	if err := scOps.Finalize(ctx, finalizeOpts); err != nil {
		logger.Warn("Finalization had issues", "error", err)
	}

	// Phase 10: Send Success Notification
	logger.Info("Phase 10: Sending success notification")

	duration := time.Since(startTime)
	if err := notifier.SendNotification(ctx, notifications.StatusSuccess, metadata, deployVersion, duration); err != nil {
		logger.Warn("Failed to send success notification", "error", err)
	}

	// Set GitHub Action outputs
	if err := setGitHubOutputs(cfg, deployVersion, metadata, duration); err != nil {
		logger.Warn("Failed to set GitHub outputs", "error", err)
	}

	logger.Info("Deployment completed successfully",
		"duration", duration,
		"stack", cfg.StackName,
		"environment", cfg.Environment,
		"version", deployVersion)

	return nil
}

// setGitHubOutputs sets outputs for the GitHub Action
func setGitHubOutputs(cfg *config.Config, version string, metadata *git.Metadata, duration time.Duration) error {
	if cfg.GitHubOutput == "" {
		return nil // No output file configured
	}

	outputs := map[string]string{
		"version":     version,
		"environment": cfg.Environment,
		"stack-name":  cfg.StackName,
		"duration":    formatDuration(duration),
		"status":      "success",
		"build-url":   metadata.BuildURL,
		"commit-sha":  metadata.CommitSHA,
		"branch":      metadata.Branch,
	}

	// Add preview URL if this was a PR preview
	if cfg.PRPreview && cfg.PRNumber != "" {
		previewURL := fmt.Sprintf("https://pr%s-%s", cfg.PRNumber, cfg.PreviewDomainBase)
		outputs["preview-url"] = previewURL
	}

	return writeGitHubOutputs(cfg.GitHubOutput, outputs)
}

// writeGitHubOutputs writes outputs to the GitHub Actions output file
func writeGitHubOutputs(outputFile string, outputs map[string]string) error {
	// This would write to the GitHub Actions output file
	// For now, we'll just print the outputs (GitHub Actions will capture them)
	for key, value := range outputs {
		fmt.Printf("%s=%s\n", key, value)
	}
	return nil
}

// formatDuration formats a duration in a human-readable format
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}

	minutes := int(d.Minutes())
	seconds := int(d.Seconds()) % 60

	if minutes < 60 {
		return fmt.Sprintf("%dm%ds", minutes, seconds)
	}

	hours := minutes / 60
	minutes = minutes % 60

	return fmt.Sprintf("%dh%dm%ds", hours, minutes, seconds)
}
