package cloudflare

import (
	"github.com/simple-container-com/api/pkg/clouds/cloudflare"
	"github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

func init() {
	api.RegisterRegistrar(cloudflare.ProviderType, Registrar)
}
