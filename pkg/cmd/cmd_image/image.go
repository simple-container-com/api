package cmd_image

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/simple-container-com/api/pkg/security/tools"
)

// ensureTool checks if a tool is installed and auto-installs it if missing.
func ensureTool(ctx context.Context, name string) error {
	return tools.NewToolInstaller().InstallIfMissing(ctx, name)
}

// NewImageCmd creates the image command group
func NewImageCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "image",
		Short: "Container image security operations",
		Long:  `Perform security operations on container images including signing and verification`,
	}

	// Add subcommands
	cmd.AddCommand(NewSignCmd())
	cmd.AddCommand(NewVerifyCmd())
	cmd.AddCommand(NewScanCmd())

	return cmd
}
