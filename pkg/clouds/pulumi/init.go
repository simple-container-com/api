// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package pulumi

import (
	"context"
	"os"

	"github.com/pkg/errors"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/logger"
	"github.com/simple-container-com/api/pkg/clouds/fs"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	// Register all the providers. Blank imports only; registration
	// happens in each package's init().
	// The GCP in-tree package was deleted in phase 5 of the GCP plugin
	// extraction — sc-plugin-gcp now owns every GCP type-token.
	// plugin_bootstrap.go's canonical table carries the auto-inject row
	// for operators who paste an unknown-but-canonical token.
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
