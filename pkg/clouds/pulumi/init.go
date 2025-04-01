package pulumi

import (
	"context"
	"os"

	"github.com/pkg/errors"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/logger"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	// Register all the providers
	_ "github.com/simple-container-com/api/pkg/clouds/pulumi/aws"
	_ "github.com/simple-container-com/api/pkg/clouds/pulumi/cloudflare"
	_ "github.com/simple-container-com/api/pkg/clouds/pulumi/gcp"
	_ "github.com/simple-container-com/api/pkg/clouds/pulumi/mongodb"
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
}
