// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package vm

import (
	"github.com/pkg/errors"

	"github.com/pulumi/pulumi-command/sdk/go/command"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/cloudru"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	"github.com/simple-container-com/api/pkg/clouds/vm"
	sdkSbercloud "github.com/simple-container-com/pulumi-sbercloud/sdk/go/sbercloud"
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
	// A `vm-compose` stack arrives here too, because initProvider is called for
	// templates as well as resources. It needs the command provider and nothing else:
	// the machine already exists, every host fact including the deploy key is read from
	// the parent stack, and there is no cloud API in a delivery that is only SSH. Which
	// cloud created the machine is therefore not knowable here and not needed — the
	// `vm` resource in the parent stack is where a driver is dispatched.
	if _, ok := input.Descriptor.Config.Config.(*vm.VmComposeInput); ok {
		provider, err := command.NewProvider(ctx, input.ToResName(input.Descriptor.Name), &command.ProviderArgs{})
		if err != nil {
			return nil, errors.Wrapf(err, "failed to init command provider for stack %q", input.Descriptor.Name)
		}
		return &api.ResourceOutput{Ref: provider}, nil
	}

	cfg, ok := input.Descriptor.Config.Config.(*vm.VmConfig)
	if !ok {
		return nil, errors.Errorf("failed to cast config to vm.VmConfig for %q (given %T)",
			input.Descriptor.Name, input.Descriptor.Config.Config)
	}
	if err := cfg.Validate(); err != nil {
		return nil, errors.Wrapf(err, "invalid vm resource %q in stack %q", input.Descriptor.Name, stack.Name)
	}

	switch cfg.Driver {
	// Both of these get pulumi-command's provider, for opposite reasons. With `byo`
	// there is no cloud API in play at all. With `cloudru-evolution-vm` there is one,
	// but no Pulumi provider for it exists — the machine is converged by
	// `sc cloudru compute-vm` inside a local.Command, so the only provider the
	// resource needs is the one that runs commands (see cloudru_evolution.go).
	case vm.DriverByo, vm.DriverCloudruEvolution:
		provider, err := command.NewProvider(ctx, input.ToResName(input.Descriptor.Name), &command.ProviderArgs{})
		if err != nil {
			return nil, errors.Wrapf(err, "failed to init command provider for vm %q", input.Descriptor.Name)
		}
		return &api.ResourceOutput{Ref: provider}, nil
	case vm.DriverCloudru:
		provider, err := cloudruProvider(ctx, input, cfg)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to init cloudru provider for vm %q", input.Descriptor.Name)
		}
		return &api.ResourceOutput{Ref: provider}, nil
	default:
		// Unreachable: Validate above refuses every provider without a driver. Kept
		// as a real error rather than a panic so that adding a driver to
		// vm.Drivers without adding it here fails legibly.
		return nil, errors.Errorf("no provider implementation for vm provider %q", cfg.Driver)
	}
}

// cloudruProvider builds the Cloud.ru Advanced provider from the resource's
// `credentials:` blob.
//
// Note what drives the region: the RESOURCE's `region:`, not the credential's. A vm
// resource always has one (Validate requires it), and WithRegion carries it into the
// Keystone endpoint and the project name as well — because on this cloud those three
// are one decision, and setting only the provider's region authenticates against one
// region's tenancy while creating in another.
func cloudruProvider(ctx *sdk.Context, input api.ResourceInput, cfg *vm.VmConfig) (*sdkSbercloud.Provider, error) {
	var account cloudru.AccountConfig
	if err := api.ConvertAuth(cfg, &account); err != nil {
		return nil, errors.Wrapf(err,
			"failed to read Cloud.ru credentials; `credentials:` must reference an `auth:` entry of type %q",
			cloudru.AuthTypeCloudruAccessKey)
	}
	if err := account.Validate(); err != nil {
		return nil, err
	}
	account = *account.WithRegion(cfg.Region)

	args := &sdkSbercloud.ProviderArgs{
		AccessKey: sdk.StringPtr(account.AccessKey),
		// secretStringPtr, never sdk.StringPtr: the provider's own inputs are printed
		// in a Pulumi update summary and archived in the CI log unless marked, which
		// is how a Yandex service-account private key reached a world-readable Actions
		// log on 2026-09-30.
		SecretKey:   secretStringPtr(account.SecretKey),
		AuthUrl:     sdk.StringPtr(account.EffectiveAuthURL()),
		Region:      sdk.StringPtr(account.EffectiveRegion()),
		ProjectName: sdk.StringPtr(account.EffectiveProjectName()),
	}
	// Set only when given. The provider derives each of these from the key pair
	// otherwise, and an empty string is not the same as absent: it is a scope that
	// matches nothing, and Keystone answers it with a 401 that names neither field.
	if account.SecurityToken != "" {
		args.SecurityToken = secretStringPtr(account.SecurityToken)
	}
	if account.DomainName != "" {
		args.DomainName = sdk.StringPtr(account.DomainName)
	}
	if account.DomainID != "" {
		args.DomainId = sdk.StringPtr(account.DomainID)
	}
	if account.EnterpriseProjectID != "" {
		args.EnterpriseProjectId = sdk.StringPtr(account.EnterpriseProjectID)
	}

	return sdkSbercloud.NewProvider(ctx, input.ToResName(input.Descriptor.Name), args)
}

// secretStringPtr is secretString for a provider argument that takes a pointer.
// Same reasoning, same reason it is three duplicated lines rather than a utility
// package: see secretString in byo.go.
func secretStringPtr(value string) sdk.StringPtrInput {
	return secretString(value).ToStringPtrOutput()
}
