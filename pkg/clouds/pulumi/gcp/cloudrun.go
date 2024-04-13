package gcp

import (
	"fmt"

	"github.com/pkg/errors"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/gcloud"
)

func Cloudrun(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	if input.Descriptor.Type != gcloud.TemplateTypeGcpCloudrun {
		return nil, errors.Errorf("unsupported template type %q", input.Descriptor.Type)
	}

	for _, dep := range params.ComputeContext.Dependencies() {
		dep.URN().ApplyT(func(urn string) (any, error) {
			fmt.Println(urn)
			return nil, nil
		})
	}
	cloudrunInput, ok := input.Descriptor.Config.Config.(*gcloud.CloudRunInput)
	if !ok {
		return nil, errors.Errorf("failed to convert cloudrun config for %q", input.Descriptor.Type)
	}
	params.Log.Debug(ctx.Context(), "configure cloud run for %q", cloudrunInput)

	params.Log.Error(ctx.Context(), "not implemented for %q", input.Descriptor.Type)
	return &api.ResourceOutput{Ref: nil}, nil
}
