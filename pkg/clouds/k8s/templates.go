package k8s

import "github.com/simple-container-com/api/pkg/api"

const (
	TemplateTypeKubernetes = "kubernetes-cloudrun"
)

type TemplateConfig struct {
	KubernetesConfig `json:",inline" yaml:",inline"`

	DockerRegistryURL      *string `json:"dockerRegistryURL,omitempty" yaml:"dockerRegistryURL,omitempty"`
	DockerRegistryUsername *string `json:"dockerRegistryUsername,omitempty" yaml:"dockerRegistryUsername,omitempty"`
	DockerRegistryPassword *string `json:"dockerRegistryPassword,omitempty" yaml:"dockerRegistryPassword,omitempty"`
	CaddyResource          *string `json:"caddyResource,omitempty" yaml:"caddyResource,omitempty"`
}

func ReadTemplateConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &TemplateConfig{})
}
