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

func ToKubernetesRunConfig(tpl any, composeCfg compose.Config, stackCfg *api.StackConfigCompose) (any, error) {
	templateCfg, ok := tpl.(*CloudrunTemplate)
	if !ok {
		return nil, errors.Errorf("template config is not of type *gcloud.TemplateConfig")
	}
	if templateCfg == nil {
		return nil, errors.Errorf("template config is nil")
	}

	res := &KubeRunInput{
		CloudrunTemplate: *templateCfg,
		Deployment: DeploymentConfig{
			StackConfig: stackCfg,
		},
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
