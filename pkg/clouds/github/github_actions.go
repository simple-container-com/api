package github

import "github.com/simple-container-com/api/pkg/api"

const CiCdTypeGithubActions = "github-actions"

type ActionsCiCdConfig struct {
	AuthToken string `json:"auth-token" yaml:"auth-token"`
}

func ReadCiCdConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &ActionsCiCdConfig{})
}

func (r *ActionsCiCdConfig) CredentialsValue() string {
	return r.AuthToken
}

func (r *ActionsCiCdConfig) ProjectIdValue() string {
	return "" // todo: figure out
}

func (r *ActionsCiCdConfig) ProviderType() string {
	return ProviderType
}
