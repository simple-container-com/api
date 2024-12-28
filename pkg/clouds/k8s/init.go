package k8s

import (
	"github.com/simple-container-com/api/pkg/api"
)

const ProviderType = "kubernetes"

func init() {
	api.RegisterProviderConfig(api.ConfigRegisterMap{
		// kubernetes
		TemplateTypeKubernetes: ReadTemplateConfig,
		AuthTypeKubeconfig:     ReadKubernetesConfig,

		// caddy
		ResourceTypeCaddy: CaddyReadConfig,
	})

	api.RegisterCloudComposeConverter(api.CloudComposeConfigRegister{
		TemplateTypeKubernetes: ToKubernetesRunConfig,
	})
}
