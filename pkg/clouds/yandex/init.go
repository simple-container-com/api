package yandex

import (
	"github.com/simple-container-com/api/pkg/api"
)

const ProviderType = "yandex-cloud"

func init() {
	api.RegisterProviderConfig(api.ConfigRegisterMap{
		TemplateTypeYandexServerlessContainer: ReadTemplateConfig,
		AuthTypeYandex:                        ReadYandexAuthConfig,
	})

	api.RegisterCloudSingleImageConverter(api.CloudSingleImageConfigRegister{
		TemplateTypeYandexServerlessContainer: ToServerlessContainerConfig,
	})
}
