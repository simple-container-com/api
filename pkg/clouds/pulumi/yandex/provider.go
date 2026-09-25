// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package yandex

import (
	"context"
	"os"

	"github.com/pkg/errors"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/logger"
	"github.com/simple-container-com/api/pkg/clouds/yandex"
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
