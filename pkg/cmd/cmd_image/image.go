// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package cmd_image

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/simple-container-com/api/pkg/security"
	"github.com/simple-container-com/api/pkg/security/tools"
)

// ensureTool checks if a tool is installed and auto-installs it if missing.
func ensureTool(ctx context.Context, name string) error {
	return tools.NewToolInstaller().InstallIfMissing(ctx, name)
}

// validateImage validates the image reference before passing to external tools.
func validateImage(image string) error {
	return security.ValidateImageRef(image)
}

// NewImageCmd creates the image command group
func NewImageCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "image",
		Short: "Container image operations",
		Long:  `Sign, verify and scan container images, and ship them to a machine without a registry`,
	}

	// Add subcommands
	cmd.AddCommand(NewSignCmd())
	cmd.AddCommand(NewVerifyCmd())
	cmd.AddCommand(NewScanCmd())
	cmd.AddCommand(NewShipCmd())

	return cmd
}
