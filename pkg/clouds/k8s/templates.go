package k8s

import (
	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/docker"
)

const (
	TemplateTypeKubernetesCloudrun = "kubernetes-cloudrun"
)

type CloudrunTemplate struct {
	KubernetesConfig           `json:",inline" yaml:",inline"`
	docker.RegistryCredentials `json:",inline" yaml:",inline"`
	CaddyResource              *string `json:"caddyResource,omitempty" yaml:"caddyResource,omitempty"`
}

func ReadTemplateConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &CloudrunTemplate{})
}
