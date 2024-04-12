package cmd_cancel

import (
	"github.com/simple-container-com/api/pkg/api"
	"github.com/spf13/cobra"

	"github.com/simple-container-com/api/pkg/cmd/root_cmd"
)

type cancelCmd struct {
	Root   *root_cmd.RootCmd
	Params api.DeployParams
}

func NewCancelCmd(rootCmd *root_cmd.RootCmd) *cobra.Command {
	pCmd := cancelCmd{
		Root: rootCmd,
	}
	cmd := &cobra.Command{
		Use:   "cancel",
		Short: "Cancels deployment for a stack",
		RunE: func(cmd *cobra.Command, args []string) error {
			return pCmd.Root.Provisioner.Cancel(cmd.Context(), pCmd.Params.StackParams)
		},
	}

	root_cmd.RegisterStackFlags(cmd, &pCmd.Params.StackParams, false)
	return cmd
}
