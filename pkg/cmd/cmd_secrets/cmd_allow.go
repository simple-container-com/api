package cmd_secrets

import (
	"github.com/spf13/cobra"
)

func NewAllowCmd(sCmd *secretsCmd) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "allow",
		Short: "Allow public key to read secrets",
		RunE: func(cmd *cobra.Command, args []string) error {
			pubKey := args[0]
			return sCmd.provisioner.Cryptor().AddPublicKey(pubKey)
		},
	}
	return cmd
}
