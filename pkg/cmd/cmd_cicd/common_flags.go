package cmd_cicd

import (
	"github.com/spf13/cobra"
)

// CICDCommonParams contains the common parameters used across all CI/CD commands
type CICDCommonParams struct {
	StackName   string
	ConfigFile  string
	Parent      bool
	Staging     bool
	SkipRefresh bool
}

// RegisterCICDCommonFlags registers the common flags used across all CI/CD commands
func RegisterCICDCommonFlags(cmd *cobra.Command, params *CICDCommonParams, configFileDefault string) {
	// Stack name flag (required across all CI/CD commands)
	cmd.Flags().StringVarP(&params.StackName, "stack", "s", params.StackName, "Stack name (required)")
	_ = cmd.MarkFlagRequired("stack")

	// Config file flag (common across all CI/CD commands)
	if configFileDefault == "" {
		configFileDefault = "server.yaml"
	}
	cmd.Flags().StringVarP(&params.ConfigFile, "config", "c", configFileDefault, "Server config file path")

	// Parent and staging flags (common across all CI/CD commands)
	cmd.Flags().BoolVar(&params.Parent, "parent", params.Parent, "Operation for parent stack (infrastructure/provisioning)")
	cmd.Flags().BoolVar(&params.Staging, "staging", params.Staging, "Use staging optimizations instead of production")

	// Skip refresh flag (generates workflows with skip-refresh enabled)
	cmd.Flags().BoolVar(&params.SkipRefresh, "skip-refresh", params.SkipRefresh, "Generate workflows with skip-refresh enabled for faster deployments")

	// Register auto-completion for stack names
	registerStackCompletion(cmd, "stack")
}
