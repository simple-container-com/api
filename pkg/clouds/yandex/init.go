package yandex

import (
	"github.com/simple-container-com/api/pkg/api"
)

const ProviderType = "yandex-cloud"

func init() {
	api.RegisterProviderConfig(api.ConfigRegisterMap{
		TemplateTypeYandexCloudFunction: ReadTemplateConfig,
	})

	api.RegisterCloudSingleImageConverter(api.CloudSingleImageConfigRegister{
		TemplateTypeYandexCloudFunction: ToCloudFunctionConfig,
	})

}
