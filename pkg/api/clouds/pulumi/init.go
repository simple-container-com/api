package pulumi

import (
	"api/pkg/api"
)

func init() {
	api.RegisterProviderConfig(api.ConfigRegisterMap{
		// pulumi
		ProvisionerTypePulumi: PulumiReadProvisionerConfig,
		AuthTypePulumiToken:   PulumiReadAuthConfig,
	})
}
