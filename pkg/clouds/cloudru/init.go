// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

// Package cloudru is the config tier for Cloud.ru — both of the clouds sold
// under that brand.
//
// There is no `cloudru` *resource* type: a machine in Cloud.ru is a `type: vm`
// resource with `provider: cloudru-vm`, and everything above the cloud API —
// cloud-init, readiness, compose, k3s — is the cloud-independent code in
// pkg/clouds/vm and pkg/clouds/pulumi/vm.
//
// There is one template type, TemplateTypeCloudruContainerApp: a `type:
// single-image` client stack deployed as an Evolution Container App over the
// public REST API (pkg/clouds/cloudru/evolution). That one does not ride a
// Terraform provider at all, because Container Apps appear in none of them.
//
// There is one registrar type, RegistrarTypeCloudruDns, writing records into an
// Evolution Cloud DNS zone over the same REST client — also absent from every
// provider.
//
// Which Cloud.ru this is matters. Cloud.ru sells two unrelated estates:
//
//   - **Advanced**, the ex-SberCloud estate, whose Terraform provider
//     (sbercloud-terraform/terraform-provider-sbercloud, MPL-2.0) has open Go
//     sources — so it is bridged into Pulumi the classic way, which is what
//     simple-container-com/pulumi-sbercloud is. AuthTypeCloudruAccessKey, and the
//     `cloudru-vm` driver, belong to it.
//   - **Evolution**, Cloud.ru's own platform, whose provider
//     (cloud-ru/evo-terraform) ships as a binary only. A binary is still bridgeable
//     — Pulumi's *parameterized* bridge consumes one and yields a full typed SDK
//     (measured 2026-10-22: 61 resources, 87 data sources) — but Container Apps
//     appear nowhere in it, so this package reaches them over REST instead.
//     AuthTypeCloudruEvolutionKey and TemplateTypeCloudruContainerApp belong to it.
//
// The two have different APIs and different credentials. A key pair from one does
// not authenticate against the other, which is why there are two auth types and
// not one with optional fields.
//
// See forge/docs/design/2026-10-13/sc-russia-market-readiness/ (R4, the 2026-10-15
// amendment to §6, and the 2026-10-22 amendments: the estate correction, the live
// Container Apps skeleton, and the bridge spike).
package cloudru

import (
	"github.com/simple-container-com/api/pkg/api"
)

// ProviderType is the discriminator every Cloud.ru config reports from
// ProviderType(), and the directory generated schemas land in
// (docs/schemas/cloudru).
const ProviderType = "cloudru"

func init() {
	api.RegisterProviderConfig(api.ConfigRegisterMap{
		AuthTypeCloudruAccessKey:        ReadAuthAccessKeyConfig,
		AuthTypeCloudruEvolutionKey:     ReadAuthEvolutionKeyConfig,
		TemplateTypeCloudruContainerApp: ReadContainerAppTemplateConfig,
		RegistrarTypeCloudruDns:         ReadDnsRegistrarConfig,
	})

	// A Container App is this cloud's Lambda replacement, so it converts from a
	// single-image client stack — the same seam yc-serverless-container uses.
	// There is deliberately no compose converter: a multi-container compose stack
	// on Cloud.ru is the `vm`/`k3s` path, not this one.
	api.RegisterCloudSingleImageConverter(api.CloudSingleImageConfigRegister{
		TemplateTypeCloudruContainerApp: ToContainerAppConfig,
	})
}
