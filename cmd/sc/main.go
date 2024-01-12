package main

import (
	"api/pkg/cmd/cmd_secrets"
	"api/pkg/cmd/root_cmd"
	"github.com/spf13/cobra"
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "sc",
		Short: "Simple Container is a handy tool for provisioning your cloud clusters",
		Long:  "A fast and flexible way of deploying your whole infrastructure with the underlying use of Pulumi.\nComplete documentation is available at https://simple-container.com/docs",
	}

	rootParams := root_cmd.Params{}

	rootCmd.AddCommand(
		cmd_secrets.NewSecretsCmd(rootParams),
	)

	err := rootCmd.Execute()
	if err != nil {
		panic(err)
	}
}
