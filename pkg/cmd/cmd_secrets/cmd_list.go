package cmd_secrets

import (
	"fmt"

	"github.com/spf13/cobra"
)

func NewListCmd(sCmd *secretsCmd) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List repository secrets",
		RunE: func(cmd *cobra.Command, args []string) error {
			for _, secretFile := range sCmd.provisioner.Cryptor().GetSecretFiles().Registry.Files {
				fmt.Println(secretFile)
			}
			return nil
		},
	}
	return cmd
}
