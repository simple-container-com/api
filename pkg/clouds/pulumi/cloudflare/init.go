// SPDX-License-Identifier: MIT
// Copyright (c) 2025 Simple Container

package cloudflare

import (
	"github.com/simple-container-com/api/pkg/clouds/cloudflare"
	"github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

func init() {
	api.RegisterRegistrar(cloudflare.ProviderType, Registrar)
}
