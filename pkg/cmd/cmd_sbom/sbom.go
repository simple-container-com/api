// Package cmd_sbom provides CLI commands for SBOM operations
package cmd_sbom

import (
	"github.com/spf13/cobra"
)

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
