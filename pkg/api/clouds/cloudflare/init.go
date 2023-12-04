package cloudflare

import (
	"api/pkg/api"
)

func init() {
	api.RegisterProviderConfig(api.ConfigRegisterMap{
		// cloudflare
		RegistrarTypeCloudflare: CloudflareReadRegistrarConfig,
	})
}
