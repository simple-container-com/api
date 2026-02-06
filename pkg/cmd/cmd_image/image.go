package cmd_image

import (
	"github.com/spf13/cobra"
)

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

	return cmd
}
