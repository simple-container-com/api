// Package cmd provides the SC CLI command tree for embedding in other binaries.
// Used by the github-actions binary to serve security subcommands (image sign/scan,
// sbom generate/attach, provenance generate/attach) when invoked via the "sc" symlink.
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/simple-container-com/api/pkg/cmd/cmd_image"
	"github.com/simple-container-com/api/pkg/cmd/cmd_provenance"
	"github.com/simple-container-com/api/pkg/cmd/cmd_sbom"
)

// Execute runs the SC CLI command tree. Called by the github-actions binary
// when it detects it was invoked via the "sc" symlink.
func Execute() {
	rootCmd := &cobra.Command{
		Use:   "sc",
		Short: "Simple Container CLI",
		Long:  "Simple Container CLI — security subcommands for container image operations.",
		// Silence usage on errors — Pulumi captures stderr and the usage text is noise.
		SilenceUsage: true,
	}

	rootCmd.AddCommand(
		cmd_image.NewImageCmd(),
		cmd_sbom.NewSBOMCommand(),
		cmd_provenance.NewProvenanceCommand(),
	)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
