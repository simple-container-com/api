package pulumi

import (
	"context"
	"os"

	"github.com/pkg/errors"

	"github.com/pulumi/pulumi/pkg/v3/backend/httpstate"

	"github.com/simple-container-com/api/pkg/api"
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
	setPulumiCloudAccessToken := func(ctx context.Context, stateStoreCfg api.StateStorageConfig) error {
		authCfg, ok := stateStoreCfg.(api.AuthConfig)
		if !ok {
			return errors.Errorf("failed to convert pulumi state storage config to api.AuthConfig")
		}

		// hackily set access token env variable, so that lm can access it
		if err := os.Setenv(httpstate.AccessTokenEnvVar, authCfg.CredentialsValue()); err != nil {
			return err
		}
		return nil
	}
	pApi.RegisterInitStateStore(ProvisionerTypePulumi, setPulumiCloudAccessToken)
	pApi.RegisterInitStateStore(BackendTypePulumiCloud, setPulumiCloudAccessToken)
}
