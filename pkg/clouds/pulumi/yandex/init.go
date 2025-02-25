package yandex

import (
	"github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	"github.com/simple-container-com/api/pkg/clouds/yandex"
)

func init() {
	api.RegisterResources(map[string]api.ProvisionFunc{
		yandex.TemplateTypeYandexServerlessContainer: ServerlessContainer,
	})
	api.RegisterProvider(yandex.ProviderType, Provider)
}
