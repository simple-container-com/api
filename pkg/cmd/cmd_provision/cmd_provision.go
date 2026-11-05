// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package cmd_provision

import (
	"fmt"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/cmd/root_cmd"
)

type provisionCmd struct {
	Root    *root_cmd.RootCmd
	Preview bool
	Params  api.ProvisionParams
}

func NewProvisionCmd(rootCmd *root_cmd.RootCmd) *cobra.Command {
	// provisionEnv captures -e/--environment on `sc provision`. A parent
	// stack is not per-env, so this flag has no runtime effect — but
	// every recipe written from a deploy/destroy example reaches for -e
	// and used to see "unknown shorthand flag: 'e' in -e" (sc-cli-unattended-gaps,
	// 2026-10-24). Accepting it with a clear sentence is more useful
	// than silently ignoring it: the caller learns parent stacks are not
	// per-env before the next deploy fails for a different reason.
	var provisionEnv string
	pCmd := provisionCmd{
		Root: rootCmd,
		Params: api.ProvisionParams{
			DetailedDiff: true, // Enable detailed diff by default for better visibility
		},
	}
	cmd := &cobra.Command{
		Use:   "provision",
		Short: "Provisions stacks defined in stacks directory",
		RunE: func(cmd *cobra.Command, args []string) error {
			if provisionEnv != "" {
				return errors.Errorf("--environment / -e is not applicable to `sc provision`: parent stacks are not per-env. "+
					"Drop the flag; `sc provision -s %s` provisions every env declared under that stack's resources.", pCmd.Params.Stacks)
			}
			if pCmd.Preview {
				res, err := pCmd.Root.Provisioner.PreviewProvision(cmd.Context(), pCmd.Params)
				if err != nil {
					return err
				}
				fmt.Println("Summary:")
				for _, pRes := range res {
					PrintPreview(pRes)
				}
				return nil
			}
			return pCmd.Root.Provisioner.Provision(cmd.Context(), pCmd.Params)
		},
	}
	cmd.Flags().BoolVarP(&pCmd.Preview, "preview", "P", pCmd.Preview, "Preview instead of provision (dry-run)")
	cmd.Flags().StringVarP(&provisionEnv, "environment", "e", "", "Not applicable to `sc provision` — parent stacks are not per-env. Rejected with a clear error if set, so a recipe copied from `sc deploy` surfaces the mismatch before the deploy")
	RegisterProvisionFlags(cmd, &pCmd.Params)
	return cmd
}

func RegisterProvisionFlags(cmd *cobra.Command, p *api.ProvisionParams) {
	cmd.Flags().StringVarP(&p.Profile, "profile", "p", p.Profile, "Use profile (default: 'default')")
	cmd.Flags().StringSliceVarP(&p.Stacks, "stacks", "s", []string{}, "Stacks to provision (default: all)")
	cmd.Flags().StringVarP(&p.StacksDir, "dir", "d", p.StacksDir, "Root directory for stack configurations (default: .sc/stacks)")
	cmd.Flags().BoolVarP(&p.SkipRefresh, "skip-refresh", "R", p.SkipRefresh, "Skip the refresh inside the update")
	cmd.Flags().BoolVarP(&p.SkipPreview, "skip-preview", "S", p.SkipPreview, "Skip preview before provision")
	cmd.Flags().BoolVarP(&p.DetailedDiff, "diff", "D", p.DetailedDiff, "Show detailed diff with granular changes for nested properties (e.g., redisConfigs)")
	cmd.Flags().BoolVar(&p.DetailedDiff, "detailed-diff", p.DetailedDiff, "Alias for --diff")
	_ = cmd.Flags().MarkHidden("detailed-diff") // Hide the alias from help output

	cmd.Flags().StringVarP(&p.Timeouts.PreviewTimeout, "preview-timeout", "M", p.Timeouts.PreviewTimeout, "Timeout on preview operations (in Go's duration format, e.g. `20m`)")
	cmd.Flags().StringVarP(&p.Timeouts.ExecutionTimeout, "execution-timeout", "O", p.Timeouts.ExecutionTimeout, "Timeout on whole command execution (in Go's duration format, e.g. `20m`)")
	cmd.Flags().StringVarP(&p.Timeouts.DeployTimeout, "timeout", "T", p.Timeouts.DeployTimeout, "Timeout on deploy/provision operations (in Go's duration format, e.g. `20m`)")
}

func PrintPreview(pRes *api.PreviewResult) {
	fmt.Println("=== Stack: " + pRes.StackName)
	for op, cnt := range pRes.Operations {
		fmt.Printf("    %s: %d\n", op, cnt)
	}
	fmt.Println(pRes.Summary)
}
