package gcp

import (
	"github.com/pkg/errors"

	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/gcloud"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

func GkeAutopilot(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	if input.Descriptor.Type != gcloud.ResourceTypeGkeAutopilot {
		return nil, errors.Errorf("unsupported resource type %q", input.Descriptor.Type)
	}

	gkeAutopilotConfig, ok := input.Descriptor.Config.Config.(*gcloud.GkeAutopilotResource)
	if !ok {
		return nil, errors.Errorf("failed to convert gke autopilot config for %q", input.Descriptor.Type)
	}

	params.Log.Info(ctx.Context(), "not implemented for gke autopilot %v", gkeAutopilotConfig)

	return &api.ResourceOutput{Ref: nil}, nil
}

func GkeAutopilotComputeProcessor(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, collector pApi.ComputeContextCollector, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	params.Log.Error(ctx.Context(), "not implemented for gke autopilot")

	return &api.ResourceOutput{
		Ref: nil,
	}, nil
}
