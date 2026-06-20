// SPDX-License-Identifier: MIT
// Copyright (c) 2025 Simple Container

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
