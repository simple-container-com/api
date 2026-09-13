// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package gcloud

import (
	"fmt"
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
			wantErr: "is a single address, it needs an explicit /32 or /128",
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
			name: "ipv6 passes local validation",
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
			// allowGcpPublicCidrs authorises Google Cloud address space, which
			// is every other tenant and nobody in this organisation, so it is
			// not a way in and must not satisfy the lockout check.
			name: "gcp public cidrs alone is not an access path",
			cfg: &ControlPlaneAccessConfig{
				AllowGcpPublicCidrs: boolPtr(true),
			},
			wantErr: "authorized network list is empty",
		},
		{
			name: "an allow list on a disabled ip endpoint governs nothing",
			cfg: &ControlPlaneAccessConfig{
				IpEndpoint:         boolPtr(false),
				DnsEndpoint:        boolPtr(true),
				AuthorizedNetworks: []AuthorizedNetwork{{Cidr: "203.0.113.0/24"}},
			},
			wantErr: "no effect while ipEndpoint is disabled",
		},
		{
			// The index has to be real, otherwise a loop that stops after the
			// first entry looks identical to one that checks them all.
			name: "the reported index names the offending entry",
			cfg: &ControlPlaneAccessConfig{
				DnsEndpoint: boolPtr(true),
				AuthorizedNetworks: []AuthorizedNetwork{
					{Cidr: "203.0.113.0/24"},
					{Cidr: "10.0.0.1/8"},
				},
			},
			wantErr: "authorizedNetworks[1]",
		},
		{
			name: "duplicate networks are rejected and name the first one",
			cfg: &ControlPlaneAccessConfig{
				DnsEndpoint: boolPtr(true),
				AuthorizedNetworks: []AuthorizedNetwork{
					{Cidr: "203.0.113.0/24"},
					{Name: "same range, different name", Cidr: "203.0.113.0/24"},
				},
			},
			wantErr: "duplicates entry 0",
		},
		{
			name: "surrounding whitespace is tolerated",
			cfg: &ControlPlaneAccessConfig{
				DnsEndpoint:        boolPtr(true),
				AuthorizedNetworks: []AuthorizedNetwork{{Cidr: "  203.0.113.0/24 "}},
			},
		},
		{
			name: "garbage is not reported as a missing prefix length",
			cfg: &ControlPlaneAccessConfig{
				AuthorizedNetworks: []AuthorizedNetwork{{Cidr: "nonsense"}},
			},
			wantErr: "is not a CIDR",
		},
		{
			name: "a correct single address is accepted",
			cfg: &ControlPlaneAccessConfig{
				DnsEndpoint:        boolPtr(true),
				AuthorizedNetworks: []AuthorizedNetwork{{Cidr: "203.0.113.7/32"}},
			},
		},
		{
			name: "the ipv6 default route is rejected too",
			cfg: &ControlPlaneAccessConfig{
				AuthorizedNetworks: []AuthorizedNetwork{{Cidr: "::/0"}},
			},
			wantErr: "allows every address",
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

func TestControlPlaneAccessConfig_TooManyNetworks(t *testing.T) {
	RegisterTestingT(t)

	nets := make([]AuthorizedNetwork, 0, 51)
	for i := 0; i < 51; i++ {
		nets = append(nets, AuthorizedNetwork{Cidr: fmt.Sprintf("10.%d.0.0/16", i)})
	}
	cfg := &ControlPlaneAccessConfig{DnsEndpoint: boolPtr(true), AuthorizedNetworks: nets}

	// Past GKE's cap the cluster is rejected by the API after the update has
	// already started, so it is caught here instead.
	err := cfg.Validate()
	Expect(err).ToNot(BeNil())
	Expect(err.Error()).To(ContainSubstring("exceeds GKE's limit of 50"))

	Expect((&ControlPlaneAccessConfig{DnsEndpoint: boolPtr(true), AuthorizedNetworks: nets[:50]}).Validate()).To(BeNil())
}
