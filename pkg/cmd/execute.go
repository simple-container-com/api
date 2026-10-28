// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

// Package cmd provides the SC CLI command tree for embedding in other binaries.
// Used by the github-actions binary to serve the subcommands a deploy invokes as
// child processes (image sign/scan/ship, sbom generate/attach, provenance
// generate/attach, cloudru container-app apply/destroy) when it is invoked via the
// "sc" symlink.
//
// Anything a pulumi-command resource shells out to `sc` for has to be registered
// HERE as well as in cmd/sc/main.go, or it works on a workstation and is "sc:
// unknown command" on a runner.
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/simple-container-com/api/pkg/api/logger"
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
		fmt.Fprint(os.Stderr, logger.Redact(fmt.Sprintf("Error: %v\n", err)))
		os.Exit(1)
	}
}
