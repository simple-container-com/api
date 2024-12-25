package k8s

import (
	"github.com/simple-container-com/api/pkg/api"
)

const (
	AuthTypeKubeconfig = "kubernetes"
)

type KubernetesConfig struct {
	Kubeconfig string `json:"kubeconfig" yaml:"kubeconfig"`
}

func (r *KubernetesConfig) ProviderType() string {
	return ProviderType
}

func (r *KubernetesConfig) ProjectIdValue() string {
	return "n/a"
}

func (r *KubernetesConfig) CredentialsValue() string {
	return r.Kubeconfig
}

func ReadKubernetesConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &KubernetesConfig{})
}
