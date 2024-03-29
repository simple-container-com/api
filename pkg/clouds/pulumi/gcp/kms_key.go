package gcp

import (
	"fmt"

	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"

	"github.com/pkg/errors"

	"github.com/pulumi/pulumi-gcp/sdk/v6/go/gcp/kms"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/samber/lo"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/gcloud"
)

func ProvisionKmsKey(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	kmsInput, ok := input.Descriptor.Config.Config.(*gcloud.SecretsProviderConfig)
	if !ok {
		return nil, errors.Errorf("failed to convert KmsKeyInput for %q", input.Descriptor.Type)
	}

	keyRingName := kmsInput.KeyRingName
	if keyRingName == "" {
		keyRingName = fmt.Sprintf("%s-keyring", stack.Name)
	}

	// Create a new KeyRing for stack
	keyRing, err := kms.NewKeyRing(ctx, stack.Name, &kms.KeyRingArgs{
		Name:     sdk.String(keyRingName),
		Location: sdk.String(kmsInput.KeyLocation),
	}, sdk.Provider(params.Provider))
	if err != nil {
		return nil, err
	}

	// Create a new CryptoKey associated with the KeyRing.
	rotationPeriod := lo.If(kmsInput.KeyRotationPeriod == "", "100000s").Else(kmsInput.KeyRotationPeriod)

	key, err := kms.NewCryptoKey(ctx, kmsInput.KeyName, &kms.CryptoKeyArgs{
		Name:           sdk.String(kmsInput.KeyName),
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

	// Output the KeyRing name to access after the program runs
	ctx.Export(fmt.Sprintf("%s-keyring", kmsInput.KeyRingName), keyRing.Name)
	ctx.Export(fmt.Sprintf("%s-key", kmsInput.KeyName), key.Name)
	return &api.ResourceOutput{Ref: key}, nil
}
