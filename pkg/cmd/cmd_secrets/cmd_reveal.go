// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package cmd_secrets

import (
	"github.com/spf13/cobra"
)

func NewRevealCmd(sCmd *secretsCmd) *cobra.Command {
	var forceReveal bool

	cmd := &cobra.Command{
		Use:   "reveal",
		Short: "Reveal repository secrets",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := refuseInteractiveUnderAgent(cmd, "--force", forceReveal); err != nil {
				return err
			}
			return sCmd.Root.Provisioner.Cryptor().DecryptAll(forceReveal)
		},
	}
	cmd.Flags().BoolVarP(&forceReveal, "force", "F", forceReveal, "Force decrypt secrets (default: false)")
	return cmd
}
