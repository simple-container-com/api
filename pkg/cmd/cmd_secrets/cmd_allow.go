package cmd_secrets

import (
	"github.com/spf13/cobra"
)

func NewAllowCmd(sCmd *secretsCmd) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "allow",
		Short: "Allow public key to read secrets",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Reveal secrets first to ensure we're working with the latest state
			if err := sCmd.Root.Provisioner.Cryptor().DecryptAll(false); err != nil {
				return err
			}
			pubKey := args[0]
			return sCmd.Root.Provisioner.Cryptor().AddPublicKey(pubKey)
		},
	}
	return cmd
}
