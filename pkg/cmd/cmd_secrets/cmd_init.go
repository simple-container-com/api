package cmd_secrets

import (
	"github.com/spf13/cobra"
)

func NewInitCmd(sCmd *secretsCmd) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Init repository secrets with initial commit",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := sCmd.Root.Init(true); err != nil {
				return err
			}
			if err := sCmd.Root.Provisioner.InitProfile(false); err != nil {
				return err
			}
			return sCmd.Root.Provisioner.MakeInitialCommit()
		},
	}
	return cmd
}
