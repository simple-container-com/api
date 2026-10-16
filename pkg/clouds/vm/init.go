// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package vm

import (
	"github.com/simple-container-com/api/pkg/api"
)

// ProviderType is the discriminator VmConfig reports from ProviderType(), the key
// pkg/clouds/pulumi/vm registers its Provider under, and the directory generated
// schemas land in (docs/schemas/vm).
//
// It is "vm" rather than a cloud name on purpose: which cloud creates the machine is
// the resource's `provider:` field, and dispatching on it is the driver tier's job
// (pkg/clouds/pulumi/vm/driver.go). Everything above the driver — cloud-init,
// readiness, the SSH connection, compose, k3s — is written once.
const ProviderType = "vm"

func init() {
	api.RegisterProviderConfig(api.ConfigRegisterMap{
		ResourceTypeVm:        ReadVmConfig,
		TemplateTypeVmCompose: ReadComposeTemplateConfig,
	})

	api.RegisterCloudComposeConverter(api.CloudComposeConfigRegister{
		TemplateTypeVmCompose: ToVmComposeConfig,
	})

	// Both converters, because the first stack this path has to carry (forge-atrium) is
	// `type: single-image`, not `cloud-compose`. A template that registered only the
	// compose converter would fail that stack at deploy time with "no converter for
	// template type", which reads like a broken installation rather than a missing
	// feature.
	api.RegisterCloudSingleImageConverter(api.CloudSingleImageConfigRegister{
		TemplateTypeVmCompose: ToVmComposeSingleImageConfig,
	})
}
