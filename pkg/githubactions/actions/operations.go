package actions

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/simple-container-com/api/pkg/api"
)

// DeployClientStack deploys a client stack using SC's internal APIs
func (e *Executor) DeployClientStack(ctx context.Context) error {
	// Generate CalVer version if not provided or empty
	version := strings.TrimSpace(os.Getenv("VERSION"))
	generatedVersion := false
	if version == "" {
		var err error
		version, err = e.generateCalVerVersion(ctx)
		if err != nil {
			e.logger.Warn(ctx, "Failed to generate CalVer version: %v, using 'latest'", err)
			version = "latest"
		} else {
			generatedVersion = true
		}
	}

	// Create deployment parameters
	deployParams := api.DeployParams{
		StackParams: api.StackParams{
			StackName:    os.Getenv("STACK_NAME"),
			Environment:  os.Getenv("ENVIRONMENT"),
			DetailedDiff: true,                                // Enable detailed diff for better visibility in GitHub Actions
			SkipRefresh:  os.Getenv("SKIP_REFRESH") == "true", // Skip Pulumi refresh if requested
		},
	}

	// Wrap the deployment with signal handling and panic recovery
	err := e.signalHandler.WithSignalHandling(ctx, opTypeDeploy, deployParams, func(opCtx context.Context) error {
		return e.executeOperation(opCtx, OperationConfig{
			Type:      OperationDeploy,
			Scope:     ScopeClient,
			StackName: deployParams.StackName,
			Env:       deployParams.Environment,
			Version:   version,
		})
	})

	// Only tag the repository if deployment succeeded and we generated a version
	if err == nil && generatedVersion {
		if tagErr := e.tagRepository(ctx, version); tagErr != nil {
			e.logger.Warn(ctx, "Failed to tag repository with version %s: %v", version, tagErr)
		}
	}

	return err
}

// ProvisionParentStack provisions a parent stack using SC's internal APIs
func (e *Executor) ProvisionParentStack(ctx context.Context) error {
	stackName := os.Getenv("STACK_NAME")
	if stackName == "" {
		stackName = "infrastructure" // Default for parent stacks
	}

	// Create provision parameters
	provisionParams := api.ProvisionParams{
		Stacks:       []string{stackName},
		DetailedDiff: true, // Enable detailed diff for better visibility in GitHub Actions
	}

	// Wrap the provision with signal handling and panic recovery
	return e.signalHandler.WithSignalHandling(ctx, opTypeProvision, provisionParams, func(opCtx context.Context) error {
		return e.executeOperation(opCtx, OperationConfig{
			Type:      OperationProvision,
			Scope:     ScopeParent,
			StackName: stackName,
		})
	})
}

// DestroyClientStack destroys a client stack using SC's internal APIs
func (e *Executor) DestroyClientStack(ctx context.Context) error {
	// Create destroy parameters
	destroyParams := api.DestroyParams{
		StackParams: api.StackParams{
			StackName:   os.Getenv("STACK_NAME"),
			Environment: os.Getenv("ENVIRONMENT"),
		},
	}

	// Wrap the destroy with signal handling and panic recovery
	return e.signalHandler.WithSignalHandling(ctx, opTypeDestroy, destroyParams, func(opCtx context.Context) error {
		return e.executeOperation(opCtx, OperationConfig{
			Type:      OperationDestroy,
			Scope:     ScopeClient,
			StackName: destroyParams.StackName,
			Env:       destroyParams.Environment,
		})
	})
}

// DestroyParentStack destroys a parent stack using SC's internal APIs
func (e *Executor) DestroyParentStack(ctx context.Context) error {
	stackName := os.Getenv("STACK_NAME")
	if stackName == "" {
		stackName = "infrastructure" // Default for parent stacks
	}

	// Create destroy parameters
	destroyParams := api.DestroyParams{
		StackParams: api.StackParams{
			StackName: stackName,
			Parent:    true, // Mark as parent operation for proper cancellation
		},
	}

	// Wrap the destroy with signal handling and panic recovery
	return e.signalHandler.WithSignalHandling(ctx, opTypeDestroy, destroyParams, func(opCtx context.Context) error {
		return e.executeOperation(opCtx, OperationConfig{
			Type:      OperationDestroy,
			Scope:     ScopeParent,
			StackName: stackName,
		})
	})
}

// CancelStack cancels a running stack operation using SC's internal APIs
func (e *Executor) CancelStack(ctx context.Context) error {
	stackType := os.Getenv("STACK_TYPE")
	stackName := os.Getenv("STACK_NAME")
	environment := os.Getenv("ENVIRONMENT")
	operationID := os.Getenv("OPERATION_ID")
	forceCancel := os.Getenv("FORCE_CANCEL") == "true"

	e.logger.Info(ctx, "🛑 Starting stack cancellation: type=%s, stack=%s, env=%s", stackType, stackName, environment)

	if operationID != "" {
		e.logger.Info(ctx, "🎯 Targeting specific operation: %s", operationID)
	} else {
		e.logger.Info(ctx, "🔄 Will cancel all active operations for stack")
	}

	// Validate stack type
	if stackType != "client" && stackType != "parent" {
		return fmt.Errorf("invalid stack type: %s (must be 'client' or 'parent')", stackType)
	}

	// Initialize notifications for cancellation alerts
	e.logger.Info(ctx, "📢 Initializing notifications for cancellation alerts...")
	if err := e.setupNotificationsForCancellation(ctx, stackName, environment, stackType == "client"); err != nil {
		e.logger.Warn(ctx, "Failed to setup notifications: %v", err)
	}

	// Send cancellation start notification
	e.sendCancellationStartAlert(ctx, stackType, stackName, environment, operationID, forceCancel)

	// Execute cancellation with timeout
	cleanupTimeoutStr := os.Getenv("CLEANUP_TIMEOUT")
	cleanupTimeout, err := time.ParseDuration(cleanupTimeoutStr + "s")
	if err != nil {
		cleanupTimeout = 5 * time.Minute // Default 5 minutes
	}

	e.logger.Info(ctx, "⏱️ Cleanup timeout set to: %v", cleanupTimeout)

	// Create cancellation context with timeout
	cancelCtx, cancelFunc := context.WithTimeout(ctx, cleanupTimeout)
	defer cancelFunc()

	// Perform the actual cancellation
	startTime := time.Now()

	if forceCancel {
		e.logger.Warn(ctx, "⚠️ Force cancellation enabled - will terminate operations aggressively")
	}

	// Call the appropriate cancellation method based on stack type
	var cancelErr error
	switch stackType {
	case "parent":
		e.logger.Info(ctx, "📋 Cancelling parent stack operation")
		cancelErr = e.provisioner.CancelParent(cancelCtx, api.StackParams{
			StackName: stackName,
			Parent:    true,
		})
	case "client":
		e.logger.Info(ctx, "📋 Cancelling client stack operation")
		cancelErr = e.provisioner.Cancel(cancelCtx, api.StackParams{
			StackName:   stackName,
			Environment: environment,
		})
	}

	duration := time.Since(startTime)

	if cancelErr != nil {
		// Send failure notification
		e.sendCancellationFailureAlert(ctx, stackType, stackName, environment, cancelErr, duration)

		if errors.Is(cancelErr, context.DeadlineExceeded) {
			e.logger.Error(ctx, "⏰ Cancellation timed out after %v", duration)
			return fmt.Errorf("stack cancellation timed out after %v: %w", duration, cancelErr)
		}
		e.logger.Error(ctx, "❌ Cancellation failed after %v: %v", duration, cancelErr)
		return fmt.Errorf("stack cancellation failed: %w", cancelErr)
	}

	// Send success notification
	e.sendCancellationSuccessAlert(ctx, stackType, stackName, environment, duration)

	e.logger.Info(ctx, "✅ Stack cancellation completed successfully in %v", duration)

	// Set outputs for GitHub Actions
	if err := e.setActionOutput("duration", duration.String()); err != nil {
		e.logger.Warn(ctx, "Failed to set duration output: %v", err)
	}

	if err := e.setActionOutput("cleanup-status", "completed"); err != nil {
		e.logger.Warn(ctx, "Failed to set cleanup-status output: %v", err)
	}

	return nil
}
