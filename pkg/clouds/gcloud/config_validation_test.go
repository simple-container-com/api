// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package gcloud

import (
	"testing"

	. "github.com/onsi/gomega"
	"github.com/samber/lo"
)

// Cloud NAT port tuning is validated against the EFFECTIVE values (defaults
// applied) and GCP's real bounds, so misconfigurations fail here instead of late
// at pulumi up.
func TestExternalEgressIpConfig_NatTuningValidate(t *testing.T) {
	tests := []struct {
		name      string
		cfg       ExternalEgressIpConfig
		errSubstr string
	}{
		{name: "valid dynamic port allocation", cfg: ExternalEgressIpConfig{Enabled: true, MinPortsPerVm: lo.ToPtr(1024), MaxPortsPerVm: lo.ToPtr(8192), DynamicPortAllocation: lo.ToPtr(true), EndpointIndependentMapping: lo.ToPtr(false)}},
		{name: "valid raised min without dpa", cfg: ExternalEgressIpConfig{Enabled: true, MinPortsPerVm: lo.ToPtr(1024)}},
		{name: "min below floor", cfg: ExternalEgressIpConfig{Enabled: true, MinPortsPerVm: lo.ToPtr(16)}, errSubstr: "minPortsPerVm must be between"},
		{name: "min above ceiling", cfg: ExternalEgressIpConfig{Enabled: true, MinPortsPerVm: lo.ToPtr(70000)}, errSubstr: "minPortsPerVm must be between"},
		{name: "max below floor", cfg: ExternalEgressIpConfig{Enabled: true, MaxPortsPerVm: lo.ToPtr(16)}, errSubstr: "maxPortsPerVm must be between"},
		{name: "max above ceiling", cfg: ExternalEgressIpConfig{Enabled: true, MaxPortsPerVm: lo.ToPtr(70000)}, errSubstr: "maxPortsPerVm must be between"},
		{name: "lone max below default min", cfg: ExternalEgressIpConfig{Enabled: true, MaxPortsPerVm: lo.ToPtr(48)}, errSubstr: "must be >= minPortsPerVm"},
		{name: "explicit max below min", cfg: ExternalEgressIpConfig{Enabled: true, MinPortsPerVm: lo.ToPtr(4096), MaxPortsPerVm: lo.ToPtr(1024)}, errSubstr: "must be >= minPortsPerVm"},
		{name: "dpa requires eim off when unset", cfg: ExternalEgressIpConfig{Enabled: true, DynamicPortAllocation: lo.ToPtr(true)}, errSubstr: "endpointIndependentMapping: false"},
		{name: "dpa requires eim off when explicitly true", cfg: ExternalEgressIpConfig{Enabled: true, DynamicPortAllocation: lo.ToPtr(true), EndpointIndependentMapping: lo.ToPtr(true)}, errSubstr: "endpointIndependentMapping: false"},
		{name: "dpa min not power of two", cfg: ExternalEgressIpConfig{Enabled: true, DynamicPortAllocation: lo.ToPtr(true), EndpointIndependentMapping: lo.ToPtr(false), MinPortsPerVm: lo.ToPtr(1000)}, errSubstr: "minPortsPerVm must be a power of two"},
		{name: "dpa max not power of two", cfg: ExternalEgressIpConfig{Enabled: true, DynamicPortAllocation: lo.ToPtr(true), EndpointIndependentMapping: lo.ToPtr(false), MaxPortsPerVm: lo.ToPtr(3000)}, errSubstr: "maxPortsPerVm must be a power of two"},
		{name: "dpa max equals min", cfg: ExternalEgressIpConfig{Enabled: true, DynamicPortAllocation: lo.ToPtr(true), EndpointIndependentMapping: lo.ToPtr(false), MinPortsPerVm: lo.ToPtr(1024), MaxPortsPerVm: lo.ToPtr(1024)}, errSubstr: "must be > minPortsPerVm"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			err := tc.cfg.Validate()
			if tc.errSubstr == "" {
				Expect(err).ToNot(HaveOccurred())
			} else {
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(tc.errSubstr))
			}
		})
	}
}

func TestPostgresGcpCloudsqlConfig_Validate(t *testing.T) {
	tests := []struct {
		name      string
		cfg       PostgresGcpCloudsqlConfig
		errSubstr string
	}{
		{name: "empty is valid", cfg: PostgresGcpCloudsqlConfig{}},
		{name: "availabilityType invalid", cfg: PostgresGcpCloudsqlConfig{AvailabilityType: lo.ToPtr("HA")}, errSubstr: "availabilityType must be"},
		{name: "availabilityType regional ok", cfg: PostgresGcpCloudsqlConfig{AvailabilityType: lo.ToPtr("REGIONAL")}},
		{name: "privateNetwork bad format", cfg: PostgresGcpCloudsqlConfig{PrivateNetwork: lo.ToPtr("my-vpc")}, errSubstr: "privateNetwork must be a full"},
		{name: "privateNetwork ok", cfg: PostgresGcpCloudsqlConfig{PrivateNetwork: lo.ToPtr("projects/p/global/networks/vpc")}},
		{name: "public off without network", cfg: PostgresGcpCloudsqlConfig{PublicIpEnabled: lo.ToPtr(false)}, errSubstr: "requires privateNetwork"},
		{name: "public off with empty network", cfg: PostgresGcpCloudsqlConfig{PublicIpEnabled: lo.ToPtr(false), PrivateNetwork: lo.ToPtr("")}, errSubstr: "requires privateNetwork"},
		{name: "public off with network ok", cfg: PostgresGcpCloudsqlConfig{PublicIpEnabled: lo.ToPtr(false), PrivateNetwork: lo.ToPtr("projects/p/global/networks/vpc")}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			err := tc.cfg.Validate()
			if tc.errSubstr == "" {
				Expect(err).ToNot(HaveOccurred())
			} else {
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(tc.errSubstr))
			}
		})
	}
}

func TestPostgresGcpCloudsqlConfig_ProxyAndNetworkHelpers(t *testing.T) {
	RegisterTestingT(t)
	Expect((&PostgresGcpCloudsqlConfig{}).HasPrivateNetwork()).To(BeFalse())
	Expect((&PostgresGcpCloudsqlConfig{PrivateNetwork: lo.ToPtr("")}).HasPrivateNetwork()).To(BeFalse())
	Expect((&PostgresGcpCloudsqlConfig{PrivateNetwork: lo.ToPtr("projects/p/global/networks/vpc")}).HasPrivateNetwork()).To(BeTrue())

	// Proxy stays on the public endpoint until the public IP is disabled.
	Expect((&PostgresGcpCloudsqlConfig{}).UsesPrivateIpProxy()).To(BeFalse())
	Expect((&PostgresGcpCloudsqlConfig{PublicIpEnabled: lo.ToPtr(true)}).UsesPrivateIpProxy()).To(BeFalse())
	Expect((&PostgresGcpCloudsqlConfig{PrivateNetwork: lo.ToPtr("projects/p/global/networks/vpc")}).UsesPrivateIpProxy()).To(BeFalse())
	Expect((&PostgresGcpCloudsqlConfig{PublicIpEnabled: lo.ToPtr(false)}).UsesPrivateIpProxy()).To(BeTrue())
}
