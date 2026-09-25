// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package yandex

import (
	"github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	"github.com/simple-container-com/api/pkg/clouds/yandex"
)

// Only the state store is registered so far. Resource provisioning
// (api.RegisterProvider / RegisterResources) waits on the choice of Yandex Pulumi
// provider — pulumi/pulumi-yandex is deprecated and pinned to v0.13.0 (2022),
// which predates Serverless Containers entirely. See the design doc's §0.3.
//
// Until then a YC stack parses, and its Pulumi state lives in Object Storage, but
// `sc provision` has nothing to create.
func init() {
	api.RegisterInitStateStore(yandex.ProviderType, InitStateStore)
}
