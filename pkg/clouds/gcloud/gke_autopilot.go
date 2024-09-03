package gcloud

import (
	"github.com/pkg/errors"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/compose"
)

const (
	ResourceTypeGkeAutopilot = "gcp-gke-autopilot-cluster"
	TemplateTypeGkeAutopilot = "gcp-gke-autopilot"
)

type GkeAutopilotResource struct {
	Credentials   `json:",inline" yaml:",inline"`
	GkeMinVersion string `json:"gkeMinVersion" yaml:"gkeMinVersion"`
}

type GkeAutopilotTemplate struct {
	Credentials              `json:",inline" yaml:",inline"`
	GkeClusterResource       string `json:"gkeClusterResource" yaml:"gkeClusterResource"`
	ArtifactRegistryResource string `json:"artifactRegistryResource" yaml:"artifactRegistryResource"`
}

type GkeAutopilotInput struct {
	TemplateConfig GkeAutopilotTemplate `json:"templateConfig" yaml:"templateConfig"`
	Containers     []CloudRunContainer  `json:"containers" yaml:"containers"`
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
		TemplateConfig: *templateCfg,
	}

	containers, err := convertComposeToContainers(composeCfg, stackCfg)
	if err != nil {
		return nil, err
	}
	res.Containers = containers

	return res, nil
}
