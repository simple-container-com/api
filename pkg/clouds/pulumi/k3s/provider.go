// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package k3s

import (
	"github.com/pkg/errors"

	"github.com/pulumi/pulumi-command/sdk/go/command"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/k3s"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

// Provider instantiates the Pulumi provider a k3s-cluster resource provisions
// through: pulumi-command's, because standing up k3s is SSH and nothing else. The
// cloud that created the machine is the vm resource's business and has already
// happened by the time this runs.
//
// It is also the earliest point at which a k3s config is seen, so it is where
// Validate runs — an operator with a typo should hear about the typo, not about a
// provider that could not be built. Same shape as pkg/clouds/pulumi/vm/provider.go.
func Provider(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	cfg, ok := input.Descriptor.Config.Config.(*k3s.K3sClusterConfig)
	if !ok {
		return nil, errors.Errorf("failed to cast config to k3s.K3sClusterConfig for %q (given %T)",
			input.Descriptor.Name, input.Descriptor.Config.Config)
	}
	if err := cfg.Validate(); err != nil {
		return nil, errors.Wrapf(err, "invalid %s resource %q in stack %q",
			k3s.ResourceTypeK3sCluster, input.Descriptor.Name, stack.Name)
	}

	provider, err := command.NewProvider(ctx, input.ToResName(input.Descriptor.Name), &command.ProviderArgs{})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to init command provider for %s %q",
			k3s.ResourceTypeK3sCluster, input.Descriptor.Name)
	}
	return &api.ResourceOutput{Ref: provider}, nil
}
