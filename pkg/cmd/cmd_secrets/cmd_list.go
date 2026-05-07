package cmd_secrets

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/simple-container-com/api/pkg/api"
)

func NewListCmd(sCmd *secretsCmd) *cobra.Command {
	var environment string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List repository secrets",
		RunE: func(cmd *cobra.Command, args []string) error {
			secretsPath := ".sc/secrets.yaml"

			// Read the secrets file
			data, err := os.ReadFile(secretsPath)
			if err != nil {
				return fmt.Errorf("failed to read secrets file: %w", err)
			}

			var descriptor api.SecretsDescriptor
			if err := yaml.Unmarshal(data, &descriptor); err != nil {
				return fmt.Errorf("failed to parse secrets file: %w", err)
			}

			// Display schema version
			fmt.Printf("Schema Version: %s\n", descriptor.SchemaVersion)

			// List environments if available
			if descriptor.IsV2Schema() {
				fmt.Println("\nEnvironments:")
				environments := descriptor.GetEnvironments()
				if len(environments) > 0 {
					for _, env := range environments {
						fmt.Printf("  - %s\n", env)
					}
				} else {
					fmt.Println("  (none)")
				}
			}

			// List shared secrets
			if len(descriptor.Values) > 0 {
				fmt.Println("\nShared Secrets:")
				for name := range descriptor.Values {
					fmt.Printf("  - %s\n", name)
				}
			}

			// List environment-specific secrets
			if environment != "" {
				if descriptor.HasEnvironment(environment) {
					fmt.Printf("\nSecrets for environment '%s':\n", environment)
					for name := range descriptor.Environments[environment].Values {
						fmt.Printf("  - %s\n", name)
					}
				} else {
					return fmt.Errorf("environment '%s' not found in secrets configuration", environment)
				}
			} else if descriptor.IsV2Schema() {
				// List all environment-specific secrets grouped by environment
				fmt.Println("\nEnvironment-Specific Secrets:")
				for envName, envSecrets := range descriptor.Environments {
					if len(envSecrets.Values) > 0 {
						fmt.Printf("  %s:\n", envName)
						for name := range envSecrets.Values {
							fmt.Printf("    - %s\n", name)
						}
					}
				}
			}

			// List encrypted files
			fmt.Println("\nEncrypted Files:")
			for _, secretFile := range sCmd.Root.Provisioner.Cryptor().GetSecretFiles().Registry.Files {
				fmt.Printf("  - %s\n", secretFile)
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&environment, "environment", "e", "", "Filter secrets by environment")
	return cmd
}
