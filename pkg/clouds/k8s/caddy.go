package k8s

import "github.com/simple-container-com/api/pkg/api"

const ResourceTypeCaddy = "kubernetes-caddy"

type CaddyResource struct {
	*KubernetesConfig `json:",inline" yaml:",inline"`
	*CaddyConfig      `json:",inline" yaml:",inline"`
}

func CaddyReadConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &CaddyResource{})
}
