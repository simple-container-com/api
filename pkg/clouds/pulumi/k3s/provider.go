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
//
// Two config shapes reach here, because initProvider runs on the deploy path too
// (deploy.go:193 → getProvisionParams → provision.go:281) against the converted
// client-stack config:
//
//   - *k3s.K3sClusterConfig — provisioning a cluster. The command provider is the one
//     actually used: standing up k3s is SSH and nothing else.
//   - *k3s.K3sCloudrunInput — deploying a workload ONTO a cluster. The returned
//     provider is never used; K3sCloudrun replaces params.Provider with a Kubernetes
//     provider built from the parent's kubeconfig export, which is the only place that
//     kubeconfig exists. The config still has to report a provider type that resolves
//     to something, and this is why it reports k3s rather than "kubernetes" — see
//     K3sCloudrunTemplate.ProviderType.
func Provider(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	switch cfg := input.Descriptor.Config.Config.(type) {
	case *k3s.K3sClusterConfig:
		if err := cfg.Validate(); err != nil {
			return nil, errors.Wrapf(err, "invalid %s resource %q in stack %q",
				k3s.ResourceTypeK3sCluster, input.Descriptor.Name, stack.Name)
		}
	case *k3s.K3sCloudrunInput:
		// Nothing to validate that has not been validated already: the template's own
		// Validate ran in ToK3sCloudrunConfig, before the client stack was converted.
	default:
		return nil, errors.Errorf("failed to cast config to a k3s config for %q (given %T)",
			input.Descriptor.Name, input.Descriptor.Config.Config)
	}

	provider, err := command.NewProvider(ctx, input.ToResName(input.Descriptor.Name), &command.ProviderArgs{})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to init command provider for %q of type %q",
			input.Descriptor.Name, input.Descriptor.Type)
	}
	return &api.ResourceOutput{Ref: provider}, nil
}
