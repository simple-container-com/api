package cmd_cicd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/simple-container-com/api/pkg/api/logger/color"
	"github.com/simple-container-com/api/pkg/assistant/cicd"
	"github.com/simple-container-com/api/pkg/cmd/root_cmd"
)

type SyncParams struct {
	CICDCommonParams
	WorkflowsDir   string
	DryRun         bool
	Force          bool
	BackupExisting bool
	Verbose        bool
}

func NewSyncCmd(rootCmd *root_cmd.RootCmd) *cobra.Command {
	params := SyncParams{
		CICDCommonParams: CICDCommonParams{
			ConfigFile: "server.yaml",
		},
		WorkflowsDir:   ".github/workflows",
		BackupExisting: true,
	}

	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Synchronize existing workflow files with server.yaml configuration",
		Long: `Synchronize existing GitHub Actions workflow files with the current CI/CD 
configuration in server.yaml. This command updates outdated workflows and 
creates missing ones while preserving existing customizations where possible.

Examples:
  # Sync workflows for a specific stack
  sc cicd sync --stack myapp

  # Preview changes without applying them
  sc cicd sync --stack myapp --dry-run

  # Force sync without backing up existing files
  sc cicd sync --stack myapp --force --no-backup

  # Sync with custom workflows directory
  sc cicd sync --stack myapp --workflows-dir .github/custom-workflows`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSync(rootCmd, params)
		},
	}

	// Register common CI/CD flags
	RegisterCICDCommonFlags(cmd, &params.CICDCommonParams, params.ConfigFile)

	// Register sync-specific flags
	cmd.Flags().StringVarP(&params.WorkflowsDir, "workflows-dir", "w", params.WorkflowsDir, "GitHub workflows directory")
	cmd.Flags().BoolVar(&params.DryRun, "dry-run", params.DryRun, "Preview changes without applying them")
	cmd.Flags().BoolVar(&params.Force, "force", params.Force, "Force sync without confirmation")
	cmd.Flags().BoolVar(&params.BackupExisting, "backup", params.BackupExisting, "Backup existing files before modification")
	cmd.Flags().BoolVar(&params.Verbose, "verbose", params.Verbose, "Verbose output")

	return cmd
}

func runSync(rootCmd *root_cmd.RootCmd, params SyncParams) error {
	fmt.Printf("%s Synchronizing CI/CD workflows...\n", color.BlueString("🔄"))

	// Create CI/CD service and run sync
	service := cicd.NewService()

	serviceParams := cicd.SyncParams{
		StackName:  params.StackName,
		ConfigFile: params.ConfigFile,
		DryRun:     params.DryRun,
		Force:      params.Force,
		Parent:     params.Parent,
		Staging:    params.Staging,
	}

	result, err := service.SyncWorkflows(serviceParams)
	if err != nil {
		return fmt.Errorf("failed to sync CI/CD workflows: %w", err)
	}

	if !result.Success {
		// Handle interactive confirmation
		if needsConfirmation, ok := result.Data["needs_confirmation"].(bool); ok && needsConfirmation {
			fmt.Printf("\n%s\n", result.Message)

			// Ask for confirmation
			fmt.Print(color.YellowString("Continue with sync? [y/N]: "))
			scanner := bufio.NewScanner(os.Stdin)
			scanner.Scan()
			response := strings.ToLower(strings.TrimSpace(scanner.Text()))

			if response == "y" || response == "yes" {
				// User confirmed, retry with force
				serviceParams.Force = true
				result, err = service.SyncWorkflows(serviceParams)
				if err != nil {
					return fmt.Errorf("failed to sync CI/CD workflows: %w", err)
				}
				if !result.Success {
					return fmt.Errorf("%s", result.Message)
				}
			} else {
				fmt.Printf("%s Sync cancelled by user.\n", color.RedString("❌"))
				return nil
			}
		} else {
			// Handle other error cases
			if existingFiles, ok := result.Data["existing_files"].([]string); ok {
				fmt.Printf("\n%s Existing workflow files found:\n", color.YellowString("⚠️"))
				for _, file := range existingFiles {
					fmt.Printf("  - %s\n", file)
				}
				fmt.Printf("\nUse --force to overwrite existing files\n")
			}
			return fmt.Errorf("%s", result.Message)
		}
	}

	// Display basic info
	if stackName, ok := result.Data["stack_name"].(string); ok {
		fmt.Printf("📋 Stack: %s\n", color.CyanString(stackName))
	}
	if configFile, ok := result.Data["config_file"].(string); ok {
		fmt.Printf("📁 Config file: %s\n", color.CyanString(configFile))
	}
	if workflowsDir, ok := result.Data["workflows_dir"].(string); ok {
		fmt.Printf("📂 Workflows directory: %s\n", color.CyanString(workflowsDir))
	}

	// Success - display result
	fmt.Printf("\n%s\n", result.Message)

	// Show additional details if verbose
	if params.Verbose {
		if files, ok := result.Data["synchronized_files"].([]string); ok && len(files) > 0 {
			fmt.Printf("\n%s Files Synchronized:\n", color.BlueString("📄"))
			for _, file := range files {
				fmt.Printf("  - %s\n", file)
			}
		}

		if templates, ok := result.Data["templates"].([]string); ok && len(templates) > 0 {
			fmt.Printf("\n%s Templates:\n", color.BlueString("📋"))
			for _, template := range templates {
				fmt.Printf("  - %s\n", template)
			}
		}

		if environments, ok := result.Data["environments"].([]string); ok && len(environments) > 0 {
			fmt.Printf("\n%s Environments: %s\n", color.BlueString("🌐"), strings.Join(environments, ", "))
		}
	}

	// Show next steps
	fmt.Printf("\n%s Next steps:\n", color.BlueString("💡"))
	fmt.Printf("  1. Review the synchronized workflow files\n")
	fmt.Printf("  2. Test the workflows in your repository\n")
	fmt.Printf("  3. Commit and push the changes\n")

	return nil
}
