package cmd_release

import (
	"github.com/spf13/cobra"
	"github.com/simple-container-com/api/pkg/cmd/root_cmd"
)

// NewReleaseCommand creates the release command group
func NewReleaseCommand(rootCmd *root_cmd.RootCmd) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "release",
		Short: "Manage releases with integrated security operations",
		Long:  `Manage releases with integrated security operations including vulnerability scanning, signing, SBOM generation, and provenance attestation.`,
	}

	cmd.AddCommand(NewCreateCmd(rootCmd))

	return cmd
}
