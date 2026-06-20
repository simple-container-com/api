// SPDX-License-Identifier: MIT
// Copyright (c) 2025 Simple Container

package k8s

import (
	"github.com/simple-container-com/api/pkg/api"
)

const ProviderType = "kubernetes"

func init() {
	api.RegisterProviderConfig(api.ConfigRegisterMap{
		// kubernetes
		TemplateTypeKubernetesCloudrun: ReadTemplateConfig,
		AuthTypeKubeconfig:             ReadKubernetesConfig,

		// caddy
		ResourceTypeCaddy: CaddyReadConfig,

		// helm charts
		ResourceTypeHelmPostgresOperator: ReadHelmPostgresOperatorConfig,
		ResourceTypeHelmMongodbOperator:  ReadHelmMongodbOperatorConfig,
		ResourceTypeHelmRabbitmqOperator: ReadHelmRabbitmqOperatorConfig,
		ResourceTypeHelmRedisOperator:    ReadHelmRedisOperatorConfig,
	})

	api.RegisterCloudComposeConverter(api.CloudComposeConfigRegister{
		TemplateTypeKubernetesCloudrun: ToKubernetesRunConfig,
	})
}
