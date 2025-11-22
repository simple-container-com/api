package cmd_provision

import (
	"fmt"

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
	RegisterProvisionFlags(cmd, &pCmd.Params)
	return cmd
}

func RegisterProvisionFlags(cmd *cobra.Command, p *api.ProvisionParams) {
	cmd.Flags().StringVarP(&p.Profile, "profile", "p", p.Profile, "Use profile (default: 'default')")
	cmd.Flags().StringSliceVarP(&p.Stacks, "stacks", "s", []string{}, "Stacks to provision (default: all)")
	cmd.Flags().StringVarP(&p.StacksDir, "dir", "d", p.StacksDir, "Root directory for stack configurations (default: .sc/stacks)")
	cmd.Flags().BoolVarP(&p.SkipRefresh, "skip-refresh", "R", p.SkipRefresh, "Skip refresh before provision")
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
