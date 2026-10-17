// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package vm

import (
	"fmt"

	"github.com/pkg/errors"

	"github.com/pulumi/pulumi-command/sdk/go/command/remote"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	"github.com/simple-container-com/api/pkg/clouds/vm"
)

// VmOutput is what a provisioned vm resource hands to the templates above it.
type VmOutput struct {
	Hosts []HostFacts
	// Ready is the readiness gate per host, in the same order as Hosts. A template
	// that runs anything on a machine must DependsOn the matching entry.
	Ready []*remote.Command
}

// Vm provisions the machines of one `type: vm` resource and gates them on readiness.
//
// What it deliberately does NOT do: install anything application-shaped. A vm
// resource produces a reachable, docker-capable machine and a set of facts; what runs
// on it is the business of the templates above (vm-compose next, then k3s). That
// boundary is the reason the VM path does not have to be written twice
// (vm-matrix.md §1, D9).
func Vm(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	cfg, ok := input.Descriptor.Config.Config.(*vm.VmConfig)
	if !ok {
		return nil, errors.Errorf("failed to cast config to vm.VmConfig for %q (given %T)",
			input.Descriptor.Name, input.Descriptor.Config.Config)
	}
	// Validated again rather than trusted: Provider is the normal path in, but this
	// ProvisionFunc is reachable on its own and validation is a pure check.
	if err := cfg.Validate(); err != nil {
		return nil, errors.Wrapf(err, "invalid vm resource %q in stack %q", input.Descriptor.Name, stack.Name)
	}
	if params.Provider == nil {
		return nil, errors.Errorf("provider must not be nil for vm %q in stack %q", input.Descriptor.Name, stack.Name)
	}

	drv, err := driverFor(cfg)
	if err != nil {
		return nil, err
	}
	params.Log.Info(ctx.Context(), "configure vm %q for stack %q using driver %q",
		input.Descriptor.Name, stack.Name, drv.Name())

	hosts, err := drv.Hosts(ctx, input, cfg, params)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to resolve hosts of vm %q in stack %q", input.Descriptor.Name, stack.Name)
	}
	if len(hosts) == 0 {
		return nil, errors.Errorf("driver %q produced no hosts for vm %q", drv.Name(), input.Descriptor.Name)
	}

	out := &VmOutput{Hosts: hosts}
	for i, h := range hosts {
		ready, err := ensureReady(ctx, input, cfg, h, drv, params)
		if err != nil {
			return nil, err
		}
		out.Ready = append(out.Ready, ready)

		// The ingress, if the machine runs one. Installed by the RESOURCE rather than
		// by any stack's template, and that is the decision the whole TLS path rests
		// on: one machine has one port 443, so a Caddy inside a stack's compose
		// project would fail to bind the moment a second stack was deployed onto the
		// same host — and would be destroyed and recreated by every blue-green flip
		// it is supposed to be performing (compose-on-vm.md §3, D15).
		if cfg.IngressEnabled() {
			if err := ensureIngress(ctx, input, cfg, h, params, ready); err != nil {
				return nil, err
			}
		} else if h.Role == vm.RoleCompose {
			// Said once, here. Without the ingress a stack is reachable only on a
			// host port, which is a supported mode (`ingress.enable: false`) but not
			// one anybody chooses by accident.
			params.Log.Info(ctx.Context(),
				"vm %q host %q has no ingress: stacks deployed onto it publish a host port and are served over "+
					"plain HTTP. Set `ingress.enable: true` and `ingress.email` for HTTPS",
				input.Descriptor.Name, h.Name)
		}

		ctx.Export(ToHostAddressExport(input.Descriptor.Name, i), h.Address)
		ctx.Export(ToHostUserExport(input.Descriptor.Name, i), sdk.String(h.User))
		ctx.Export(ToHostPortExport(input.Descriptor.Name, i), sdk.Int(h.Port))
		// The deploy key has to cross the stack boundary: the machine is described in
		// the parent stack, and the template that deploys onto it runs in the child
		// stack and reads these outputs. Exported as a secret — which is also what
		// encrypts it in the checkpoint — and never logged.
		ctx.Export(ToHostPrivateKeyExport(input.Descriptor.Name, i), sdk.ToSecret(h.PrivateKey))
		// Exported even though it is empty for every key SC itself would generate.
		// An operator's existing deploy key may be an encrypted PEM, and without
		// this the parent stack's own readiness check would connect while every
		// child deploy failed to decrypt the same key — a split that would read as
		// "the machine is fine but deploys don't work".
		ctx.Export(ToHostPrivateKeyPasswordExport(input.Descriptor.Name, i), sdk.ToSecret(h.PrivateKeyPassword))
		// The host key crosses the same boundary and for the same reason — the
		// template that ships images and writes env files onto this machine runs in
		// the child stack. Exported in the clear: it is the machine's public key.
		ctx.Export(ToHostKeyExport(input.Descriptor.Name, i), sdk.String(h.HostKey))
		if h.HostKey == "" {
			// Said once, here, rather than on every command that connects. It is
			// also the only place that knows the host was declared without one.
			params.Log.Warn(ctx.Context(),
				"vm %q host %q has no `hostKey`: SSH cannot verify the machine's identity, so the deploy key "+
					"and every secret of every stack deployed onto it go to whoever answers on that address. "+
					"Pin it with the output of `ssh-keyscan -t ed25519 <address>`",
				input.Descriptor.Name, h.Name)
		}
	}
	ctx.Export(ToHostCountExport(input.Descriptor.Name), sdk.Int(len(hosts)))
	ctx.Export(ToIngressEnabledExport(input.Descriptor.Name), sdk.Bool(cfg.IngressEnabled()))

	return &api.ResourceOutput{Ref: out}, nil
}

// The stack-output names a vm resource publishes. Unlike the kubeconfig export name,
// which has to match gcp-gke-autopilot's byte for byte to reuse the whole layer above
// it, these are new and only have to be stable — so they are defined once, here, and
// read back through the same helpers.
func ToHostCountExport(vmName string) string {
	return fmt.Sprintf("%s-vm-host-count", vmName)
}

func ToHostAddressExport(vmName string, idx int) string {
	return fmt.Sprintf("%s-vm-host-%d-address", vmName, idx)
}

func ToHostUserExport(vmName string, idx int) string {
	return fmt.Sprintf("%s-vm-host-%d-user", vmName, idx)
}

func ToHostPortExport(vmName string, idx int) string {
	return fmt.Sprintf("%s-vm-host-%d-port", vmName, idx)
}

func ToHostPrivateKeyExport(vmName string, idx int) string {
	return fmt.Sprintf("%s-vm-host-%d-private-key", vmName, idx)
}

func ToHostPrivateKeyPasswordExport(vmName string, idx int) string {
	return fmt.Sprintf("%s-vm-host-%d-private-key-password", vmName, idx)
}

func ToHostKeyExport(vmName string, idx int) string {
	return fmt.Sprintf("%s-vm-host-%d-host-key", vmName, idx)
}

func ToIngressEnabledExport(vmName string) string {
	return fmt.Sprintf("%s-vm-ingress-enabled", vmName)
}
