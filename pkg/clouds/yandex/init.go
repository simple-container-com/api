// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package yandex

import (
	"github.com/simple-container-com/api/pkg/api"
)

// ProviderType is the discriminator every Yandex Cloud config reports from
// ProviderType(), and the directory generated schemas land in (docs/schemas/yandex).
const ProviderType = "yandex"

func init() {
	api.RegisterProviderConfig(api.ConfigRegisterMap{
		AuthTypeYandexServiceAccount:          ReadAuthServiceAccountConfig,
		SecretsTypeYandexLockbox:              ReadSecretsConfig,
		TemplateTypeYandexServerlessContainer: ReadTemplateConfig,
	})

	api.RegisterProvisionerFieldConfig(api.ProvisionerFieldConfigRegister{
		StateStorageTypeYandexObjectStorage: ReadStateStorageConfig,
		SecretsProviderTypeYandexKms:        ReadSecretsProviderConfig,
	})

	// A Serverless Container is the fleet's Lambda replacement, so it converts
	// from a single-image client stack. There is deliberately no compose or
	// static-site converter yet: neither has a YC shape decided (see the design
	// doc's open question on API Gateway).
	api.RegisterCloudSingleImageConverter(api.CloudSingleImageConfigRegister{
		TemplateTypeYandexServerlessContainer: ToServerlessContainerConfig,
	})
}
