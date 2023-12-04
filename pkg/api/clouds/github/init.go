package github

import (
	"api/pkg/api"
)

func init() {
	api.RegisterProviderConfig(api.ConfigRegisterMap{
		// github actions
		CiCdTypeGithubActions: GithubActionsReadCiCdConfig,
	})
}
