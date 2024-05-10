package cmd_secrets

import (
	"github.com/spf13/cobra"

	"github.com/simple-container-com/api/pkg/cmd/root_cmd"
)

type initCmd struct {
	MakeInitialCommit bool
	GenerateKeyPair   bool
}

func NewInitCmd(sCmd *secretsCmd) *cobra.Command {
	iCmd := &initCmd{}
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Init repository secrets with initial commit",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := sCmd.Root.Init(root_cmd.InitOpts{
				SkipScDirCreation:    false,
				IgnoreConfigDirError: true,
				ReturnOnGitError:     false,
			}); err != nil {
				return err
			}
			if err := sCmd.Root.Provisioner.InitProfile(iCmd.GenerateKeyPair); err != nil {
				return err
			}
			if iCmd.MakeInitialCommit {
				return sCmd.Root.Provisioner.MakeInitialCommit()
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&iCmd.GenerateKeyPair, "generate", "g", iCmd.GenerateKeyPair, "Generate RSA ssh key inside profile of .sc directory")
	cmd.Flags().BoolVarP(&iCmd.MakeInitialCommit, "commit", "C", iCmd.MakeInitialCommit, "Make initial commit after initialization")
	return cmd
}
