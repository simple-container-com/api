// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package cloudru

import (
	"github.com/pkg/errors"

	"github.com/pulumi/pulumi-command/sdk/go/command"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

// Provider returns the provider a `cloudru-container-app` stack provisions
// through: pulumi-command's.
//
// There is no Pulumi (or Terraform) provider for Cloud.ru Evolution Container
// Apps, which is the whole reason this driver exists. The resources it does create
// are a docker.Image and three local.Commands, and only the commands need a
// provider.
//
// A Provider function is nevertheless mandatory, not optional: provision.go's
// initProvider looks one up by the config's ProviderType() — "cloudru" — and
// returns `unsupported provider type %q` when there is none, on the deploy path as
// well as the provision path (deploy.go:193). Without this registration the first
// `sc deploy` of a Container App fails before a single API call. Same shape as
// pkg/clouds/pulumi/vm/provider.go's `byo` branch, for the same reason.
//
// Note which Cloud.ru reaches here. ProviderType is shared by both estates, but a
// machine on either is a `vm` resource whose config reports "vm", so the `vm`
// package's Provider handles it. Only the Evolution Container App template arrives
// here, and it talks REST over an IAM bearer token rather than through a provider.
func Provider(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	provider, err := command.NewProvider(ctx, input.ToResName(input.Descriptor.Name), &command.ProviderArgs{})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to init command provider for cloud.ru stack %q", input.Descriptor.Name)
	}
	return &api.ResourceOutput{Ref: provider}, nil
}
