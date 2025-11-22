package gcp

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	gcpOptions "google.golang.org/api/option"
	"google.golang.org/api/serviceusage/v1"

	"github.com/pkg/errors"
	"github.com/samber/lo"

	"github.com/pulumi/pulumi-gcp/sdk/v8/go/gcp/kms"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/logger/color"
	"github.com/simple-container-com/api/pkg/clouds/gcloud"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

func KmsKeySecretsProvider(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	kmsInput, ok := input.Descriptor.Config.Config.(*gcloud.SecretsProviderConfig)
	if !ok {
		return nil, errors.Errorf("failed to convert KmsKeyInput for %q", input.Descriptor.Type)
	}

	if err := enableServicesAPI(ctx.Context(), input.Descriptor.Config.Config,
		fmt.Sprintf("projects/%s/services/serviceusage.googleapis.com", kmsInput.ProjectId)); err != nil {
		_, _ = os.Stderr.WriteString(color.RedFmt("service usage API seems to be disabled on project %q, "+
			"please enable it manually with command or in the GCP Console ", kmsInput.ProjectId))
		_, _ = os.Stderr.WriteString(color.YellowFmt("`gcloud services enable serviceusage.googleapis.com --project %s`", kmsInput.ProjectId))
		return nil, errors.Wrapf(err, "serviceusage API is not enabled on project %q", kmsInput.ProjectId)
	}
	kmsServiceName := fmt.Sprintf("projects/%s/services/cloudkms.googleapis.com", kmsInput.ProjectId)
	if err := enableServicesAPI(ctx.Context(), input.Descriptor.Config.Config, kmsServiceName); err != nil {
		return nil, errors.Wrapf(err, "failed to enable %s", kmsServiceName)
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

func enableServicesAPI(ctx context.Context, authConfig any, apiName string) error {
	svc, err := initServicesAPIClient(ctx, authConfig)
	if err != nil {
		return errors.Wrapf(err, "failed to init services API client")
	}

	if info, err := svc.Services.Get(apiName).Do(); err == nil {
		if info.State == "ENABLED" {
			// already enabled
			return nil
		}
	}
	op, err := svc.Services.Enable(apiName, &serviceusage.EnableServiceRequest{}).Do()
	if err != nil {
		return errors.Wrapf(err, "failed to enable %s", apiName)
	}

	// Handle immediate completion or operations that don't need polling
	if op.Done {
		// Operation completed immediately
		if op.Error != nil {
			return errors.Errorf("failed to enable API %q: %s", apiName, op.Error.Message)
		}
		return nil
	}

	// Poll for operation completion with improved error handling
	maxRetries := 60 // Maximum 60 seconds
	for i := 0; i < maxRetries; i++ {
		// Add context cancellation check
		select {
		case <-ctx.Done():
			return errors.Wrapf(ctx.Err(), "context cancelled while enabling API %q", apiName)
		default:
		}

		op, err = svc.Operations.Get(op.Name).Do()
		if err != nil {
			// Handle specific "DONE_OPERATION" error
			if strings.Contains(err.Error(), "DONE_OPERATION") {
				// Operation is already done, check final state
				if info, checkErr := svc.Services.Get(apiName).Do(); checkErr == nil {
					if info.State == "ENABLED" {
						return nil
					}
				}
				return errors.Wrapf(err, "API enablement operation completed with error for %q", apiName)
			}
			return errors.Wrapf(err, "failed to check operation status for API %q", apiName)
		}

		if op.Done {
			if op.Error != nil {
				return errors.Errorf("failed to enable API %q: %s", apiName, op.Error.Message)
			}
			return nil
		}

		time.Sleep(1 * time.Second)
	}

	return errors.Errorf("timeout waiting for API %q to be enabled after %d seconds", apiName, maxRetries)
}

func initServicesAPIClient(ctx context.Context, resourceConfig any) (*serviceusage.Service, error) {
	authCfg, ok := resourceConfig.(api.AuthConfig)
	if !ok {
		return nil, errors.Errorf("failed to convert config to api.AuthConfig")
	}
	svc, err := serviceusage.NewService(ctx, gcpOptions.WithCredentialsJSON([]byte(authCfg.CredentialsValue())))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to init google API services client")
	}
	return svc, nil
}
