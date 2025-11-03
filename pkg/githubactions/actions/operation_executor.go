package actions

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/simple-container-com/api/pkg/api"
)

// OperationType defines the type of operation
type OperationType string

const (
	OperationDeploy    OperationType = "deploy"
	OperationProvision OperationType = "provision"
	OperationDestroy   OperationType = "destroy"
)

// OperationScope defines whether this is a parent or client operation
type OperationScope string

const (
	ScopeParent OperationScope = "parent"
	ScopeClient OperationScope = "client"
)

// OperationConfig defines the configuration for a stack operation
type OperationConfig struct {
	Type      OperationType
	Scope     OperationScope
	StackName string
	Env       string
	Version   string
}

// capitalize returns the string with first letter capitalized (ASCII only)
func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// executeOperation is the unified operation executor that handles all stack operations
func (e *Executor) executeOperation(ctx context.Context, config OperationConfig) error {
	startTime := time.Now()

	// Phase 1: Setup and logging
	e.logOperationStart(ctx, config)

	// Phase 2: Send start notification
	e.sendStartAlert(ctx, config)

	// Phase 3: Repository and configuration setup
	if err := e.setupRepositoryAndConfig(ctx, config); err != nil {
		e.sendFailureAlert(ctx, config, err, time.Since(startTime))
		return err
	}

	// Phase 4: Secret revelation
	if err := e.revealSecrets(ctx, config); err != nil {
		e.sendFailureAlert(ctx, config, err, time.Since(startTime))
		return err
	}

	// Phase 5: Load stacks and initialize notifications
	if err := e.loadStacksForNotifications(ctx); err != nil {
		e.logger.Warn(ctx, "Failed to load stacks for notifications: %v", err)
	}
	e.initializeNotifications(ctx)

	// Phase 6: Execute the actual operation
	if err := e.performOperation(ctx, config); err != nil {
		e.sendFailureAlert(ctx, config, err, time.Since(startTime))
		return err
	}

	// Phase 7: Success notification and outputs
	duration := time.Since(startTime)
	e.sendSuccessAlert(ctx, config, duration)
	e.setOperationOutputs(config, duration)

	return nil
}

// logOperationStart logs the start of an operation
func (e *Executor) logOperationStart(ctx context.Context, config OperationConfig) {
	emoji := e.getOperationEmoji(config)
	action := e.getOperationAction(config)

	e.logger.Info(ctx, "%s Starting %s %s stack %s using SC internal APIs",
		emoji, config.Scope, action, config.Type)

	if config.Type == OperationDeploy {
		e.logger.Info(ctx, "Deploying stack: %s, environment: %s, version: %s",
			config.StackName, config.Env, config.Version)
	} else if config.Scope == ScopeParent {
		e.logger.Info(ctx, "%s %s stack: %s",
			capitalize(string(config.Type))+"ing", config.Scope, config.StackName)
	} else {
		e.logger.Info(ctx, "%s stack: %s, environment: %s",
			capitalize(string(config.Type))+"ing", config.StackName, config.Env)
	}
}

// setupRepositoryAndConfig handles repository cloning and SC config creation
func (e *Executor) setupRepositoryAndConfig(ctx context.Context, config OperationConfig) error {
	// For client operations, clone parent repository
	if config.Scope == ScopeClient {
		if err := e.cloneParentRepository(ctx); err != nil {
			return fmt.Errorf("parent repository setup failed: %w", err)
		}
	}

	// Create SC configuration file (required for all operations)
	if err := e.createSCConfigFromEnv(ctx); err != nil {
		return fmt.Errorf("SC configuration creation failed: %w", err)
	}

	return nil
}

// revealSecrets handles secret revelation based on operation scope
func (e *Executor) revealSecrets(ctx context.Context, config OperationConfig) error {
	if config.Scope == ScopeClient {
		// For client operations, try to reveal client secrets (optional)
		e.logger.Info(ctx, "📋 Revealing client repository secrets...")
		if err := e.provisioner.Cryptor().DecryptAll(false); err != nil {
			// For client operations, missing secrets is OK if parent secrets are available
			if strings.Contains(err.Error(), "not found in secrets") ||
				strings.Contains(err.Error(), "public key is not configured") {
				e.logger.Info(ctx, "ℹ️  No client secrets found - using parent repository secrets")
			} else {
				return fmt.Errorf("secret decryption failed: %w", err)
			}
		} else {
			e.logger.Info(ctx, "✅ Client secrets revealed successfully")
		}
	} else {
		// For parent operations, secrets are already revealed
		e.logger.Info(ctx, "📋 Using parent repository secrets (revealed during repository setup)")
		e.logger.Info(ctx, "✅ Parent repository secrets available")
	}

	return nil
}

// performOperation executes the actual stack operation
func (e *Executor) performOperation(ctx context.Context, config OperationConfig) error {
	previewMode := e.isPreviewMode()

	switch config.Type {
	case OperationDeploy:
		return e.executeDeploy(ctx, config, previewMode)
	case OperationProvision:
		return e.executeProvision(ctx, config, previewMode)
	case OperationDestroy:
		if config.Scope == ScopeParent {
			return e.executeDestroyParent(ctx, config, previewMode)
		}
		return e.executeDestroy(ctx, config, previewMode)
	default:
		return fmt.Errorf("unsupported operation type: %s", config.Type)
	}
}

// executeDeploy performs a deployment operation
func (e *Executor) executeDeploy(ctx context.Context, config OperationConfig, previewMode bool) error {
	deployParams := api.DeployParams{
		StackParams: api.StackParams{
			StackName:   config.StackName,
			Environment: config.Env,
			Version:     config.Version,
			SkipRefresh: previewMode,
		},
	}

	if previewMode {
		e.logger.Info(ctx, "🔍 Executing deployment in PREVIEW MODE (no real changes will be made)...")
		e.logger.Info(ctx, "Simple Container CLI version: %s", "latest")
		e.logger.Info(ctx, "Deploy version: %s", config.Version)

		if _, err := e.provisioner.Preview(ctx, deployParams); err != nil {
			return fmt.Errorf("deployment preview failed: %w", err)
		}
		e.logger.Info(ctx, "✅ Preview completed - no actual deployment performed")
	} else {
		e.logger.Info(ctx, "🚀 Executing ACTUAL deployment (changes will be applied)...")
		e.logger.Info(ctx, "Simple Container CLI version: %s", "latest")
		e.logger.Info(ctx, "Deploy version: %s", config.Version)

		if err := e.provisioner.Deploy(ctx, deployParams); err != nil {
			return fmt.Errorf("deployment failed: %w", err)
		}
		e.logger.Info(ctx, "✅ %s stack deployment completed successfully", config.Scope)
	}

	return nil
}

// executeProvision performs a provisioning operation
func (e *Executor) executeProvision(ctx context.Context, config OperationConfig, previewMode bool) error {
	profile := os.Getenv("ENVIRONMENT")
	if profile == "" {
		profile = "default"
	}

	provisionParams := api.ProvisionParams{
		StacksDir:   ".sc/stacks",
		Profile:     profile,
		Stacks:      []string{config.StackName},
		SkipRefresh: previewMode,
	}

	if previewMode {
		e.logger.Info(ctx, "🔍 Executing provisioning in PREVIEW MODE (no real changes will be made)...")
		e.logger.Info(ctx, "Simple Container CLI version: %s", "latest")

		if _, err := e.provisioner.PreviewProvision(ctx, provisionParams); err != nil {
			return fmt.Errorf("provisioning preview failed: %w", err)
		}
		e.logger.Info(ctx, "✅ Preview completed - no actual provisioning performed")
	} else {
		e.logger.Info(ctx, "🚀 Executing ACTUAL provisioning (changes will be applied)...")
		e.logger.Info(ctx, "Simple Container CLI version: %s", "latest")

		if err := e.provisioner.Provision(ctx, provisionParams); err != nil {
			return fmt.Errorf("provisioning failed: %w", err)
		}
		e.logger.Info(ctx, "✅ Parent stack provisioning completed successfully")
	}

	return nil
}

// executeDestroy performs a client stack destruction operation
func (e *Executor) executeDestroy(ctx context.Context, config OperationConfig, previewMode bool) error {
	destroyParams := api.DestroyParams{
		StackParams: api.StackParams{
			StackName:   config.StackName,
			Environment: config.Env,
			SkipRefresh: previewMode,
		},
	}

	if previewMode {
		e.logger.Info(ctx, "🔍 Executing destruction in PREVIEW MODE (no real changes will be made)...")
		e.logger.Info(ctx, "Simple Container CLI version: %s", "latest")

		if err := e.provisioner.Destroy(ctx, destroyParams, true); err != nil {
			return fmt.Errorf("destruction preview failed: %w", err)
		}
		e.logger.Info(ctx, "✅ Preview completed - no actual destruction performed")
	} else {
		e.logger.Info(ctx, "🚀 Executing ACTUAL destruction (changes will be applied)...")
		e.logger.Info(ctx, "Simple Container CLI version: %s", "latest")

		if err := e.provisioner.Destroy(ctx, destroyParams, false); err != nil {
			return fmt.Errorf("destruction failed: %w", err)
		}
		e.logger.Info(ctx, "✅ Client stack destruction completed successfully")
	}

	return nil
}

// executeDestroyParent performs a parent stack destruction operation
func (e *Executor) executeDestroyParent(ctx context.Context, config OperationConfig, previewMode bool) error {
	destroyParams := api.DestroyParams{
		StackParams: api.StackParams{
			StackName:   config.StackName,
			SkipRefresh: previewMode,
		},
	}

	if previewMode {
		e.logger.Info(ctx, "🔍 Executing parent stack destruction in PREVIEW MODE (no real changes will be made)...")

		if err := e.provisioner.DestroyParent(ctx, destroyParams, true); err != nil {
			return fmt.Errorf("parent stack destruction preview failed: %w", err)
		}
		e.logger.Info(ctx, "✅ Preview completed - no actual parent stack destruction performed")
	} else {
		e.logger.Info(ctx, "🚀 Executing ACTUAL parent stack destruction (changes will be applied)...")

		if err := e.provisioner.DestroyParent(ctx, destroyParams, false); err != nil {
			return fmt.Errorf("parent stack destruction failed: %w", err)
		}
		e.logger.Info(ctx, "✅ Parent stack destruction completed successfully")
	}

	return nil
}

// Helper functions for alerts and outputs

func (e *Executor) sendStartAlert(ctx context.Context, config OperationConfig) {
	title := fmt.Sprintf("%s Started", e.getAlertTitle(config))
	message := e.getStartMessage(config)
	envName := e.getEnvName(config)

	e.sendAlert(ctx, api.BuildStarted, title, message, config.StackName, envName)
}

func (e *Executor) sendSuccessAlert(ctx context.Context, config OperationConfig, duration time.Duration) {
	title := fmt.Sprintf("%s Succeeded", e.getAlertTitle(config))
	message := e.getSuccessMessage(config, duration)
	envName := e.getEnvName(config)

	e.sendAlert(ctx, api.BuildSucceeded, title, message, config.StackName, envName)
}

func (e *Executor) sendFailureAlert(ctx context.Context, config OperationConfig, err error, duration time.Duration) {
	title := fmt.Sprintf("%s Failed", e.getAlertTitle(config))
	message := e.getFailureMessage(config, err, duration)
	envName := e.getEnvName(config)

	e.sendAlert(ctx, api.BuildFailed, title, message, config.StackName, envName)
}

func (e *Executor) getAlertTitle(config OperationConfig) string {
	action := capitalize(string(config.Type))
	if config.Scope == ScopeParent {
		return action + " Parent"
	}
	return action
}

func (e *Executor) getStartMessage(config OperationConfig) string {
	action := strings.ToLower(string(config.Type))
	if config.Scope == ScopeParent {
		return fmt.Sprintf("Started %s of parent stack %s", action, config.StackName)
	}
	if config.Type == OperationDeploy {
		return fmt.Sprintf("Started %s of %s to %s", action, config.StackName, config.Env)
	}
	return fmt.Sprintf("Started %s of %s in %s", action, config.StackName, config.Env)
}

func (e *Executor) getSuccessMessage(config OperationConfig, duration time.Duration) string {
	action := strings.ToLower(string(config.Type))
	if config.Scope == ScopeParent {
		return fmt.Sprintf("Successfully %sed parent stack %s in %v", action, config.StackName, duration)
	}
	if config.Type == OperationDeploy {
		return fmt.Sprintf("Successfully %sed %s to %s in %v", action, config.StackName, config.Env, duration)
	}
	return fmt.Sprintf("Successfully %sed %s in %s in %v", action, config.StackName, config.Env, duration)
}

func (e *Executor) getFailureMessage(config OperationConfig, err error, duration time.Duration) string {
	action := capitalize(string(config.Type))
	scope := ""
	if config.Scope == ScopeParent {
		scope = "parent stack "
	}

	if duration > 0 {
		return fmt.Sprintf("%s of %s%s failed after %v: %v", action, scope, config.StackName, duration, err)
	}
	return fmt.Sprintf("Failed to setup %s%s: %v", scope, config.StackName, err)
}

func (e *Executor) getEnvName(config OperationConfig) string {
	if config.Scope == ScopeParent {
		return "infrastructure"
	}
	return config.Env
}

func (e *Executor) getOperationEmoji(config OperationConfig) string {
	switch config.Type {
	case OperationDeploy:
		return "🚀"
	case OperationProvision:
		return "🏗️"
	case OperationDestroy:
		if config.Scope == ScopeParent {
			return "💥"
		}
		return "🗑️"
	default:
		return "⚙️"
	}
}

func (e *Executor) getOperationAction(config OperationConfig) string {
	return string(config.Type)
}

func (e *Executor) setOperationOutputs(config OperationConfig, duration time.Duration) {
	outputs := map[string]string{
		"stack-name":   config.StackName,
		"duration":     duration.String(),
		"preview-mode": fmt.Sprintf("%v", e.isPreviewMode()),
	}

	if config.Env != "" {
		outputs["environment"] = config.Env
	}
	if config.Version != "" {
		outputs["version"] = config.Version
	}

	e.setGitHubOutputs(outputs)
}
