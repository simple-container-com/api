package cmd_deploy

import (
	"github.com/simple-container-com/api/pkg/api"
	"github.com/spf13/cobra"

	"github.com/simple-container-com/api/pkg/cmd/root_cmd"
)

type deployCmd struct {
	Root   *root_cmd.RootCmd
	Params api.DeployParams
}

func NewDeployCmd(rootCmd *root_cmd.RootCmd) *cobra.Command {
	pCmd := deployCmd{
		Root: rootCmd,
	}
	cmd := &cobra.Command{
		Use:   "deploy",
		Short: "Deploys stacks defined in stacks directory",
		RunE: func(cmd *cobra.Command, args []string) error {
			return pCmd.Root.Provisioner.Deploy(cmd.Context(), pCmd.Params)
		},
	}
	cmd.Flags().StringVarP(&pCmd.Params.Profile, "profile", "p", pCmd.Params.Profile, "Use profile")
	cmd.Flags().StringVarP(&pCmd.Params.StackName, "stack", "s", pCmd.Params.StackName, "Stack name to deploy")
	cmd.Flags().StringVarP(&pCmd.Params.Environment, "env", "e", pCmd.Params.Environment, "Environment to deploy")
	return cmd
}
