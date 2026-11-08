// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package vm

import (
	"github.com/pkg/errors"

	"github.com/pulumi/pulumi-command/sdk/go/command/remote"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	"github.com/simple-container-com/api/pkg/clouds/vm"
)

// ensureDocker installs docker + the compose v2 plugin on a byo host that is going
// to need them, and returns the gate everything downstream of docker must DependsOn.
// nil means this host needs no install — see hostNeedsDocker.
//
// It is NOT part of ensureReady, and that separation is the point: readiness is a
// property of the MACHINE (does it boot, can SC sudo, do SC's directories exist),
// and a k3s node with no docker on it is a perfectly ready machine. This is a
// property of what the machine is FOR. Folding the two together is what produced the
// fault the split fixed — a byo host declared for a k3s cluster failing its vm
// readiness check on `docker is not installed`, with the k3s install never running
// because it DependsOn that gate (live-caught 2026-10-18).
//
// Ordering matters as much as the install: the vm's Caddy ingress is a docker
// container brought up during `sc provision`, so ensureIngress has to run after
// this, which is why the gate is returned rather than just created.
func ensureDocker(
	ctx *sdk.Context,
	input api.ResourceInput,
	cfg *vm.VmConfig,
	h HostFacts,
	drv Driver,
	params pApi.ProvisionParams,
	ready *remote.Command,
) (*remote.Command, error) {
	if !hostNeedsDocker(cfg, h, drv) {
		return nil, nil
	}

	script := vm.ByoDockerInstallScript(h.User)
	cmd, err := remote.NewCommand(ctx, input.ToResName(h.Name+"-docker"), &remote.CommandArgs{
		Connection: ConnectionArgs(h),
		Create:     sdk.String(script),
		Update:     sdk.String(script),
		// Guarded change: the script begins by asking the machine whether docker is
		// there, and on every provision after the first it downloads nothing,
		// restarts nothing and changes nothing. That satisfies all three
		// preconditions of TriggersForGuardedChange — including the third, no Delete.
		// There deliberately is none: SC installed docker on a machine it does not
		// own, and uninstalling an operator's docker because a stack was destroyed
		// would be the single most destructive thing in this package (every other
		// container on that host goes with it).
		//
		// The nonce is what makes it repair drift. Nothing in the trigger set moves
		// when a host is rebuilt, re-imaged, or has docker removed between deploys —
		// the config did not change, the machine did — so without it the one provision
		// that would put docker back is the one that is skipped. Same fault shape as
		// the ingress had until 2026-10-18 (see nonce.go).
		Triggers: TriggersForGuardedChange(h.Address, sdk.String(hashOf(script))),
		// The same uniform shape as every other command in this package, for the
		// reason ensureReady's comment gives: a new command inherits the safe
		// defaults rather than relying on whoever adds it. Nothing is lost by
		// silencing the install — every guard in the script writes its own sentence
		// to stderr, and pulumi-command puts a failed command's stderr in the error.
		Logging:                remote.LoggingNone.ToLoggingPtrOutput(),
		AddPreviousOutputInEnv: sdk.Bool(false),
	}, sdk.DependsOn([]sdk.Resource{ready}))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to install docker on host %q of vm %q", h.Name, input.Descriptor.Name)
	}
	params.Log.Info(ctx.Context(),
		"vm %q ensures docker + compose v2 on byo host %q (role %q, ingress: %t)",
		input.Descriptor.Name, h.Name, h.Role, cfg.IngressEnabled())
	return cmd, nil
}

// hostNeedsDocker decides whether SC should put docker on this machine.
//
// Three rules, in the order they are checked:
//
//  1. A machine SC created already installed it from cloud-init
//     (vm.DefaultDockerInstallScript, same guard), so there is nothing to do and no
//     reason to open an SSH session to find that out.
//  2. An ingressed host runs the vm resource's Caddy as a docker container,
//     whatever else it is for — including a k3s node, which is a permitted
//     combination (the port-80 race is refused separately, in
//     pkg/clouds/pulumi/k3s).
//  3. Otherwise, only a host that will run `docker compose`. A k3s node brings its
//     own containerd and genuinely does not need docker, and installing it there is
//     not free: docker sets the iptables FORWARD policy to DROP and adds its own
//     chains, on a node whose pod networking is flannel's business.
//
// Note what rule 3 does with a host whose `role:` was left empty: VmHost.EffectiveRole
// resolves that to `compose`, so such a host gets docker. That is the right default
// for the single-host compose stack every pre-role config is, and it is one more
// reason a k3s pool should name its roles — which the field's own documentation
// already asks for.
func hostNeedsDocker(cfg *vm.VmConfig, h HostFacts, drv Driver) bool {
	if drv.Capabilities().CloudInit {
		return false
	}
	if cfg.IngressEnabled() {
		return true
	}
	return !h.Role.IsK3s()
}
