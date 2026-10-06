// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package gcp

import (
	"strings"
	"sync"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/samber/lo"

	gcpsdk "github.com/pulumi/pulumi-gcp/sdk/v8/go/gcp"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/gcloud"
)

const (
	psaTestNetwork    = "projects/p/global/networks/vpc"
	psaTestConnection = "gcp:servicenetworking/connection:Connection"
	psaTestAddress    = "gcp:compute/globalAddress:GlobalAddress"
	psaTestInstance   = "gcp:sql/databaseInstance:DatabaseInstance"
)

type psaMocks struct {
	mu        sync.Mutex
	resources map[string][]pulumi.MockResourceArgs
}

func (m *psaMocks) NewResource(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.resources[args.TypeToken] = append(m.resources[args.TypeToken], args)
	return args.Name + "_id", args.Inputs, nil
}

func (m *psaMocks) Call(args pulumi.MockCallArgs) (resource.PropertyMap, error) {
	return args.Args, nil
}

func (m *psaMocks) only(t string) pulumi.MockResourceArgs {
	Expect(m.resources[t]).To(HaveLen(1), "exactly one %s", t)
	return m.resources[t][0]
}

func runPostgresWithMocks(t *testing.T, pgCfg *gcloud.PostgresGcpCloudsqlConfig) (*psaMocks, *mockServicesAPIClient, error) {
	t.Helper()
	services := newMockServicesAPIClient()
	setGlobalServicesAPIClient(services)
	t.Cleanup(resetGlobalServicesAPIClient)

	mocks := &psaMocks{resources: map[string][]pulumi.MockResourceArgs{}}
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		prov, err := gcpsdk.NewProvider(ctx, "gcp", &gcpsdk.ProviderArgs{Project: pulumi.String("p")})
		if err != nil {
			return err
		}
		params := createBasicProvisionParams()
		params.Provider = prov
		_, err = Postgres(ctx, api.Stack{}, api.ResourceInput{
			Descriptor: &api.ResourceDescriptor{
				Name:   "postgres",
				Type:   gcloud.ResourceTypePostgresGcpCloudsql,
				Config: api.Config{Config: pgCfg},
			},
			StackParams: &api.StackParams{Environment: "production"},
		}, params)
		return err
	}, pulumi.WithMocks("project", "stack", mocks))
	delete(mocks.resources, "pulumi:providers:gcp")
	return mocks, services, err
}

func psaTestConfig(psaRange *string) *gcloud.PostgresGcpCloudsqlConfig {
	cfg := &gcloud.PostgresGcpCloudsqlConfig{
		Version:                    "POSTGRES_14",
		PrivateNetwork:             lo.ToPtr(psaTestNetwork),
		PrivateServicesAccessRange: psaRange,
	}
	cfg.ProjectId = "p"
	return cfg
}

func TestPostgres_PrivateServicesAccessCreatedBeforeInstance(t *testing.T) {
	RegisterTestingT(t)
	mocks, services, err := runPostgresWithMocks(t, psaTestConfig(lo.ToPtr("10.30.0.0/20")))
	Expect(err).ToNot(HaveOccurred())

	Expect(services.isServiceEnabled("projects/p/services/servicenetworking.googleapis.com")).To(BeTrue())

	addr := mocks.only(psaTestAddress).Inputs
	Expect(addr["purpose"].StringValue()).To(Equal("VPC_PEERING"))
	Expect(addr["addressType"].StringValue()).To(Equal("INTERNAL"))
	Expect(addr["address"].StringValue()).To(Equal("10.30.0.0"))
	Expect(addr["prefixLength"].NumberValue()).To(BeEquivalentTo(20))
	Expect(addr["network"].StringValue()).To(Equal(psaTestNetwork))
	rangeName := addr["name"].StringValue()
	Expect(rangeName).To(Equal("postgres--production-psa"))
	addrArgs := mocks.only(psaTestAddress)
	Expect(addrArgs.RegisterRPC.GetRetainOnDelete()).To(BeTrue(), "the range must outlive the abandoned peering")

	conn := mocks.only(psaTestConnection)
	Expect(conn.Inputs["service"].StringValue()).To(Equal("servicenetworking.googleapis.com"))
	Expect(conn.Inputs["network"].StringValue()).To(Equal(psaTestNetwork))
	Expect(conn.Inputs["deletionPolicy"].StringValue()).To(Equal("ABANDON"))
	Expect(conn.RegisterRPC.GetDependencies()).To(ContainElement(ContainSubstring(psaTestAddress+"::"+addrArgs.Name)),
		"the connection must reference the reserved range resource, not a literal name")
	ranges := conn.Inputs["reservedPeeringRanges"].ArrayValue()
	Expect(ranges).To(HaveLen(1))
	Expect(ranges[0].StringValue()).To(Equal(rangeName))

	instance := mocks.only(psaTestInstance)
	Expect(instance.RegisterRPC).ToNot(BeNil())
	dependsOnConnection := lo.ContainsBy(instance.RegisterRPC.GetDependencies(), func(urn string) bool {
		return strings.Contains(urn, psaTestConnection+"::"+conn.Name)
	})
	Expect(dependsOnConnection).To(BeTrue(), "instance must wait for the peering, got %v", instance.RegisterRPC.GetDependencies())
	ip := instance.Inputs["settings"].ObjectValue()["ipConfiguration"].ObjectValue()
	Expect(ip["privateNetwork"].StringValue()).To(Equal(psaTestNetwork))
	Expect(ip["ipv4Enabled"].BoolValue()).To(BeTrue())
}

// Without the range the instance must not touch Service Networking, so stacks
// whose VPC already has Private Services Access are unchanged.
func TestPostgres_NoPrivateServicesAccessWithoutRange(t *testing.T) {
	RegisterTestingT(t)
	for _, psaRange := range []*string{nil, lo.ToPtr("")} {
		mocks, services, err := runPostgresWithMocks(t, psaTestConfig(psaRange))
		Expect(err).ToNot(HaveOccurred())
		Expect(mocks.resources[psaTestAddress]).To(BeEmpty())
		Expect(mocks.resources[psaTestConnection]).To(BeEmpty())
		Expect(services.isServiceEnabled("projects/p/services/servicenetworking.googleapis.com")).To(BeFalse())
		Expect(mocks.only(psaTestInstance).RegisterRPC.GetDependencies()).ToNot(ContainElement(ContainSubstring(psaTestConnection)))
	}
}

func TestPostgres_InvalidPrivateServicesAccessRangeFailsBeforeAnyResource(t *testing.T) {
	RegisterTestingT(t)
	mocks, services, err := runPostgresWithMocks(t, psaTestConfig(lo.ToPtr("10.30.0.0/25")))
	Expect(err).To(MatchError(ContainSubstring("/24 or larger")))
	Expect(mocks.resources).To(BeEmpty())
	Expect(services.isServiceEnabled("projects/p/services/servicenetworking.googleapis.com")).To(BeFalse())
}

func TestPostgres_PrivateServicesAccessRangeNameTooLong(t *testing.T) {
	RegisterTestingT(t)
	services := newMockServicesAPIClient()
	setGlobalServicesAPIClient(services)
	t.Cleanup(resetGlobalServicesAPIClient)

	mocks := &psaMocks{resources: map[string][]pulumi.MockResourceArgs{}}
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		prov, err := gcpsdk.NewProvider(ctx, "gcp", &gcpsdk.ProviderArgs{Project: pulumi.String("p")})
		if err != nil {
			return err
		}
		params := createBasicProvisionParams()
		params.Provider = prov
		_, err = Postgres(ctx, api.Stack{}, api.ResourceInput{
			Descriptor: &api.ResourceDescriptor{
				Name:   strings.Repeat("x", 50),
				Type:   gcloud.ResourceTypePostgresGcpCloudsql,
				Config: api.Config{Config: psaTestConfig(lo.ToPtr("10.30.0.0/20"))},
			},
			StackParams: &api.StackParams{Environment: "production"},
		}, params)
		return err
	}, pulumi.WithMocks("project", "stack", mocks))
	Expect(err).To(MatchError(ContainSubstring("exceeds 63 characters")))
	Expect(mocks.resources[psaTestAddress]).To(BeEmpty())
	Expect(services.isServiceEnabled("projects/p/services/servicenetworking.googleapis.com")).To(BeFalse())
}
