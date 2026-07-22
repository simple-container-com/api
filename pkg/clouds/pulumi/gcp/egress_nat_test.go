// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package gcp

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/simple-container-com/api/pkg/clouds/gcloud"
)

func natIntPtr(i int) *int    { return &i }
func natBoolPtr(b bool) *bool { return &b }

// A nil or empty egress config must reproduce the historical Cloud NAT defaults
// exactly, so existing clusters are unchanged until they opt in.
func TestResolveNatPortSettings_Defaults(t *testing.T) {
	for _, cfg := range []*gcloud.ExternalEgressIpConfig{nil, {Enabled: true}} {
		s := resolveNatPortSettings(cfg)
		assert.Equal(t, 64, s.minPortsPerVm)
		assert.Equal(t, 65536, s.maxPortsPerVm)
		assert.True(t, s.endpointIndependentMapping)
		assert.False(t, s.dynamicPortAllocation)
	}
}

func TestResolveNatPortSettings_Overrides(t *testing.T) {
	s := resolveNatPortSettings(&gcloud.ExternalEgressIpConfig{
		Enabled:                    true,
		MinPortsPerVm:              natIntPtr(1024),
		MaxPortsPerVm:              natIntPtr(4096),
		DynamicPortAllocation:      natBoolPtr(true),
		EndpointIndependentMapping: natBoolPtr(false),
	})
	assert.Equal(t, 1024, s.minPortsPerVm)
	assert.Equal(t, 4096, s.maxPortsPerVm)
	assert.False(t, s.endpointIndependentMapping)
	assert.True(t, s.dynamicPortAllocation)
}

// Dynamic port allocation must be rejected unless endpoint-independent mapping is
// explicitly off and both bounds are powers of two (GCP API constraints).
func TestExternalEgressIpConfig_NatTuningValidation(t *testing.T) {
	cases := []struct {
		name    string
		cfg     gcloud.ExternalEgressIpConfig
		wantErr string
	}{
		{
			name: "valid dynamic port allocation",
			cfg: gcloud.ExternalEgressIpConfig{
				Enabled:                    true,
				MinPortsPerVm:              natIntPtr(1024),
				MaxPortsPerVm:              natIntPtr(8192),
				DynamicPortAllocation:      natBoolPtr(true),
				EndpointIndependentMapping: natBoolPtr(false),
			},
		},
		{
			name: "valid raised min ports without dpa",
			cfg:  gcloud.ExternalEgressIpConfig{Enabled: true, MinPortsPerVm: natIntPtr(1000)},
		},
		{
			name:    "dpa requires eim off when unset",
			cfg:     gcloud.ExternalEgressIpConfig{Enabled: true, DynamicPortAllocation: natBoolPtr(true)},
			wantErr: "endpointIndependentMapping: false",
		},
		{
			name: "dpa requires eim off when explicitly true",
			cfg: gcloud.ExternalEgressIpConfig{
				Enabled: true, DynamicPortAllocation: natBoolPtr(true), EndpointIndependentMapping: natBoolPtr(true),
			},
			wantErr: "endpointIndependentMapping: false",
		},
		{
			name: "dpa min ports must be power of two",
			cfg: gcloud.ExternalEgressIpConfig{
				Enabled: true, DynamicPortAllocation: natBoolPtr(true), EndpointIndependentMapping: natBoolPtr(false),
				MinPortsPerVm: natIntPtr(1000),
			},
			wantErr: "power of two",
		},
		{
			name:    "min ports out of range",
			cfg:     gcloud.ExternalEgressIpConfig{Enabled: true, MinPortsPerVm: natIntPtr(70000)},
			wantErr: "minPortsPerVm must be between",
		},
		{
			name: "max below min",
			cfg: gcloud.ExternalEgressIpConfig{
				Enabled: true, MinPortsPerVm: natIntPtr(4096), MaxPortsPerVm: natIntPtr(1024),
			},
			wantErr: "must be >=",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.Validate()
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}
