package actions

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v2"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/git"
	"github.com/simple-container-com/api/pkg/api/logger"
	"github.com/simple-container-com/api/pkg/clouds/discord"
	"github.com/simple-container-com/api/pkg/clouds/slack"
	"github.com/simple-container-com/api/pkg/clouds/telegram"
	"github.com/simple-container-com/api/pkg/provisioner"
)

// SCConfig represents the structure of SIMPLE_CONTAINER_CONFIG
type SCConfig struct {
	PrivateKey       string `yaml:"privateKey"`
	PublicKey        string `yaml:"publicKey"`
	ParentRepository string `yaml:"parentRepository"`
}

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

	// Initialize SC's Slack alert sender if webhook URL is provided
	if slackWebhookURL := os.Getenv("SLACK_WEBHOOK_URL"); slackWebhookURL != "" {
		if slackSender, err := slack.New(slackWebhookURL); err == nil {
			executor.slackSender = slackSender
		} else {
			log.Warn(context.Background(), "Failed to initialize Slack notifications: %v", err)
		}
	}

	// Initialize SC's Discord alert sender if webhook URL is provided
	if discordWebhookURL := os.Getenv("DISCORD_WEBHOOK_URL"); discordWebhookURL != "" {
		if discordSender, err := discord.New(discordWebhookURL); err == nil {
			executor.discordSender = discordSender
		} else {
			log.Warn(context.Background(), "Failed to initialize Discord notifications: %v", err)
		}
	}

	// Initialize SC's Telegram alert sender if chat ID and token are provided
	telegramChatID := os.Getenv("TELEGRAM_CHAT_ID")
	telegramToken := os.Getenv("TELEGRAM_TOKEN")
	if telegramChatID != "" && telegramToken != "" {
		telegramSender := telegram.New(telegramChatID, telegramToken)
		executor.telegramSender = telegramSender
	}

	return executor
}

// cloneParentRepository clones the parent stack repository and copies stack configurations
func (e *Executor) cloneParentRepository(ctx context.Context) error {
	e.logger.Info(ctx, "📦 Setting up parent stack repository...")

	// Get SC config from environment
	scConfigYAML := os.Getenv("SC_CONFIG")
	if scConfigYAML == "" {
		scConfigYAML = os.Getenv("SIMPLE_CONTAINER_CONFIG")
	}

	if scConfigYAML == "" {
		e.logger.Warn(ctx, "No SC_CONFIG or SIMPLE_CONTAINER_CONFIG provided, skipping parent repository setup")
		return nil
	}

	// Parse SC config
	var scConfig SCConfig
	if err := yaml.Unmarshal([]byte(scConfigYAML), &scConfig); err != nil {
		return fmt.Errorf("failed to parse SC config: %w", err)
	}

	// Skip if no parent repository is configured
	if scConfig.ParentRepository == "" {
		e.logger.Info(ctx, "No parent repository configured, skipping")
		return nil
	}

	// Use privateKey for SSH git operations (publicKey mentioned in request might be a mistake)
	sshKey := scConfig.PrivateKey
	if sshKey == "" {
		sshKey = scConfig.PublicKey // fallback to publicKey if privateKey is not available
	}

	if sshKey == "" {
		e.logger.Warn(ctx, "No SSH key found in SC config for parent repository clone")
		return nil
	}

	e.logger.Info(ctx, "Cloning parent repository: %s", scConfig.ParentRepository)

	// Setup SSH key for git operations
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get user home directory: %w", err)
	}

	sshDir := filepath.Join(homeDir, ".ssh")
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		return fmt.Errorf("failed to create .ssh directory: %w", err)
	}

	// Write SSH private key
	keyPath := filepath.Join(sshDir, "github_actions_key")
	if err := os.WriteFile(keyPath, []byte(sshKey), 0o600); err != nil {
		return fmt.Errorf("failed to write SSH key: %w", err)
	}

	// Write SSH config for git operations
	sshConfigPath := filepath.Join(sshDir, "config")
	sshConfig := fmt.Sprintf(`Host github.com
    HostName github.com
    User git
    IdentityFile %s
    StrictHostKeyChecking no
`, keyPath)

	if err := os.WriteFile(sshConfigPath, []byte(sshConfig), 0o600); err != nil {
		return fmt.Errorf("failed to write SSH config: %w", err)
	}

	// Clone parent repository to .devops directory
	devopsDir := ".devops"
	if err := os.RemoveAll(devopsDir); err != nil {
		e.logger.Warn(ctx, "Failed to remove existing .devops directory: %v", err)
	}

	// Use git command directly since we need SSH key support
	cloneCmd := exec.CommandContext(ctx, "git", "clone", "--depth", "1", scConfig.ParentRepository, devopsDir)
	cloneCmd.Env = append(os.Environ(), "GIT_SSH_COMMAND=ssh -o StrictHostKeyChecking=no -i "+keyPath)

	if output, err := cloneCmd.CombinedOutput(); err != nil {
		e.logger.Error(ctx, "Failed to clone parent repository: %s", string(output))
		return fmt.Errorf("failed to clone parent repository %s: %w", scConfig.ParentRepository, err)
	}

	e.logger.Info(ctx, "Successfully cloned parent repository")

	// Copy .sc/stacks/* from parent repository to current workspace
	parentStacksDir := filepath.Join(devopsDir, ".sc", "stacks")
	currentStacksDir := filepath.Join(".sc", "stacks")

	// Ensure current .sc/stacks directory exists
	if err := os.MkdirAll(currentStacksDir, 0o755); err != nil {
		return fmt.Errorf("failed to create .sc/stacks directory: %w", err)
	}

	// Copy all stacks from parent repository
	if _, err := os.Stat(parentStacksDir); err == nil {
		if err := e.copyDirectory(parentStacksDir, currentStacksDir); err != nil {
			return fmt.Errorf("failed to copy parent stacks: %w", err)
		}
		e.logger.Info(ctx, "Successfully copied parent stack configurations")
	} else {
		e.logger.Warn(ctx, "No .sc/stacks directory found in parent repository")
	}

	// Clean up SSH key and config files
	os.Remove(keyPath)
	os.Remove(sshConfigPath)

	e.logger.Info(ctx, "✅ Parent repository setup completed")
	return nil
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

	// Send start notification using SC's alert system
	e.sendAlert(ctx, api.BuildStarted, "Deploy Started", fmt.Sprintf("Started deployment of %s to %s", stackName, environment), stackName, environment)

	// Clone parent stack repository if configured
	if err := e.cloneParentRepository(ctx); err != nil {
		e.sendAlert(ctx, api.BuildFailed, "Deploy Failed", fmt.Sprintf("Failed to setup parent repository for %s: %v", stackName, err), stackName, environment)
		return fmt.Errorf("parent repository setup failed: %w", err)
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
		e.sendAlert(ctx, api.BuildFailed, "Deploy Failed", fmt.Sprintf("Deployment of %s to %s failed: %v", stackName, environment, err), stackName, environment)
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
	e.sendAlert(ctx, api.BuildSucceeded, "Deploy Completed", fmt.Sprintf("Successfully deployed %s to %s in %v", stackName, environment, time.Since(startTime)), stackName, environment)

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
	e.sendAlert(ctx, api.BuildStarted, "Provision Started", fmt.Sprintf("Started provisioning of parent stack %s", stackName), stackName, "infrastructure")

	// Provision using SC's provisioner API
	provisionParams := api.ProvisionParams{
		Stacks:  []string{stackName},
		Profile: os.Getenv("ENVIRONMENT"),
	}

	e.logger.Info(ctx, "🔧 Executing provisioning...")
	err := e.provisioner.Provision(ctx, provisionParams)
	if err != nil {
		// Send failure notification
		e.sendAlert(ctx, api.BuildFailed, "Provision Failed", fmt.Sprintf("Provisioning of %s failed: %v", stackName, err), stackName, "infrastructure")
		return fmt.Errorf("provisioning failed: %w", err)
	}

	// Set GitHub Action outputs
	e.setGitHubOutputs(map[string]string{
		"stack-name": stackName,
		"status":     "success",
		"duration":   time.Since(startTime).String(),
	})

	// Send success notification
	e.sendAlert(ctx, api.BuildSucceeded, "Provision Completed", fmt.Sprintf("Successfully provisioned parent stack %s in %v", stackName, time.Since(startTime)), stackName, "infrastructure")

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
	e.sendAlert(ctx, api.BuildStarted, "Destroy Started", fmt.Sprintf("Started destruction of %s in %s", stackName, environment), stackName, environment)

	// Clone parent stack repository if configured
	if err := e.cloneParentRepository(ctx); err != nil {
		e.sendAlert(ctx, api.BuildFailed, "Destroy Failed", fmt.Sprintf("Failed to setup parent repository for %s: %v", stackName, err), stackName, environment)
		return fmt.Errorf("parent repository setup failed: %w", err)
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
		e.sendAlert(ctx, api.BuildFailed, "Destroy Failed", fmt.Sprintf("Destruction of %s in %s failed: %v", stackName, environment, err), stackName, environment)
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
	e.sendAlert(ctx, api.BuildSucceeded, "Destroy Completed", fmt.Sprintf("Successfully destroyed %s in %s after %v", stackName, environment, time.Since(startTime)), stackName, environment)

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
	e.sendAlert(ctx, api.BuildStarted, "Destroy Parent Started", fmt.Sprintf("Started destruction of parent stack %s", stackName), stackName, "infrastructure")

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
		e.sendAlert(ctx, api.BuildFailed, "Destroy Parent Failed", fmt.Sprintf("Parent stack destruction of %s failed: %v", stackName, err), stackName, "infrastructure")
		return fmt.Errorf("parent stack destruction failed: %w", err)
	}

	// Set GitHub Action outputs
	e.setGitHubOutputs(map[string]string{
		"stack-name": stackName,
		"status":     "success",
		"duration":   time.Since(startTime).String(),
	})

	// Send success notification
	e.sendAlert(ctx, api.BuildSucceeded, "Destroy Parent Completed", fmt.Sprintf("Successfully destroyed parent stack %s in %v", stackName, time.Since(startTime)), stackName, "infrastructure")

	e.logger.Info(ctx, "✅ Parent stack destruction completed successfully")
	return nil
}

// sendAlert sends notifications using SC's internal alert system
func (e *Executor) sendAlert(ctx context.Context, alertType api.AlertType, title, description, stackName, stackEnv string) {
	// Extract git metadata using SC's git API
	branch, _ := e.gitRepo.Branch()
	commitHash, _ := e.gitRepo.Hash()

	buildURL := fmt.Sprintf("%s/%s/actions/runs/%s", os.Getenv("GITHUB_SERVER_URL"), os.Getenv("GITHUB_REPOSITORY"), os.Getenv("GITHUB_RUN_ID"))

	alert := api.Alert{
		Name:        "github-actions",
		Title:       title,
		Description: fmt.Sprintf("%s\nBranch: %s\nCommit: %s\nActor: %s", description, branch, commitHash, os.Getenv("GITHUB_ACTOR")),
		StackName:   stackName,
		StackEnv:    stackEnv,
		DetailsUrl:  buildURL,
		AlertType:   alertType,
	}

	// Send to Slack if configured
	if e.slackSender != nil {
		if err := e.slackSender.Send(alert); err != nil {
			e.logger.Warn(ctx, "Failed to send Slack notification: %v", err)
		} else {
			e.logger.Info(ctx, "Slack notification sent successfully")
		}
	}

	// Send to Discord if configured
	if e.discordSender != nil {
		if err := e.discordSender.Send(alert); err != nil {
			e.logger.Warn(ctx, "Failed to send Discord notification: %v", err)
		} else {
			e.logger.Info(ctx, "Discord notification sent successfully")
		}
	}

	// Send to Telegram if configured
	if e.telegramSender != nil {
		if err := e.telegramSender.Send(alert); err != nil {
			e.logger.Warn(ctx, "Failed to send Telegram notification: %v", err)
		} else {
			e.logger.Info(ctx, "Telegram notification sent successfully")
		}
	}

	if e.slackSender == nil && e.discordSender == nil && e.telegramSender == nil {
		e.logger.Info(ctx, "No notification webhooks configured, skipping notifications")
	}
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
