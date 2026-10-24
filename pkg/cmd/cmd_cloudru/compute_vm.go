// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package cmd_cloudru

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/simple-container-com/api/pkg/clouds/cloudru/evolution"
)

// NewComputeVmCmd creates the `cloudru compute-vm` command group.
func NewComputeVmCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "compute-vm",
		Short: "Cloud.ru Evolution compute VM operations",
		Long: `Create, converge and delete a Cloud.ru Evolution virtual machine.

These are what the ` + "`cloudru-evolution-vm`" + ` driver runs. They exist as a CLI
subcommand for the same reason the Container Apps ones do: Cloud.ru's own
evo-terraform provider ships as a binary with no Pulumi SDK, and Pulumi's Go SDK
has no dynamic providers, so the only way to put a VM inside the resource model is
to drive an external process.

What this cloud does NOT let SC do, and why the driver looks the way it does: a VM
create accepts no ` + "`user_data`" + ` (the image renders a fixed template from its own
declared ` + "`image_metadata`" + ` slots) and no networking at all — subnet and security
group are the availability zone's defaults, full stop. So there is no cloud-init
here, no pre-seeded SSH host key, and docker is installed over SSH afterwards.`,
	}
	cmd.AddCommand(newComputeVmApplyCmd())
	cmd.AddCommand(newComputeVmDestroyCmd())
	return cmd
}

func newComputeVmApplyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "apply",
		Short: "Create or converge one VM and wait until it is running and reachable",
		Long: `Converge one Evolution VM to the spec on stdin: boot disk, machine, floating IP,
ingress rules — then wait for it to report ` + "`running`" + `.

Find-or-create at every step, keyed by name, so a retried deploy (or a deploy
following a create that failed between the disk and the machine) converges instead
of piling up a second disk. Nothing is ever recreated on SC's own initiative: a
size change is reported as an error, because Cloud.ru has no in-place resize and
replacing a VM destroys its disk.

` + "`flavor`" + ` is accepted but discouraged — the catalogue renames sizes (the 2 vCPU /
4 GiB flavor was ` + "`low-2-4`" + ` on 2026-10-23 and ` + "`lowcost10-2-4`" + ` a day later), so
` + "`cores`" + ` + ` + "`memoryGiB`" + ` is the spelling that keeps working.

Reads from stdin:
  {"keyId": "...", "keySecret": "...", "projectId": "...",
   "waitTimeout": "10m", "pollInterval": "5s",
   "vmSpec": {"name": "my-vm", "zone": "ru.AZ-3",
              "cores": 2, "memoryGiB": 4, "image": "ubuntu-2404-lts",
              "diskGiB": 30, "diskType": "SSD",
              "sshUser": "sc", "publicKey": "ssh-ed25519 AAAA...",
              "publicIp": true,
              "ingressRules": [{"port": 80, "description": "HTTP"}]}}

Prints:
  {"name": "my-vm", "id": "...", "vmId": "...", "diskId": "...",
   "floatingIpId": "...", "address": "176.108.x.y", "internalAddress": "10.0.0.5",
   "user": "sc", "zone": "ru.AZ-3", "flavorName": "lowcost10-2-4", "status": "running"}
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(cmd.Context(), func(ctx context.Context, c *evolution.Client, req evolution.Request) (*evolution.Result, error) {
				progress("applying cloud.ru vm %q...", vmNameOf(req))
				res, err := c.ApplyVm(ctx, req, progress)
				if err != nil {
					return nil, err
				}
				progress("vm %q is %s at %s (internal %s)", res.Name, res.Status, res.Address, res.InternalAddress)
				return res, nil
			})
		},
	}
}

func newComputeVmDestroyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "destroy",
		Short: "Delete one VM, its floating IP and its boot disk",
		Long: `Delete the named VM and the objects SC created alongside it.

The order is forced: the VM holds the interface the floating IP is attached to, and
its boot disk stays ` + "`in_use`" + ` until the machine is gone. So the VM goes first and
is polled until it has actually disappeared, then the address, then the disk. An
object that is already gone is success at every step.

Reads from stdin:
  {"keyId": "...", "keySecret": "...", "projectId": "...", "name": "my-vm"}
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(cmd.Context(), func(ctx context.Context, c *evolution.Client, req evolution.Request) (*evolution.Result, error) {
				progress("destroying cloud.ru vm %q...", vmNameOf(req))
				res, err := c.DestroyVm(ctx, req, progress)
				if err != nil {
					return nil, err
				}
				progress("vm %q and its disk and address are gone", res.Name)
				return res, nil
			})
		},
	}
}

// vmNameOf is only for the progress line: the authoritative resolution (including
// the name-grammar fixups this cloud needs) happens inside the client.
func vmNameOf(req evolution.Request) string {
	if req.Name != "" {
		return req.Name
	}
	if req.VmSpec != nil {
		return req.VmSpec.Name
	}
	return ""
}
