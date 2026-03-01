package cmd_secrets

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/cmd/root_cmd"
)

type deleteCmd struct {
	RemoveFile bool
}

func NewDeleteCmd(sCmd *secretsCmd) *cobra.Command {
	var environment string
	dCmd := &deleteCmd{}

	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete repository secret",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			secretName := args[0]

			// If environment is specified, delete from environment-specific secrets
			if environment != "" {
				secretsFilePath := ".sc/secrets.yaml"
				data, err := os.ReadFile(secretsFilePath)
				if err != nil {
					return errors.Wrapf(err, "failed to read secrets file")
				}

				var descriptor api.SecretsDescriptor
				if err := yaml.Unmarshal(data, &descriptor); err != nil {
					return errors.Wrapf(err, "failed to parse secrets file")
				}

				// Check if environment exists
				if !descriptor.HasEnvironment(environment) {
					return fmt.Errorf("environment '%s' not found in secrets configuration", environment)
				}

				// Check if secret exists in environment
				if _, exists := descriptor.Environments[environment].Values[secretName]; !exists {
					return fmt.Errorf("secret '%s' not found in environment '%s'", secretName, environment)
				}

				// Delete secret from environment
				delete(descriptor.Environments[environment].Values, secretName)

				// Clean up empty environment
				if len(descriptor.Environments[environment].Values) == 0 {
					delete(descriptor.Environments, environment)
				}

				// Write back to file
				output, err := yaml.Marshal(descriptor)
				if err != nil {
					return errors.Wrapf(err, "failed to marshal secrets descriptor")
				}

				if err := os.WriteFile(secretsFilePath, output, 0600); err != nil {
					return errors.Wrapf(err, "failed to write secrets file")
				}

				fmt.Printf("Deleted environment-specific secret '%s' from environment '%s'\n", secretName, environment)
				return nil
			}

			// Otherwise, delete from encrypted secrets (original behavior)
			if err := sCmd.Root.Provisioner.Cryptor().RemoveFile(secretName); err != nil {
				return err
			}
			if dCmd.RemoveFile {
				if err := os.Remove(filepath.Join(sCmd.Root.Provisioner.GitRepo().Workdir(), secretName)); err != nil {
					return err
				}
			}
			return nil
		},
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			if len(args) != 0 {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			if err := sCmd.Root.Init(root_cmd.IgnoreAllErrors); err == nil {
				return sCmd.Root.Provisioner.Cryptor().GetSecretFiles().Registry.Files, cobra.ShellCompDirectiveNoFileComp
			}
			return nil, cobra.ShellCompDirectiveNoFileComp
		},
	}
	cmd.Flags().BoolVarP(&dCmd.RemoveFile, "file", "f", dCmd.RemoveFile, "Delete file from file system")
	cmd.Flags().StringVarP(&environment, "environment", "e", "", "Delete secret from specific environment")
	return cmd
}
