package cmd_preview

import (
	"fmt"
	"github.com/simple-container-com/api/pkg/api"
	"github.com/spf13/cobra"

	"github.com/simple-container-com/api/pkg/cmd/root_cmd"
)

type previewCmd struct {
	Root   *root_cmd.RootCmd
	Params api.DeployParams
}

func NewPreviewCmd(rootCmd *root_cmd.RootCmd) *cobra.Command {
	pCmd := previewCmd{
		Root: rootCmd,
	}
	cmd := &cobra.Command{
		Use:   "preview",
		Short: "Prints preview for stacks defined in stacks directory",
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := pCmd.Root.Provisioner.Preview(cmd.Context(), pCmd.Params)
			if err != nil {
				return err
			}
			fmt.Println(res)
			return nil
		},
	}
	cmd.Flags().StringVarP(&pCmd.Params.Profile, "profile", "p", pCmd.Params.Profile, "Use profile")
	cmd.Flags().StringVarP(&pCmd.Params.StackName, "stack", "s", pCmd.Params.StackName, "Stack name to deploy")
	cmd.Flags().StringVarP(&pCmd.Params.Environment, "env", "e", pCmd.Params.Environment, "Environment to deploy")
	return cmd
}
