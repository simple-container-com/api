package cmd_provision

import (
	"github.com/simple-container-com/api/pkg/api"
	"github.com/spf13/cobra"

	"github.com/simple-container-com/api/pkg/cmd/root_cmd"
)

type provisionCmd struct {
	Root   *root_cmd.RootCmd
	Params api.ProvisionParams
}

func NewProvisionCmd(rootCmd *root_cmd.RootCmd) *cobra.Command {
	pCmd := provisionCmd{
		Root: rootCmd,
	}
	cmd := &cobra.Command{
		Use:   "provision",
		Short: "Provisions stacks defined in stacks directory",
		RunE: func(cmd *cobra.Command, args []string) error {
			return pCmd.Root.Provisioner.Provision(cmd.Context(), pCmd.Params)
		},
	}
	cmd.Flags().StringVarP(&pCmd.Params.Profile, "profile", "p", pCmd.Params.Profile, "Use profile")
	cmd.Flags().StringSliceVarP(&pCmd.Params.Stacks, "stacks", "s", []string{}, "Stacks to provision")
	return cmd
}
