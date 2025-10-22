package gcp

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	gcpStorage "cloud.google.com/go/storage"
	gcpOptions "google.golang.org/api/option"

	"github.com/pkg/errors"
	"github.com/samber/lo"

	"github.com/pulumi/pulumi-gcp/sdk/v8/go/gcp"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/logger"
	"github.com/simple-container-com/api/pkg/clouds/gcloud"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

func InitStateStore(ctx context.Context, stateStoreCfg api.StateStorageConfig, log logger.Logger) error {
	authCfg, ok := stateStoreCfg.(api.AuthConfig)
	if !ok {
		return errors.Errorf("failed to convert gcloud state storage config to api.AuthConfig")
	}
	log.Info(ctx, "Initializing gcp statestore...")
	log.Debug(ctx, "🔍 GCP Auth Debug - Raw credentials value: %s", authCfg.CredentialsValue())
	log.Debug(ctx, "🔍 GCP Auth Debug - Credentials length: %d", len(authCfg.CredentialsValue()))

	credValue := authCfg.CredentialsValue()
	if credValue == "" {
		log.Debug(ctx, "❌ GCP credentials are EMPTY!")
	} else if credValue[0] == '$' {
		log.Debug(ctx, "❌ GCP credentials contain unresolved placeholder: %s", credValue[:50])
	} else if credValue[0] == '{' {
		log.Debug(ctx, "✅ GCP credentials appear to be valid JSON")
	} else {
		log.Debug(ctx, "⚠️  GCP credentials format unknown, first 50 chars: %s", credValue[:50])
	}

	// hackily set google creds env variable, so that bucket can access it (see github.com/pulumi/pulumi/pkg/v3/authhelpers/gcpauth.go:28)
	if err := os.Setenv("GOOGLE_CREDENTIALS", credValue); err != nil {
		fmt.Println("Failed to set GOOGLE_CREDENTIALS env variable: ", err.Error())
	}

	if gcloudPath, err := exec.LookPath("gcloud"); err != nil {
		fmt.Println("WARN: Failed to find gcloud command")
	} else if f, err := os.CreateTemp(os.TempDir(), "google-creds.json"); err != nil {
		fmt.Println("WARN: failed to create temp file for google creds: ", err.Error())
	} else if _, err := f.Write([]byte(authCfg.CredentialsValue())); err != nil {
		fmt.Println("WARN: failed to write temp file for google creds: ", err.Error())
	} else if err := exec.Command(gcloudPath, "auth", "activate-service-account", "--key-file", f.Name()).Run(); err != nil {
		fmt.Println("WARN: failed to activate gcloud service account: ", err.Error())
	}

	if !stateStoreCfg.IsProvisionEnabled() {
		return nil
	}

	// provision bucket
	gcpStateCfg, ok := authCfg.(*gcloud.StateStorageConfig)
	if !ok {
		return errors.Errorf("failed to convert auth config to *gcloud.Credentials")
	}
	client, err := gcpStorage.NewClient(ctx, gcpOptions.WithCredentialsJSON([]byte(authCfg.CredentialsValue())))
	if err != nil {
		return errors.Wrapf(err, "failed to initialize gcp client")
	}
	defer func(client *gcpStorage.Client) {
		_ = client.Close()
	}(client)
	bucketRef := client.Bucket(gcpStateCfg.BucketName)

	_, err = bucketRef.Attrs(ctx)
	if err != nil {
		// does not exist
		return bucketRef.Create(ctx, gcpStateCfg.ProjectId, &gcpStorage.BucketAttrs{
			Location: lo.FromPtr(gcpStateCfg.Location),
		})
	}
	return nil
}

func Provider(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	pcfg, ok := input.Descriptor.Config.Config.(api.AuthConfig)
	if !ok {
		return nil, errors.Errorf("failed to cast config to api.AuthConfig")
	}

	creds := pcfg.CredentialsValue()
	projectId := pcfg.ProjectIdValue()

	provider, err := gcp.NewProvider(ctx, input.ToResName(input.Descriptor.Name), &gcp.ProviderArgs{
		Credentials: sdk.String(creds),
		Project:     sdk.String(projectId),
	})
	return &api.ResourceOutput{
		Ref: provider,
	}, err
}
