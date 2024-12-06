package aws

import (
	"fmt"

	"github.com/pkg/errors"
	"github.com/samber/lo"

	awsImpl "github.com/pulumi/pulumi-aws/sdk/v5/go/aws"
	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/ec2"
	ec2V5 "github.com/pulumi/pulumi-aws/sdk/v5/go/aws/ec2"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/clouds/aws"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	"github.com/simple-container-com/api/pkg/util"
)

type LookedupSubnet struct {
	id            sdk.IDOutput
	arn           sdk.StringOutput
	cidrBlock     sdk.StringOutput
	ipv6CidrBlock sdk.StringOutput
	az            sdk.StringOutput
	azName        string
}

type Subnet struct {
	LookedupSubnet
	resource sdk.Resource
}

type (
	LookedupSubnets []LookedupSubnet
	defaultSubnets  []Subnet
)

func (s *LookedupSubnets) Arns() sdk.StringArrayInput {
	return sdk.StringArray(lo.Map(*s, func(subnet LookedupSubnet, _ int) sdk.StringInput {
		return subnet.arn
	}))
}

func (s *defaultSubnets) Ids() sdk.StringArrayInput {
	return sdk.StringArray(lo.Map(*s, func(subnet Subnet, _ int) sdk.StringInput {
		return subnet.id
	}))
}

func (s *LookedupSubnets) Ids() sdk.StringArrayInput {
	return sdk.StringArray(lo.Map(*s, func(subnet LookedupSubnet, _ int) sdk.StringInput {
		return subnet.id
	}))
}

func (s *defaultSubnets) Resources() []sdk.Resource {
	return lo.Map(*s, func(subnet Subnet, _ int) sdk.Resource {
		return subnet.resource
	})
}

func GetAvailabilityZones(ctx *sdk.Context, account aws.AccountConfig, provider sdk.ProviderResource) (*awsImpl.GetAvailabilityZonesResult, error) {
	// Get all availability zones in provided region
	availabilityZones, err := awsImpl.GetAvailabilityZones(ctx, &awsImpl.GetAvailabilityZonesArgs{
		Filters: []awsImpl.GetAvailabilityZonesFilter{
			{
				Name:   "region-name",
				Values: []string{account.Region},
			},
		},
	}, sdk.Provider(provider))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get availability zones in region %q", account.Region)
	}
	return availabilityZones, nil
}

func NewVpcInAccount(ctx *sdk.Context, vpcName string, opts ...sdk.ResourceOption) (*ec2.DefaultVpc, error) {
	return ec2.NewDefaultVpc(ctx, vpcName, nil, opts...)
}

func LookupSubnetsInAccount(ctx *sdk.Context, account aws.AccountConfig, provider sdk.ProviderResource) (LookedupSubnets, error) {
	// Get all availability zones in provided region
	availabilityZones, err := GetAvailabilityZones(ctx, account, provider)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get availability zones in region %q", account.Region)
	}

	// Create default subnet in each availability zone
	subnets, err := util.MapErr(availabilityZones.Names, func(zone string, _ int) (LookedupSubnet, error) {
		// Try to look up the default subnet in the specified availability zone
		subnet, err := ec2V5.LookupSubnet(ctx, &ec2V5.LookupSubnetArgs{
			Filters: []ec2V5.GetSubnetFilter{
				{
					Name:   "availability-zone",
					Values: []string{zone},
				},
				{
					Name:   "default-for-az",
					Values: []string{"true"},
				},
			},
		}, sdk.Provider(provider))
		if err != nil {
			return LookedupSubnet{}, errors.Wrapf(err, "failed to lookup subnet in zone %q", zone)
		}
		return LookedupSubnet{
			id:  sdk.ID(subnet.Id).ToIDOutput(),
			arn: sdk.String(subnet.Arn).ToStringOutput(),
		}, nil
	})
	return subnets, err
}

func createDefaultSubnetsInRegionV5(ctx *sdk.Context, account aws.AccountConfig, env string, params pApi.ProvisionParams) (defaultSubnets, error) {
	// Get all availability zones in provided region
	availabilityZones, err := awsImpl.GetAvailabilityZones(ctx, &awsImpl.GetAvailabilityZonesArgs{
		Filters: []awsImpl.GetAvailabilityZonesFilter{
			{
				Name:   "region-name",
				Values: []string{account.Region},
			},
		},
	}, sdk.Provider(params.Provider))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get availability zones in region %q", account.Region)
	}

	// Create default subnet in each availability zone
	subnets, err := util.MapErr(availabilityZones.Names, func(zone string, _ int) (Subnet, error) {
		subnetName := fmt.Sprintf("default-subnet-%s-%s", env, zone)
		subnet, err := ec2V5.NewDefaultSubnet(ctx, subnetName, &ec2V5.DefaultSubnetArgs{
			AvailabilityZone: sdk.String(zone),
		}, sdk.Provider(params.Provider))
		if err != nil {
			return Subnet{}, errors.Wrapf(err, "failed to create default subnet %s in %q", subnetName, account.Region)
		}
		return Subnet{
			LookedupSubnet: LookedupSubnet{
				id:            subnet.ID(),
				arn:           subnet.Arn,
				cidrBlock:     subnet.CidrBlock,
				ipv6CidrBlock: subnet.Ipv6CidrBlock,
				az:            sdk.String(zone).ToStringOutput(),
				azName:        zone,
			},
			resource: subnet,
		}, nil
	})
	return subnets, err
}
