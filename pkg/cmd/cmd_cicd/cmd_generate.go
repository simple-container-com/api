package cmd_cicd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/simple-container-com/api/pkg/api/logger/color"
	"github.com/simple-container-com/api/pkg/clouds/github"
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

	// Process stack name and auto-detect config file
	stackName := processStackName(params.StackName)
	configFile, err := autoDetectConfigFile(params.ConfigFile, stackName)
	if err != nil {
		return err
	}

	fmt.Printf("📖 Reading configuration from: %s\n", color.CyanString(configFile))

	// Load and validate server configuration
	serverDesc, err := validateAndLoadServerConfig(configFile)
	if err != nil {
		return err
	}

	// Configuration and type validation already done in validateAndLoadServerConfig

	// Create enhanced config with logging
	enhancedConfig := setupEnhancedConfigWithLogging(serverDesc, stackName, configFile)

	// Check output directory
	outputDir := params.Output
	if !filepath.IsAbs(outputDir) {
		abs, err := filepath.Abs(outputDir)
		if err != nil {
			return fmt.Errorf("failed to resolve output path: %w", err)
		}
		outputDir = abs
	}

	fmt.Printf("📁 Output directory: %s\n", color.CyanString(outputDir))

	if params.DryRun {
		fmt.Printf("\n%s Dry run mode - no files will be written\n", color.YellowString("🔍"))
		return previewGeneration(enhancedConfig, stackName, outputDir)
	}

	// Check for existing files
	if !params.Force {
		existingFiles := checkExistingWorkflows(enhancedConfig, stackName, outputDir)
		if len(existingFiles) > 0 {
			fmt.Printf("\n%s Existing workflow files found:\n", color.YellowString("⚠️"))
			for _, file := range existingFiles {
				fmt.Printf("  - %s\n", file)
			}
			fmt.Printf("\nUse --force to overwrite existing files\n")
			return fmt.Errorf("workflow files already exist")
		}
	}

	// Generate workflows
	fmt.Printf("\n%s Generating workflows...\n", color.GreenString("🚀"))

	generator := github.NewWorkflowGenerator(enhancedConfig, stackName, outputDir)
	if err := generator.GenerateWorkflows(); err != nil {
		return fmt.Errorf("failed to generate workflows: %w", err)
	}

	fmt.Printf("\n%s Workflow generation completed successfully!\n", color.GreenString("✅"))
	fmt.Printf("\nGenerated workflows in: %s\n", color.CyanString(outputDir))

	// Show next steps
	fmt.Printf("\n%s Next steps:\n", color.BlueString("💡"))
	fmt.Printf("  1. Review the generated workflow files\n")
	fmt.Printf("  2. Commit and push the workflows to your repository\n")
	fmt.Printf("  3. Configure required secrets in your GitHub repository:\n")

	// Get required secrets based on configuration
	requiredSecrets := getRequiredSecrets(enhancedConfig)
	for _, secret := range requiredSecrets {
		fmt.Printf("     - %s\n", color.YellowString(secret))
	}

	if enhancedConfig.Notifications.SlackWebhook != "" {
		fmt.Printf("     - %s (for Slack notifications)\n", color.YellowString("SLACK_WEBHOOK_URL"))
	}
	if enhancedConfig.Notifications.DiscordWebhook != "" {
		fmt.Printf("     - %s (for Discord notifications)\n", color.YellowString("DISCORD_WEBHOOK_URL"))
	}

	return nil
}

func checkExistingWorkflows(config *github.EnhancedActionsCiCdConfig, stackName, outputDir string) []string {
	var existing []string

	for _, template := range config.WorkflowGeneration.Templates {
		filename := fmt.Sprintf("%s-%s.yml", template, stackName)
		filePath := filepath.Join(outputDir, filename)

		if _, err := os.Stat(filePath); err == nil {
			existing = append(existing, filePath)
		}
	}

	return existing
}

func previewGeneration(config *github.EnhancedActionsCiCdConfig, stackName, outputDir string) error {
	fmt.Printf("\n%s Files that would be generated:\n", color.BlueString("📋"))

	for _, template := range config.WorkflowGeneration.Templates {
		filename := fmt.Sprintf("%s-%s.yml", template, stackName)
		filePath := filepath.Join(outputDir, filename)
		fmt.Printf("  - %s\n", color.GreenString(filePath))
	}

	fmt.Printf("\n%s Configuration summary:\n", color.BlueString("📊"))
	fmt.Printf("  Organization: %s\n", config.Organization.Name)
	fmt.Printf("  Environments: %d\n", len(config.Environments))
	fmt.Printf("  Templates: %v\n", config.WorkflowGeneration.Templates)
	fmt.Printf("  Custom Actions: %v\n", config.WorkflowGeneration.CustomActions)

	return nil
}
