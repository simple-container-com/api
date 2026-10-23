// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package cloudru

import (
	"github.com/simple-container-com/api/pkg/clouds/cloudru"
	"github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

// Templates are not a separate registry — they are dispatched out of the same map
// as resources (see pkg/clouds/pulumi/yandex/init.go). This package registers only
// a template: a machine in Cloud.ru is a `vm` resource with `provider: cloudru-vm`
// and lives in pkg/clouds/pulumi/vm.
func init() {
	api.RegisterProvider(cloudru.ProviderType, Provider)
	api.RegisterResources(map[string]api.ProvisionFunc{
		cloudru.TemplateTypeCloudruContainerApp: ContainerApp,
	})
}
