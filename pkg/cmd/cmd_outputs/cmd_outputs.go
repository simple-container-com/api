package cmd_outputs

import (
	"encoding/json"
	"fmt"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/spf13/cobra"

	"github.com/simple-container-com/api/pkg/cmd/root_cmd"
)

type outputsCmd struct {
	Root   *root_cmd.RootCmd
	Params api.StackParams
}

func NewOutputsCmd(rootCmd *root_cmd.RootCmd) *cobra.Command {
	pCmd := outputsCmd{
		Root: rootCmd,
	}
	cmd := &cobra.Command{
		Use:   "outputs",
		Short: "Displays outputs of a stacks defined in stacks directory",
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SetContext(rootCmd.Logger.Silent(cmd.Context()))
			if res, err := pCmd.Root.Provisioner.Outputs(cmd.Context(), pCmd.Params); err != nil {
				return err
			} else if j, err := json.Marshal(*res); err != nil {
				return err
			} else {
				fmt.Println(string(j))
			}
			return nil
		},
	}

	RegisterStackFlags(cmd, &pCmd.Params)
	return cmd
}

func RegisterStackFlags(cmd *cobra.Command, p *api.StackParams) {
	cmd.Flags().StringVarP(&p.Profile, "profile", "p", p.Profile, "Use profile (default: `default`)")
	cmd.Flags().StringVarP(&p.StackName, "stack", "s", p.StackName, "Stack name to deploy (required)")
	cmd.Flags().StringVarP(&p.Environment, "env", "e", p.Environment, "Environment to deploy (required)")
	cmd.Flags().StringVarP(&p.StacksDir, "dir", "d", p.StacksDir, "Root directory for stack configurations (default: .sc/stacks)")
}
