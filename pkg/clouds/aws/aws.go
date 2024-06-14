package aws

import "github.com/simple-container-com/api/pkg/api"

type TemplateConfig struct {
	AccountConfig `json:",inline" yaml:",inline"`
}

type CloudExtras struct {
	AwsRoles []string `json:"awsRoles" yaml:"awsRoles"`
}

func ReadTemplateConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &TemplateConfig{})
}
