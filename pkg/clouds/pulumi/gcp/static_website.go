package gcp

import (
	"github.com/pkg/errors"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/gcloud"
)

func ProvisionStaticWebsite(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	if input.Descriptor.Type != gcloud.TemplateTypeStaticWebsite {
		return nil, errors.Errorf("unsupported bucket type %q", input.Descriptor.Type)
	}

	input.Log.Error(ctx.Context(), "TODO: Not implemented yet for gcp static website")

	return &api.ResourceOutput{Ref: nil}, nil
}
