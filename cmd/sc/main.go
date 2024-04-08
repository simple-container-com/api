package main

import (
	"github.com/simple-container-com/api/internal/build"
	"github.com/simple-container-com/api/pkg/api/logger"
	"github.com/simple-container-com/api/pkg/cmd/cmd_cancel"
	"github.com/simple-container-com/api/pkg/cmd/cmd_deploy"
	"github.com/simple-container-com/api/pkg/cmd/cmd_init"
	"github.com/simple-container-com/api/pkg/cmd/cmd_provision"
	"github.com/simple-container-com/api/pkg/cmd/cmd_secrets"
	"github.com/simple-container-com/api/pkg/cmd/root_cmd"
	"github.com/spf13/cobra"
)

func main() {
	rootParams := root_cmd.Params{
		Verbose: false,
		Silent:  false,
	}

	rootCmdInstance := root_cmd.RootCmd{
		Params: rootParams,
	}

	rootCmd := &cobra.Command{
		Use:     "sc",
		Version: build.Version,
		Short:   "Simple Container is a handy tool for provisioning your cloud clusters",
		Long:    "A fast and flexible way of deploying your whole infrastructure with the underlying use of Pulumi.\nComplete documentation is available at https://simple-container.com/docs",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if cmd.Name() != "init" {
				if err := rootCmdInstance.Init(false); err != nil {
					return err
				}
			}
			if rootParams.Verbose {
				cmd.SetContext(rootCmdInstance.Logger.SetLogLevel(cmd.Context(), logger.LogLevelDebug))
			}
			if rootParams.Silent {
				cmd.SetContext(rootCmdInstance.Logger.SetLogLevel(cmd.Context(), logger.LogLevelError))
			}
			return nil
		},
	}

	rootCmd.AddCommand(
		cmd_secrets.NewSecretsCmd(&rootCmdInstance),
		cmd_init.NewInitCmd(&rootCmdInstance),
		cmd_provision.NewProvisionCmd(&rootCmdInstance),
		cmd_deploy.NewDeployCmd(&rootCmdInstance),
		cmd_cancel.NewCancelCmd(&rootCmdInstance),
	)

	rootCmd.Flags().BoolVarP(&rootParams.Verbose, "verbose", "v", rootParams.Verbose, "Verbose mode")
	rootCmd.Flags().StringVarP(&rootParams.Profile, "profile", "p", rootParams.Profile, "Use profile")

	err := rootCmd.Execute()
	if err != nil {
		panic(err)
	}
}
