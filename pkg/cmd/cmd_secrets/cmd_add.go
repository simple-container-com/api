package cmd_secrets

import (
	"fmt"
	"os"
	"strings"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/simple-container-com/api/pkg/api"
)

func NewAddCmd(sCmd *secretsCmd) *cobra.Command {
	var environment string

	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add repository secret",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			secretPath := args[0]

			// If environment is specified, add to environment-specific secrets
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

				// Ensure schema version is up to date
				if descriptor.SchemaVersion != api.SecretsSchemaVersion {
					descriptor.SchemaVersion = api.SecretsSchemaVersion
				}

				// Initialize environments map if needed
				if descriptor.Environments == nil {
					descriptor.Environments = make(map[string]api.EnvironmentSecrets)
				}

				// Initialize environment if needed
				if _, exists := descriptor.Environments[environment]; !exists {
					descriptor.Environments[environment] = api.EnvironmentSecrets{
						Values: make(map[string]string),
					}
				}

				// Extract secret name from path
				parts := strings.Split(secretPath, "/")
				secretName := parts[len(parts)-1]

				// Add secret to environment
				descriptor.Environments[environment].Values[secretName] = fmt.Sprintf("${secret:%s}", secretName)

				// Write back to file
				output, err := yaml.Marshal(descriptor)
				if err != nil {
					return errors.Wrapf(err, "failed to marshal secrets descriptor")
				}

				if err := os.WriteFile(secretsFilePath, output, 0600); err != nil {
					return errors.Wrapf(err, "failed to write secrets file")
				}

				fmt.Printf("Added environment-specific secret '%s' to environment '%s'\n", secretName, environment)
				fmt.Println("Note: Please run 'sc secrets hide' to encrypt the secret")
				return nil
			}

			// Otherwise, add file to encrypted secrets (original behavior)
			return sCmd.Root.Provisioner.Cryptor().AddFile(secretPath)
		},
	}

	cmd.Flags().StringVarP(&environment, "environment", "e", "", "Add secret to specific environment")
	return cmd
}
