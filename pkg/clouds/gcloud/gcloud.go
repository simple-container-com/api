package gcloud

import "api/pkg/api"

const (
	TemplateTypeGcpCloudrun = "cloudrun"
)

type GcloudTemplateConfig struct {
	Credentials string `json:"credentials" yaml:"credentials"`
	ProjectId   string `json:"projectId" yaml:"projectId"`
}

func ReadTemplateConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &GcloudTemplateConfig{})
}
