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
		k8s.ResourceTypeCaddy:                CaddyResource,
		k8s.ResourceTypeHelmPostgresOperator: HelmPostgresOperator,
		k8s.ResourceTypeHelmMongodbOperator:  HelmMongodbOperator,
		k8s.ResourceTypeHelmRabbitmqOperator: HelmRabbitmqOperator,
		k8s.ResourceTypeHelmRedisOperator:    HelmRedisOperator,
	})
	api.RegisterComputeProcessor(map[string]api.ComputeProcessorFunc{
		k8s.ResourceTypeHelmPostgresOperator: HelmPostgresOperatorComputeProcessor,
		k8s.ResourceTypeHelmRabbitmqOperator: HelmRabbitmqOperatorComputeProcessor,
		k8s.ResourceTypeHelmRedisOperator:    HelmRedisOperatorComputeProcessor,
		k8s.ResourceTypeHelmMongodbOperator:  HelmMongodbOperatorComputeProcessor,
	})
}
