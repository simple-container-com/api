package aws

import "github.com/simple-container-com/api/pkg/api"

type TemplateConfig struct {
	api.AuthConfig
	api.Credentials `json:",inline" yaml:",inline"`
	Account         string `json:"account" yaml:"account"`
}

func ReadTemplateConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &TemplateConfig{})
}

func (r *TemplateConfig) CredentialsValue() string {
	return api.AuthToString(r)
}

func (r *TemplateConfig) ProjectIdValue() string {
	return r.Account
}
