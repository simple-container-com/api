// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package vm

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"

	"github.com/pulumi/pulumi-command/sdk/go/command/remote"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	"github.com/simple-container-com/api/pkg/clouds/vm"
)

const (
	// cloudInitWaitAttempts × cloudInitWaitInterval = 600s. Long enough for a cold
	// boot plus `get.docker.com` over a slow link; short enough that a cloud-init
	// that died reports within the patience of one deploy.
	cloudInitWaitAttempts = 120
	cloudInitWaitInterval = 5
)

// ensureReady gates everything downstream on the machine actually being able to run
// a deploy. There are exactly two modes, and which one applies is a property of the
// driver, not of the cloud:
//
//   - a machine SC created runs cloud-init, so readiness is waiting for the marker
//     file cloud-init writes last;
//   - a machine the operator created has already booted, so readiness is checking
//     the preconditions and creating SC's directories — never installing docker on
//     a host SC does not own.
//
// Why a gate at all: SSH answers before docker exists. A deploy that waits only for
// SSH fails intermittently on `docker: command not found` — success reported before
// readiness, the same class of fault as a green YC deploy with zero revisions
// (vm-matrix.md §3.1, §3.4).
func ensureReady(ctx *sdk.Context, input api.ResourceInput, cfg *vm.VmConfig, h HostFacts, drv Driver, params pApi.ProvisionParams) (*remote.Command, error) {
	script := readinessScript(drv.Capabilities().CloudInit, h.User)

	cmd, err := remote.NewCommand(ctx, input.ToResName(h.Name+"-ready"), &remote.CommandArgs{
		Connection: ConnectionArgs(h),
		Create:     sdk.String(script),
		// Without Triggers the command does not re-run once it has succeeded, and a
		// later `sc deploy` reports green having checked nothing — including against
		// a host that was rebuilt in the meantime. The trigger set is everything
		// that changes what "ready" means for this host.
		Triggers: sdk.Array{
			h.Address,
			sdk.String(h.User),
			sdk.Int(h.Port),
			sdk.String(script),
		},
		// Logging and AddPreviousOutputInEnv: the output of a readiness check is
		// harmless, but every command in this package shares the same defaults, and
		// the ones that follow (env files, k3s join tokens, kubeconfig) are not.
		// Setting them uniformly means a new command inherits the safe shape instead
		// of relying on whoever adds it to remember.
		Logging:                remote.LoggingNone.ToLoggingPtrOutput(),
		AddPreviousOutputInEnv: sdk.Bool(false),
	}, sdk.DependsOn(h.DependsOn))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create readiness check for host %q of vm %q", h.Name, input.Descriptor.Name)
	}
	params.Log.Info(ctx.Context(), "vm %q waits for host %q to become ready (cloud-init: %t)",
		input.Descriptor.Name, h.Name, drv.Capabilities().CloudInit)
	return cmd, nil
}

// readinessScript is the remote half of the gate.
func readinessScript(cloudInit bool, user string) string {
	if cloudInit {
		// `cloud-init status --long` on stderr is the only thing that distinguishes
		// "did not finish in time" from "cloud-init failed". Without it the next
		// person debugging this is guessing (vm-matrix.md §3.4).
		return strings.Join([]string{
			fmt.Sprintf(`for i in $(seq 1 %d); do [ -f %s ] && exit 0; sleep %d; done`,
				cloudInitWaitAttempts, vm.CloudInitDoneMarker, cloudInitWaitInterval),
			fmt.Sprintf(`echo "cloud-init did not finish in %ds" >&2`, cloudInitWaitAttempts*cloudInitWaitInterval),
			`sudo cloud-init status --long >&2 || true`,
			`exit 1`,
		}, "; ")
	}

	// The byo mode. Each check names what the operator has to go and do, because
	// this runs against a machine SC did not build and cannot fix: the alternative
	// to a specific message is `exit 1` from a remote command, which tells them
	// nothing.
	return strings.Join([]string{
		`set -eu`,
		fail(`command -v docker >/dev/null 2>&1`,
			"docker is not installed on this host; install Docker Engine with the compose v2 plugin, "+
				"or let SC create the machine with a cloud vm provider"),
		fail(`docker version >/dev/null 2>&1`,
			fmt.Sprintf("user %q cannot talk to the docker daemon; add it to the docker group "+
				"(usermod -aG docker %s) and reconnect", user, user)),
		fail(`docker compose version >/dev/null 2>&1`,
			"the docker compose v2 plugin is missing; `docker-compose` v1 is not supported"),
		fail(`sudo -n true >/dev/null 2>&1`,
			fmt.Sprintf("user %q needs passwordless sudo to create %s and %s", user, vm.ConfigDir, vm.StateDir)),
		// Created, not assumed: these are SC's own directories on a host SC does not
		// own, and 0750 keeps rendered env files out of reach of other accounts.
		fmt.Sprintf(`sudo install -d -m 0750 -o %s -g %s %s %s`, user, user, vm.ConfigDir, vm.StateDir),
		`echo ready`,
	}, "\n")
}

// fail turns a check into a guard that explains itself on stderr.
func fail(check, message string) string {
	return fmt.Sprintf(`%s || { echo %q >&2; exit 1; }`, check, message)
}
