// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package cmd_stack

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/simple-container-com/api/pkg/api"
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
	// Registered here rather than in root_cmd.RegisterStackFlags: that registrar is
	// shared with deploy/cancel/secret-get, and masking output is only meaningful
	// for this command. Same shape as cmd_cancel.go's `--parent`.
	cmd.Flags().BoolVar(&sCmd.Params.ShowSecrets, "show-secrets", sCmd.Params.ShowSecrets,
		"Print secret-marked outputs in the clear instead of "+api.SecretOutputPlaceholder)
	return cmd
}
