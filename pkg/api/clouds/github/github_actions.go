package github

import "api/pkg/api"

const CiCdTypeGithubActions = "github-actions"

type GithubActionsCiCdConfig struct {
	AuthToken string `json:"auth-token" yaml:"auth-token"`
}

func GithubActionsReadCiCdConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &GithubActionsCiCdConfig{})
}
