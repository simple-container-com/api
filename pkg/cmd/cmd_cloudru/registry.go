// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package cmd_cloudru

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/simple-container-com/api/pkg/clouds/cloudru/evolution"
)

// NewRegistryCmd creates the `cloudru registry` command group.
func NewRegistryCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "registry",
		Short: "Cloud.ru Artifact Registry operations",
	}
	cmd.AddCommand(newRegistryEnsureCmd())
	return cmd
}

func newRegistryEnsureCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ensure",
		Short: "Make a DOCKER Artifact Registry exist in the project and wait until it is ACTIVE",
		Long: `Create the named registry if the project does not have it, then wait for ACTIVE.

A Container App resolves its image from the calling project's own Artifact Registry
and from nowhere else — Docker Hub, docker.io and even Cloud.ru's own quickstart
image all come back "Failed to resolve image". So this is part of a deploy, not a
prerequisite the operator is asked to arrange, and it runs before the image push.

Reads from stdin:
  {"keyId": "...", "keySecret": "...", "projectId": "...",
   "registryName": "my-registry", "waitTimeout": "5m"}

Prints:
  {"id": "...", "name": "my-registry", "status": "ACTIVE",
   "registryHost": "my-registry.cr.cloud.ru"}
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(cmd.Context(), func(ctx context.Context, c *evolution.Client, req evolution.Request) (*evolution.Result, error) {
				progress("ensuring cloud.ru artifact registry %q...", req.RegistryName)
				res, err := c.ApplyRegistry(ctx, req)
				if err != nil {
					return nil, err
				}
				progress("registry %q is %s at %s", res.Name, res.Status, res.RegistryHost)
				return res, nil
			})
		},
	}
}
