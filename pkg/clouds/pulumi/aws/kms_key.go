package aws

import (
	"github.com/pkg/errors"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/kms"
	"github.com/simple-container-com/api/pkg/clouds/aws"

	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"

	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/simple-container-com/api/pkg/api"
)

func ProvisionKmsKey(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	kmsInput, ok := input.Descriptor.Config.Config.(*aws.SecretsProviderConfig)
	if !ok {
		return nil, errors.Errorf("failed to convert KmsKeyInput for %q", input.Descriptor.Type)
	}

	// Create a new KMS Key for encryption/decryption operations
	key, err := kms.NewKey(ctx, kmsInput.KeyName, &kms.KeyArgs{
		Tags: sdk.StringMap{
			"stack": sdk.String(stack.Name),
		},
	}, sdk.Provider(params.Provider))
	if err != nil {
		return nil, errors.Wrap(err, "failed to provision KMS key")
	}

	return &api.ResourceOutput{
		Ref: key,
	}, nil
}
