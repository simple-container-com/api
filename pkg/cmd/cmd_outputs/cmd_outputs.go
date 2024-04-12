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

	root_cmd.RegisterStackFlags(cmd, &pCmd.Params, false)
	return cmd
}
