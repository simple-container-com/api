package cmd_cicd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/simple-container-com/api/pkg/api/logger/color"
	"github.com/simple-container-com/api/pkg/assistant/cicd"
	"github.com/simple-container-com/api/pkg/cmd/root_cmd"
)

type ValidateParams struct {
	ConfigFile   string
	StackName    string
	WorkflowsDir string
	ShowDiff     bool
	Verbose      bool
	Parent       bool
	Staging      bool
}

func NewValidateCmd(rootCmd *root_cmd.RootCmd) *cobra.Command {
	params := ValidateParams{
		ConfigFile:   "server.yaml",
		WorkflowsDir: ".github/workflows",
	}

	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate existing workflow files against server.yaml configuration",
		Long: `Validate existing GitHub Actions workflow files against the CI/CD configuration 
defined in server.yaml. This command checks if the workflows are up-to-date and 
consistent with the current configuration.

Examples:
  # Validate workflows for a specific stack
  sc cicd validate --stack myapp

  # Validate with custom workflows directory
  sc cicd validate --stack myapp --workflows-dir .github/custom-workflows

  # Show detailed differences
  sc cicd validate --stack myapp --show-diff --verbose`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runValidate(rootCmd, params)
		},
	}

	cmd.Flags().StringVarP(&params.StackName, "stack", "s", "", "Stack name (required)")
	cmd.Flags().StringVarP(&params.ConfigFile, "config", "c", params.ConfigFile, "Server config file path")
	cmd.Flags().StringVarP(&params.WorkflowsDir, "workflows-dir", "w", params.WorkflowsDir, "GitHub workflows directory")
	cmd.Flags().BoolVar(&params.ShowDiff, "show-diff", params.ShowDiff, "Show differences between expected and actual workflows")
	cmd.Flags().BoolVarP(&params.Verbose, "verbose", "v", params.Verbose, "Verbose output")
	cmd.Flags().BoolVar(&params.Parent, "parent", params.Parent, "Validate workflows for parent stack (infrastructure/provisioning)")
	cmd.Flags().BoolVar(&params.Staging, "staging", params.Staging, "Validate workflows optimized for staging branch instead of main")

	_ = cmd.MarkFlagRequired("stack")

	return cmd
}

func runValidate(rootCmd *root_cmd.RootCmd, params ValidateParams) error {
	fmt.Printf("%s Validating CI/CD workflows...\n", color.BlueString("🔍"))

	// Create CI/CD service and run validation
	service := cicd.NewService()

	serviceParams := cicd.ValidateParams{
		StackName:    params.StackName,
		ConfigFile:   params.ConfigFile,
		WorkflowsDir: params.WorkflowsDir,
		ShowDiff:     params.ShowDiff,
		Verbose:      params.Verbose,
		Parent:       params.Parent,
		Staging:      params.Staging,
	}

	result, err := service.ValidateWorkflows(serviceParams)
	if err != nil {
		return fmt.Errorf("validation failed: %w", err)
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

	// Display validation results
	fmt.Printf("\n%s\n", result.Message)

	if len(result.Warnings) > 0 {
		fmt.Printf("\n%s Validation Details:\n", color.BlueString("📝"))
		for _, warning := range result.Warnings {
			fmt.Printf("  %s\n", warning)
		}
	}

	if !result.Success {
		return fmt.Errorf("validation failed")
	}

	return nil
}
