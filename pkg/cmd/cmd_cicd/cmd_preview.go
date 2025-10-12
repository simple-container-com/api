package cmd_cicd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/simple-container-com/api/pkg/api/logger/color"
	"github.com/simple-container-com/api/pkg/clouds/github"
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
}

func NewPreviewCmd(rootCmd *root_cmd.RootCmd) *cobra.Command {
	params := PreviewParams{
		ConfigFile: "server.yaml",
		Format:     "summary", // summary, detailed, json
	}

	cmd := &cobra.Command{
		Use:   "preview [stack-name]",
		Short: "Preview workflow files that would be generated",
		Long: `Preview the GitHub Actions workflow files that would be generated based on 
the CI/CD configuration in server.yaml. This command shows the expected 
workflow structure, content, and configuration without creating any files.

Examples:
  # Preview workflows for a specific stack
  sc cicd preview myapp

  # Preview with detailed content
  sc cicd preview myapp --show-content --verbose

  # Preview and show differences with existing files
  sc cicd preview myapp --show-diff

  # Save preview to a file
  sc cicd preview myapp --output preview.yaml --format detailed`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			params.StackName = args[0]
			return runPreview(rootCmd, params)
		},
	}

	cmd.Flags().StringVarP(&params.ConfigFile, "config", "c", params.ConfigFile, "Server config file path")
	cmd.Flags().StringVarP(&params.Output, "output", "o", params.Output, "Output file for preview (optional)")
	cmd.Flags().BoolVar(&params.ShowContent, "show-content", params.ShowContent, "Show workflow file contents")
	cmd.Flags().BoolVar(&params.ShowDiff, "show-diff", params.ShowDiff, "Show differences with existing files")
	cmd.Flags().StringVar(&params.Format, "format", params.Format, "Output format: summary, detailed, json")
	cmd.Flags().BoolVarP(&params.Verbose, "verbose", "v", params.Verbose, "Verbose output")

	return cmd
}

func runPreview(rootCmd *root_cmd.RootCmd, params PreviewParams) error {
	fmt.Printf("%s Generating workflow preview...\n", color.BlueString("👀"))

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

	// Extract CI/CD configuration
	if serverConfig.CiCd.Type != github.CiCdTypeGithubActions {
		return fmt.Errorf("no GitHub Actions CI/CD configuration found in %s", params.ConfigFile)
	}

	// Create enhanced config based on server descriptor
	enhancedConfig := createEnhancedConfig(serverConfig, stackName)

	fmt.Printf("🏢 Organization: %s\n", color.GreenString(enhancedConfig.Organization.Name))
	fmt.Printf("📄 Templates: %v\n", enhancedConfig.WorkflowGeneration.Templates)
	fmt.Printf("🌐 Environments: %v\n", getEnvironmentNames(enhancedConfig.Environments))

	// Generate preview
	fmt.Printf("\n%s Generating preview...\n", color.BlueString("🔮"))

	// Use a temporary directory for preview generation
	tempDir := filepath.Join(os.TempDir(), "sc-cicd-preview", stackName)
	defer os.RemoveAll(tempDir)

	generator := github.NewWorkflowGenerator(enhancedConfig, stackName, tempDir)
	preview, err := generator.PreviewWorkflow()
	if err != nil {
		return fmt.Errorf("failed to generate preview: %w", err)
	}

	// Display or save preview
	if params.Output != "" {
		return savePreview(preview, params)
	}

	return displayPreview(preview, params)
}

func displayPreview(preview *github.WorkflowPreview, params PreviewParams) error {
	switch params.Format {
	case "summary":
		return displayPreviewSummary(preview, params)
	case "detailed":
		return displayPreviewDetailed(preview, params)
	case "json":
		return displayPreviewJSON(preview, params)
	default:
		return fmt.Errorf("unknown format: %s (supported: summary, detailed, json)", params.Format)
	}
}

func displayPreviewSummary(preview *github.WorkflowPreview, params PreviewParams) error {
	fmt.Printf("\n%s Workflow Preview Summary\n", color.BlueString("📋"))
	fmt.Printf("═══════════════════════════\n")

	fmt.Printf("\n%s Generated Workflows:\n", color.GreenString("📄"))
	for _, workflow := range preview.Workflows {
		fmt.Printf("  ✨ %s\n", color.CyanString(workflow.Name))
		fmt.Printf("     File: %s\n", workflow.FileName)
		fmt.Printf("     Jobs: %d\n", len(workflow.Jobs))

		if params.Verbose {
			for _, job := range workflow.Jobs {
				fmt.Printf("       - %s (%d steps)\n", job.Name, len(job.Steps))
			}
		}
	}

	fmt.Printf("\n%s Configuration Details:\n", color.BlueString("⚙️"))
	fmt.Printf("  Organization: %s\n", preview.Config.Organization.Name)
	fmt.Printf("  Environments: %d configured\n", len(preview.Config.Environments))
	fmt.Printf("  Custom Actions: %v\n", preview.Config.WorkflowGeneration.CustomActions)

	if preview.Config.Notifications.SlackWebhook != "" || preview.Config.Notifications.DiscordWebhook != "" {
		fmt.Printf("  Notifications: ")
		var notifyTypes []string
		if preview.Config.Notifications.SlackWebhook != "" {
			notifyTypes = append(notifyTypes, "Slack")
		}
		if preview.Config.Notifications.DiscordWebhook != "" {
			notifyTypes = append(notifyTypes, "Discord")
		}
		fmt.Printf("%s\n", color.GreenString(fmt.Sprintf("%v", notifyTypes)))
	}

	// Show differences if requested and applicable
	if params.ShowDiff {
		return showWorkflowDifferences(preview, params)
	}

	return nil
}

func displayPreviewDetailed(preview *github.WorkflowPreview, params PreviewParams) error {
	fmt.Printf("\n%s Detailed Workflow Preview\n", color.BlueString("📋"))
	fmt.Printf("═══════════════════════════════\n")

	for i, workflow := range preview.Workflows {
		if i > 0 {
			fmt.Printf("%s", "\n"+strings.Repeat("─", 50)+"\n")
		}

		fmt.Printf("\n%s Workflow: %s\n", color.GreenString("📄"), color.CyanString(workflow.Name))
		fmt.Printf("File: %s\n", workflow.FileName)
		fmt.Printf("Description: %s\n", workflow.Description)

		// Show triggers
		if len(workflow.Triggers) > 0 {
			fmt.Printf("\n%s Triggers:\n", color.YellowString("🎯"))
			for _, trigger := range workflow.Triggers {
				fmt.Printf("  - %s\n", trigger)
			}
		}

		// Show jobs
		fmt.Printf("\n%s Jobs:\n", color.BlueString("🏃"))
		for _, job := range workflow.Jobs {
			fmt.Printf("  %s %s\n", color.CyanString("📋"), job.Name)
			fmt.Printf("    Runner: %s\n", job.Runner)
			if job.Environment != "" {
				fmt.Printf("    Environment: %s\n", job.Environment)
			}

			fmt.Printf("    Steps (%d):\n", len(job.Steps))
			for _, step := range job.Steps {
				fmt.Printf("      - %s\n", step.Name)
				if params.Verbose && step.Action != "" {
					fmt.Printf("        Uses: %s\n", step.Action)
				}
			}
		}

		// Show content if requested
		if params.ShowContent {
			fmt.Printf("\n%s Workflow Content:\n", color.BlueString("📝"))
			fmt.Printf("```yaml\n%s```\n", workflow.Content)
		}
	}

	return nil
}

func displayPreviewJSON(preview *github.WorkflowPreview, params PreviewParams) error {
	// This would marshal the preview struct to JSON
	fmt.Printf("{\n")
	fmt.Printf("  \"stack_name\": \"%s\",\n", preview.StackName)
	fmt.Printf("  \"workflows\": [\n")

	for i, workflow := range preview.Workflows {
		if i > 0 {
			fmt.Printf(",\n")
		}
		fmt.Printf("    {\n")
		fmt.Printf("      \"name\": \"%s\",\n", workflow.Name)
		fmt.Printf("      \"file_name\": \"%s\",\n", workflow.FileName)
		fmt.Printf("      \"description\": \"%s\",\n", workflow.Description)
		fmt.Printf("      \"jobs_count\": %d\n", len(workflow.Jobs))
		fmt.Printf("    }")
	}

	fmt.Printf("\n  ]\n")
	fmt.Printf("}\n")
	return nil
}

func showWorkflowDifferences(preview *github.WorkflowPreview, params PreviewParams) error {
	fmt.Printf("\n%s Comparing with existing workflows...\n", color.BlueString("🔍"))

	workflowsDir := ".github/workflows"
	foundDifferences := false

	for _, workflow := range preview.Workflows {
		existingPath := filepath.Join(workflowsDir, workflow.FileName)

		if _, err := os.Stat(existingPath); os.IsNotExist(err) {
			fmt.Printf("  + %s (new file)\n", color.GreenString(workflow.FileName))
			foundDifferences = true
			continue
		}

		// Read existing file
		existingContent, err := os.ReadFile(existingPath)
		if err != nil {
			fmt.Printf("  ? %s (could not read existing file)\n", color.YellowString(workflow.FileName))
			continue
		}

		// Compare content
		if string(existingContent) != workflow.Content {
			fmt.Printf("  ~ %s (modified)\n", color.YellowString(workflow.FileName))
			foundDifferences = true

			if params.Verbose {
				// Show simplified diff (just indicate changes)
				fmt.Printf("    Content differs from existing file\n")
			}
		} else {
			fmt.Printf("  = %s (unchanged)\n", color.GreenString(workflow.FileName))
		}
	}

	if !foundDifferences {
		fmt.Printf("  %s All workflows match existing files\n", color.GreenString("✅"))
	}

	return nil
}

func savePreview(preview *github.WorkflowPreview, params PreviewParams) error {
	fmt.Printf("💾 Saving preview to: %s\n", color.CyanString(params.Output))

	file, err := os.Create(params.Output)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer file.Close()

	// Write preview content based on format
	switch params.Format {
	case "summary", "detailed":
		return writePreviewText(file, preview, params)
	case "json":
		return writePreviewJSON(file, preview, params)
	default:
		return fmt.Errorf("unsupported output format: %s", params.Format)
	}
}

func writePreviewText(file *os.File, preview *github.WorkflowPreview, params PreviewParams) error {
	// Write text-based preview to file
	if _, err := file.WriteString(fmt.Sprintf("# Workflow Preview for %s\n\n", preview.StackName)); err != nil {
		return err
	}

	for _, workflow := range preview.Workflows {
		if _, err := file.WriteString(fmt.Sprintf("## %s\n", workflow.Name)); err != nil {
			return err
		}
		if _, err := file.WriteString(fmt.Sprintf("File: %s\n", workflow.FileName)); err != nil {
			return err
		}
		if _, err := file.WriteString(fmt.Sprintf("Jobs: %d\n\n", len(workflow.Jobs))); err != nil {
			return err
		}

		if params.ShowContent {
			if _, err := file.WriteString("### Content:\n"); err != nil {
				return err
			}
			if _, err := file.WriteString("```yaml\n"); err != nil {
				return err
			}
			if _, err := file.WriteString(workflow.Content); err != nil {
				return err
			}
			if _, err := file.WriteString("\n```\n\n"); err != nil {
				return err
			}
		}
	}

	return nil
}

func writePreviewJSON(file *os.File, preview *github.WorkflowPreview, params PreviewParams) error {
	// Write JSON preview to file
	if _, err := file.WriteString("{\n"); err != nil {
		return err
	}
	if _, err := file.WriteString(fmt.Sprintf("  \"stack_name\": \"%s\",\n", preview.StackName)); err != nil {
		return err
	}
	if _, err := file.WriteString("  \"workflows\": [\n"); err != nil {
		return err
	}

	for i, workflow := range preview.Workflows {
		if i > 0 {
			if _, err := file.WriteString(",\n"); err != nil {
				return err
			}
		}
		if _, err := file.WriteString("    {\n"); err != nil {
			return err
		}
		if _, err := file.WriteString(fmt.Sprintf("      \"name\": \"%s\",\n", workflow.Name)); err != nil {
			return err
		}
		if _, err := file.WriteString(fmt.Sprintf("      \"file_name\": \"%s\",\n", workflow.FileName)); err != nil {
			return err
		}
		if _, err := file.WriteString(fmt.Sprintf("      \"description\": \"%s\"", workflow.Description)); err != nil {
			return err
		}

		if params.ShowContent {
			if _, err := file.WriteString(",\n"); err != nil {
				return err
			}
			if _, err := file.WriteString(fmt.Sprintf("      \"content\": %q", workflow.Content)); err != nil {
				return err
			}
		}

		if _, err := file.WriteString("\n    }"); err != nil {
			return err
		}
	}

	if _, err := file.WriteString("\n  ]\n"); err != nil {
		return err
	}
	if _, err := file.WriteString("}\n"); err != nil {
		return err
	}
	return nil
}
