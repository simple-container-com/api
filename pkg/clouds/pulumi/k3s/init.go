// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package k3s

import (
	"github.com/simple-container-com/api/pkg/clouds/k3s"
	"github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

func init() {
	api.RegisterProvider(k3s.ProviderType, Provider)
	// Resources and templates share this map — see pkg/clouds/pulumi/yandex/init.go:27.
	api.RegisterResources(map[string]api.ProvisionFunc{
		k3s.ResourceTypeK3sCluster:  K3sCluster,
		k3s.TemplateTypeK3sCloudrun: K3sCloudrun,
	})
}
