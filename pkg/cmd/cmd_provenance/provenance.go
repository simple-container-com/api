package cmd_provenance

import "github.com/spf13/cobra"

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
