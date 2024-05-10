package cmd_upgrade

import (
	"fmt"
	"os/exec"

	"github.com/spf13/cobra"

	"github.com/simple-container-com/api/pkg/cmd/root_cmd"
)

type upgradeCmd struct {
	Root    *root_cmd.RootCmd
	Version string
}

func NewUpgradeCmd(rootCmd *root_cmd.RootCmd) *cobra.Command {
	_ = upgradeCmd{
		Root: rootCmd,
	}
	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: "Upgrades simple-container-com CLI",
		RunE: func(cmd *cobra.Command, args []string) error {
			eCmd := exec.Command("bash", "-c", "bash <(curl -Ls \"https://dist.simple-container.com/sc.sh\") --version")
			stdout, err := eCmd.Output()
			if err != nil {
				return err
			}

			fmt.Printf("Upgraded sc CLI to v%s\n", string(stdout))
			return nil
		},
	}

	return cmd
}
