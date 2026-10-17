// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package k3s

import (
	"github.com/simple-container-com/api/pkg/clouds/k3s"
	"github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

func init() {
	api.RegisterProvider(k3s.ProviderType, Provider)
	api.RegisterResources(map[string]api.ProvisionFunc{
		k3s.ResourceTypeK3sCluster: K3sCluster,
	})
}
