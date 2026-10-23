// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package cmd_cloudru

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/simple-container-com/api/pkg/clouds/cloudru/evolution"
)

// NewContainerAppCmd creates the `cloudru container-app` command group.
func NewContainerAppCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "container-app",
		Short: "Cloud.ru Evolution Container Apps operations",
	}
	cmd.AddCommand(newContainerAppApplyCmd())
	cmd.AddCommand(newContainerAppDestroyCmd())
	return cmd
}

func newContainerAppApplyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "apply",
		Short: "Create or update one Container App and wait until its newest revision serves",
		Long: `Converge one Container App to the spec on stdin, then wait for it to actually serve.

An existing app is PATCHed, not recreated: a patch publishes a new revision while
the old one keeps serving, which is blue-green at no cost. The exception is
` + "`configuration.privileged`" + `, which Cloud.ru's patch request has no field for —
changing it is refused here rather than silently dropped by the API.

Waiting is not optional and does not use the obvious signals. The create operation
reports done in ~21 s whether or not the container ever started, the app's own
` + "`status`" + ` field has been seen reading "publishing" for four minutes against a
CrashLooping pod, and the public URL answers 404 in the meantime. Only
GET /v2/containers/{name}/revisions tells the truth, and only /logs holds the
reason — so on a failure or a timeout the container's recent output is folded into
the error.

Reads from stdin:
  {"keyId": "...", "keySecret": "...", "projectId": "...",
   "name": "my-app", "waitTimeout": "10m", "pollInterval": "5s",
   "spec": {"name": "my-app", "configuration": {...}, "template": {...}}}

Prints:
  {"id": "...", "name": "my-app", "url": "https://my-app.containerapps.ru",
   "status": "...", "revision": "my-app-00002"}
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(cmd.Context(), func(ctx context.Context, c *evolution.Client, req evolution.Request) (*evolution.Result, error) {
				progress("applying cloud.ru container app %q...", req.Name)
				res, err := c.ApplyApp(ctx, req)
				if err != nil {
					return nil, err
				}
				progress("container app %q is serving revision %q at %s", res.Name, res.Revision, res.URL)
				return res, nil
			})
		},
	}
}

func newContainerAppDestroyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "destroy",
		Short: "Delete one Container App",
		Long: `Delete the named Container App. An app that is already gone is success.

Reads from stdin:
  {"keyId": "...", "keySecret": "...", "projectId": "...", "name": "my-app"}
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(cmd.Context(), func(ctx context.Context, c *evolution.Client, req evolution.Request) (*evolution.Result, error) {
				progress("destroying cloud.ru container app %q...", req.Name)
				res, err := c.DestroyApp(ctx, req)
				if err != nil {
					return nil, err
				}
				progress("container app %q is gone", res.Name)
				return res, nil
			})
		},
	}
}
