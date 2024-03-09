package gcloud

import "github.com/simple-container-com/api/pkg/api"

type TemplateConfig struct {
	ServiceAccountConfig `json:",inline" yaml:",inline"`
	api.Credentials      `json:",inline" yaml:",inline"`
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
