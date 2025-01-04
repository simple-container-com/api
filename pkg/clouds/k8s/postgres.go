package k8s

import (
	"github.com/simple-container-com/api/pkg/api"
)

const (
	ResourceTypeHelmPostgresOperator = "kubernetes-helm-postgres-operator"
	ResourceTypeHelmRedisOperator    = "kubernetes-helm-redis-operator"
	ResourceTypeHelmRabbitmqOperator = "kubernetes-helm-rabbitmq-operator"
	ResourceTypeHelmMongodbOperator  = "kubernetes-helm-mongodb-operator"
)

type HelmValues map[string]any

type HelmChartConfig struct {
	ValuesMap             HelmValues `json:"values,omitempty" yaml:"values,omitempty" `
	OperatorNamespaceName *string    `json:"operatorNamespace,omitempty" yaml:"operatorNamespace,omitempty"`
	NamespaceName         *string    `json:"namespace,omitempty" yaml:"namespace,omitempty"`
}

type HelmOperatorChart interface {
	OperatorNamespace() *string
	Values() HelmValues
}

func (c *HelmChartConfig) Namespace() *string {
	return c.NamespaceName
}

func (c *HelmChartConfig) OperatorNamespace() *string {
	return c.OperatorNamespaceName
}

func (c *HelmChartConfig) Values() HelmValues {
	return c.ValuesMap
}

type HelmRedisOperator struct {
	*KubernetesConfig `json:",inline" yaml:",inline"`
	HelmChartConfig   `json:",inline" yaml:",inline"`
}

type HelmPostgresOperator struct {
	*KubernetesConfig `json:",inline" yaml:",inline"`
	HelmChartConfig   `json:",inline" yaml:",inline"`
	VolumeSize        *string `json:"volumeSize,omitempty" yaml:"volumeSize,omitempty"`
	NumberOfInstances *int    `json:"numberOfInstances,omitempty" yaml:"numberOfInstances,omitempty"`
	Version           *string `json:"version,omitempty" yaml:"version,omitempty"`
}

type HelmRabbitmqOperator struct {
	*KubernetesConfig `json:",inline" yaml:",inline"`
	HelmChartConfig   `json:",inline" yaml:",inline"`
}

type HelmMongodbOperator struct {
	*KubernetesConfig `json:",inline" yaml:",inline"`
	HelmChartConfig   `json:",inline" yaml:",inline"`
}

func ReadHelmPostgresOperatorConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &HelmPostgresOperator{})
}

func ReadHelmRedisOperatorConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &HelmRedisOperator{})
}

func ReadHelmRabbitmqOperatorConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &HelmRabbitmqOperator{})
}

func ReadHelmMongodbOperatorConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &HelmMongodbOperator{})
}
