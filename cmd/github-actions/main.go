package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/git"
	"github.com/simple-container-com/api/pkg/api/logger"
	"github.com/simple-container-com/api/pkg/githubactions/actions"
	"github.com/simple-container-com/api/pkg/provisioner"
)

func main() {
	// Setup context with cancellation for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle signals for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\nReceived shutdown signal, cancelling operations...")
		cancel()
	}()

	// Determine action type from command line arguments or environment
	actionType := os.Getenv("GITHUB_ACTION_TYPE")
	if len(os.Args) > 1 {
		actionType = os.Args[1]
	}

	if actionType == "" {
		fmt.Fprintf(os.Stderr, "Error: Action type not specified. Set GITHUB_ACTION_TYPE or provide as argument.\n")
		fmt.Fprintf(os.Stderr, "Valid actions: deploy-client-stack, provision-parent-stack, destroy-client-stack, destroy-parent-stack\n")
		os.Exit(1)
	}

	// Validate action type early
	validActions := map[string]bool{
		"deploy-client-stack":    true,
		"provision-parent-stack": true,
		"destroy-client-stack":   true,
		"destroy-parent-stack":   true,
	}

	if !validActions[actionType] {
		fmt.Fprintf(os.Stderr, "Unknown action type: %s\n", actionType)
		fmt.Fprintf(os.Stderr, "Valid actions: deploy-client-stack, provision-parent-stack, destroy-client-stack, destroy-parent-stack\n")
		os.Exit(1)
	}

	// Initialize SC's internal logger
	log := logger.New()
	log.Info(ctx, "Starting Simple Container GitHub Action: %s", actionType)
	log.Info(ctx, "Repository: %s, Run ID: %s", os.Getenv("GITHUB_REPOSITORY"), os.Getenv("GITHUB_RUN_ID"))

	// Initialize git repository
	gitRepo, err := git.New(git.WithDetectRootDir())
	if err != nil {
		log.Error(ctx, "Failed to initialize git repository: %v", err)
		os.Exit(1)
	}

	// Initialize provisioner with SC's internal APIs
	prov, err := provisioner.New(
		provisioner.WithGitRepo(gitRepo),
		provisioner.WithLogger(log),
	)
	if err != nil {
		log.Error(ctx, "Failed to initialize provisioner: %v", err)
		os.Exit(1)
	}

	// Initialize provisioner
	workDir, _ := os.Getwd()
	err = prov.Init(ctx, api.InitParams{
		ProjectName:         os.Getenv("STACK_NAME"),
		RootDir:             workDir,
		SkipInitialCommit:   true,
		SkipProfileCreation: true,
		Profile:             os.Getenv("ENVIRONMENT"),
	})
	if err != nil {
		log.Error(ctx, "Failed to initialize provisioner: %v", err)
		os.Exit(1)
	}

	// Execute action using SC's internal APIs
	executor := actions.NewExecutor(prov, log, gitRepo)
	var execErr error

	switch actionType {
	case "deploy-client-stack":
		execErr = executor.DeployClientStack(ctx)

	case "provision-parent-stack":
		execErr = executor.ProvisionParentStack(ctx)

	case "destroy-client-stack":
		execErr = executor.DestroyClientStack(ctx)

	case "destroy-parent-stack":
		execErr = executor.DestroyParentStack(ctx)
	}

	// Handle execution result
	if execErr != nil {
		if ctx.Err() != nil {
			log.Warn(ctx, "Action cancelled: %s, error: %v", actionType, execErr)
			fmt.Fprintf(os.Stderr, "Action cancelled: %v\n", execErr)
		} else {
			log.Error(ctx, "Action failed: %s, error: %v", actionType, execErr)
			fmt.Fprintf(os.Stderr, "Action failed: %v\n", execErr)
		}
		os.Exit(1)
	}

	log.Info(ctx, "Action completed successfully: %s", actionType)
}
