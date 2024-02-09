package cloudflare

import (
	"github.com/simple-container-com/api/pkg/api"
)

func init() {
	api.RegisterProviderConfig(api.ConfigRegisterMap{
		// cloudflare
		RegistrarTypeCloudflare: ReadRegistrarConfig,
	})
}
