package cmd_destroy

import (
	"context"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/spf13/cobra"

	"github.com/simple-container-com/api/pkg/cmd/root_cmd"
)

type destroyCmd struct {
	Root        *root_cmd.RootCmd
	ParentStack bool
	Params      api.DestroyParams
}

func NewDestroyCmd(rootCmd *root_cmd.RootCmd) *cobra.Command {
	pCmd := destroyCmd{
		Root: rootCmd,
	}
	cmd := &cobra.Command{
		Use:   "destroy",
		Short: "Destroys stacks defined in stacks directory",
		RunE: func(cmd *cobra.Command, args []string) error {
			if pCmd.ParentStack {
				err := pCmd.Root.Provisioner.DestroyParent(cmd.Context(), api.ProvisionParams{
					StacksDir: pCmd.Params.StacksDir,
					Profile:   pCmd.Params.Profile,
					Stacks:    []string{pCmd.Params.StackName},
				})
				if err != nil && !rootCmd.IsCanceled.Load() {
					return err
				} else if rootCmd.IsCanceled.Load() {
					ctx, _ := context.WithCancel(context.Background())
					err = pCmd.Root.Provisioner.Cancel(ctx, pCmd.Params.StackParams)
				}
				return nil
			}
			err := pCmd.Root.Provisioner.Destroy(cmd.Context(), pCmd.Params)
			if err != nil && !rootCmd.IsCanceled.Load() {
				return err
			} else if rootCmd.IsCanceled.Load() {
				ctx, _ := context.WithCancel(context.Background())
				err = pCmd.Root.Provisioner.Cancel(ctx, pCmd.Params.StackParams)
			}
			return nil
		},
	}

	root_cmd.RegisterStackFlags(cmd, &pCmd.Params.StackParams)
	cmd.Flags().BoolVar(&pCmd.ParentStack, "parent", pCmd.ParentStack, "Destroy parent stack")
	return cmd
}
