package cmd_deploy

import (
	"context"
	"fmt"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/cmd/cmd_provision"
	"github.com/spf13/cobra"

	"github.com/simple-container-com/api/pkg/cmd/root_cmd"
)

type deployCmd struct {
	Root    *root_cmd.RootCmd
	Params  api.DeployParams
	Preview bool
}

func NewDeployCmd(rootCmd *root_cmd.RootCmd) *cobra.Command {
	pCmd := deployCmd{
		Root: rootCmd,
	}
	cmd := &cobra.Command{
		Use:   "deploy",
		Short: "Deploys stacks defined in stacks directory",
		RunE: func(cmd *cobra.Command, args []string) error {
			if pCmd.Preview {
				res, err := pCmd.Root.Provisioner.Preview(cmd.Context(), pCmd.Params)
				if err != nil {
					return err
				}
				fmt.Println("Summary:")
				cmd_provision.PrintPreview(res)
				return nil
			}
			err := pCmd.Root.Provisioner.Deploy(cmd.Context(), pCmd.Params)
			if err != nil && !rootCmd.IsCanceled.Load() {
				return err
			} else if rootCmd.IsCanceled.Load() {
				ctx, _ := context.WithCancel(context.Background())
				err = pCmd.Root.Provisioner.Cancel(ctx, pCmd.Params)
			} else {
				return nil
			}
			return err
		},
	}

	RegisterStackFlags(cmd, &pCmd.Params.StackParams)
	cmd.Flags().BoolVarP(&pCmd.Preview, "preview", "P", pCmd.Preview, "Preview instead of provision (dry-run)")
	return cmd
}

func RegisterStackFlags(cmd *cobra.Command, p *api.StackParams) {
	cmd.Flags().StringVarP(&p.Profile, "profile", "p", p.Profile, "Use profile (default: `default`)")
	cmd.Flags().StringVarP(&p.StackName, "stack", "s", p.StackName, "Stack name to deploy (required)")
	_ = cmd.MarkFlagRequired("stack")
	cmd.Flags().StringVarP(&p.Environment, "env", "e", p.Environment, "Environment to deploy (required)")
	_ = cmd.MarkFlagRequired("env")
	cmd.Flags().StringVarP(&p.StacksDir, "dir", "d", p.StacksDir, "Root directory for stack configurations (default: .sc/stacks)")
}
