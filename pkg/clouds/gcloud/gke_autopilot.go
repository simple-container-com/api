package gcloud

import (
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

func ReadGkeAutopilotTemplateConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &GkeAutopilotTemplate{})
}

func ReadGkeAutopilotResourceConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &GkeAutopilotResource{})
}

func ToGkeAutopilotConfig(tpl any, composeCfg compose.Config, stackCfg *api.StackConfigCompose) (any, error) {
	return ToCloudRunConfig(tpl, composeCfg, stackCfg)
}
