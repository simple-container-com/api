package aws

import (
	"fmt"

	"github.com/pkg/errors"
	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/kms"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/aws"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

func KmsKeySecretsProvider(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	kmsInput, ok := input.Descriptor.Config.Config.(*aws.SecretsProviderConfig)
	if !ok {
		return nil, errors.Errorf("failed to convert KmsKeyInput for %q", input.Descriptor.Type)
	}

	var pcfg aws.AccountConfig
	if err := api.ConvertAuth(kmsInput, &pcfg); err != nil {
		return nil, errors.Wrapf(err, "failed to convert auth config to aws.AccountConfig")
	}

	// Create a new KMS Key for encryption/decryption operations
	key, err := kms.NewKey(ctx, input.Descriptor.Name, &kms.KeyArgs{
		Tags: sdk.StringMap{
			"stack": sdk.String(stack.Name),
		},
	}, sdk.Provider(params.Provider))
	if err != nil {
		return nil, errors.Wrap(err, "failed to provision KMS key")
	}

	ctx.Export(input.Descriptor.Name, key.KeyId.ApplyT(func(keyId string) (string, error) {
		return fmt.Sprintf("awskms://%s?region=%s", keyId, pcfg.Region), nil
	}))

	return &api.ResourceOutput{
		Ref: key.KeyId,
	}, nil
}
