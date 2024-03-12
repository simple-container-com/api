package gcloud

import "github.com/simple-container-com/api/pkg/api"

type TemplateConfig struct {
	Credentials `json:",inline" yaml:",inline"`
}

func ReadTemplateConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &TemplateConfig{})
}
