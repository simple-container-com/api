// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package yandex

import (
	"context"
	"os"

	"github.com/pkg/errors"

	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/logger"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	"github.com/simple-container-com/api/pkg/clouds/yandex"
	// The Yandex provider SDK is our own bridge fork, so gci files it under the
	// simple-container-com prefix rather than the pulumi one where every other
	// provider SDK sits. That is correct output from .golangci.yml — longest
	// prefix wins. Don't "fix" it by re-sectioning the lint config.
	sdkYandex "github.com/simple-container-com/pulumi-yandex/sdk/go/yandex"
)

// InitStateStore points the SigV4 credential chain at Yandex Object Storage.
//
// This is the whole state-backend implementation, and it needs no Yandex Pulumi
// provider. Pulumi's DIY backend hands any non-file:// URL straight to gocloud
// (backend/diy.massageBlobPath), gocloud's s3blob opener reads `endpoint`,
// `region` and `s3ForcePathStyle` off the query string, and
// yandex.StateStorageConfig.StorageUrl emits exactly that. All that is left is
// the static key pair, which the AWS SDK only reads from the environment.
//
// Unlike the AWS InitStateStore, this one REQUIRES explicit static credentials.
// There is no ambient-credential case to preserve — a YC bucket is never reachable
// with a runner's AWS OIDC identity — and leaving the environment untouched would
// hand the Yandex endpoint whatever AWS credentials happen to be exported, which
// fails much later and much less legibly.
func InitStateStore(ctx context.Context, stateStoreCfg api.StateStorageConfig, log logger.Logger) error {
	var pcfg yandex.AccountConfig

	log.Info(ctx, "Initializing yandex statestore...")

	if err := api.ConvertAuth(stateStoreCfg, &pcfg); err != nil {
		return errors.Wrapf(err, "failed to convert auth config to yandex.AccountConfig")
	}

	if pcfg.AccessKey == "" || pcfg.SecretAccessKey == "" {
		return errors.Errorf("yandex state storage requires a static access key pair " +
			"(accessKey/secretAccessKey); the service-account key alone cannot sign Object Storage requests")
	}

	// NOTE: these are the AWS_* names on purpose — Object Storage speaks SigV4 and
	// the SDK reads no other variables. A process deploying both an AWS and a YC
	// stack would clobber one with the other, which is why dual-cloud stacks are
	// separate `sc deploy` invocations.
	for name, value := range map[string]string{
		"AWS_ACCESS_KEY_ID":     pcfg.AccessKey,
		"AWS_SECRET_ACCESS_KEY": pcfg.SecretAccessKey,
		"AWS_REGION":            pcfg.EffectiveRegion(),
		"AWS_DEFAULT_REGION":    pcfg.EffectiveRegion(),
	} {
		if err := os.Setenv(name, value); err != nil {
			return errors.Wrapf(err, "failed to set %s env variable", name)
		}
	}

	return nil
}

// Provider instantiates the Yandex Cloud Pulumi provider for one `${auth:...}`
// descriptor, mirroring aws.Provider.
//
// The plugin behind sdkYandex is simple-container-com/pulumi-yandex — our static
// bridge over yandex-cloud/terraform-provider-yandex. Nothing here wires up the
// plugin download: the generated SDK bakes PluginDownloadURL and Version into every
// resource through PkgResourceDefaultOpts, so the engine resolves
// pulumi-resource-yandex from that repo's GitHub releases on its own. If a deploy
// fails to find the plugin, the bug is in the fork's ProviderInfo, not here — do not
// add plugin plumbing to SC to paper over it.
func Provider(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	authCfg, ok := input.Descriptor.Config.Config.(api.AuthConfig)
	if !ok {
		return nil, errors.Errorf("failed to cast config to api.AuthConfig")
	}

	var pcfg yandex.AccountConfig
	if err := api.ConvertAuth(authCfg, &pcfg); err != nil {
		return nil, errors.Wrapf(err, "failed to convert auth config to yandex.AccountConfig")
	}
	if pcfg.FolderID == "" {
		return nil, errors.Errorf("yandex provider requires folderId: every YC resource is created in a folder")
	}

	args := &sdkYandex.ProviderArgs{
		CloudId:  sdk.StringPtr(pcfg.CloudID),
		FolderId: sdk.StringPtr(pcfg.FolderID),
		RegionId: sdk.StringPtr(pcfg.EffectiveRegion()),
		Zone:     sdk.StringPtr(pcfg.EffectiveZone()),
	}

	// ServiceAccountKeyFile takes either a path or the key document itself, and we
	// always have the document (SC resolves `${auth:...}` to the value, never to a
	// path). Left empty when unset so the provider's own chain can fall back to
	// YC_TOKEN / an instance service account — the local-dev and inside-VM cases.
	if pcfg.ServiceAccountKey != "" {
		args.ServiceAccountKeyFile = sdk.StringPtr(pcfg.ServiceAccountKey)
	}

	// The static pair is a *separate* credential from the service-account key (see
	// yandex.AccountConfig). It is what the SigV4-compatible services need, and
	// setting it on the provider means Object Storage and Message Queue resources
	// don't each have to carry their own keys.
	if pcfg.AccessKey != "" {
		args.StorageAccessKey = sdk.StringPtr(pcfg.AccessKey)
		args.StorageSecretKey = sdk.StringPtr(pcfg.SecretAccessKey)
		args.YmqAccessKey = sdk.StringPtr(pcfg.AccessKey)
		args.YmqSecretKey = sdk.StringPtr(pcfg.SecretAccessKey)
	}

	provider, err := sdkYandex.NewProvider(ctx, input.ToResName(input.Descriptor.Name), args)
	return &api.ResourceOutput{
		Ref: provider,
	}, err
}
