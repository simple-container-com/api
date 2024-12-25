package gcloud

import (
	"github.com/pkg/errors"
	"github.com/samber/lo"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/compose"
	"github.com/simple-container-com/api/pkg/clouds/k8s"
)

const (
	ResourceTypeGkeAutopilot = "gcp-gke-autopilot-cluster"
	TemplateTypeGkeAutopilot = "gcp-gke-autopilot"
)

type GkeAutopilotResource struct {
	Credentials   `json:",inline" yaml:",inline"`
	GkeMinVersion string       `json:"gkeMinVersion" yaml:"gkeMinVersion"`
	Location      string       `json:"location" yaml:"location"`
	Zone          string       `json:"zone" yaml:"zone"`
	Timeouts      *Timeouts    `json:"timeouts,omitempty" yaml:"timeouts,omitempty"`
	Caddy         *CaddyConfig `json:"caddy,omitempty" yaml:"caddy,omitempty"`
}

type CaddyConfig struct {
	Enable    *bool   `json:"enable,omitempty" yaml:"enable,omitempty"`
	Caddyfile *string `json:"caddyfile,omitempty" yaml:"caddyfile,omitempty"` // TODO: support overwriting
	Namespace *string `json:"namespace,omitempty" yaml:"namespace,omitempty"`
	Image     *string `json:"image,omitempty" yaml:"image,omitempty"`
	Replicas  *int    `json:"replicas,omitempty" yaml:"replicas,omitempty"`
}

type Timeouts struct {
	Create string `json:"create" yaml:"create"`
	Update string `json:"update" yaml:"update"`
	Delete string `json:"delete" yaml:"delete"`
}

type GkeAutopilotTemplate struct {
	Credentials              `json:",inline" yaml:",inline"`
	GkeClusterResource       string `json:"gkeClusterResource" yaml:"gkeClusterResource"`
	ArtifactRegistryResource string `json:"artifactRegistryResource" yaml:"artifactRegistryResource"`
}

type GkeAutopilotInput struct {
	GkeAutopilotTemplate `json:"templateConfig" yaml:"templateConfig"`
	Deployment           k8s.DeploymentConfig `json:"deployment" yaml:"deployment"`
}

func (i *GkeAutopilotInput) Uses() []string {
	return i.Deployment.StackConfig.Uses
}

func (i *GkeAutopilotInput) OverriddenBaseZone() string {
	return i.Deployment.StackConfig.BaseDnsZone
}

func (i *GkeAutopilotInput) DependsOnResources() []api.StackConfigDependencyResource {
	return i.Deployment.StackConfig.Dependencies
}

func ReadGkeAutopilotTemplateConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &GkeAutopilotTemplate{})
}

func ReadGkeAutopilotResourceConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &GkeAutopilotResource{})
}

func ToGkeAutopilotConfig(tpl any, composeCfg compose.Config, stackCfg *api.StackConfigCompose) (any, error) {
	templateCfg, ok := tpl.(*GkeAutopilotTemplate)
	if !ok {
		return nil, errors.Errorf("template config is not of type *gcloud.GkeAutopilotTemplate")
	}
	if templateCfg == nil {
		return nil, errors.Errorf("template config is nil")
	}
	res := &GkeAutopilotInput{
		GkeAutopilotTemplate: *templateCfg,
		Deployment: k8s.DeploymentConfig{
			StackConfig: stackCfg,
		},
	}

	containers, err := k8s.ConvertComposeToContainers(composeCfg, stackCfg)
	if err != nil {
		return nil, err
	}
	iContainer, err := k8s.FindIngressContainer(composeCfg, containers)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to detect ingress container")
	}
	res.Deployment.Containers = containers
	res.Deployment.IngressContainer = iContainer
	res.Deployment.Headers = lo.ToPtr(k8s.ToHeaders(stackCfg.Headers))
	res.Deployment.Scale = k8s.ToScale(stackCfg)
	res.Deployment.TextVolumes = k8s.ToSimpleTextVolumes(stackCfg)

	return res, nil
}
