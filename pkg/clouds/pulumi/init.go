package pulumi

import (
	"github.com/simple-container-com/api/pkg/api"
)

func init() {
	api.RegisterProviderConfig(api.ConfigRegisterMap{
		ProvisionerTypePulumi: ReadProvisionerConfig,
		AuthTypePulumiToken:   ReadAuthConfig,
	})

	api.RegisterProvisioner(api.ProvisionerRegisterMap{
		ProvisionerTypePulumi: InitPulumiProvisioner,
	})
}
