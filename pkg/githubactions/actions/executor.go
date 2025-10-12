package actions

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/simple-container-com/api/pkg/api"
	scgit "github.com/simple-container-com/api/pkg/api/git"
	"github.com/simple-container-com/api/pkg/api/logger"
	"github.com/simple-container-com/api/pkg/githubactions/common/git"
	"github.com/simple-container-com/api/pkg/githubactions/common/notifications"
	"github.com/simple-container-com/api/pkg/githubactions/config"
	"github.com/simple-container-com/api/pkg/provisioner"
)

// Executor handles GitHub Actions using SC's internal APIs
type Executor struct {
	provisioner provisioner.Provisioner
	logger      logger.Logger
	gitRepo     scgit.Repo
	notifier    *notifications.Manager
}

// Logger interface for githubactions notifications
type Logger interface {
	Info(msg string, keysAndValues ...interface{})
	Warn(msg string, keysAndValues ...interface{})
	Error(msg string, keysAndValues ...interface{})
	Debug(msg string, keysAndValues ...interface{})
}

// LoggerAdapter adapts SC's logger to githubactions logging interface
type LoggerAdapter struct {
	scLogger logger.Logger
	ctx      context.Context
}

func (l *LoggerAdapter) Info(msg string, args ...interface{}) {
	l.scLogger.Info(l.ctx, msg, args...)
}

func (l *LoggerAdapter) Warn(msg string, args ...interface{}) {
	l.scLogger.Warn(l.ctx, msg, args...)
}

func (l *LoggerAdapter) Error(msg string, args ...interface{}) {
	l.scLogger.Error(l.ctx, msg, args...)
}

func (l *LoggerAdapter) Debug(msg string, args ...interface{}) {
	l.scLogger.Debug(l.ctx, msg, args...)
}

// NewExecutor creates a new GitHub Actions executor using SC's internal APIs
func NewExecutor(prov provisioner.Provisioner, log logger.Logger, gitRepo scgit.Repo) *Executor {
	// Create logger adapter for existing notifications
	logAdapter := &LoggerAdapter{
		scLogger: log,
		ctx:      context.Background(),
	}

	// Create config compatible with existing notifications
	cfg := &config.Config{
		StackName:         os.Getenv("STACK_NAME"),
		Environment:       os.Getenv("ENVIRONMENT"),
		GitHubRepository:  os.Getenv("GITHUB_REPOSITORY"),
		GitHubRunID:       os.Getenv("GITHUB_RUN_ID"),
		GitHubServerURL:   os.Getenv("GITHUB_SERVER_URL"),
		GitHubActor:       os.Getenv("GITHUB_ACTOR"),
		SlackWebhookURL:   os.Getenv("SLACK_WEBHOOK_URL"),
		DiscordWebhookURL: os.Getenv("DISCORD_WEBHOOK_URL"),
	}

	// Initialize notification manager using existing implementation
	notifier := notifications.NewManager(cfg, logAdapter)

	return &Executor{
		provisioner: prov,
		logger:      log,
		gitRepo:     gitRepo,
		notifier:    notifier,
	}
}

// DeployClientStack deploys a client stack using SC's internal APIs
func (e *Executor) DeployClientStack(ctx context.Context) error {
	e.logger.Info(ctx, "🚀 Starting client stack deployment using SC internal APIs")
	startTime := time.Now()

	// Extract configuration from environment
	stackName := os.Getenv("STACK_NAME")
	environment := os.Getenv("ENVIRONMENT")
	version := os.Getenv("VERSION")

	if stackName == "" || environment == "" {
		return fmt.Errorf("STACK_NAME and ENVIRONMENT are required")
	}

	if version == "" {
		version = "latest"
	}

	e.logger.Info(ctx, "Deploying stack: %s, environment: %s, version: %s", stackName, environment, version)

	// Send start notification
	if err := e.sendNotification(ctx, notifications.StatusStarted, startTime); err != nil {
		e.logger.Warn(ctx, "Failed to send start notification: %v", err)
	}

	// Reveal secrets using SC's internal API
	e.logger.Info(ctx, "📋 Revealing secrets...")
	if err := e.provisioner.Cryptor().DecryptAll(false); err != nil {
		e.logger.Warn(ctx, "Failed to decrypt secrets: %v", err)
	}

	// Deploy using SC's provisioner API
	deployParams := api.DeployParams{
		StackParams: api.StackParams{
			StackName:   stackName,
			Environment: environment,
			Version:     version,
		},
	}

	e.logger.Info(ctx, "🔧 Executing deployment...")
	err := e.provisioner.Deploy(ctx, deployParams)
	if err != nil {
		// Send failure notification
		if notifyErr := e.sendNotification(ctx, notifications.StatusFailure, startTime); notifyErr != nil {
			e.logger.Warn(ctx, "Failed to send failure notification: %v", notifyErr)
		}
		return fmt.Errorf("deployment failed: %w", err)
	}

	// Set GitHub Action outputs
	e.setGitHubOutputs(map[string]string{
		"version":     version,
		"environment": environment,
		"stack-name":  stackName,
		"status":      "success",
		"duration":    time.Since(startTime).String(),
	})

	// Send success notification
	if err := e.sendNotification(ctx, notifications.StatusSuccess, startTime); err != nil {
		e.logger.Warn(ctx, "Failed to send success notification: %v", err)
	}

	e.logger.Info(ctx, "✅ Client stack deployment completed successfully")
	return nil
}

// ProvisionParentStack provisions a parent stack using SC's internal APIs
func (e *Executor) ProvisionParentStack(ctx context.Context) error {
	e.logger.Info(ctx, "🏗️ Starting parent stack provisioning using SC internal APIs")
	startTime := time.Now()

	stackName := os.Getenv("STACK_NAME")
	if stackName == "" {
		return fmt.Errorf("STACK_NAME is required")
	}

	// Send start notification
	if err := e.sendNotification(ctx, notifications.StatusStarted, startTime); err != nil {
		e.logger.Warn(ctx, "Failed to send start notification: %v", err)
	}

	// Provision using SC's provisioner API
	provisionParams := api.ProvisionParams{
		Stacks:  []string{stackName},
		Profile: os.Getenv("ENVIRONMENT"),
	}

	e.logger.Info(ctx, "🔧 Executing provisioning...")
	err := e.provisioner.Provision(ctx, provisionParams)
	if err != nil {
		// Send failure notification
		if notifyErr := e.sendNotification(ctx, notifications.StatusFailure, startTime); notifyErr != nil {
			e.logger.Warn(ctx, "Failed to send failure notification: %v", notifyErr)
		}
		return fmt.Errorf("provisioning failed: %w", err)
	}

	// Set GitHub Action outputs
	e.setGitHubOutputs(map[string]string{
		"stack-name": stackName,
		"status":     "success",
		"duration":   time.Since(startTime).String(),
	})

	// Send success notification
	if err := e.sendNotification(ctx, notifications.StatusSuccess, startTime); err != nil {
		e.logger.Warn(ctx, "Failed to send success notification: %v", err)
	}

	e.logger.Info(ctx, "✅ Parent stack provisioning completed successfully")
	return nil
}

// DestroyClientStack destroys a client stack using SC's internal APIs
func (e *Executor) DestroyClientStack(ctx context.Context) error {
	e.logger.Info(ctx, "🗑️ Starting client stack destruction using SC internal APIs")
	startTime := time.Now()

	stackName := os.Getenv("STACK_NAME")
	environment := os.Getenv("ENVIRONMENT")

	if stackName == "" || environment == "" {
		return fmt.Errorf("STACK_NAME and ENVIRONMENT are required")
	}

	// Send start notification
	if err := e.sendNotification(ctx, notifications.StatusStarted, startTime); err != nil {
		e.logger.Warn(ctx, "Failed to send start notification: %v", err)
	}

	// Destroy using SC's provisioner API
	destroyParams := api.DestroyParams{
		StackParams: api.StackParams{
			StackName:   stackName,
			Environment: environment,
		},
	}

	e.logger.Info(ctx, "🔧 Executing destruction...")
	err := e.provisioner.Destroy(ctx, destroyParams, false) // preview = false
	if err != nil {
		// Send failure notification
		if notifyErr := e.sendNotification(ctx, notifications.StatusFailure, startTime); notifyErr != nil {
			e.logger.Warn(ctx, "Failed to send failure notification: %v", notifyErr)
		}
		return fmt.Errorf("destruction failed: %w", err)
	}

	// Set GitHub Action outputs
	e.setGitHubOutputs(map[string]string{
		"environment": environment,
		"stack-name":  stackName,
		"status":      "success",
		"duration":    time.Since(startTime).String(),
	})

	// Send success notification
	if err := e.sendNotification(ctx, notifications.StatusSuccess, startTime); err != nil {
		e.logger.Warn(ctx, "Failed to send success notification: %v", err)
	}

	e.logger.Info(ctx, "✅ Client stack destruction completed successfully")
	return nil
}

// DestroyParentStack destroys a parent stack using SC's internal APIs
func (e *Executor) DestroyParentStack(ctx context.Context) error {
	e.logger.Info(ctx, "💥 Starting parent stack destruction using SC internal APIs")
	startTime := time.Now()

	stackName := os.Getenv("STACK_NAME")
	if stackName == "" {
		return fmt.Errorf("STACK_NAME is required")
	}

	// Send start notification
	if err := e.sendNotification(ctx, notifications.StatusStarted, startTime); err != nil {
		e.logger.Warn(ctx, "Failed to send start notification: %v", err)
	}

	// Destroy parent using SC's provisioner API
	destroyParams := api.DestroyParams{
		StackParams: api.StackParams{
			StackName: stackName,
		},
	}

	e.logger.Info(ctx, "🔧 Executing parent stack destruction...")
	err := e.provisioner.DestroyParent(ctx, destroyParams, false) // preview = false
	if err != nil {
		// Send failure notification
		if notifyErr := e.sendNotification(ctx, notifications.StatusFailure, startTime); notifyErr != nil {
			e.logger.Warn(ctx, "Failed to send failure notification: %v", notifyErr)
		}
		return fmt.Errorf("parent stack destruction failed: %w", err)
	}

	// Set GitHub Action outputs
	e.setGitHubOutputs(map[string]string{
		"stack-name": stackName,
		"status":     "success",
		"duration":   time.Since(startTime).String(),
	})

	// Send success notification
	if err := e.sendNotification(ctx, notifications.StatusSuccess, startTime); err != nil {
		e.logger.Warn(ctx, "Failed to send success notification: %v", err)
	}

	e.logger.Info(ctx, "✅ Parent stack destruction completed successfully")
	return nil
}

// sendNotification sends notification using existing notification manager
func (e *Executor) sendNotification(ctx context.Context, status notifications.Status, startTime time.Time) error {
	// Extract git metadata using SC's git API
	branch, _ := e.gitRepo.Branch()
	commitHash, _ := e.gitRepo.Hash()

	// Create metadata compatible with existing notifications system
	metadata := &git.Metadata{
		Branch:    branch,
		CommitSHA: commitHash,
		Author:    os.Getenv("GITHUB_ACTOR"),
		BuildURL:  fmt.Sprintf("%s/%s/actions/runs/%s", os.Getenv("GITHUB_SERVER_URL"), os.Getenv("GITHUB_REPOSITORY"), os.Getenv("GITHUB_RUN_ID")),
	}

	version := os.Getenv("VERSION")
	if version == "" {
		version = "latest"
	}

	return e.notifier.SendNotification(ctx, status, metadata, version, time.Since(startTime))
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

	// Write to GITHUB_OUTPUT file
	f, err := os.OpenFile(outputFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		e.logger.Error(context.Background(), "Failed to open GITHUB_OUTPUT file: %v", err)
		return
	}
	defer f.Close()

	for key, value := range outputs {
		if _, err := f.WriteString(fmt.Sprintf("%s=%s\n", key, value)); err != nil {
			e.logger.Error(context.Background(), "Failed to write to GITHUB_OUTPUT: %v", err)
		}
	}
}
