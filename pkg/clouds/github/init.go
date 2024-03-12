package github

import (
	"github.com/simple-container-com/api/pkg/api"
)

const ProviderType = "github"

func init() {
	api.RegisterProviderConfig(api.ConfigRegisterMap{
		// github actions
		CiCdTypeGithubActions: ReadCiCdConfig,
	})
}
