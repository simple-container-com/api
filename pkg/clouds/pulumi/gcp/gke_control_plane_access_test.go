// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package gcp

import (
	"testing"

	. "github.com/onsi/gomega"
	"github.com/pulumi/pulumi-gcp/sdk/v8/go/gcp/container"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/clouds/gcloud"
)

func cpaBool(v bool) *bool { return &v }

func TestApplyControlPlaneAccess_NilLeavesArgsUntouched(t *testing.T) {
	RegisterTestingT(t)

	args := &container.ClusterArgs{}
	applyControlPlaneAccess(args, nil)

	// Writing either field would make Pulumi start managing it, and an omitted
	// allow list then reads as "remove the one that is there".
	Expect(args.MasterAuthorizedNetworksConfig).To(BeNil())
	Expect(args.ControlPlaneEndpointsConfig).To(BeNil())
}

func TestApplyControlPlaneAccess_AuthorizedNetworks(t *testing.T) {
	RegisterTestingT(t)

	args := &container.ClusterArgs{}
	applyControlPlaneAccess(args, &gcloud.ControlPlaneAccessConfig{
		DnsEndpoint: cpaBool(true),
		AuthorizedNetworks: []gcloud.AuthorizedNetwork{
			{Name: "office", Cidr: "203.0.113.0/24"},
			{Cidr: "198.51.100.8/29"},
		},
	})

	Expect(args.MasterAuthorizedNetworksConfig).ToNot(BeNil())
	man, ok := args.MasterAuthorizedNetworksConfig.(*container.ClusterMasterAuthorizedNetworksConfigArgs)
	Expect(ok).To(BeTrue())

	blocks, ok := man.CidrBlocks.(container.ClusterMasterAuthorizedNetworksConfigCidrBlockArray)
	Expect(ok).To(BeTrue())
	Expect(blocks).To(HaveLen(2))

	first, ok := blocks[0].(container.ClusterMasterAuthorizedNetworksConfigCidrBlockArgs)
	Expect(ok).To(BeTrue())
	Expect(first.CidrBlock).To(Equal(sdk.String("203.0.113.0/24")))
	Expect(first.DisplayName).To(Equal(sdk.String("office")))

	second, ok := blocks[1].(container.ClusterMasterAuthorizedNetworksConfigCidrBlockArgs)
	Expect(ok).To(BeTrue())
	Expect(second.CidrBlock).To(Equal(sdk.String("198.51.100.8/29")))
	// An unnamed entry must stay unset rather than become an empty string.
	Expect(second.DisplayName).To(BeNil())

	// GKE defaults this to true, which would admit every Google Cloud tenant
	// alongside the list, so an unset field must still be sent as false.
	Expect(man.GcpPublicCidrsAccessEnabled).To(Equal(sdk.Bool(false)))
}

func TestApplyControlPlaneAccess_GcpPublicCidrsOptIn(t *testing.T) {
	RegisterTestingT(t)

	args := &container.ClusterArgs{}
	applyControlPlaneAccess(args, &gcloud.ControlPlaneAccessConfig{
		DnsEndpoint:         cpaBool(true),
		AllowGcpPublicCidrs: cpaBool(true),
	})

	man, ok := args.MasterAuthorizedNetworksConfig.(*container.ClusterMasterAuthorizedNetworksConfigArgs)
	Expect(ok).To(BeTrue())
	Expect(man.GcpPublicCidrsAccessEnabled).To(Equal(sdk.Bool(true)))
}

func TestApplyControlPlaneAccess_Endpoints(t *testing.T) {
	RegisterTestingT(t)

	args := &container.ClusterArgs{}
	applyControlPlaneAccess(args, &gcloud.ControlPlaneAccessConfig{
		DnsEndpoint: cpaBool(true),
		IpEndpoint:  cpaBool(false),
	})

	endpoints, ok := args.ControlPlaneEndpointsConfig.(*container.ClusterControlPlaneEndpointsConfigArgs)
	Expect(ok).To(BeTrue())

	dns, ok := endpoints.DnsEndpointConfig.(*container.ClusterControlPlaneEndpointsConfigDnsEndpointConfigArgs)
	Expect(ok).To(BeTrue())
	Expect(dns.AllowExternalTraffic).To(Equal(sdk.Bool(true)))

	ip, ok := endpoints.IpEndpointsConfig.(*container.ClusterControlPlaneEndpointsConfigIpEndpointsConfigArgs)
	Expect(ok).To(BeTrue())
	Expect(ip.Enabled).To(Equal(sdk.Bool(false)))
}

func TestApplyControlPlaneAccess_OnlyAllowListLeavesEndpointsAlone(t *testing.T) {
	RegisterTestingT(t)

	args := &container.ClusterArgs{}
	applyControlPlaneAccess(args, &gcloud.ControlPlaneAccessConfig{
		AuthorizedNetworks: []gcloud.AuthorizedNetwork{{Cidr: "203.0.113.0/24"}},
	})

	Expect(args.MasterAuthorizedNetworksConfig).ToNot(BeNil())
	// Neither endpoint flag was named, so neither is taken over.
	Expect(args.ControlPlaneEndpointsConfig).To(BeNil())
}
