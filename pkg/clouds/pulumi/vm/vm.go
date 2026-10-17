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

		ctx.Export(ToHostAddressExport(input.Descriptor.Name, i), h.Address)
		ctx.Export(ToHostUserExport(input.Descriptor.Name, i), sdk.String(h.User))
		ctx.Export(ToHostPortExport(input.Descriptor.Name, i), sdk.Int(h.Port))
		// The deploy key has to cross the stack boundary: the machine is described in
		// the parent stack, and the template that deploys onto it runs in the child
		// stack and reads these outputs. Exported as a secret — which is also what
		// encrypts it in the checkpoint — and never logged.
		ctx.Export(ToHostPrivateKeyExport(input.Descriptor.Name, i), sdk.ToSecret(h.PrivateKey))
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

func ToIngressEnabledExport(vmName string) string {
	return fmt.Sprintf("%s-vm-ingress-enabled", vmName)
}
