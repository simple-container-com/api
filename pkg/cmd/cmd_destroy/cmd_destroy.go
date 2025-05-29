package cmd_destroy

import (
	"context"
	"strings"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/logger/color"
	"github.com/simple-container-com/api/pkg/cmd/root_cmd"
	"github.com/simple-container-com/api/pkg/util"
)

type destroyCmd struct {
	Root        *root_cmd.RootCmd
	ParentStack bool
	Params      api.DestroyParams
}

func NewDestroyCmd(rootCmd *root_cmd.RootCmd) *cobra.Command {
	var preview bool
	pCmd := destroyCmd{
		Root: rootCmd,
	}
	consoleWriter := util.DefaultConsoleWriter
	consoleReader := util.DefaultConsoleReader
	cmd := &cobra.Command{
		Use:   "destroy",
		Short: "Destroys stacks defined in stacks directory",
		RunE: func(cmd *cobra.Command, args []string) error {
			consoleWriter.Println("================================")
			var readString string
			var attempts int
			for !preview && strings.ToLower(readString) != "y" && strings.ToLower(readString) != "n" {
				if pCmd.ParentStack {
					consoleWriter.Print(color.RedFmt("Are you sure you want do destroy parent stack %q [Y/N]? >", pCmd.Params.StackName))
				} else {
					consoleWriter.Print(color.RedFmt("Are you sure you want do destroy %q in %q [Y/N]? >", pCmd.Params.StackName, pCmd.Params.Environment))
				}
				readString, _ = consoleReader.ReadLine()
				attempts++
				if attempts > 3 {
					return errors.Errorf("'Y' or 'N' expected, but got %q after 3 attempts", readString)
				}
			}
			if !preview && strings.ToLower(readString) != "y" {
				return errors.Errorf("Destroying stack cancelled")
			}

			if pCmd.ParentStack {
				err := pCmd.Root.Provisioner.DestroyParent(cmd.Context(), pCmd.Params, preview)
				if err != nil && !rootCmd.IsCanceled.Load() {
					return err
				} else if rootCmd.IsCanceled.Load() {
					err = pCmd.Root.Provisioner.Cancel(context.Background(), pCmd.Params.StackParams)
				}
				return err
			}
			err := pCmd.Root.Provisioner.Destroy(cmd.Context(), pCmd.Params, preview)
			if err != nil && !rootCmd.IsCanceled.Load() {
				return err
			} else if rootCmd.IsCanceled.Load() {
				err = pCmd.Root.Provisioner.Cancel(context.Background(), pCmd.Params.StackParams)
			}
			return err
		},
	}

	root_cmd.RegisterStackFlags(cmd, &pCmd.Params.StackParams, false)
	cmd.Flags().BoolVar(&pCmd.ParentStack, "parent", pCmd.ParentStack, "Destroy parent stack")
	cmd.Flags().BoolVarP(&preview, "preview", "P", preview, "Preview destroy")
	cmd.Flags().BoolVar(&pCmd.Params.DestroySecretsStack, "with-secrets", pCmd.Params.DestroySecretsStack, "Destroy secrets stack as well (e.g. when no envs remained)")
	return cmd
}
