package cmd_cicd

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/simple-container-com/api/pkg/api/logger/color"
	"github.com/simple-container-com/api/pkg/clouds/github"
	"github.com/simple-container-com/api/pkg/cmd/root_cmd"
)

type SyncParams struct {
	ConfigFile     string
	StackName      string
	WorkflowsDir   string
	DryRun         bool
	Force          bool
	BackupExisting bool
	Verbose        bool
}

func NewSyncCmd(rootCmd *root_cmd.RootCmd) *cobra.Command {
	params := SyncParams{
		ConfigFile:     "server.yaml",
		WorkflowsDir:   ".github/workflows",
		BackupExisting: true,
	}

	cmd := &cobra.Command{
		Use:   "sync [stack-name]",
		Short: "Synchronize existing workflow files with server.yaml configuration",
		Long: `Synchronize existing GitHub Actions workflow files with the current CI/CD 
configuration in server.yaml. This command updates outdated workflows and 
creates missing ones while preserving existing customizations where possible.

Examples:
  # Sync workflows for a specific stack
  sc cicd sync myapp

  # Preview changes without applying them
  sc cicd sync myapp --dry-run

  # Force sync without backing up existing files
  sc cicd sync myapp --force --no-backup

  # Sync with custom workflows directory
  sc cicd sync myapp --workflows-dir .github/custom-workflows`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			params.StackName = args[0]
			return runSync(rootCmd, params)
		},
	}

	cmd.Flags().StringVarP(&params.ConfigFile, "config", "c", params.ConfigFile, "Server config file path")
	cmd.Flags().StringVarP(&params.WorkflowsDir, "workflows-dir", "w", params.WorkflowsDir, "GitHub workflows directory")
	cmd.Flags().BoolVar(&params.DryRun, "dry-run", params.DryRun, "Preview changes without applying them")
	cmd.Flags().BoolVar(&params.Force, "force", params.Force, "Force sync without confirmation")
	cmd.Flags().BoolVar(&params.BackupExisting, "backup", params.BackupExisting, "Backup existing files before modification")
	cmd.Flags().BoolVar(&params.Verbose, "verbose", params.Verbose, "Verbose output")

	return cmd
}

func runSync(rootCmd *root_cmd.RootCmd, params SyncParams) error {
	fmt.Printf("%s Synchronizing CI/CD workflows...\n", color.BlueString("🔄"))

	// Read and validate server configuration
	serverConfig, err := readServerConfig(params.ConfigFile)
	if err != nil {
		return fmt.Errorf("failed to read server config: %w", err)
	}

	stackName := params.StackName
	if stackName == "" {
		stackName = "default-stack"
	}

	fmt.Printf("📋 Stack: %s\n", color.CyanString(stackName))
	fmt.Printf("📁 Config file: %s\n", color.CyanString(params.ConfigFile))
	fmt.Printf("📂 Workflows directory: %s\n", color.CyanString(params.WorkflowsDir))

	// Extract CI/CD configuration
	if serverConfig.CiCd.Type != github.CiCdTypeGithubActions {
		return fmt.Errorf("no GitHub Actions CI/CD configuration found in %s", params.ConfigFile)
	}

	// Create enhanced config based on server descriptor
	enhancedConfig := createEnhancedConfig(serverConfig, stackName)

	fmt.Printf("🏢 Organization: %s\n", color.GreenString(enhancedConfig.Organization.Name))
	fmt.Printf("📄 Templates: %v\n", enhancedConfig.WorkflowGeneration.Templates)

	if params.DryRun {
		fmt.Printf("\n%s Dry run mode - no files will be modified\n", color.YellowString("🔍"))
		return previewSync(enhancedConfig, stackName, params.WorkflowsDir)
	}

	// Ensure workflows directory exists
	if err := os.MkdirAll(params.WorkflowsDir, 0o755); err != nil {
		return fmt.Errorf("failed to create workflows directory: %w", err)
	}

	// Get sync plan
	fmt.Printf("\n%s Analyzing existing workflows...\n", color.BlueString("📊"))

	generator := github.NewWorkflowGenerator(enhancedConfig, stackName, params.WorkflowsDir)
	syncPlan, err := generator.GetSyncPlan()
	if err != nil {
		return fmt.Errorf("failed to create sync plan: %w", err)
	}

	if syncPlan.IsUpToDate() {
		fmt.Printf("\n%s All workflows are already up-to-date! ✨\n", color.GreenString("✅"))
		return nil
	}

	// Display sync plan
	displaySyncPlan(syncPlan, params.Verbose)

	// Get confirmation if not forced
	if !params.Force {
		fmt.Printf("\nProceed with sync? [y/N]: ")
		var response string
		_, _ = fmt.Scanln(&response)
		if response != "y" && response != "Y" {
			fmt.Println("Sync cancelled.")
			return nil
		}
	}

	// Backup existing files if requested
	if params.BackupExisting {
		fmt.Printf("\n%s Creating backups...\n", color.BlueString("💾"))
		if err := createBackups(syncPlan, params.WorkflowsDir); err != nil {
			return fmt.Errorf("failed to create backups: %w", err)
		}
	}

	// Execute sync
	fmt.Printf("\n%s Synchronizing workflows...\n", color.GreenString("🚀"))

	if err := generator.SyncWorkflows(syncPlan); err != nil {
		return fmt.Errorf("failed to sync workflows: %w", err)
	}

	fmt.Printf("\n%s Workflow synchronization completed successfully!\n", color.GreenString("✅"))

	// Show summary
	displaySyncSummary(syncPlan)

	return nil
}

func previewSync(config *github.EnhancedActionsCiCdConfig, stackName, workflowsDir string) error {
	generator := github.NewWorkflowGenerator(config, stackName, workflowsDir)
	syncPlan, err := generator.GetSyncPlan()
	if err != nil {
		return fmt.Errorf("failed to create sync plan: %w", err)
	}

	if syncPlan.IsUpToDate() {
		fmt.Printf("\n%s All workflows are already up-to-date! ✨\n", color.GreenString("✅"))
		return nil
	}

	fmt.Printf("\n%s Changes that would be made:\n", color.BlueString("📋"))
	displaySyncPlan(syncPlan, true)

	return nil
}

func displaySyncPlan(plan *github.SyncPlan, verbose bool) {
	if len(plan.FilesToCreate) > 0 {
		fmt.Printf("\n%s Files to create:\n", color.GreenString("📄"))
		for _, file := range plan.FilesToCreate {
			fmt.Printf("  + %s\n", color.GreenString(file))
		}
	}

	if len(plan.FilesToUpdate) > 0 {
		fmt.Printf("\n%s Files to update:\n", color.YellowString("🔄"))
		for _, update := range plan.FilesToUpdate {
			fmt.Printf("  ~ %s", color.YellowString(update.File))
			if verbose && len(update.Changes) > 0 {
				fmt.Printf(" (%d changes)\n", len(update.Changes))
				for _, change := range update.Changes {
					fmt.Printf("    - %s\n", change)
				}
			} else {
				fmt.Println()
			}
		}
	}

	if len(plan.FilesToRemove) > 0 {
		fmt.Printf("\n%s Obsolete files (will be backed up):\n", color.RedString("🗑️"))
		for _, file := range plan.FilesToRemove {
			fmt.Printf("  - %s\n", color.RedString(file))
		}
	}

	fmt.Printf("\n%s Summary: %s\n", color.BlueString("📊"),
		color.CyanString(fmt.Sprintf("%d to create, %d to update, %d to remove",
			len(plan.FilesToCreate), len(plan.FilesToUpdate), len(plan.FilesToRemove))))
}

func createBackups(plan *github.SyncPlan, workflowsDir string) error {
	timestamp := time.Now().Format("20060102-150405")
	backupDir := filepath.Join(workflowsDir, ".backup", timestamp)

	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return fmt.Errorf("failed to create backup directory: %w", err)
	}

	// Backup files that will be updated
	for _, update := range plan.FilesToUpdate {
		srcFile := update.File
		srcPath := filepath.Join(workflowsDir, srcFile)
		dstPath := filepath.Join(backupDir, srcFile)

		// Ensure destination directory exists
		if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
			return fmt.Errorf("failed to create backup subdirectory: %w", err)
		}

		// Copy file
		if err := copyFile(srcPath, dstPath); err != nil {
			return fmt.Errorf("failed to backup %s: %w", srcFile, err)
		}

		fmt.Printf("  💾 %s → %s\n", srcFile, filepath.Join(".backup", timestamp, srcFile))
	}

	// Backup files that will be removed
	for _, srcFile := range plan.FilesToRemove {
		srcPath := filepath.Join(workflowsDir, srcFile)
		dstPath := filepath.Join(backupDir, srcFile)

		// Ensure destination directory exists
		if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
			return fmt.Errorf("failed to create backup subdirectory: %w", err)
		}

		// Copy file
		if err := copyFile(srcPath, dstPath); err != nil {
			return fmt.Errorf("failed to backup %s: %w", srcFile, err)
		}

		fmt.Printf("  💾 %s → %s\n", srcFile, filepath.Join(".backup", timestamp, srcFile))
	}

	return nil
}

func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = destFile.ReadFrom(sourceFile)
	return err
}

func displaySyncSummary(plan *github.SyncPlan) {
	fmt.Printf("\n%s Sync completed:\n", color.BlueString("📊"))

	if len(plan.FilesToCreate) > 0 {
		fmt.Printf("  ✅ Created %d new workflow file(s)\n", len(plan.FilesToCreate))
	}

	if len(plan.FilesToUpdate) > 0 {
		fmt.Printf("  🔄 Updated %d existing workflow file(s)\n", len(plan.FilesToUpdate))
	}

	if len(plan.FilesToRemove) > 0 {
		fmt.Printf("  🗑️  Removed %d obsolete workflow file(s)\n", len(plan.FilesToRemove))
	}

	fmt.Printf("\n%s Next steps:\n", color.BlueString("💡"))
	fmt.Printf("  1. Review the synchronized workflow files\n")
	fmt.Printf("  2. Test the workflows in your repository\n")
	fmt.Printf("  3. Commit and push the changes\n")
}
