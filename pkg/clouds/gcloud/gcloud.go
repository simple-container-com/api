package gcloud

import "github.com/simple-container-com/api/pkg/api"

const (
	TemplateTypeGcpCloudrun = "cloudrun"
)

type TemplateConfig struct {
	api.AuthConfig
	Credentials string `json:"credentials" yaml:"credentials"`
	ProjectId   string `json:"projectId" yaml:"projectId"`
}

func ReadTemplateConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &TemplateConfig{})
}

func (r *TemplateConfig) CredentialsValue() string {
	return r.Credentials
}

func (r *TemplateConfig) ProjectIdValue() string {
	return r.ProjectId
}
