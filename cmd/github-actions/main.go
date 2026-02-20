package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
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
		fmt.Fprintf(os.Stderr, "Valid actions: deploy-client-stack, provision-parent-stack, destroy, cancel-stack\n")
		os.Exit(1)
	}

	// Validate action type early
	validActions := map[string]bool{
		"deploy-client-stack":    true,
		"provision-parent-stack": true,
		"destroy":                true,
		"cancel-stack":           true,
	}

	if !validActions[actionType] {
		fmt.Fprintf(os.Stderr, "Unknown action type: %s\n", actionType)
		fmt.Fprintf(os.Stderr, "Valid actions: deploy-client-stack, provision-parent-stack, destroy, cancel-stack\n")
		os.Exit(1)
	}

	// Initialize SC's internal logger
	log := logger.New()

	// Check for verbose mode and set debug logging if enabled
	if os.Getenv("VERBOSE") == "true" {
		ctx = log.SetLogLevel(ctx, logger.LogLevelDebug)
		log.Debug(ctx, "🔍 Verbose mode enabled - debug logging activated")
		log.Debug(ctx, "Environment variables:")
		log.Debug(ctx, "  - GITHUB_REPOSITORY: %s", os.Getenv("GITHUB_REPOSITORY"))
		log.Debug(ctx, "  - GITHUB_RUN_ID: %s", os.Getenv("GITHUB_RUN_ID"))
		log.Debug(ctx, "  - GITHUB_ACTION_TYPE: %s", os.Getenv("GITHUB_ACTION_TYPE"))
		log.Debug(ctx, "  - STACK_NAME: %s", os.Getenv("STACK_NAME"))
		log.Debug(ctx, "  - ENVIRONMENT: %s", os.Getenv("ENVIRONMENT"))
		log.Debug(ctx, "  - DRY_RUN: %s", os.Getenv("DRY_RUN"))
	}

	log.Info(ctx, "Starting Simple Container GitHub Action: %s", actionType)
	log.Info(ctx, "Repository: %s, Run ID: %s", os.Getenv("GITHUB_REPOSITORY"), os.Getenv("GITHUB_RUN_ID"))

	// Initialize git repository - handle GitHub Actions environment properly
	var gitRepo git.Repo
	workDir, _ := os.Getwd()

	// Try to detect existing git repository with proper content first
	log.Debug(ctx, "🔧 Attempting to detect git repository in: %s", workDir)
	gitRepo, err := git.New(git.WithDetectRootDir())
	if err != nil || !isProperRepository(workDir) {
		if err != nil {
			log.Warn(ctx, "No git repository detected: %v", err)
			log.Debug(ctx, "🔍 Repository detection failed, will attempt cloning")
		} else {
			log.Warn(ctx, "Git repository exists but appears to be empty or incomplete")
			log.Debug(ctx, "🔍 Repository incomplete, will attempt cloning")
		}

		// Clone the repository like actions/checkout does
		log.Debug(ctx, "🔧 Starting repository cloning process...")
		if err := cloneRepository(ctx, log, workDir); err != nil {
			log.Error(ctx, "Failed to clone repository: %v", err)
			os.Exit(1)
		}

		// Try to initialize git repo again after cloning
		log.Debug(ctx, "🔧 Re-initializing git repository after successful clone...")
		gitRepo, err = git.New(git.WithDetectRootDir())
		if err != nil {
			log.Error(ctx, "Failed to initialize git repository after cloning: %v", err)
			os.Exit(1)
		}

		log.Info(ctx, "Successfully initialized git repository from clone")
	} else {
		log.Info(ctx, "Using existing git repository")
		log.Debug(ctx, "🔍 Existing repository validated and ready")
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

	case "destroy":
		// Determine stack type based on environment parameter
		environment := os.Getenv("ENVIRONMENT")
		if environment != "" {
			// Environment provided = client stack destruction
			log.Info(ctx, "🎯 Environment '%s' provided - destroying client stack", environment)
			execErr = executor.DestroyClientStack(ctx)
		} else {
			// No environment = parent stack destruction
			log.Info(ctx, "🏗️ No environment provided - destroying parent stack")
			execErr = executor.DestroyParentStack(ctx)
		}

	case "cancel-stack":
		execErr = executor.CancelStack(ctx)
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

// isProperRepository checks if the directory has a proper git repository with content
func isProperRepository(workDir string) bool {
	// Check if .git exists
	gitDir := workDir + "/.git"
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		return false
	}

	// Check if we have any actual repository content (not just an empty git repo)
	entries, err := os.ReadDir(workDir)
	if err != nil {
		return false
	}

	// Count non-git files/directories
	contentCount := 0
	for _, entry := range entries {
		if entry.Name() != ".git" && entry.Name() != ".github" {
			contentCount++
		}
	}

	// If we only have .git and .github, it's likely an incomplete checkout
	return contentCount > 0
}

// cloneRepository performs a git clone similar to actions/checkout
func cloneRepository(ctx context.Context, log logger.Logger, workDir string) error {
	// Get repository info - try input first, then fallback to GitHub Actions automatic env vars
	repository := os.Getenv("GITHUB_REPOSITORY")
	if repository == "" {
		log.Error(ctx, "GITHUB_REPOSITORY not available - ensure workflow passes repository context")
		return fmt.Errorf("GITHUB_REPOSITORY environment variable not set")
	}

	// Get token for authentication - try multiple sources following GitHub best practices
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		// Check for other potential token locations
		if actionsToken := os.Getenv("GITHUB_ACTIONS_TOKEN"); actionsToken != "" {
			token = actionsToken
			log.Info(ctx, "Using GITHUB_ACTIONS_TOKEN for authentication")
		}
	} else {
		log.Info(ctx, "Using GITHUB_TOKEN for authentication")
	}

	if token == "" {
		log.Error(ctx, "No GitHub token available for private repository access")
		log.Error(ctx, "Ensure workflow passes token: ${{ secrets.GITHUB_TOKEN }} to action inputs")
		log.Error(ctx, "Private repository %s requires authentication", repository)
		return fmt.Errorf("GitHub token required for repository authentication - check workflow configuration")
	}

	// Get the ref to checkout (default to the current SHA)
	ref := os.Getenv("GITHUB_SHA")
	if ref == "" {
		ref = os.Getenv("GITHUB_REF")
		if ref == "" {
			ref = "main" // fallback
		}
	}

	log.Info(ctx, "Cloning repository %s at ref %s with authentication", repository, ref)

	// Construct authenticated clone URL
	repoURL := fmt.Sprintf("https://x-access-token:%s@github.com/%s.git", token, repository)

	// Create a temporary directory for cloning
	tempDir, err := os.MkdirTemp("", "github-actions-clone-")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// Clone the repository
	cloneCmd := []string{"git", "clone", "--depth=1", repoURL, tempDir}
	if err := runGitCommand(ctx, log, ".", cloneCmd); err != nil {
		return fmt.Errorf("failed to clone repository: %w", err)
	}

	// Checkout specific ref if not main/master
	if ref != "main" && ref != "master" && !strings.HasPrefix(ref, "refs/heads/") {
		// For SHA or specific refs, we need to fetch and checkout
		fetchCmd := []string{"git", "fetch", "origin", ref}
		if err := runGitCommand(ctx, log, tempDir, fetchCmd); err != nil {
			log.Warn(ctx, "Failed to fetch specific ref %s, using default: %v", ref, err)
		} else {
			checkoutCmd := []string{"git", "checkout", ref}
			if err := runGitCommand(ctx, log, tempDir, checkoutCmd); err != nil {
				log.Warn(ctx, "Failed to checkout ref %s, using default: %v", ref, err)
			}
		}
	}

	// Copy contents from temp directory to work directory
	if err := copyRepositoryContents(tempDir, workDir); err != nil {
		return err
	}

	// Initialize and update submodules if they exist
	return initializeSubmodules(ctx, log, workDir, token, repository)
}

// initializeSubmodules handles git submodule initialization and updates
func initializeSubmodules(ctx context.Context, log logger.Logger, workDir, token, repository string) error {
	// First check if .gitmodules exists
	gitmodulesPath := workDir + "/.gitmodules"
	if _, err := os.Stat(gitmodulesPath); os.IsNotExist(err) {
		log.Debug(ctx, "No .gitmodules found - skipping submodule initialization")
		return nil
	}

	log.Info(ctx, "Git submodules detected - initializing and updating...")

	// Fix for "detected dubious ownership" error - add workDir to safe.directory
	safeDirCmd := []string{"git", "config", "--global", "--add", "safe.directory", workDir}
	if err := runGitCommand(ctx, log, workDir, safeDirCmd); err != nil {
		log.Debug(ctx, "Failed to add safe.directory config: %v", err)
		// Continue anyway - this is best-effort
	}

	// Also add wildcard to cover any nested submodule directories
	wildcardCmd := []string{"git", "config", "--global", "--add", "safe.directory", "*"}
	if err := runGitCommand(ctx, log, workDir, wildcardCmd); err != nil {
		log.Debug(ctx, "Failed to add wildcard safe.directory: %v", err)
	}

	// Configure submodules to use the authenticated URL
	// This ensures submodules use the same token for authentication
	submoduleUpdateCmd := []string{
		"git", "submodule", "update", "--init", "--recursive",
		"--depth=1",
	}
	if err := runGitCommandWithAuth(ctx, log, workDir, submoduleUpdateCmd, token, repository); err != nil {
		log.Warn(ctx, "Failed to update submodules: %v", err)
		// Don't fail the entire operation if submodules fail - just warn
		return nil
	}

	log.Info(ctx, "Git submodules initialized successfully")
	return nil
}

// runGitCommandWithAuth executes a git command with authentication configured for submodules
func runGitCommandWithAuth(ctx context.Context, log logger.Logger, dir string, args []string, token, repository string) error {
	log.Info(ctx, "Executing git command with auth: %s", strings.Join(args, " "))

	// Set up environment with authentication for submodule URLs
	// This converts https://github.com/ URLs to use the token
	env := append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0",
		"GIT_SSH_COMMAND=ssh -o StrictHostKeyChecking=no",
		"GIT_ASKPASS=echo",
	)

	// If we have a token, configure git to use it for all HTTPS URLs
	if token != "" {
		// Use git config to set credentials for the specific repository
		cmd := exec.Command("git", "config", "--local", "credential.helper", "")
		cmd.Dir = dir
		cmd.Env = env
		_ = cmd.Run() // Ignore errors, this just clears the default helper

		// Configure submodule URLs to use authentication
		// This uses a credential helper that injects the token
		submoduleConfig := fmt.Sprintf("https://x-access-token:%s@github.com/%s.git", token, repository)
		cmd = exec.Command("git", "config", "--local", "url."+submoduleConfig+".insteadOf", "https://github.com/")
		cmd.Dir = dir
		cmd.Env = env
		if output, err := cmd.CombinedOutput(); err != nil {
			log.Debug(ctx, "Failed to configure submodule URL rewrite (HTTPS): %s", string(output))
			// Continue anyway - this is a best-effort configuration
		}

		// Also rewrite SSH URLs (git@github.com:) to use HTTPS with token
		cmd = exec.Command("git", "config", "--local", "url."+submoduleConfig+".insteadOf", "git@github.com:")
		cmd.Dir = dir
		cmd.Env = env
		if output, err := cmd.CombinedOutput(); err != nil {
			log.Debug(ctx, "Failed to configure submodule URL rewrite (SSH): %s", string(output))
			// Continue anyway - this is a best-effort configuration
		}
	}

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = dir
	cmd.Env = env

	output, err := cmd.CombinedOutput()
	if err != nil {
		outputStr := string(output)
		if strings.Contains(outputStr, "could not read Username") ||
			strings.Contains(outputStr, "permission denied") ||
			strings.Contains(outputStr, "Authentication failed") {
			log.Error(ctx, "Submodule authentication failed - check token permissions")
			return fmt.Errorf("submodule authentication failed")
		}
		log.Error(ctx, "Git submodule command failed: %s, output: %s", strings.Join(args, " "), outputStr)
		return fmt.Errorf("git submodule command failed: %w", err)
	}

	return nil
}

// runGitCommand executes a git command with proper logging and error handling
func runGitCommand(ctx context.Context, log logger.Logger, dir string, args []string) error {
	// Create a safe version of args for logging (mask tokens)
	safeArgs := make([]string, len(args))
	copy(safeArgs, args)
	for i, arg := range safeArgs {
		if strings.Contains(arg, "x-access-token:") {
			// Replace token with masked version for logging
			safeArgs[i] = strings.ReplaceAll(arg,
				arg[strings.Index(arg, "x-access-token:"):strings.Index(arg, "@")],
				"x-access-token:***")
		}
	}

	log.Info(ctx, "Executing git command: %s", strings.Join(safeArgs, " "))

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0",
		"GIT_SSH_COMMAND=ssh -o StrictHostKeyChecking=no",
		"GIT_ASKPASS=echo", // Prevent any password prompts
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		// Provide specific error context based on the output
		outputStr := string(output)
		if strings.Contains(outputStr, "could not read Username") {
			log.Error(ctx, "Authentication failed - GITHUB_TOKEN may be invalid or missing")
			return fmt.Errorf("git authentication failed: ensure GITHUB_TOKEN is set and valid for repository access")
		} else if strings.Contains(outputStr, "repository not found") {
			log.Error(ctx, "Repository not found - check repository name and access permissions")
			return fmt.Errorf("git clone failed: repository not found or access denied")
		} else if strings.Contains(outputStr, "permission denied") {
			log.Error(ctx, "Permission denied - GITHUB_TOKEN may lack repository access")
			return fmt.Errorf("git clone failed: permission denied, check token permissions")
		}

		// Generic error with masked output for logging
		maskedOutput := strings.ReplaceAll(outputStr,
			os.Getenv("GITHUB_TOKEN"), "***")
		log.Error(ctx, "Git command failed: %s, output: %s", strings.Join(safeArgs, " "), maskedOutput)
		return fmt.Errorf("git command failed: %w", err)
	}

	log.Info(ctx, "Git command completed successfully")
	return nil
}

// copyRepositoryContents copies all contents from source to destination directory
func copyRepositoryContents(srcDir, dstDir string) error {
	// Remove any existing content in destination (except .git if it exists)
	entries, err := os.ReadDir(dstDir)
	if err == nil {
		for _, entry := range entries {
			if entry.Name() != ".git" {
				path := dstDir + "/" + entry.Name()
				os.RemoveAll(path)
			}
		}
	}

	// Copy all contents from source
	srcEntries, err := os.ReadDir(srcDir)
	if err != nil {
		return fmt.Errorf("failed to read source directory: %w", err)
	}

	for _, entry := range srcEntries {
		srcPath := srcDir + "/" + entry.Name()
		dstPath := dstDir + "/" + entry.Name()

		if entry.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				return fmt.Errorf("failed to copy directory %s: %w", entry.Name(), err)
			}
		} else {
			if err := copyFile(srcPath, dstPath); err != nil {
				return fmt.Errorf("failed to copy file %s: %w", entry.Name(), err)
			}
		}
	}

	return nil
}

// copyDir recursively copies a directory
func copyDir(src, dst string) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dst, srcInfo.Mode()); err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := src + "/" + entry.Name()
		dstPath := dst + "/" + entry.Name()

		if entry.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}

	return nil
}

// copyFile copies a single file
func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	if _, err := srcFile.WriteTo(dstFile); err != nil {
		return err
	}

	// Copy file permissions
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	return os.Chmod(dst, srcInfo.Mode())
}
