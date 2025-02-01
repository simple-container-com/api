package cmd_cancel

import (
	"github.com/spf13/cobra"

	"github.com/simple-container-com/api/pkg/api"
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
			if pCmd.Params.Parent {
				return pCmd.Root.Provisioner.CancelParent(cmd.Context(), pCmd.Params.StackParams)
			}
			return pCmd.Root.Provisioner.Cancel(cmd.Context(), pCmd.Params.StackParams)
		},
	}

	cmd.Flags().BoolVar(&pCmd.Params.Parent, "parent", pCmd.Params.Parent, "Cancel parent stack")

	root_cmd.RegisterStackFlags(cmd, &pCmd.Params.StackParams, false)
	return cmd
}
