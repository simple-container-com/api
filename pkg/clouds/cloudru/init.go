// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

// Package cloudru is the credential tier for Cloud.ru Advanced (ex-SberCloud).
//
// Only `auth:` lives here. There is no `cloudru` resource or template type: a
// machine in Cloud.ru is a `type: vm` resource with `provider: cloudru-vm`, and
// everything above the cloud API — cloud-init, readiness, compose, k3s — is the
// cloud-independent code in pkg/clouds/vm and pkg/clouds/pulumi/vm. This package
// exists so that `${auth:<name>}` can resolve to a Keystone key pair.
//
// Which Cloud.ru this is matters. Cloud.ru sells two unrelated estates:
//
//   - **Advanced**, the ex-SberCloud estate, whose Terraform provider
//     (sbercloud-terraform/terraform-provider-sbercloud, MPL-2.0) has open Go
//     sources — so it can be bridged into Pulumi the classic way, which is what
//     simple-container-com/pulumi-sbercloud is. That is this package.
//   - **Evolution**, whose provider (cloud-ru/evo-terraform) ships as a binary
//     only. It cannot be statically bridged and needs either the dynamic bridge or
//     a hand-written provider — a separate, unstarted piece of work.
//
// The two have different APIs and different credentials. A key pair from one does
// not authenticate against the other.
//
// See forge/docs/design/2026-10-13/sc-russia-market-readiness/ (R4, and the
// 2026-10-15 amendment to §6).
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
		AuthTypeCloudruAccessKey: ReadAuthAccessKeyConfig,
	})
}
