package cmd_secrets

import (
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

func NewHideCmd(sCmd *secretsCmd) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "hide",
		Short: "Hide repository secrets",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := sCmd.Root.Provisioner.Cryptor().EncryptChanged(false); err != nil {
				return errors.Wrapf(err, "failed to encrypt secrets")
			} else {
				return sCmd.Root.Provisioner.Cryptor().MarshalSecretsFile()
			}
		},
	}
	return cmd
}
