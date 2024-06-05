package gcp

import (
	"fmt"

	"github.com/pkg/errors"
	"github.com/samber/lo"

	"github.com/pulumi/pulumi-gcp/sdk/v6/go/gcp/kms"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/gcloud"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

func KmsKeySecretsProvider(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	kmsInput, ok := input.Descriptor.Config.Config.(*gcloud.SecretsProviderConfig)
	if !ok {
		return nil, errors.Errorf("failed to convert KmsKeyInput for %q", input.Descriptor.Type)
	}

	projectId := kmsInput.ProjectIdValue()
	keyRingName := input.Descriptor.Name

	// Create a new KeyRing for stack
	keyRing, err := kms.NewKeyRing(ctx, keyRingName, &kms.KeyRingArgs{
		Name:     sdk.String(keyRingName),
		Location: sdk.String(kmsInput.KeyLocation),
	}, sdk.Provider(params.Provider))
	if err != nil {
		return nil, err
	}

	// Create a new CryptoKey associated with the KeyRing.
	rotationPeriod := lo.If(kmsInput.KeyRotationPeriod == "", "100000s").Else(kmsInput.KeyRotationPeriod)

	key, err := kms.NewCryptoKey(ctx, input.ToResName(input.Descriptor.Name), &kms.CryptoKeyArgs{
		Name:           sdk.String(input.Descriptor.Name),
		KeyRing:        keyRing.ID(),               // Reference the ID of the KeyRing created above.
		RotationPeriod: sdk.String(rotationPeriod), // Define key rotation period in seconds.
		VersionTemplate: &kms.CryptoKeyVersionTemplateArgs{
			Algorithm:       sdk.String("GOOGLE_SYMMETRIC_ENCRYPTION"),
			ProtectionLevel: sdk.String("SOFTWARE"),
		},
	}, sdk.Provider(params.Provider))
	if err != nil {
		return nil, err
	}

	ctx.Export(input.Descriptor.Name, sdk.All(keyRing.Name, key.Name).ApplyT(func(args []any) (string, error) {
		keyRingName, keyName := args[0].(string), args[1].(string)
		return fmt.Sprintf("gcpkms://projects/%s/locations/%s/keyRings/%s/cryptoKeys/%s",
			projectId, kmsInput.KeyLocation, keyRingName, keyName), nil
	}))

	return &api.ResourceOutput{Ref: key}, nil
}
