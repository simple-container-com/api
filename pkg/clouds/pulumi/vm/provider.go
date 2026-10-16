// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package vm

import (
	"github.com/pkg/errors"

	"github.com/pulumi/pulumi-command/sdk/go/command"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	"github.com/simple-container-com/api/pkg/clouds/vm"
)

// Provider instantiates the Pulumi provider a vm resource provisions through.
//
// Unusual shape worth stating plainly: provision.go's initProvider looks a provider
// up by the config's ProviderType(), which for every vm resource is "vm" regardless
// of which cloud creates the machine. So this one function dispatches on the
// resource's `provider:` field, and the cloud difference stays inside the driver tier
// instead of leaking into SC's provider registry as six near-identical entries.
//
// For `byo` the provider is pulumi-command's: there is no cloud API in play, only
// SSH. For a cloud driver it will be that cloud's provider, built from the
// credentials blob — which is why this stays a switch rather than a constant.
//
// It is also the earliest point at which a vm config is seen, so it is where
// Validate runs: an operator with a typo should hear about the typo, not about a
// provider that could not be built.
func Provider(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	cfg, ok := input.Descriptor.Config.Config.(*vm.VmConfig)
	if !ok {
		return nil, errors.Errorf("failed to cast config to vm.VmConfig for %q (given %T)",
			input.Descriptor.Name, input.Descriptor.Config.Config)
	}
	if err := cfg.Validate(); err != nil {
		return nil, errors.Wrapf(err, "invalid vm resource %q in stack %q", input.Descriptor.Name, stack.Name)
	}

	switch cfg.Driver {
	case vm.DriverByo:
		provider, err := command.NewProvider(ctx, input.ToResName(input.Descriptor.Name), &command.ProviderArgs{})
		if err != nil {
			return nil, errors.Wrapf(err, "failed to init command provider for vm %q", input.Descriptor.Name)
		}
		return &api.ResourceOutput{Ref: provider}, nil
	default:
		// Unreachable: Validate above refuses every provider without a driver. Kept
		// as a real error rather than a panic so that adding a driver to
		// vm.Drivers without adding it here fails legibly.
		return nil, errors.Errorf("no provider implementation for vm provider %q", cfg.Driver)
	}
}
