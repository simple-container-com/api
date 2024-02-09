package github

import (
	"github.com/simple-container-com/api/pkg/api"
)

func init() {
	api.RegisterProviderConfig(api.ConfigRegisterMap{
		// github actions
		CiCdTypeGithubActions: GithubActionsReadCiCdConfig,
	})
}
