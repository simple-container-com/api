package cmd_secrets

import (
	"github.com/spf13/cobra"
)

func NewHideCmd(sCmd *secretsCmd) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "hide",
		Short: "Hide repository secrets",
		RunE: func(cmd *cobra.Command, args []string) error {
			return sCmd.provisioner.Cryptor().EncryptChanged()
		},
	}
	return cmd
}
