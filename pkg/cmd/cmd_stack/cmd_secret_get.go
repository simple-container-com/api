package cmd_stack

import (
	"fmt"

	"github.com/pkg/errors"

	"github.com/spf13/cobra"
)

func NewSecretGetCmd(sCmd *stackCmd) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "secret-get",
		Short: "Get secret value specified in secrets.yaml for stack",
		RunE: func(cmd *cobra.Command, args []string) error {
			stack, err := sCmd.Root.Provisioner.GetStack(cmd.Context(), sCmd.Params)
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
