package cmd_init

import (
	"github.com/spf13/cobra"

	"github.com/simple-container-com/api/pkg/cmd/root_cmd"
)

type initCmd struct {
	Root *root_cmd.RootCmd
}

func NewInitCmd(rootCmd *root_cmd.RootCmd) *cobra.Command {
	sCmd := &initCmd{
		Root: rootCmd,
	}
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Init simple-container.com managed repository",
		RunE: func(cmd *cobra.Command, args []string) error {
			return sCmd.Root.Init(false)
		},
	}

	return cmd
}
