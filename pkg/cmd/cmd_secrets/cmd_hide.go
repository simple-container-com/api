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
			if err := sCmd.provisioner.Cryptor().EncryptChanged(); err != nil {
				return errors.Wrapf(err, "failed to encrypt secrets")
			} else {
				return sCmd.provisioner.Cryptor().MarshalSecretsFile()
			}
		},
	}
	return cmd
}
