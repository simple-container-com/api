// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package gcloud

import (
	"strconv"
	"strings"
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

// Cloud KMS bills every ACTIVE key version (ENABLED, DISABLED and
// DESTROY_SCHEDULED; only DESTROYED is free) and rotation never re-encrypts
// existing ciphertext, so every version a key mints stays billed for the life
// of the key. With one provisioned key per stack, the rotation period is a
// compounding cost multiplier rather than a cosmetic setting.
func TestSecretsProviderConfig_EffectiveKeyRotationPeriod(t *testing.T) {
	RegisterTestingT(t)

	Expect((&SecretsProviderConfig{}).EffectiveKeyRotationPeriod()).To(Equal(DefaultKeyRotationPeriod))
	Expect((&SecretsProviderConfig{KeyRotationPeriod: "31536000s"}).EffectiveKeyRotationPeriod()).
		To(Equal("31536000s"), "an explicit value must win over the default")
}

func TestDefaultKeyRotationPeriodIsSane(t *testing.T) {
	RegisterTestingT(t)

	// Pin the value, not just its properties: asserting only "effective ==
	// Default" passes for any constant, including the 100000s (27.8h) typo
	// this default replaced.
	Expect(DefaultKeyRotationPeriod).To(Equal("7776000s"), "90 days")

	secs, err := strconv.Atoi(strings.TrimSuffix(DefaultKeyRotationPeriod, "s"))
	Expect(err).To(BeNil())
	Expect(secs).To(BeNumerically(">=", MinKeyRotationPeriodSeconds))
	Expect(secs).To(BeNumerically(">", GcpMinKeyRotationPeriodSeconds),
		"must be well clear of GCP's 24h minimum")
	Expect(secs).To(BeNumerically("<=", GcpMaxKeyRotationPeriodSeconds))
}

func TestSecretsProviderConfig_Validate(t *testing.T) {
	tests := []struct {
		name      string
		cfg       SecretsProviderConfig
		errSubstr string
	}{
		{name: "unset means default", cfg: SecretsProviderConfig{Provision: true}},
		{name: "90 days", cfg: SecretsProviderConfig{Provision: true, KeyRotationPeriod: "7776000s"}},
		{name: "exactly the 30-day floor", cfg: SecretsProviderConfig{Provision: true, KeyRotationPeriod: "2592000s"}},
		// keyRotationPeriod is meaningless for a BYO key referenced by keyName,
		// so a stale value must not block those consumers.
		{name: "provision disabled ignores the period", cfg: SecretsProviderConfig{Provision: false, KeyRotationPeriod: "100000s"}},
		{name: "below policy floor", cfg: SecretsProviderConfig{Provision: true, KeyRotationPeriod: "100000s"}, errSubstr: "30 day"},
		{name: "one second under the floor", cfg: SecretsProviderConfig{Provision: true, KeyRotationPeriod: "2591999s"}, errSubstr: "30 day"},
		{name: "policy floor waivable", cfg: SecretsProviderConfig{Provision: true, KeyRotationPeriod: "604800s", AllowShortKeyRotation: true}},
		// The opt-out waives the POLICY floor only. GCP's own bounds stay hard:
		// breaching them fails at the KMS API after the KeyRing already exists,
		// and a KeyRing can never be deleted.
		{name: "gcp floor NOT waivable", cfg: SecretsProviderConfig{Provision: true, KeyRotationPeriod: "3600s", AllowShortKeyRotation: true}, errSubstr: "at least 86400"},
		{name: "zero NOT waivable", cfg: SecretsProviderConfig{Provision: true, KeyRotationPeriod: "0s", AllowShortKeyRotation: true}, errSubstr: "at least 86400"},
		{name: "negative NOT waivable", cfg: SecretsProviderConfig{Provision: true, KeyRotationPeriod: "-100s", AllowShortKeyRotation: true}, errSubstr: "at least 86400"},
		{name: "above gcp maximum", cfg: SecretsProviderConfig{Provision: true, KeyRotationPeriod: "3153600001s", AllowShortKeyRotation: true}, errSubstr: "at most"},
		{name: "missing seconds suffix", cfg: SecretsProviderConfig{Provision: true, KeyRotationPeriod: "7776000"}, errSubstr: "'s' suffix"},
		{name: "not a number but ends in s", cfg: SecretsProviderConfig{Provision: true, KeyRotationPeriod: "ninetydays"}, errSubstr: "whole number of seconds"},
		{name: "duration shorthand", cfg: SecretsProviderConfig{Provision: true, KeyRotationPeriod: "90d"}, errSubstr: "'s' suffix"},
		{name: "malformed still fails under the opt-out", cfg: SecretsProviderConfig{Provision: true, KeyRotationPeriod: "90d", AllowShortKeyRotation: true}, errSubstr: "'s' suffix"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			err := tt.cfg.Validate()
			if tt.errSubstr == "" {
				Expect(err).To(BeNil())
				return
			}
			Expect(err).NotTo(BeNil(), "expected %q to be rejected", tt.cfg.KeyRotationPeriod)
			Expect(err.Error()).To(ContainSubstring(tt.errSubstr))
		})
	}
}
