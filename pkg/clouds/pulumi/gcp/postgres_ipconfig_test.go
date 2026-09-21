// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package gcp

import (
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/clouds/gcloud"
)

// With nothing that must be managed set, ipConfiguration returns nil so Pulumi
// leaves the instance's IP configuration (and authorized networks) untouched.
func TestIpConfiguration_NilWhenUnset(t *testing.T) {
	assert.Nil(t, ipConfiguration(&gcloud.PostgresGcpCloudsqlConfig{}))
}

// publicIpEnabled:true is the existing default, so it must stay a no-op and not
// start managing (and thereby wiping) the IP configuration.
func TestIpConfiguration_PublicEnabledTrueIsNoop(t *testing.T) {
	assert.Nil(t, ipConfiguration(&gcloud.PostgresGcpCloudsqlConfig{PublicIpEnabled: lo.ToPtr(true)}))
}

// An empty privateNetwork string is treated as unset.
func TestIpConfiguration_EmptyPrivateNetworkIsNoop(t *testing.T) {
	assert.Nil(t, ipConfiguration(&gcloud.PostgresGcpCloudsqlConfig{PrivateNetwork: lo.ToPtr("")}))
}

// requireSsl-only must stay byte-identical to the prior behaviour.
func TestIpConfiguration_RequireSslTrueUnchanged(t *testing.T) {
	args := ipConfiguration(&gcloud.PostgresGcpCloudsqlConfig{RequireSsl: lo.ToPtr(true)})
	require.NotNil(t, args)
	assert.Equal(t, sdk.Bool(true), args.Ipv4Enabled)
	assert.Equal(t, sdk.String("ENCRYPTED_ONLY"), args.SslMode)
	assert.Nil(t, args.PrivateNetwork)
}

func TestIpConfiguration_RequireSslFalse(t *testing.T) {
	args := ipConfiguration(&gcloud.PostgresGcpCloudsqlConfig{RequireSsl: lo.ToPtr(false)})
	require.NotNil(t, args)
	assert.Equal(t, sdk.Bool(true), args.Ipv4Enabled)
	assert.Equal(t, sdk.String("ALLOW_UNENCRYPTED_AND_ENCRYPTED"), args.SslMode)
}

// A private network keeps the public IP on by default (safe migration) and wires
// the private network path.
func TestIpConfiguration_PrivateNetworkKeepsPublicByDefault(t *testing.T) {
	net := "projects/p/global/networks/vpc"
	args := ipConfiguration(&gcloud.PostgresGcpCloudsqlConfig{PrivateNetwork: lo.ToPtr(net)})
	require.NotNil(t, args)
	assert.Equal(t, sdk.Bool(true), args.Ipv4Enabled)
	assert.Equal(t, sdk.String(net), args.PrivateNetwork)
}

// Explicitly disabling the public IP (only valid alongside a private network)
// sets Ipv4Enabled false.
func TestIpConfiguration_PublicDisabled(t *testing.T) {
	net := "projects/p/global/networks/vpc"
	args := ipConfiguration(&gcloud.PostgresGcpCloudsqlConfig{
		PrivateNetwork:  lo.ToPtr(net),
		PublicIpEnabled: lo.ToPtr(false),
	})
	require.NotNil(t, args)
	assert.Equal(t, sdk.Bool(false), args.Ipv4Enabled)
	assert.Equal(t, sdk.String(net), args.PrivateNetwork)
}
