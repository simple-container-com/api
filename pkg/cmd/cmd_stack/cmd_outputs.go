// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package cmd_stack

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

func NewOutputsCmd(sCmd *stackCmd) *cobra.Command {
	var compact bool
	cmd := &cobra.Command{
		Use:   "outputs",
		Short: "Displays outputs of a stack defined in stacks directory",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Silent for the whole operation: this command's stdout is a JSON
			// document someone pipes into jq, and SC's logger writes to stdout
			// too, so any INFO line would land inside the payload. The level
			// rides the context from here down — including into the plugin
			// LogStream consumers and go-plugin's own hclog.
			cmd.SetContext(sCmd.Root.Logger.Silent(cmd.Context()))
			res, err := sCmd.Root.Provisioner.Outputs(cmd.Context(), sCmd.Params)
			if err != nil {
				return err
			}
			// Indented by default. These payloads are read by a person at a
			// terminal at least as often as by a script — forge's infra stack
			// returns dozens of keys, which as one line wraps into an
			// unreadable block — and indenting is what every comparable CLI
			// (`gh api`, `aws`, `pulumi stack output --json`) does. --compact
			// restores the single line for anything that depended on it.
			j, err := marshalOutputs(*res, compact)
			if err != nil {
				return err
			}
			fmt.Println(string(j))
			return nil
		},
	}
	cmd.Flags().BoolVar(&compact, "compact", false,
		"Print the JSON on a single line instead of indented")
	return cmd
}

// marshalOutputs is split out of RunE so the format is testable without
// standing up a cobra command and a provisioner.
func marshalOutputs(res any, compact bool) ([]byte, error) {
	if compact {
		return json.Marshal(res)
	}
	return json.MarshalIndent(res, "", "  ")
}
