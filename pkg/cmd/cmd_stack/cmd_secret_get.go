// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package cmd_stack

import (
	"fmt"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/simple-container-com/api/pkg/api"
)

func NewSecretGetCmd(sCmd *stackCmd) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "secret-get",
		Short: "Get secret value specified in secrets.yaml for stack",
		RunE: func(cmd *cobra.Command, args []string) error {
			// ReadSecretsOnly, not ReadIgnoreNoClientCfg: this command prints one
			// value from secrets.yaml and exits. It never provisions, deploys or
			// converts a stack config, so it has no use for plugin-owned type
			// knowledge — and paying for it meant spawning (in CI, downloading)
			// every plugin the stack declares. See api.ReadOpts.PluginTypesNotNeeded.
			stack, err := sCmd.Root.Provisioner.GetStack(cmd.Context(), sCmd.Params, api.ReadSecretsOnly)
			if err != nil {
				return err
			}
			if len(args) != 1 {
				return errors.Errorf("secret name must be specified")
			}
			secretName := args[0]
			if value, found := stack.Secrets.Values[secretName]; !found {
				return errors.Errorf("secret %q not found in stack %q", secretName, stack.Name)
			} else {
				fmt.Println(value)
			}
			return nil
		},
	}

	return cmd
}
