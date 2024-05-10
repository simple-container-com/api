package cmd_secrets

import (
	"github.com/samber/lo"
	"github.com/spf13/cobra"

	"github.com/simple-container-com/api/pkg/api/secrets"
	"github.com/simple-container-com/api/pkg/cmd/root_cmd"
)

func NewDisallowCmd(sCmd *secretsCmd) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "disallow",
		Short: "Disallow public key to read secrets",
		RunE: func(cmd *cobra.Command, args []string) error {
			pubKey := args[0]
			return sCmd.Root.Provisioner.Cryptor().RemovePublicKey(pubKey)
		},
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			if len(args) != 0 {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			if err := sCmd.Root.Init(root_cmd.IgnoreAllErrors); err == nil {
				return lo.MapToSlice(sCmd.Root.Provisioner.Cryptor().GetSecretFiles().Secrets, func(key string, _ secrets.EncryptedSecrets) string {
					return key
				}), cobra.ShellCompDirectiveNoFileComp
			}
			return nil, cobra.ShellCompDirectiveNoFileComp
		},
	}
	return cmd
}
