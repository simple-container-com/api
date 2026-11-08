// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package kubernetes

import (
	"github.com/pkg/errors"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

func Provider(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	pcfg, ok := input.Descriptor.Config.Config.(api.AuthConfig)
	if !ok {
		return nil, errors.Errorf("failed to cast config to api.AuthConfig")
	}

	kubeconfig, err := providerKubeconfig(pcfg, params)
	if err != nil {
		return nil, errors.Wrapf(err, "invalid kubeconfig for %q", input.Descriptor.Name)
	}

	provider, err := kubernetes.NewProvider(ctx, input.ToResName(input.Descriptor.Name), &kubernetes.ProviderArgs{
		Kubeconfig:            kubeconfig,
		EnableServerSideApply: sdk.BoolPtr(true), // Required for DeploymentPatch resources
	})

	return &api.ResourceOutput{
		Ref: provider,
	}, err
}

// providerKubeconfig produces the provider's kubeconfig from either a literal
// value or a `${resource:<cluster>.kubeconfig}` reference to a sibling resource
// of the same parent stack and environment.
//
// The reference is resolved HERE, at the one point that has both the sibling's
// outputs and a Pulumi context, and not by the placeholder engine: the value does
// not exist until the referenced cluster has been provisioned in this same
// update, so it can only be an Output. api.ResourceRef records why the literal-
// only field was worth fixing.
//
// A reference that cannot be resolved is a refusal, never a fallback to the
// literal string. Handing `${resource:...}` to kubernetes.NewProvider as a
// kubeconfig would fail later, from inside an apply, as a connection error naming
// the cluster — the one place an operator would not look for a typo.
func providerKubeconfig(pcfg api.AuthConfig, params pApi.ProvisionParams) (sdk.StringOutput, error) {
	ref, isRef, err := api.ParseResourceRef(pcfg.CredentialsValue())
	if err != nil {
		return sdk.StringOutput{}, err
	}
	if !isRef {
		return SecretKubeconfig(pcfg.CredentialsValue()), nil
	}
	value, err := pApi.ResolveResourceRef(ref, params.ResourceOutputs)
	if err != nil {
		return sdk.StringOutput{}, err
	}
	// Marked again rather than trusted. A producer that already marked it (k3s
	// does) loses nothing, and a producer that did not would otherwise put a
	// cluster-admin credential into the provider's properties in the Pulumi
	// checkpoint in cleartext — see SecretKubeconfigOutput.
	return SecretKubeconfigOutput(value), nil
}
