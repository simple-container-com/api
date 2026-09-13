// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package gcloud

import (
	"testing"

	. "github.com/onsi/gomega"
)

func boolPtr(v bool) *bool { return &v }

func TestControlPlaneAccessConfig_Validate(t *testing.T) {
	for _, tt := range []struct {
		name    string
		cfg     *ControlPlaneAccessConfig
		wantErr string
	}{
		{
			name: "nil config is valid",
			cfg:  nil,
		},
		{
			name: "authorized networks with a dns fallback",
			cfg: &ControlPlaneAccessConfig{
				DnsEndpoint:        boolPtr(true),
				AuthorizedNetworks: []AuthorizedNetwork{{Name: "office", Cidr: "203.0.113.0/24"}},
			},
		},
		{
			name: "single address needs a prefix length",
			cfg: &ControlPlaneAccessConfig{
				AuthorizedNetworks: []AuthorizedNetwork{{Cidr: "203.0.113.7"}},
			},
			wantErr: "is not a CIDR",
		},
		{
			name: "empty cidr is rejected",
			cfg: &ControlPlaneAccessConfig{
				AuthorizedNetworks: []AuthorizedNetwork{{Name: "office"}},
			},
			wantErr: "cidr is required",
		},
		{
			name: "a default route defeats the allow list",
			cfg: &ControlPlaneAccessConfig{
				AuthorizedNetworks: []AuthorizedNetwork{{Cidr: "0.0.0.0/0"}},
			},
			wantErr: "allows every address",
		},
		{
			name: "host bits set is rejected with the corrected form",
			cfg: &ControlPlaneAccessConfig{
				AuthorizedNetworks: []AuthorizedNetwork{{Cidr: "203.0.113.7/24"}},
			},
			wantErr: "use 203.0.113.0/24",
		},
		{
			name: "ipv6 is accepted",
			cfg: &ControlPlaneAccessConfig{
				DnsEndpoint:        boolPtr(true),
				AuthorizedNetworks: []AuthorizedNetwork{{Cidr: "2001:db8::/32"}},
			},
		},
		{
			name: "both endpoints off locks everyone out",
			cfg: &ControlPlaneAccessConfig{
				IpEndpoint:  boolPtr(false),
				DnsEndpoint: boolPtr(false),
			},
			wantErr: "leaves no way to reach the control plane",
		},
		{
			name: "ip endpoint off with dns on is valid",
			cfg: &ControlPlaneAccessConfig{
				IpEndpoint:  boolPtr(false),
				DnsEndpoint: boolPtr(true),
			},
		},
		{
			name: "empty allow list without a dns fallback locks everyone out",
			cfg: &ControlPlaneAccessConfig{
				AllowGcpPublicCidrs: boolPtr(false),
			},
			wantErr: "authorized network list is empty",
		},
		{
			name: "empty allow list is fine once dns is open",
			cfg: &ControlPlaneAccessConfig{
				DnsEndpoint:         boolPtr(true),
				AllowGcpPublicCidrs: boolPtr(false),
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)

			err := tt.cfg.Validate()
			if tt.wantErr == "" {
				Expect(err).To(BeNil())
				return
			}
			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(ContainSubstring(tt.wantErr))
		})
	}
}

func TestControlPlaneAccessConfig_Defaults(t *testing.T) {
	RegisterTestingT(t)

	var nilCfg *ControlPlaneAccessConfig
	// An absent block must read as "GKE default", which is an IP endpoint that
	// is reachable and no allow list, so nothing gets written to the cluster.
	Expect(nilCfg.IpEndpointEnabled()).To(BeTrue())
	Expect(nilCfg.DnsEndpointEnabled()).To(BeFalse())
	Expect(nilCfg.AuthorizedNetworksEnabled()).To(BeFalse())
	Expect(nilCfg.GcpPublicCidrsAllowed()).To(BeFalse())

	empty := &ControlPlaneAccessConfig{}
	Expect(empty.IpEndpointEnabled()).To(BeTrue())
	Expect(empty.AuthorizedNetworksEnabled()).To(BeFalse())

	// Asking only to drop the Google Cloud exemption still turns the allow list
	// on, otherwise the request would be silently ignored.
	onlyGcpCidrs := &ControlPlaneAccessConfig{AllowGcpPublicCidrs: boolPtr(false)}
	Expect(onlyGcpCidrs.AuthorizedNetworksEnabled()).To(BeTrue())
	Expect(onlyGcpCidrs.GcpPublicCidrsAllowed()).To(BeFalse())
}

// An explicitly empty list is a request to authorise nobody, and it must not be
// read as "no opinion". Read as no opinion, the whole block is dropped and the
// control plane keeps GKE's default, which is reachable from anywhere.
func TestControlPlaneAccessConfig_ExplicitlyEmptyListIsNotAbsent(t *testing.T) {
	RegisterTestingT(t)

	explicit := &ControlPlaneAccessConfig{AuthorizedNetworks: []AuthorizedNetwork{}}
	Expect(explicit.AuthorizedNetworksEnabled()).To(BeTrue())
	Expect(explicit.Validate()).ToNot(BeNil())

	absent := &ControlPlaneAccessConfig{}
	Expect(absent.AuthorizedNetworksEnabled()).To(BeFalse())
	Expect(absent.Validate()).To(BeNil())
}
