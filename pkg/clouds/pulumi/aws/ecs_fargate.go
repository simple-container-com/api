package aws

import (
	"github.com/pkg/errors"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/aws"
	"github.com/simple-container-com/api/pkg/clouds/pulumi/params"
)

func ProvisionEcsFargate(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params params.ProvisionParams) (*api.ResourceOutput, error) {
	if input.Descriptor.Type != aws.TemplateTypeEcsFargate {
		return nil, errors.Errorf("unsupported template type %q", input.Descriptor.Type)
	}

	cloudrunInput, ok := input.Descriptor.Config.Config.(*aws.EcsFargateInput)
	if !ok {
		return nil, errors.Errorf("failed to convert ecs_fargate config for %q", input.Descriptor.Type)
	}

	return nil, errors.Errorf("not implemented for %q", cloudrunInput)
	//return &api.ResourceOutput{Ref: nil}, nil
}
