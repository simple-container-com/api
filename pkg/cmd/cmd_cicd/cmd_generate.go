package cmd_cicd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/simple-container-com/api/pkg/api/logger/color"
	"github.com/simple-container-com/api/pkg/assistant/cicd"
	"github.com/simple-container-com/api/pkg/cmd/root_cmd"
)

type generateParams struct {
	StackName  string
	Output     string
	ConfigFile string
	Force      bool
	DryRun     bool
}

// NewGenerateCmd creates the generate subcommand
func NewGenerateCmd(rootCmd *root_cmd.RootCmd) *cobra.Command {
	params := &generateParams{}

	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate GitHub Actions workflows from server.yaml configuration",
		Long: `Generate GitHub Actions workflows from Simple Container server.yaml configuration.

This command reads the CI/CD configuration from server.yaml and generates
corresponding GitHub Actions workflow files. The generated workflows use
Simple Container's self-contained GitHub Actions for deployment, provisioning,
and destruction operations.

The generated workflows will be placed in the specified output directory
(default: .github/workflows/) and named according to the pattern:
  - deploy-<stack-name>.yml
  - destroy-<stack-name>.yml  
  - provision-<stack-name>.yml
  - pr-preview-<stack-name>.yml

Only workflows for templates specified in the CI/CD configuration will be generated.`,
		Example: `  # Generate workflows for myorg/infrastructure stack
  sc cicd generate --stack myorg/infrastructure

  # Generate to custom directory
  sc cicd generate --stack myorg/infrastructure --output ./workflows/

  # Use custom server.yaml file
  sc cicd generate --stack myorg/infrastructure --config ./custom-server.yaml

  # Dry run to see what would be generated
  sc cicd generate --stack myorg/infrastructure --dry-run

  # Force overwrite existing workflows
  sc cicd generate --stack myorg/infrastructure --force`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGenerate(rootCmd, params)
		},
	}

	cmd.Flags().StringVarP(&params.StackName, "stack", "s", "", "Stack name (required, format: org/name)")
	cmd.Flags().StringVarP(&params.Output, "output", "o", ".github/workflows/", "Output directory for generated workflows")
	cmd.Flags().StringVarP(&params.ConfigFile, "config", "c", "", "Path to server.yaml file (default: auto-detect)")
	cmd.Flags().BoolVar(&params.Force, "force", false, "Force overwrite existing workflow files")
	cmd.Flags().BoolVar(&params.DryRun, "dry-run", false, "Show what would be generated without writing files")

	_ = cmd.MarkFlagRequired("stack")

	return cmd
}

func runGenerate(rootCmd *root_cmd.RootCmd, params *generateParams) error {
	// Validate stack name
	if params.StackName == "" {
		return fmt.Errorf("stack name is required (use --stack flag)")
	}

	fmt.Printf("📖 Reading configuration...\n")

	// Create CI/CD service and run generation
	service := cicd.NewService()

	serviceParams := cicd.GenerateParams{
		StackName:  params.StackName,
		Output:     params.Output,
		ConfigFile: params.ConfigFile,
		Force:      params.Force,
		DryRun:     params.DryRun,
	}

	result, err := service.GenerateWorkflows(serviceParams)
	if err != nil {
		return fmt.Errorf("failed to generate CI/CD workflows: %w", err)
	}

	if !result.Success {
		// Handle specific error cases
		if existingFiles, ok := result.Data["existing_files"].([]string); ok {
			fmt.Printf("\n%s Existing workflow files found:\n", color.YellowString("⚠️"))
			for _, file := range existingFiles {
				fmt.Printf("  - %s\n", file)
			}
			fmt.Printf("\nUse --force to overwrite existing files\n")
		}
		return fmt.Errorf("%s", result.Message)
	}

	// Success - display result
	fmt.Printf("\n%s\n", result.Message)

	if requiredSecrets, ok := result.Data["required_secrets"].([]string); ok && len(requiredSecrets) > 0 {
		fmt.Printf("\n%s Next steps:\n", color.BlueString("💡"))
		fmt.Printf("  1. Review the generated workflow files\n")
		fmt.Printf("  2. Commit and push the workflows to your repository\n")
		fmt.Printf("  3. Configure required secrets in your GitHub repository:\n")

		for _, secret := range requiredSecrets {
			fmt.Printf("     - %s\n", color.YellowString(secret))
		}
	}

	return nil
}
