package actions

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/simple-container-com/api/pkg/api"
)

// DeployClientStack deploys a client stack using SC's internal APIs
func (e *Executor) DeployClientStack(ctx context.Context) error {
	e.logger.Info(ctx, "🚀 Starting client stack deployment using SC internal APIs")
	startTime := time.Now()

	// Extract parameters from environment variables
	stackName := os.Getenv("STACK_NAME")
	environment := os.Getenv("ENVIRONMENT")
	version := os.Getenv("VERSION")

	e.logger.Info(ctx, "Deploying stack: %s, environment: %s, version: %s", stackName, environment, version)

	// Send start notification
	e.sendAlert(ctx, api.BuildStarted, "Deploy Started", fmt.Sprintf("Started deployment of %s to %s", stackName, environment), stackName, environment)

	// Setup parent repository (includes secret revelation)
	if err := e.cloneParentRepository(ctx); err != nil {
		e.sendAlert(ctx, api.BuildFailed, "Deploy Failed", fmt.Sprintf("Failed to setup parent repository for %s: %v", stackName, err), stackName, environment)
		return fmt.Errorf("parent repository setup failed: %w", err)
	}

	// Ensure SC configuration file exists (MUST happen before revealing secrets)
	if err := e.createSCConfigFromEnv(ctx); err != nil {
		e.sendAlert(ctx, api.BuildFailed, "Deploy Failed", fmt.Sprintf("Failed to create SC configuration for %s: %v", stackName, err), stackName, environment)
		return fmt.Errorf("SC configuration creation failed: %w", err)
	}

	// Try to reveal client secrets, but don't fail if none exist (parent secrets already available)
	e.logger.Info(ctx, "📋 Revealing client repository secrets...")
	if err := e.provisioner.Cryptor().DecryptAll(false); err != nil {
		// For client operations, missing secrets is OK if parent secrets are available
		if strings.Contains(err.Error(), "not found in secrets") || strings.Contains(err.Error(), "public key is not configured") {
			e.logger.Info(ctx, "ℹ️  No client secrets found - using parent repository secrets for deployment")
		} else {
			e.sendAlert(ctx, api.BuildFailed, "Deploy Failed", fmt.Sprintf("Failed to decrypt secrets for %s: %v", stackName, err), stackName, environment)
			return fmt.Errorf("secret decryption failed: %w", err)
		}
	} else {
		e.logger.Info(ctx, "✅ Client secrets revealed successfully")
	}

	// Initialize notifications after secrets are revealed (allows reading from parent stack secrets.yaml)
	e.initializeNotifications(ctx)

	// Deploy using SC's provisioner API
	deployParams := api.DeployParams{
		StackParams: api.StackParams{
			StackName:   stackName,
			Environment: environment,
			Version:     version,
		},
	}

	// Execute deployment
	previewMode := e.isPreviewMode()
	if previewMode {
		e.logger.Info(ctx, "🔍 Executing deployment in PREVIEW MODE (no real changes will be made)...")
	}

	e.logger.Info(ctx, "Simple Container CLI version: %s", "latest")
	e.logger.Info(ctx, "Deploy version: %s", version)

	if err := e.provisioner.Deploy(ctx, deployParams); err != nil {
		duration := time.Since(startTime)
		e.sendAlert(ctx, api.BuildFailed, "Deploy Failed", fmt.Sprintf("Deployment of %s failed after %v: %v", stackName, duration, err), stackName, environment)
		return fmt.Errorf("deployment preview failed: %w", err)
	}

	duration := time.Since(startTime)
	e.logger.Info(ctx, "✅ Client stack deployment completed successfully")
	e.sendAlert(ctx, api.BuildSucceeded, "Deploy Succeeded", fmt.Sprintf("Successfully deployed %s to %s in %v", stackName, environment, duration), stackName, environment)

	// Set GitHub Action outputs
	e.setGitHubOutputs(map[string]string{
		"stack-name":   stackName,
		"environment":  environment,
		"version":      version,
		"duration":     duration.String(),
		"preview-mode": fmt.Sprintf("%v", previewMode),
	})

	return nil
}

// ProvisionParentStack provisions a parent stack using SC's internal APIs
func (e *Executor) ProvisionParentStack(ctx context.Context) error {
	e.logger.Info(ctx, "🏗️ Starting parent stack provisioning using SC internal APIs")
	startTime := time.Now()

	// Extract parameters from environment variables
	stackName := os.Getenv("STACK_NAME")
	if stackName == "" {
		stackName = "infrastructure" // Default for parent stacks
	}

	e.logger.Debug(ctx, "🔧 Provisioning parameters:")
	e.logger.Debug(ctx, "  - Stack Name: %s", stackName)
	e.logger.Debug(ctx, "  - Environment: %s", os.Getenv("ENVIRONMENT"))
	e.logger.Debug(ctx, "  - DRY_RUN: %s", os.Getenv("DRY_RUN"))
	e.logger.Debug(ctx, "  - Working Directory: %s", func() string { wd, _ := os.Getwd(); return wd }())

	e.logger.Info(ctx, "Provisioning parent stack: %s", stackName)

	// Send start notification
	e.sendAlert(ctx, api.BuildStarted, "Provision Started", fmt.Sprintf("Started provisioning of parent stack %s", stackName), stackName, "infrastructure")

	// Setup parent repository (includes secret revelation) - CRITICAL for parent operations
	if err := e.cloneParentRepository(ctx); err != nil {
		e.sendAlert(ctx, api.BuildFailed, "Provision Failed", fmt.Sprintf("Failed to setup parent repository for %s: %v", stackName, err), stackName, "infrastructure")
		return fmt.Errorf("parent repository setup failed: %w", err)
	}

	// Ensure SC configuration file exists (MUST happen before revealing secrets)
	if err := e.createSCConfigFromEnv(ctx); err != nil {
		e.sendAlert(ctx, api.BuildFailed, "Provision Failed", fmt.Sprintf("Failed to create SC configuration for %s: %v", stackName, err), stackName, "infrastructure")
		return fmt.Errorf("SC configuration creation failed: %w", err)
	}

	// Parent repository secrets should now be available after cloning and revelation
	e.logger.Info(ctx, "📋 Using parent repository secrets (revealed during repository setup)")
	e.logger.Info(ctx, "✅ Parent repository secrets available for provisioning")

	// Initialize notifications after secrets are revealed
	e.initializeNotifications(ctx)

	// Provision using SC's provisioner API
	provisionParams := api.ProvisionParams{
		StacksDir: ".sc/stacks",
		Profile:   os.Getenv("ENVIRONMENT"),
		Stacks:    []string{stackName},
	}

	e.logger.Debug(ctx, "🔧 Provision parameters:")
	e.logger.Debug(ctx, "  - StacksDir: %s", provisionParams.StacksDir)
	e.logger.Debug(ctx, "  - Profile: %s", provisionParams.Profile)
	e.logger.Debug(ctx, "  - Stacks: %v", provisionParams.Stacks)

	// Execute provisioning
	previewMode := e.isPreviewMode()
	if previewMode {
		e.logger.Info(ctx, "🔍 Executing provisioning in PREVIEW MODE (no real changes will be made)...")
		e.logger.Debug(ctx, "Preview mode detected from environment variables")
	}

	e.logger.Info(ctx, "Simple Container CLI version: %s", "latest")
	e.logger.Debug(ctx, "🚀 Calling provisioner.Provision() with parameters...")

	if err := e.provisioner.Provision(ctx, provisionParams); err != nil {
		duration := time.Since(startTime)
		e.sendAlert(ctx, api.BuildFailed, "Provision Failed", fmt.Sprintf("Provisioning of %s failed after %v: %v", stackName, duration, err), stackName, "infrastructure")
		return fmt.Errorf("provisioning failed: %w", err)
	}

	duration := time.Since(startTime)
	e.logger.Info(ctx, "✅ Parent stack provisioning completed successfully")
	e.sendAlert(ctx, api.BuildSucceeded, "Provision Succeeded", fmt.Sprintf("Successfully provisioned parent stack %s in %v", stackName, duration), stackName, "infrastructure")

	// Set GitHub Action outputs
	e.setGitHubOutputs(map[string]string{
		"stack-name":   stackName,
		"duration":     duration.String(),
		"preview-mode": fmt.Sprintf("%v", previewMode),
	})

	return nil
}

// DestroyClientStack destroys a client stack using SC's internal APIs
func (e *Executor) DestroyClientStack(ctx context.Context) error {
	e.logger.Info(ctx, "🗑️ Starting client stack destruction using SC internal APIs")
	startTime := time.Now()

	// Extract parameters from environment variables
	stackName := os.Getenv("STACK_NAME")
	environment := os.Getenv("ENVIRONMENT")

	e.logger.Info(ctx, "Destroying stack: %s, environment: %s", stackName, environment)

	// Send start notification
	e.sendAlert(ctx, api.BuildStarted, "Destroy Started", fmt.Sprintf("Started destruction of %s in %s", stackName, environment), stackName, environment)

	// Setup parent repository (includes secret revelation)
	if err := e.cloneParentRepository(ctx); err != nil {
		e.sendAlert(ctx, api.BuildFailed, "Destroy Failed", fmt.Sprintf("Failed to setup parent repository for %s: %v", stackName, err), stackName, environment)
		return fmt.Errorf("parent repository setup failed: %w", err)
	}

	// Ensure SC configuration file exists (MUST happen before revealing secrets)
	if err := e.createSCConfigFromEnv(ctx); err != nil {
		e.sendAlert(ctx, api.BuildFailed, "Destroy Failed", fmt.Sprintf("Failed to create SC configuration for %s: %v", stackName, err), stackName, environment)
		return fmt.Errorf("SC configuration creation failed: %w", err)
	}

	// Try to reveal client secrets, but don't fail if none exist (parent secrets already available)
	e.logger.Info(ctx, "📋 Revealing client repository secrets...")
	if err := e.provisioner.Cryptor().DecryptAll(false); err != nil {
		// For client operations, missing secrets is OK if parent secrets are available
		if strings.Contains(err.Error(), "not found in secrets") || strings.Contains(err.Error(), "public key is not configured") {
			e.logger.Info(ctx, "ℹ️  No client secrets found - using parent repository secrets for destruction")
		} else {
			e.sendAlert(ctx, api.BuildFailed, "Destroy Failed", fmt.Sprintf("Failed to decrypt secrets for %s: %v", stackName, err), stackName, environment)
			return fmt.Errorf("secret decryption failed: %w", err)
		}
	} else {
		e.logger.Info(ctx, "✅ Client secrets revealed successfully")
	}

	// Initialize notifications after secrets are revealed (allows reading from parent stack secrets.yaml)
	e.initializeNotifications(ctx)

	// Destroy using SC's provisioner API
	destroyParams := api.DestroyParams{
		StackParams: api.StackParams{
			StackName:   stackName,
			Environment: environment,
		},
	}

	// Execute destruction
	previewMode := e.isPreviewMode()
	if previewMode {
		e.logger.Info(ctx, "🔍 Executing destruction in PREVIEW MODE (no real changes will be made)...")
	}

	e.logger.Info(ctx, "Simple Container CLI version: %s", "latest")

	if err := e.provisioner.Destroy(ctx, destroyParams, previewMode); err != nil {
		duration := time.Since(startTime)
		e.sendAlert(ctx, api.BuildFailed, "Destroy Failed", fmt.Sprintf("Destruction of %s failed after %v: %v", stackName, duration, err), stackName, environment)
		return fmt.Errorf("destruction failed: %w", err)
	}

	duration := time.Since(startTime)
	e.logger.Info(ctx, "✅ Client stack destruction completed successfully")
	e.sendAlert(ctx, api.BuildSucceeded, "Destroy Succeeded", fmt.Sprintf("Successfully destroyed %s in %s in %v", stackName, environment, duration), stackName, environment)

	// Set GitHub Action outputs
	e.setGitHubOutputs(map[string]string{
		"stack-name":   stackName,
		"environment":  environment,
		"duration":     duration.String(),
		"preview-mode": fmt.Sprintf("%v", previewMode),
	})

	return nil
}

// DestroyParentStack destroys a parent stack using SC's internal APIs
func (e *Executor) DestroyParentStack(ctx context.Context) error {
	e.logger.Info(ctx, "💥 Starting parent stack destruction using SC internal APIs")
	startTime := time.Now()

	// Extract parameters from environment variables
	stackName := os.Getenv("STACK_NAME")
	if stackName == "" {
		stackName = "infrastructure" // Default for parent stacks
	}

	e.logger.Info(ctx, "Destroying parent stack: %s", stackName)

	// Send start notification
	e.sendAlert(ctx, api.BuildStarted, "Destroy Parent Started", fmt.Sprintf("Started destruction of parent stack %s", stackName), stackName, "infrastructure")

	// Ensure SC configuration file exists (MUST happen before revealing secrets)
	if err := e.createSCConfigFromEnv(ctx); err != nil {
		e.sendAlert(ctx, api.BuildFailed, "Destroy Parent Failed", fmt.Sprintf("Failed to create SC configuration for %s: %v", stackName, err), stackName, "infrastructure")
		return fmt.Errorf("SC configuration creation failed: %w", err)
	}

	// Skip main cryptor DecryptAll for parent operations since parent secrets are already revealed
	e.logger.Info(ctx, "📋 Using parent repository secrets (already revealed during setup)")
	e.logger.Info(ctx, "✅ Parent repository secrets available for destruction")

	// Initialize notifications after secrets are revealed
	e.initializeNotifications(ctx)

	// Destroy parent using SC's provisioner API
	destroyParams := api.DestroyParams{
		StackParams: api.StackParams{
			StackName: stackName,
		},
	}

	// Execute parent destruction
	previewMode := e.isPreviewMode()
	if previewMode {
		e.logger.Info(ctx, "🔍 Executing parent stack destruction in PREVIEW MODE (no real changes will be made)...")
	}

	if err := e.provisioner.DestroyParent(ctx, destroyParams, previewMode); err != nil {
		duration := time.Since(startTime)
		e.sendAlert(ctx, api.BuildFailed, "Destroy Parent Failed", fmt.Sprintf("Parent stack destruction of %s failed after %v: %v", stackName, duration, err), stackName, "infrastructure")
		return fmt.Errorf("parent stack destruction failed: %w", err)
	}

	duration := time.Since(startTime)
	e.logger.Info(ctx, "✅ Parent stack destruction completed successfully")
	e.sendAlert(ctx, api.BuildSucceeded, "Destroy Parent Succeeded", fmt.Sprintf("Successfully destroyed parent stack %s in %v", stackName, duration), stackName, "infrastructure")

	// Set GitHub Action outputs
	e.setGitHubOutputs(map[string]string{
		"stack-name":   stackName,
		"duration":     duration.String(),
		"preview-mode": fmt.Sprintf("%v", previewMode),
	})

	return nil
}
