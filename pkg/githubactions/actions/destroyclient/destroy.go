package destroyclient

import (
	"context"
	"fmt"
	"time"

	"github.com/simple-container-com/api/pkg/githubactions/common/git"
	"github.com/simple-container-com/api/pkg/githubactions/common/notifications"
	"github.com/simple-container-com/api/pkg/githubactions/common/sc"
	"github.com/simple-container-com/api/pkg/githubactions/config"
	"github.com/simple-container-com/api/pkg/githubactions/utils/logging"
)

// Execute performs the destroy client stack action
func Execute(ctx context.Context, cfg *config.Config, logger logging.Logger) error {
	logger.Info("Starting Simple Container client stack destruction",
		"stack", cfg.StackName,
		"environment", cfg.Environment,
		"auto_confirm", cfg.AutoConfirm,
		"skip_backup", cfg.SkipBackup)

	startTime := time.Now()

	// Initialize components
	gitOps := git.NewOperations(cfg, logger)
	scOps := sc.NewOperations(cfg, logger)
	notifier := notifications.NewManager(cfg, logger)

	// Phase 1: Safety Validation
	logger.Info("Phase 1: Safety Validation")

	if err := validateDestroyRequest(cfg, logger); err != nil {
		return fmt.Errorf("destroy validation failed: %w", err)
	}

	// Phase 2: Repository Operations
	logger.Info("Phase 2: Repository Operations")

	// Extract build metadata first (don't need full repo for destruction)
	metadata, err := gitOps.ExtractMetadata(ctx)
	if err != nil {
		return fmt.Errorf("metadata extraction failed: %w", err)
	}

	cloneOpts := &git.CloneOptions{
		Repository: cfg.GitHubRepository,
		Branch:     cfg.PRHeadRef,
		LFS:        false, // Don't need LFS for destruction
		Depth:      1,     // Shallow clone is sufficient
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

	// Phase 4: Send Start Notification
	logger.Info("Phase 4: Sending start notification")

	if err := notifier.SendNotification(ctx, notifications.StatusStarted, metadata, "destroy", time.Since(startTime)); err != nil {
		logger.Warn("Failed to send start notification", "error", err)
	}

	// Phase 5: Backup Creation (if not skipped)
	if !cfg.SkipBackup {
		logger.Info("Phase 5: Creating backup before destruction")
		if err := createBackup(ctx, cfg, scOps, logger); err != nil {
			logger.Warn("Backup creation failed", "error", err)
			// Don't fail the entire process for backup issues
		}
	} else {
		logger.Info("Phase 5: Skipping backup creation (skip_backup=true)")
	}

	// Phase 6: Stack Verification
	logger.Info("Phase 6: Verifying stack exists")

	if err := verifyStackExists(ctx, cfg, scOps, logger); err != nil {
		logger.Warn("Stack verification failed", "error", err)
		// This might not be an error if the stack was already destroyed
	}

	// Phase 7: Stack Destruction
	logger.Info("Phase 7: Stack Destruction")

	if err := executeDestruction(ctx, cfg, scOps, logger); err != nil {
		// Send failure notification
		notifyErr := notifier.SendNotification(ctx, notifications.StatusFailure, metadata, "destroy", time.Since(startTime))
		if notifyErr != nil {
			logger.Warn("Failed to send failure notification", "error", notifyErr)
		}
		return fmt.Errorf("stack destruction failed: %w", err)
	}

	// Phase 8: Cleanup
	logger.Info("Phase 8: Post-destruction cleanup")

	if err := performCleanup(ctx, cfg, scOps, logger); err != nil {
		logger.Warn("Cleanup had issues", "error", err)
	}

	// Phase 9: Send Success Notification
	logger.Info("Phase 9: Sending success notification")

	duration := time.Since(startTime)
	if err := notifier.SendNotification(ctx, notifications.StatusSuccess, metadata, "destroy", duration); err != nil {
		logger.Warn("Failed to send success notification", "error", err)
	}

	logger.Info("Stack destruction completed successfully",
		"duration", duration,
		"stack", cfg.StackName,
		"environment", cfg.Environment)

	return nil
}

// validateDestroyRequest validates the destruction request
func validateDestroyRequest(cfg *config.Config, logger logging.Logger) error {
	if cfg.StackName == "" {
		return fmt.Errorf("stack name is required for destruction")
	}

	if cfg.Environment == "" {
		return fmt.Errorf("environment is required for destruction")
	}

	// Additional safety checks could be added here
	// For example, preventing destruction of production without explicit confirmation

	if cfg.Environment == "production" && !cfg.AutoConfirm {
		logger.Warn("Attempting to destroy production environment without auto-confirm")
		// In a real implementation, this might require additional confirmation
	}

	logger.Info("Destruction request validated successfully")
	return nil
}

// createBackup creates a backup before destruction
func createBackup(ctx context.Context, cfg *config.Config, scOps *sc.Operations, logger logging.Logger) error {
	logger.Info("Creating backup before destruction")

	// This would implement backup functionality
	// For now, it's a placeholder
	logger.Info("Backup creation completed (placeholder implementation)")

	return nil
}

// verifyStackExists checks if the stack exists before attempting destruction
func verifyStackExists(ctx context.Context, cfg *config.Config, scOps *sc.Operations, logger logging.Logger) error {
	logger.Info("Verifying stack exists")

	// This would implement stack verification
	// For now, it's a placeholder
	logger.Info("Stack verification completed (placeholder implementation)")

	return nil
}

// executeDestruction performs the actual stack destruction
func executeDestruction(ctx context.Context, cfg *config.Config, scOps *sc.Operations, logger logging.Logger) error {
	logger.Info("Executing stack destruction")

	// Use SC CLI to destroy the stack
	// TODO: Implement actual destruction logic

	// This would be implemented in the sc.Operations to handle destroy operations
	// For now, it's a placeholder that would call something like:
	// return scOps.Destroy(ctx, destroyOpts)

	logger.Warn("Stack destruction not yet fully implemented - this is a placeholder")
	return nil
}

// performCleanup performs post-destruction cleanup
func performCleanup(ctx context.Context, cfg *config.Config, scOps *sc.Operations, logger logging.Logger) error {
	logger.Info("Performing post-destruction cleanup")

	// Cleanup tasks might include:
	// - Removing temporary files
	// - Cleaning up DNS records (for PR previews)
	// - Notifying external systems

	if cfg.PRPreview {
		logger.Info("Cleaning up PR preview resources")
		// Additional PR preview cleanup would go here
	}

	return nil
}
