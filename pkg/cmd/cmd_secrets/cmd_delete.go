package cmd_secrets

import (
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/simple-container-com/api/pkg/cmd/root_cmd"
)

type deleteCmd struct {
	RemoveFile bool
}

func NewDeleteCmd(sCmd *secretsCmd) *cobra.Command {
	dCmd := &deleteCmd{}

	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete repository secret",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := sCmd.Root.Provisioner.Cryptor().RemoveFile(args[0]); err != nil {
				return err
			}
			if dCmd.RemoveFile {
				if err := os.Remove(filepath.Join(sCmd.Root.Provisioner.GitRepo().Workdir(), args[0])); err != nil {
					return err
				}
			}
			return nil
		},
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			if len(args) != 0 {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			if err := sCmd.Root.Init(root_cmd.IgnoreAllErrors); err == nil {
				return sCmd.Root.Provisioner.Cryptor().GetSecretFiles().Registry.Files, cobra.ShellCompDirectiveNoFileComp
			}
			return nil, cobra.ShellCompDirectiveNoFileComp
		},
	}
	cmd.Flags().BoolVarP(&dCmd.RemoveFile, "file", "f", dCmd.RemoveFile, "Delete file from file system")
	return cmd
}
