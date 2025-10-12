package cmd_cicd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/simple-container-com/api/pkg/api/logger/color"
	"github.com/simple-container-com/api/pkg/clouds/github"
	"github.com/simple-container-com/api/pkg/cmd/root_cmd"
)

type ValidateParams struct {
	ConfigFile   string
	StackName    string
	WorkflowsDir string
	ShowDiff     bool
	Verbose      bool
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

	_ = cmd.MarkFlagRequired("stack")

	return cmd
}

func runValidate(rootCmd *root_cmd.RootCmd, params ValidateParams) error {
	fmt.Printf("%s Validating CI/CD workflows...\n", color.BlueString("🔍"))

	// Process stack name and auto-detect config file
	stackName := processStackName(params.StackName)
	configFile, err := autoDetectConfigFile(params.ConfigFile, stackName)
	if err != nil {
		return err
	}

	// Load and validate server configuration
	serverConfig, err := validateAndLoadServerConfig(configFile)
	if err != nil {
		return err
	}

	fmt.Printf("📋 Stack: %s\n", color.CyanString(stackName))
	fmt.Printf("📁 Config file: %s\n", color.CyanString(configFile))
	fmt.Printf("📂 Workflows directory: %s\n", color.CyanString(params.WorkflowsDir))

	// Create enhanced config with logging
	enhancedConfig := setupEnhancedConfigWithLogging(serverConfig, stackName, configFile)

	// Validate workflows directory exists
	if _, err := os.Stat(params.WorkflowsDir); os.IsNotExist(err) {
		return fmt.Errorf("workflows directory does not exist: %s", params.WorkflowsDir)
	}

	// Perform validation
	fmt.Printf("\n%s Validating workflow files...\n", color.BlueString("📝"))

	generator := github.NewWorkflowGenerator(enhancedConfig, stackName, params.WorkflowsDir)
	validationResults, err := generator.ValidateWorkflows()
	if err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	// Display results
	return displayValidationResults(validationResults, params)
}

func displayValidationResults(results *github.ValidationResults, params ValidateParams) error {
	if results.IsValid {
		fmt.Printf("\n%s All workflow files are valid and up-to-date! ✨\n", color.GreenString("✅"))

		if params.Verbose {
			fmt.Printf("\n%s Validated files:\n", color.BlueString("📋"))
			for _, file := range results.ValidFiles {
				fmt.Printf("  ✅ %s\n", color.GreenString(file))
			}
		}
		return nil
	}

	fmt.Printf("\n%s Validation issues found:\n", color.RedString("❌"))

	// Show missing files
	if len(results.MissingFiles) > 0 {
		fmt.Printf("\n%s Missing workflow files:\n", color.YellowString("📄"))
		for _, file := range results.MissingFiles {
			fmt.Printf("  ❌ %s\n", color.RedString(file))
		}
	}

	// Show outdated files
	if len(results.OutdatedFiles) > 0 {
		fmt.Printf("\n%s Outdated workflow files:\n", color.YellowString("🔄"))
		for _, file := range results.OutdatedFiles {
			fmt.Printf("  ⚠️  %s\n", color.YellowString(file))
		}
	}

	// Show invalid files
	if len(results.InvalidFiles) > 0 {
		fmt.Printf("\n%s Invalid workflow files:\n", color.RedString("❌"))
		for file, issues := range results.InvalidFiles {
			fmt.Printf("  ❌ %s:\n", color.RedString(file))
			for _, issue := range issues {
				fmt.Printf("     - %s\n", issue)
			}
		}
	}

	// Show differences if requested
	if params.ShowDiff && len(results.Differences) > 0 {
		fmt.Printf("\n%s Differences found:\n", color.BlueString("📊"))
		for file, diffs := range results.Differences {
			fmt.Printf("\n%s %s:\n", color.CyanString("📄"), file)
			for _, diff := range diffs {
				fmt.Printf("  %s\n", diff)
			}
		}
	}

	// Show recommendations
	fmt.Printf("\n%s Recommendations:\n", color.BlueString("💡"))
	fmt.Printf("  1. Run %s to generate missing files\n",
		color.GreenString("sc cicd generate "+params.StackName))
	fmt.Printf("  2. Run %s to update outdated files\n",
		color.GreenString("sc cicd sync "+params.StackName))

	if len(results.InvalidFiles) > 0 {
		fmt.Printf("  3. Review and fix invalid workflow configurations\n")
	}

	return fmt.Errorf("validation failed: %d issues found", results.TotalIssues())
}
