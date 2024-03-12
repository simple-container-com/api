package gcp

import (
	"github.com/pkg/errors"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/gcloud"
	"github.com/simple-container-com/api/pkg/clouds/pulumi/params"
)

func ProvisionCloudrun(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params params.ProvisionParams) (*api.ResourceOutput, error) {
	if input.Descriptor.Type != gcloud.TemplateTypeGcpCloudrun {
		return nil, errors.Errorf("unsupported template type %q", input.Descriptor.Type)
	}

	cloudrunInput, ok := input.Descriptor.Config.Config.(*gcloud.CloudRunInput)
	if !ok {
		return nil, errors.Errorf("failed to convert cloudrun config for %q", input.Descriptor.Type)
	}

	return nil, errors.Errorf("not implemented for %q", cloudrunInput)
	//return &api.ResourceOutput{Ref: nil}, nil
}
