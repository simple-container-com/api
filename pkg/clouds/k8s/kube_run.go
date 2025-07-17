package k8s

import (
	"github.com/pkg/errors"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/compose"
)

type KubeRunInput struct {
	CloudrunTemplate `json:"templateConfig" yaml:"templateConfig"`
	Deployment       DeploymentConfig `json:"deployment" yaml:"deployment"`
}

type CloudExtras struct {
	NodeSelector     map[string]string `json:"nodeSelector" yaml:"nodeSelector"`
	DisruptionBudget *DisruptionBudget `json:"disruptionBudget" yaml:"disruptionBudget"`
	RollingUpdate    *RollingUpdate    `json:"rollingUpdate" yaml:"rollingUpdate"`
}

func (i *KubeRunInput) DependsOnResources() []api.StackConfigDependencyResource {
	return i.Deployment.StackConfig.Dependencies
}

func (i *KubeRunInput) Uses() []string {
	return i.Deployment.StackConfig.Uses
}

func ToKubernetesRunConfig(tpl any, composeCfg compose.Config, stackCfg *api.StackConfigCompose) (any, error) {
	templateCfg, ok := tpl.(*CloudrunTemplate)
	if !ok {
		return nil, errors.Errorf("template config is not of type *gcloud.TemplateConfig")
	}
	if templateCfg == nil {
		return nil, errors.Errorf("template config is nil")
	}

	deployCfg := DeploymentConfig{
		StackConfig: stackCfg,
		Scale:       ToScale(stackCfg),
	}

	if stackCfg.CloudExtras != nil {
		k8sCloudExtras := &CloudExtras{}
		var err error
		k8sCloudExtras, err = api.ConvertDescriptor(stackCfg.CloudExtras, k8sCloudExtras)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to convert cloudExtras field to Kubernetes Cloud extras format")
		}
		deployCfg.RollingUpdate = k8sCloudExtras.RollingUpdate
		deployCfg.DisruptionBudget = k8sCloudExtras.DisruptionBudget
		deployCfg.NodeSelector = k8sCloudExtras.NodeSelector
	}
	res := &KubeRunInput{
		CloudrunTemplate: *templateCfg,
		Deployment:       deployCfg,
	}
	containers, err := ConvertComposeToContainers(composeCfg, stackCfg)
	if err != nil {
		return nil, err
	}
	res.Deployment.Containers = containers

	iContainer, err := FindIngressContainer(composeCfg, containers)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to detect ingress container")
	}
	res.Deployment.IngressContainer = iContainer

	return res, nil
}
