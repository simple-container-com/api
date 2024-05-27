package aws

import (
	"fmt"

	"github.com/pkg/errors"
	awsImpl "github.com/pulumi/pulumi-aws/sdk/v5/go/aws"
	ec2V5 "github.com/pulumi/pulumi-aws/sdk/v5/go/aws/ec2"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/samber/lo"

	"github.com/simple-container-com/api/pkg/clouds/aws"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	"github.com/simple-container-com/api/pkg/util"
)

type lookedupSubnet struct {
	id  sdk.IDOutput
	arn sdk.StringOutput
}

type defaultSubnet struct {
	lookedupSubnet
	resource sdk.Resource
}

type (
	lookedupSubnets []lookedupSubnet
	defaultSubnets  []defaultSubnet
)

func (s *lookedupSubnets) Arns() sdk.StringArrayInput {
	return sdk.StringArray(lo.Map(*s, func(subnet lookedupSubnet, _ int) sdk.StringInput {
		return subnet.arn
	}))
}

func (s *defaultSubnets) Ids() sdk.StringArrayInput {
	return sdk.StringArray(lo.Map(*s, func(subnet defaultSubnet, _ int) sdk.StringInput {
		return subnet.id
	}))
}

func (s *lookedupSubnets) Ids() sdk.StringArrayInput {
	return sdk.StringArray(lo.Map(*s, func(subnet lookedupSubnet, _ int) sdk.StringInput {
		return subnet.id
	}))
}

func (s *defaultSubnets) Resources() []sdk.Resource {
	return lo.Map(*s, func(subnet defaultSubnet, _ int) sdk.Resource {
		return subnet.resource
	})
}

func lookupDefaultSubnetsInRegionV5(ctx *sdk.Context, account aws.AccountConfig, params pApi.ProvisionParams) (lookedupSubnets, error) {
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
	subnets, err := util.MapErr(availabilityZones.Names, func(zone string, _ int) (lookedupSubnet, error) {
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
		}, sdk.Provider(params.Provider))
		if err != nil {
			return lookedupSubnet{}, errors.Wrapf(err, "failed to lookup subnet in zone %q", zone)
		}
		return lookedupSubnet{
			id:  sdk.ID(subnet.Id).ToIDOutput(),
			arn: sdk.String(subnet.Arn).ToStringOutput(),
		}, nil
	})
	return subnets, err
}

func createDefaultSubnetsInRegionV5(ctx *sdk.Context, account aws.AccountConfig, params pApi.ProvisionParams) (defaultSubnets, error) {
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
	subnets, err := util.MapErr(availabilityZones.Names, func(zone string, _ int) (defaultSubnet, error) {
		subnetName := fmt.Sprintf("default-subnet-%s", zone)
		subnet, err := ec2V5.NewDefaultSubnet(ctx, subnetName, &ec2V5.DefaultSubnetArgs{
			AvailabilityZone: sdk.String(zone),
		}, sdk.Provider(params.Provider))
		if err != nil {
			return defaultSubnet{}, errors.Wrapf(err, "failed to create default subnet %s in %q", subnetName, account.Region)
		}
		return defaultSubnet{
			lookedupSubnet: lookedupSubnet{
				id:  subnet.ID(),
				arn: subnet.Arn,
			},
			resource: subnet,
		}, nil
	})
	return subnets, err
}
