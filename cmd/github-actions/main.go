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

	// Initialize git repository - handle GitHub Actions environment properly
	var gitRepo git.Repo
	workDir, _ := os.Getwd()

	// Try to detect existing git repository with proper content first
	gitRepo, err := git.New(git.WithDetectRootDir())
	if err != nil || !isProperRepository(workDir) {
		if err != nil {
			log.Warn(ctx, "No git repository detected: %v", err)
		} else {
			log.Warn(ctx, "Git repository exists but appears to be empty or incomplete")
		}

		// Clone the repository like actions/checkout does
		if err := cloneRepository(ctx, log, workDir); err != nil {
			log.Error(ctx, "Failed to clone repository: %v", err)
			os.Exit(1)
		}

		// Try to initialize git repo again after cloning
		gitRepo, err = git.New(git.WithDetectRootDir())
		if err != nil {
			log.Error(ctx, "Failed to initialize git repository after cloning: %v", err)
			os.Exit(1)
		}

		log.Info(ctx, "Successfully initialized git repository from clone")
	} else {
		log.Info(ctx, "Using existing git repository")
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
	// Get required GitHub environment variables
	repository := os.Getenv("GITHUB_REPOSITORY")
	if repository == "" {
		return fmt.Errorf("GITHUB_REPOSITORY environment variable not set")
	}

	// Get token for authentication
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		log.Warn(ctx, "GITHUB_TOKEN not set, attempting clone without authentication")
	}

	// Get the ref to checkout (default to the current SHA)
	ref := os.Getenv("GITHUB_SHA")
	if ref == "" {
		ref = os.Getenv("GITHUB_REF")
		if ref == "" {
			ref = "main" // fallback
		}
	}

	log.Info(ctx, "Cloning repository %s at ref %s", repository, ref)

	// Construct the clone URL
	var repoURL string
	if token != "" {
		repoURL = fmt.Sprintf("https://x-access-token:%s@github.com/%s.git", token, repository)
	} else {
		repoURL = fmt.Sprintf("https://github.com/%s.git", repository)
	}

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
	return copyRepositoryContents(tempDir, workDir)
}

// runGitCommand executes a git command with proper logging
func runGitCommand(ctx context.Context, log logger.Logger, dir string, args []string) error {
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0",
		"GIT_SSH_COMMAND=ssh -o StrictHostKeyChecking=no",
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Error(ctx, "Git command failed: %s, output: %s", strings.Join(args, " "), string(output))
		return fmt.Errorf("git command failed: %w", err)
	}

	log.Info(ctx, "Git command successful: %s", strings.Join(args, " "))
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
