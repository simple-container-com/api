package github

import "github.com/simple-container-com/api/pkg/api"

const CiCdTypeGithubActions = "github-actions"

type GithubActionsCiCdConfig struct {
	api.AuthConfig
	AuthToken string `json:"auth-token" yaml:"auth-token"`
}

func GithubActionsReadCiCdConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &GithubActionsCiCdConfig{})
}

func (r *GithubActionsCiCdConfig) CredentialsValue() string {
	return r.AuthToken
}

func (r *GithubActionsCiCdConfig) ProjectIdValue() string {
	return "" // todo: figure out
}
