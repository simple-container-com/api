// Package cmd_sbom provides CLI commands for SBOM operations
package cmd_sbom

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/simple-container-com/api/pkg/security"
	"github.com/simple-container-com/api/pkg/security/tools"
)

// ensureTool checks if a tool is installed and auto-installs it if missing.
func ensureTool(ctx context.Context, name string) error {
	return tools.NewToolInstaller().InstallIfMissing(ctx, name)
}

func validateImage(image string) error {
	return security.ValidateImageRef(image)
}

// NewSBOMCommand creates the sbom command group
func NewSBOMCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sbom",
		Short: "Software Bill of Materials (SBOM) operations",
		Long:  `Generate, attach, and verify Software Bill of Materials (SBOM) for container images`,
	}

	// Add subcommands
	cmd.AddCommand(NewGenerateCommand())
	cmd.AddCommand(NewAttachCommand())
	cmd.AddCommand(NewVerifyCommand())

	return cmd
}
