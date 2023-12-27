package gcp

import (
	"fmt"

	"github.com/pulumi/pulumi-gcp/sdk/v6/go/gcp/kms"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/samber/lo"

	"api/pkg/api"
)

type KmsKeyInput struct {
	KeyRingName       string
	KeyName           string
	KeyLocation       string
	KeyRotationPeriod string
	Provider          sdk.ProviderResource
}

func ProvisionKmsKey(ctx *sdk.Context, stack api.Stack, params KmsKeyInput) (*kms.CryptoKey, error) {
	// Create a new KeyRing for stack
	keyRing, err := kms.NewKeyRing(ctx, stack.Name, &kms.KeyRingArgs{
		Location: sdk.String(params.KeyLocation),
	}, sdk.Provider(params.Provider))
	if err != nil {
		return nil, err
	}

	// Create a new CryptoKey associated with the KeyRing.
	rotationPeriod := lo.If(params.KeyRotationPeriod == "", "100000s").Else(params.KeyRotationPeriod)

	key, err := kms.NewCryptoKey(ctx, params.KeyName, &kms.CryptoKeyArgs{
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
	ctx.Export(fmt.Sprintf("%s-keyring", params.KeyRingName), keyRing.Name)
	ctx.Export(fmt.Sprintf("%s-key", params.KeyName), key.Name)
	return key, nil
}
