package cloudflare

import (
	"github.com/simple-container-com/api/pkg/api"
)

const ProviderType = "cloudflare"

func init() {
	api.RegisterProviderConfig(api.ConfigRegisterMap{
		// cloudflare
		RegistrarTypeCloudflare: ReadRegistrarConfig,
	})
}
