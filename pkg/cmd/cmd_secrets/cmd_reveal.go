package cmd_secrets

import (
	"github.com/spf13/cobra"
)

func NewRevealCmd(sCmd *secretsCmd) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reveal",
		Short: "Reveal repository secrets",
		RunE: func(cmd *cobra.Command, args []string) error {
			return sCmd.Root.Provisioner.Cryptor().DecryptAll()
		},
	}
	return cmd
}
