package actions

import (
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
}

// NewExecutor creates a new GitHub Actions executor using only SC's internal APIs
func NewExecutor(prov provisioner.Provisioner, log logger.Logger, gitRepo git.Repo) *Executor {
	executor := &Executor{
		provisioner: prov,
		logger:      log,
		gitRepo:     gitRepo,
	}

	// Notification initialization will be done after secrets are revealed
	return executor
}

// isPreviewMode checks if the executor should run in preview/dry-run mode
func (e *Executor) isPreviewMode() bool {
	// Check various environment variables that indicate preview mode
	return os.Getenv("SC_PREVIEW") == "true" ||
		os.Getenv("SC_DRY_RUN") == "true" ||
		os.Getenv("SC_DEPLOY_PREVIEW") == "true" ||
		os.Getenv("GITHUB_EVENT_NAME") == "pull_request"
}
