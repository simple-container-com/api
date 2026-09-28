// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package yandex

import (
	"github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	"github.com/simple-container-com/api/pkg/clouds/yandex"
)

// The Pulumi provider behind these registrations is simple-container-com/pulumi-yandex,
// our static bridge over yandex-cloud/terraform-provider-yandex. pulumi/pulumi-yandex
// was archived at v0.13.0 (2022), predating Serverless Containers entirely, and the
// dynamic `pulumi package add terraform-provider` path needs registry.opentofu.org,
// which is geo-blocked from the hosts this fleet deploys from. See the design doc's
// §0.3/§0.4.
//
// Templates register through the same RegisterResources map as resources — there
// is no separate RegisterTemplate (see aws/init.go).
func init() {
	api.RegisterInitStateStore(yandex.ProviderType, InitStateStore)
	api.RegisterProvider(yandex.ProviderType, Provider)
	// The registrar type is `yc-dns`, not ProviderType: both tiers must agree on the
	// same string, and schema-gen files anything carrying an AWS token under aws/.
	api.RegisterRegistrar(yandex.RegistrarTypeYandexDns, Registrar)
	api.RegisterResources(map[string]api.ProvisionFunc{
		yandex.ResourceTypeObjectStorageBucket:       ObjectStorageBucket,
		yandex.TemplateTypeYandexServerlessContainer: ServerlessContainer,
	})
	api.RegisterComputeProcessor(map[string]api.ComputeProcessorFunc{
		yandex.ResourceTypeObjectStorageBucket: ObjectStorageBucketComputeProcessor,
	})
}
