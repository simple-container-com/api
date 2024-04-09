package cmd_secrets

import (
	"github.com/spf13/cobra"

	"github.com/simple-container-com/api/pkg/cmd/root_cmd"
)

type secretsCmd struct {
	Root *root_cmd.RootCmd
}

func NewSecretsCmd(rootCmd *root_cmd.RootCmd) *cobra.Command {
	sCmd := &secretsCmd{
		Root: rootCmd,
	}
	cmd := &cobra.Command{
		Use:   "secrets",
		Short: "Control repository-stored secrets",
	}

	cmd.AddCommand(
		NewAllowedKeysCmd(sCmd),
		NewListCmd(sCmd),
		NewHideCmd(sCmd),
		NewRevealCmd(sCmd),
		NewAllowCmd(sCmd),
		NewAddCmd(sCmd),
		NewDeleteCmd(sCmd),
		NewInitCmd(sCmd),
	)
	return cmd
}
