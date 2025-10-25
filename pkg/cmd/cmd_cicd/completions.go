package cmd_cicd

import (
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// completeStackNames provides auto-completion for stack names from .sc/stacks directory
func completeStackNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	stacksDir := ".sc/stacks"

	// Check if .sc/stacks directory exists
	if _, err := os.Stat(stacksDir); os.IsNotExist(err) {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	// Read all directories in .sc/stacks
	entries, err := os.ReadDir(stacksDir)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	var stackNames []string
	for _, entry := range entries {
		if entry.IsDir() {
			stackName := entry.Name()
			// Filter based on the current input
			if strings.HasPrefix(stackName, toComplete) {
				stackNames = append(stackNames, stackName)
			}
		}
	}

	return stackNames, cobra.ShellCompDirectiveNoFileComp
}

// registerStackCompletion adds stack name completion to a flag
func registerStackCompletion(cmd *cobra.Command, flagName string) {
	_ = cmd.RegisterFlagCompletionFunc(flagName, completeStackNames)
}
