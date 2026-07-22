// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package gcp

import (
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"

	"github.com/simple-container-com/api/pkg/clouds/gcloud"
)

// A nil or empty egress config must reproduce the historical Cloud NAT defaults
// exactly, so existing clusters are unchanged until they opt in.
func TestResolveNatPortSettings_Defaults(t *testing.T) {
	for _, cfg := range []*gcloud.ExternalEgressIpConfig{nil, {Enabled: true}} {
		s := resolveNatPortSettings(cfg)
		assert.Equal(t, gcloud.DefaultMinPortsPerVm, s.minPortsPerVm)
		assert.Equal(t, gcloud.DefaultMaxPortsPerVm, s.maxPortsPerVm)
		assert.True(t, s.endpointIndependentMapping)
		assert.False(t, s.dynamicPortAllocation)
	}
}

func TestResolveNatPortSettings_Overrides(t *testing.T) {
	s := resolveNatPortSettings(&gcloud.ExternalEgressIpConfig{
		Enabled:                    true,
		MinPortsPerVm:              lo.ToPtr(1024),
		MaxPortsPerVm:              lo.ToPtr(4096),
		DynamicPortAllocation:      lo.ToPtr(true),
		EndpointIndependentMapping: lo.ToPtr(false),
	})
	assert.Equal(t, 1024, s.minPortsPerVm)
	assert.Equal(t, 4096, s.maxPortsPerVm)
	assert.False(t, s.endpointIndependentMapping)
	assert.True(t, s.dynamicPortAllocation)
}

// A single-field override must not drop the other field's default.
func TestResolveNatPortSettings_PartialOverride(t *testing.T) {
	s := resolveNatPortSettings(&gcloud.ExternalEgressIpConfig{Enabled: true, MinPortsPerVm: lo.ToPtr(1024)})
	assert.Equal(t, 1024, s.minPortsPerVm)
	assert.Equal(t, gcloud.DefaultMaxPortsPerVm, s.maxPortsPerVm)
}

// Enabling dynamic port allocation must force endpoint-independent mapping off in
// the resolved settings even if the config left EIM at its default, so the NAT
// args stay acceptable to GCP regardless of validation order.
func TestResolveNatPortSettings_DpaForcesEimOff(t *testing.T) {
	s := resolveNatPortSettings(&gcloud.ExternalEgressIpConfig{Enabled: true, DynamicPortAllocation: lo.ToPtr(true)})
	assert.True(t, s.dynamicPortAllocation)
	assert.False(t, s.endpointIndependentMapping)
}
