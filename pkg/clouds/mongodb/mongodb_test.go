// SPDX-License-Identifier: MIT
// Copyright (c) 2025 Simple Container

package mongodb

import (
	"testing"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/api"
)

func TestAtlasConfig_Getters(t *testing.T) {
	RegisterTestingT(t)

	c := &AtlasConfig{
		PrivateKey:    "atlas-private-key-secret",
		ProjectId:     "proj-abc-123",
		OrgId:         "org-zzz",
		ProjectName:   "scratch",
		InstanceSize:  "M10",
		Region:        "EU_WEST_1",
		CloudProvider: "AWS",
		ExtraProviders: map[string]api.AuthDescriptor{
			"backup-bucket": {Type: "s3"},
		},
	}

	Expect(c.CredentialsValue()).To(Equal("atlas-private-key-secret"))
	Expect(c.ProjectIdValue()).To(Equal("proj-abc-123"))
	Expect(c.ProviderType()).To(Equal(ProviderType))
	Expect(c.ProviderType()).To(Equal("mongodb-atlas"))

	deps := c.DependencyProviders()
	Expect(deps).To(HaveLen(1))
	Expect(deps).To(HaveKey("backup-bucket"))
}

func TestAtlasConfig_ZeroValue(t *testing.T) {
	RegisterTestingT(t)

	c := &AtlasConfig{}
	Expect(c.CredentialsValue()).To(Equal(""))
	Expect(c.ProjectIdValue()).To(Equal(""))
	Expect(c.DependencyProviders()).To(BeNil())
	// ProviderType is invariant.
	Expect(c.ProviderType()).To(Equal("mongodb-atlas"))
}

func TestProviderTypeConstants(t *testing.T) {
	RegisterTestingT(t)

	// Both constants are the same value but serve different roles:
	// - ProviderType: identifies the cloud provider in api.RegisterProviderConfig
	// - ResourceTypeMongodbAtlas: identifies the resource type in config parsing
	Expect(ProviderType).To(Equal("mongodb-atlas"))
	Expect(ResourceTypeMongodbAtlas).To(Equal("mongodb-atlas"))
}

func TestAtlasNetworkConfig_FieldRoundTrip(t *testing.T) {
	RegisterTestingT(t)

	cidrs := []string{"10.0.0.0/16", "192.168.1.0/24"}
	allow := true

	nc := &AtlasNetworkConfig{
		PrivateLinkEndpoint: &PrivateLinkEndpoint{ProviderName: "AWS"},
		AllowAllIps:         &allow,
		AllowCidrs:          &cidrs,
	}

	Expect(nc.PrivateLinkEndpoint).ToNot(BeNil())
	Expect(nc.PrivateLinkEndpoint.ProviderName).To(Equal("AWS"))
	Expect(*nc.AllowAllIps).To(BeTrue())
	Expect(*nc.AllowCidrs).To(ContainElement("10.0.0.0/16"))
}

func TestAtlasBackup_FieldRoundTrip(t *testing.T) {
	RegisterTestingT(t)

	b := &AtlasBackup{Every: "2h", Retention: "168h"}
	Expect(b.Every).To(Equal("2h"))
	Expect(b.Retention).To(Equal("168h"))
}
