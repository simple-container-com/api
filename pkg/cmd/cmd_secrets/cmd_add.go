package cmd_secrets

import (
	"github.com/spf13/cobra"
)

func NewAddCmd(sCmd *secretsCmd) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add repository secret",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return sCmd.provisioner.Cryptor().AddFile(args[0])
		},
	}
	return cmd
}
