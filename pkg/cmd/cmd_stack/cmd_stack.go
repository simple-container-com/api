package cmd_stack

import (
	"github.com/spf13/cobra"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/cmd/root_cmd"
)

type stackCmd struct {
	Root   *root_cmd.RootCmd
	Params api.StackParams
}

func NewStackCmd(rootCmd *root_cmd.RootCmd) *cobra.Command {
	sCmd := stackCmd{
		Root: rootCmd,
	}
	cmd := &cobra.Command{
		Use:   "stack",
		Short: "Manipulates stack's configuration and values",
	}

	cmd.AddCommand(
		NewSecretGetCmd(&sCmd),
		NewOutputsCmd(&sCmd),
	)

	root_cmd.RegisterStackFlags(cmd, &sCmd.Params, true)
	return cmd
}
