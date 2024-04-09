package cmd_secrets

import (
	"fmt"

	"github.com/spf13/cobra"
)

func NewAllowedKeysCmd(sCmd *secretsCmd) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "allowed-keys",
		Short: "List public keys allowed to decrypt secrets",
		RunE: func(cmd *cobra.Command, args []string) error {
			for pubKey := range sCmd.Root.Provisioner.Cryptor().GetSecretFiles().Secrets {
				fmt.Println(pubKey)
				fmt.Println()
			}
			return nil
		},
	}
	return cmd
}
