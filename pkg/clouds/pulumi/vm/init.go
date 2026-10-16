// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package vm

import (
	"github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	"github.com/simple-container-com/api/pkg/clouds/vm"
)

// The `vm-compose` template is NOT registered here yet, and that is on purpose: it
// lands with the compose render and both client-stack converters in one slice
// (roadmap item sc-vm-compose-single-vm, P5.2). Registering a converter for a
// template whose ProvisionFunc does not exist buys nothing — templates are only
// serialised into the parent stack's outputs at provision time and dispatched at
// deploy time, so the failure would simply move from `sc provision` to `sc deploy`.
func init() {
	api.RegisterProvider(vm.ProviderType, Provider)
	api.RegisterResources(map[string]api.ProvisionFunc{
		vm.ResourceTypeVm: Vm,
	})
}
