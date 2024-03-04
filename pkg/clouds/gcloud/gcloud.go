package gcloud

import "github.com/simple-container-com/api/pkg/api"

type TemplateConfig struct {
	api.AuthConfig
	api.Credentials `json:",inline" yaml:",inline"`
	ProjectId       string `json:"projectId" yaml:"projectId"`
}

func ReadTemplateConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &TemplateConfig{})
}

func (r *TemplateConfig) CredentialsValue() string {
	return r.Credentials.Credentials
}

func (r *TemplateConfig) ProjectIdValue() string {
	return r.ProjectId
}
