package cmd_provenance

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

// NewProvenanceCommand creates the provenance command group.
func NewProvenanceCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "provenance",
		Short: "Provenance attestation operations",
		Long:  `Generate, attach, and verify provenance attestations for container images.`,
	}

	cmd.AddCommand(NewGenerateCommand())
	cmd.AddCommand(NewAttachCommand())
	cmd.AddCommand(NewVerifyCommand())

	return cmd
}
