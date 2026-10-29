// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package pulumi

import (
	"context"
	"os"

	"github.com/pkg/errors"
	// External plugins' /api modules — register their config types with
	// SC core's RegisterProviderConfig map so server.yaml parsing
	// recognises `type: yc-bucket`, `type: yc-dns`, `type: cloudflare`
	// etc., and register their RegisterPluginRequirement entries so
	// ValidatePluginRequirements can emit the copy-paste-ready `plugins:`
	// block. The /plugin side (with the heavy pulumi-cloudflare /
	// pulumi-yandex SDKs) is NOT pulled in.
	//
	// Every /api module listed in go.mod MUST be blank-imported here.
	// Omitting one is silent: the module stays an unused go.mod require,
	// its type tokens never register, and a server.yaml using them fails
	// to parse with no mention of the plugin. That is exactly how the
	// cloudflare drop broke the release pipeline's own `publish sc tool`
	// step — `sc-plugin-cloudflare/api` was required but never imported.
	_ "github.com/simple-container-com/sc-plugin-cloudflare/api"
	_ "github.com/simple-container-com/sc-plugin-cloudru/api"
	_ "github.com/simple-container-com/sc-plugin-mongodb/api"
	_ "github.com/simple-container-com/sc-plugin-yandex/api"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/logger"
	"github.com/simple-container-com/api/pkg/clouds/fs"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	// Register all the providers
	_ "github.com/simple-container-com/api/pkg/clouds/pulumi/aws"
	_ "github.com/simple-container-com/api/pkg/clouds/pulumi/gcp"
	_ "github.com/simple-container-com/api/pkg/clouds/pulumi/k3s"
	_ "github.com/simple-container-com/api/pkg/clouds/pulumi/vm"
)

func init() {
	api.RegisterProviderConfig(api.ConfigRegisterMap{
		ProvisionerTypePulumi: ReadProvisionerConfig,
		AuthTypePulumiToken:   ReadAuthConfig,
	})

	api.RegisterProvisioner(api.ProvisionerRegisterMap{
		ProvisionerTypePulumi: InitPulumiProvisioner,
	})

	api.RegisterProvisionerFieldConfig(api.ProvisionerFieldConfigRegister{
		BackendTypePulumiCloud: ReadAuthConfig,
	})

	pApi.RegisterRegistrar("", NotConfiguredRegistrar)
	setPulumiCloudAccessToken := func(ctx context.Context, stateStoreCfg api.StateStorageConfig, log logger.Logger) error {
		log.Info(ctx, "Initializing pulumi statestore...")

		authCfg, ok := stateStoreCfg.(api.AuthConfig)
		if !ok {
			return errors.Errorf("failed to convert pulumi state storage config to api.AuthConfig")
		}

		// hackily set access token env variable, so that lm can access it
		if err := os.Setenv("PULUMI_ACCESS_TOKEN", authCfg.CredentialsValue()); err != nil {
			return err
		}
		return nil
	}
	pApi.RegisterInitStateStore(ProvisionerTypePulumi, setPulumiCloudAccessToken)
	pApi.RegisterInitStateStore(BackendTypePulumiCloud, setPulumiCloudAccessToken)
	pApi.RegisterInitStateStore(fs.StateStorageTypeFileSystem, initFileSystemStateStore)
}
