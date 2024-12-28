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
	})

	api.RegisterCloudComposeConverter(api.CloudComposeConfigRegister{
		TemplateTypeKubernetesCloudrun: ToKubernetesRunConfig,
	})
}
