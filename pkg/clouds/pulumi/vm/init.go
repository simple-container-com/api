// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package vm

import (
	"github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	"github.com/simple-container-com/api/pkg/clouds/vm"
)

// Both the resource and the template go through RegisterResources — templates are not
// a separate registry, they are dispatched out of the same map (see
// pkg/clouds/pulumi/yandex/init.go, which registers `yandex-serverless-container`
// alongside its resources).
func init() {
	api.RegisterProvider(vm.ProviderType, Provider)
	api.RegisterResources(map[string]api.ProvisionFunc{
		vm.ResourceTypeVm:        Vm,
		vm.TemplateTypeVmCompose: VmComposeStack,
	})
}
