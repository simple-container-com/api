package cmd_secrets

import (
	"github.com/spf13/cobra"
)

func NewDeleteCmd(sCmd *secretsCmd) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete repository secret",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return sCmd.Root.Provisioner.Cryptor().RemoveFile(args[0])
		},
	}
	return cmd
}
