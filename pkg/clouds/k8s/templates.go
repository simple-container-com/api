package k8s

import "github.com/simple-container-com/api/pkg/api"

const (
	TemplateTypeKubernetes = "kubernetes-cloudrun"
)

type TemplateConfig struct {
	KubernetesConfig `json:",inline" yaml:",inline"`
}

func ReadTemplateConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &TemplateConfig{})
}
