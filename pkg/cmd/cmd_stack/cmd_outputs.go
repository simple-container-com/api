package cmd_stack

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

func NewOutputsCmd(sCmd *stackCmd) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "outputs",
		Short: "Displays outputs of a stack defined in stacks directory",
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SetContext(sCmd.Root.Logger.Silent(cmd.Context()))
			if res, err := sCmd.Root.Provisioner.Outputs(cmd.Context(), sCmd.Params); err != nil {
				return err
			} else if j, err := json.Marshal(*res); err != nil {
				return err
			} else {
				fmt.Println(string(j))
			}
			return nil
		},
	}
	return cmd
}
