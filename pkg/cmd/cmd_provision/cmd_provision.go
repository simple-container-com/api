package cmd_provision

import (
	"fmt"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/spf13/cobra"

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
}

func PrintPreview(pRes *api.PreviewResult) {
	fmt.Println("=== Stack: " + pRes.StackName)
	for op, cnt := range pRes.Operations {
		fmt.Println(fmt.Sprintf("    %s: %d", op, cnt))
	}
	fmt.Println(pRes.Summary)
}
