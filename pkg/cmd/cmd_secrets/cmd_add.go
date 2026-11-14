// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package cmd_secrets

import (
	"github.com/spf13/cobra"
)

func NewAddCmd(sCmd *secretsCmd) *cobra.Command {
	var forceAdd bool

	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add repository secret",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := refuseInteractiveUnderAgent(cmd, "--force", forceAdd); err != nil {
				return err
			}
			return sCmd.Root.Provisioner.Cryptor().AddFile(args[0], forceAdd)
		},
	}
	// Registering a file encrypts its current contents, and on anything but an
	// empty file that means ensureDiffAcceptable prints every line of it before
	// asking for confirmation. --force accepts the change without printing it.
	cmd.Flags().BoolVarP(&forceAdd, "force", "F", forceAdd, "Accept the secret's contents without printing a diff or prompting (default: false)")
	return cmd
}
