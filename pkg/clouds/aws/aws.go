package aws

import "github.com/simple-container-com/api/pkg/api"

type TemplateConfig struct {
	AwsAccountConfig `json:",inline" yaml:",inline"`
	Region           string `json:"region" yaml:"region"`
}

func ReadTemplateConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &TemplateConfig{})
}
