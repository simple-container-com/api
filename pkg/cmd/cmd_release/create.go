package cmd_release

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/cmd/root_cmd"
	"github.com/simple-container-com/api/pkg/provisioner"
)

// NewCreateCmd creates the release create command
func NewCreateCmd(rootCmd *root_cmd.RootCmd) *cobra.Command {
	var (
		stackName   string
		environment string
		yes         bool
		preview     bool
	)

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a release with integrated security workflow",
		Long: `Create a release executing the full workflow:
  1. Load stack configuration
  2. Build and push container images
  3. Execute security operations (scan, sign, SBOM, provenance)
  4. Deploy infrastructure

Security operations are integrated into the Pulumi workflow and run automatically
when configured in the stack's security descriptor.`,
		Example: `  # Create release for production environment
  sc release create -s mystack -e production

  # Preview release without deploying
  sc release create -s mystack -e staging --preview

  # Auto-approve deployment
  sc release create -s mystack -e production --yes`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			log := rootCmd.Logger

			if stackName == "" {
				return fmt.Errorf("stack name is required (use -s or --stack)")
			}
			if environment == "" {
				return fmt.Errorf("environment is required (use -e or --environment)")
			}

			log.Info(ctx, "Creating release for stack %q in environment %q", stackName, environment)

			// Load provisioner
			p, err := provisioner.New()
			if err != nil {
				return fmt.Errorf("failed to create provisioner: %w", err)
			}

			// Build deploy params
			deployParams := api.DeployParams{
				StackParams: api.StackParams{
					StackName:   stackName,
					Environment: environment,
				},
				Vars: nil, // Can be extended to accept --var flags
			}

			// Execute deployment (security operations are integrated in build_and_push.go)
			if preview {
				log.Info(ctx, "Running preview mode (dry-run)")
				// In a full implementation, this would call a Preview method
				log.Info(ctx, "Preview mode: would build, scan, sign, generate SBOM/provenance, and deploy")
				return nil
			}

			log.Info(ctx, "Starting deployment workflow...")
			log.Info(ctx, "Security operations will be executed automatically if configured")

			// Deploy
			if err := p.Deploy(ctx, deployParams); err != nil {
				return fmt.Errorf("deployment failed: %w", err)
			}

			log.Info(ctx, "✓ Release created successfully")
			log.Info(ctx, "All security operations completed (if configured)")

			return nil
		},
	}

	cmd.Flags().StringVarP(&stackName, "stack", "s", "", "Stack name (required)")
	cmd.Flags().StringVarP(&environment, "environment", "e", "", "Environment name (required)")
	cmd.Flags().BoolVar(&yes, "yes", false, "Auto-approve deployment without prompts")
	cmd.Flags().BoolVar(&preview, "preview", false, "Preview changes without deploying")

	return cmd
}
