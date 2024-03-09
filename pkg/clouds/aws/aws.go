package aws

import "github.com/simple-container-com/api/pkg/api"

type TemplateConfig struct {
	api.Credentials  `json:",inline" yaml:",inline"`
	AwsAccountConfig `json:",inline" yaml:",inline"`
	Region           string `json:"region" yaml:"region"`
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
