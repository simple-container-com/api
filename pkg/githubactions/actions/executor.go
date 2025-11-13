package actions

import (
	"fmt"
	"os"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/git"
	"github.com/simple-container-com/api/pkg/api/logger"
	"github.com/simple-container-com/api/pkg/provisioner"
)

// Executor handles GitHub Actions using only SC's internal APIs
type Executor struct {
	provisioner    provisioner.Provisioner
	logger         logger.Logger
	gitRepo        git.Repo
	slackSender    api.AlertSender
	discordSender  api.AlertSender
	telegramSender api.AlertSender
	signalHandler  *SignalHandler
}

// NewExecutor creates a new GitHub Actions executor using only SC's internal APIs
func NewExecutor(prov provisioner.Provisioner, log logger.Logger, gitRepo git.Repo) *Executor {
	executor := &Executor{
		provisioner: prov,
		logger:      log,
		gitRepo:     gitRepo,
	}

	// Initialize signal handler
	executor.signalHandler = NewSignalHandler(log, prov)

	// Notification initialization will be done after secrets are revealed
	return executor
}

// isPreviewMode checks if the executor should run in preview/dry-run mode
func (e *Executor) isPreviewMode() bool {
	// Check various environment variables that indicate preview mode
	return os.Getenv("SC_PREVIEW") == "true" ||
		os.Getenv("SC_DRY_RUN") == "true" ||
		os.Getenv("DRY_RUN") == "true" ||
		os.Getenv("SC_DEPLOY_PREVIEW") == "true" ||
		os.Getenv("GITHUB_EVENT_NAME") == "pull_request"
}

// setActionOutput sets a GitHub Actions output variable
func (e *Executor) setActionOutput(name, value string) error {
	outputFile := os.Getenv("GITHUB_OUTPUT")
	if outputFile == "" {
		// Fallback to stdout format for older runners
		fmt.Printf("::set-output name=%s::%s\n", name, value)
		return nil
	}

	// Use the new GITHUB_OUTPUT file format
	file, err := os.OpenFile(outputFile, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("failed to open GITHUB_OUTPUT file: %w", err)
	}
	defer file.Close()

	_, err = fmt.Fprintf(file, "%s=%s\n", name, value)
	if err != nil {
		return fmt.Errorf("failed to write to GITHUB_OUTPUT file: %w", err)
	}

	return nil
}
