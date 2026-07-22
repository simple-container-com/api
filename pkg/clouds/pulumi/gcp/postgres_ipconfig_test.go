// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package gcp

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/clouds/gcloud"
)

func ipCfgStrPtr(s string) *string { return &s }
func ipCfgBoolPtr(b bool) *bool    { return &b }

// With none of requireSsl/privateNetwork/publicIpEnabled set, ipConfiguration must
// return nil so Pulumi leaves the instance's IP configuration untouched.
func TestIpConfiguration_NilWhenUnset(t *testing.T) {
	assert.Nil(t, ipConfiguration(&gcloud.PostgresGcpCloudsqlConfig{}))
}

// requireSsl-only must stay byte-identical to the prior behaviour: public IPv4 on,
// encrypted SSL, no private network.
func TestIpConfiguration_RequireSslOnlyUnchanged(t *testing.T) {
	args := ipConfiguration(&gcloud.PostgresGcpCloudsqlConfig{RequireSsl: ipCfgBoolPtr(true)})
	require.NotNil(t, args)
	assert.Equal(t, sdk.Bool(true), args.Ipv4Enabled)
	assert.Equal(t, sdk.String("ENCRYPTED_ONLY"), args.SslMode)
	assert.Nil(t, args.PrivateNetwork)
}

// A private network keeps the public IP on by default (safe migration) and wires
// the private network path.
func TestIpConfiguration_PrivateNetworkKeepsPublicByDefault(t *testing.T) {
	net := "projects/p/global/networks/vpc"
	args := ipConfiguration(&gcloud.PostgresGcpCloudsqlConfig{PrivateNetwork: ipCfgStrPtr(net)})
	require.NotNil(t, args)
	assert.Equal(t, sdk.Bool(true), args.Ipv4Enabled)
	assert.Equal(t, sdk.String(net), args.PrivateNetwork)
}

// Explicitly disabling the public IP (only valid alongside a private network) must
// set Ipv4Enabled false.
func TestIpConfiguration_PublicDisabled(t *testing.T) {
	net := "projects/p/global/networks/vpc"
	args := ipConfiguration(&gcloud.PostgresGcpCloudsqlConfig{
		PrivateNetwork:  ipCfgStrPtr(net),
		PublicIpEnabled: ipCfgBoolPtr(false),
	})
	require.NotNil(t, args)
	assert.Equal(t, sdk.Bool(false), args.Ipv4Enabled)
	assert.Equal(t, sdk.String(net), args.PrivateNetwork)
}
