package cmd_cicd

import (
	"github.com/spf13/cobra"

	"github.com/simple-container-com/api/pkg/cmd/root_cmd"
)

// NewCicdCmd creates the cicd command
func NewCicdCmd(rootCmd *root_cmd.RootCmd) *cobra.Command {
	cicdCmd := &cobra.Command{
		Use:   "cicd",
		Short: "Manage CI/CD configurations and workflows",
		Long: `Manage CI/CD configurations and workflows for Simple Container.
		
This command provides functionality to:
- Generate GitHub Actions workflows from server.yaml configuration
- Validate CI/CD configuration
- Sync workflows when configuration changes
- Preview generated workflows before writing files`,
		Example: `  # Generate workflows for infrastructure stack
  sc cicd generate --stack myorg/infrastructure --output .github/workflows/

  # Validate CI/CD configuration
  sc cicd validate

  # Sync workflows after configuration changes
  sc cicd sync

  # Preview a specific workflow template
  sc cicd preview --template deploy --stack myorg/infrastructure`,
	}

	cicdCmd.AddCommand(
		NewGenerateCmd(rootCmd),
		NewValidateCmd(rootCmd),
		NewSyncCmd(rootCmd),
		NewPreviewCmd(rootCmd),
	)

	return cicdCmd
}
