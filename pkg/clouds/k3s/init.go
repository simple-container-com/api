// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

// Package k3s is the config tier of a Kubernetes cluster SC stands up itself, on
// machines described by a `vm` resource (pkg/clouds/vm). It is the cheapest
// Kubernetes SC can offer in a cloud that has no managed Kubernetes at all — which
// is most of the Russian market — and it is deliberately a thin thing: everything
// above the kubeconfig (`kubernetes-cloudrun`, the Caddy edge, the helm operators)
// already exists and is reused unchanged.
//
// Design: forge/docs/design/2026-10-13/sc-russia-market-readiness/k3s-all-clouds.md,
// roadmap item sc-k3s-cluster-resource (slice K.0).
package k3s

import (
	"github.com/simple-container-com/api/pkg/api"
)

// ProviderType is the discriminator K3sClusterConfig reports from ProviderType(),
// the key pkg/clouds/pulumi/k3s registers its Provider under, and the directory
// generated schemas land in (docs/schemas/k3s).
//
// It is its OWN provider type rather than the vm resource's "vm", and that is not a
// stylistic choice: provision.go's initProvider looks the provider up by the config's
// ProviderType() (pkg/clouds/pulumi/provision.go:311), so a k3s-cluster config that
// reported "vm" would be handed to the vm provider, which casts its input to
// *vm.VmConfig and would refuse it with a type-assertion error.
const ProviderType = "k3s"

func init() {
	api.RegisterProviderConfig(api.ConfigRegisterMap{
		ResourceTypeK3sCluster: ReadK3sClusterConfig,
	})
}
