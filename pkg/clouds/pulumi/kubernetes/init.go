package kubernetes

import (
	"github.com/simple-container-com/api/pkg/clouds/k8s"
	"github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

func init() {
	api.RegisterProvider(k8s.ProviderType, Provider)
	api.RegisterResources(map[string]api.ProvisionFunc{
		k8s.TemplateTypeKubernetesCloudrun: KubeRun,
	})
	api.RegisterResources(map[string]api.ProvisionFunc{
		k8s.ResourceTypeCaddy: CaddyResource,
	})
}
