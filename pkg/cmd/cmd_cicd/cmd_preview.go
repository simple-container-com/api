package cmd_cicd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/simple-container-com/api/pkg/api/logger/color"
	"github.com/simple-container-com/api/pkg/assistant/cicd"
	"github.com/simple-container-com/api/pkg/cmd/root_cmd"
)

type PreviewParams struct {
	ConfigFile  string
	StackName   string
	Output      string
	ShowContent bool
	ShowDiff    bool
	Format      string
	Verbose     bool
	Parent      bool
	Staging     bool
}

func NewPreviewCmd(rootCmd *root_cmd.RootCmd) *cobra.Command {
	params := PreviewParams{
		ConfigFile: "server.yaml",
		Format:     "summary", // summary, detailed, json
	}

	cmd := &cobra.Command{
		Use:   "preview",
		Short: "Preview workflow files that would be generated",
		Long: `Preview the GitHub Actions workflow files that would be generated based on 
the CI/CD configuration in server.yaml. This command shows the expected 
workflow structure, content, and configuration without creating any files.

Examples:
  # Preview workflows for a specific stack
  sc cicd preview --stack myapp

  # Preview with detailed content
  sc cicd preview --stack myapp --show-content --verbose

  # Preview and show differences with existing files
  sc cicd preview --stack myapp --show-diff

  # Save preview to a file
  sc cicd preview --stack myapp --output preview.yaml --format detailed`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPreview(rootCmd, params)
		},
	}

	cmd.Flags().StringVarP(&params.StackName, "stack", "s", "", "Stack name (required)")
	cmd.Flags().StringVarP(&params.ConfigFile, "config", "c", params.ConfigFile, "Server config file path")
	cmd.Flags().StringVarP(&params.Output, "output", "o", params.Output, "Output file for preview (optional)")
	cmd.Flags().BoolVar(&params.ShowContent, "show-content", params.ShowContent, "Show workflow file contents")
	cmd.Flags().BoolVar(&params.ShowDiff, "show-diff", params.ShowDiff, "Show differences with existing files")
	cmd.Flags().StringVar(&params.Format, "format", params.Format, "Output format: summary, detailed, json")
	cmd.Flags().BoolVarP(&params.Verbose, "verbose", "v", params.Verbose, "Verbose output")
	cmd.Flags().BoolVar(&params.Parent, "parent", params.Parent, "Preview workflows for parent stack (infrastructure/provisioning)")
	cmd.Flags().BoolVar(&params.Staging, "staging", params.Staging, "Generate workflows optimized for staging branch instead of main")

	_ = cmd.MarkFlagRequired("stack")

	return cmd
}

func runPreview(rootCmd *root_cmd.RootCmd, params PreviewParams) error {
	fmt.Printf("%s Generating workflow preview...\n", color.BlueString("👀"))

	// Create CI/CD service and run preview
	service := cicd.NewService()

	serviceParams := cicd.PreviewParams{
		StackName:   params.StackName,
		ConfigFile:  params.ConfigFile,
		ShowContent: params.ShowContent,
		Parent:      params.Parent,
		Staging:     params.Staging,
	}

	result, err := service.PreviewWorkflows(serviceParams)
	if err != nil {
		return fmt.Errorf("failed to generate preview: %w", err)
	}

	if !result.Success {
		return fmt.Errorf("%s", result.Message)
	}

	// Display basic info
	if stackName, ok := result.Data["stack_name"].(string); ok {
		fmt.Printf("📋 Stack: %s\n", color.CyanString(stackName))
	}
	if organization, ok := result.Data["organization"].(string); ok {
		fmt.Printf("🏢 Organization: %s\n", color.CyanString(organization))
	}

	// Display preview content
	fmt.Printf("\n%s\n", result.Message)

	// Show additional details if verbose
	if params.Verbose {
		if templates, ok := result.Data["templates"].([]string); ok {
			fmt.Printf("\n%s Templates:\n", color.BlueString("📋"))
			for _, template := range templates {
				fmt.Printf("  - %s\n", template)
			}
		}

		if environments, ok := result.Data["environments"].([]string); ok {
			fmt.Printf("\n%s Environments: %s\n", color.BlueString("🌐"), strings.Join(environments, ", "))
		}
	}

	return nil
}
