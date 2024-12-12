package aws

import (
	"fmt"

	"github.com/pkg/errors"
	"github.com/samber/lo"

	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/ec2"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/clouds/aws"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	"github.com/simple-container-com/api/pkg/util"
)

type StaticEgressIPOut struct {
	SubnetID        sdk.IDOutput
	SecurityGroupID sdk.IDOutput
	SecurityGroup   *ec2.SecurityGroup
	Subnet          *ec2.Subnet
}

type zonedSubnets map[string]*ec2.Subnet

func (s *zonedSubnets) ToSubnets() defaultSubnets {
	return lo.Map(lo.Entries(lo.FromPtr(s)), func(e lo.Entry[string, *ec2.Subnet], _ int) Subnet {
		zoneName := e.Key
		subnet := e.Value
		return Subnet{
			LookedupSubnet: LookedupSubnet{
				id:            subnet.ID(),
				arn:           subnet.Arn,
				cidrBlock:     fromStringPtrOutputToStringOutput(subnet.CidrBlock),
				ipv6CidrBlock: fromStringPtrOutputToStringOutput(subnet.Ipv6CidrBlock),
				az:            sdk.String(zoneName).ToStringOutput(),
				azName:        zoneName,
			},
			resource: subnet,
		}
	})
}

type MultiStaticEgressIPOut struct {
	VPC              *ec2.Vpc
	SecurityGroupIDs []sdk.IDOutput
	SecurityGroups   []*ec2.SecurityGroup
	Subnets          zonedSubnets
}

type StaticEgressIPIn struct {
	Params        pApi.ProvisionParams
	Provider      sdk.ProviderResource
	AccountConfig aws.AccountConfig
	SecurityGroup *aws.SecurityGroup
}

func provisionStaticEgressForMultiZoneVpc(ctx *sdk.Context, resName string, input *StaticEgressIPIn, opts ...sdk.ResourceOption) (*MultiStaticEgressIPOut, error) {
	params := input.Params

	params.Log.Info(ctx.Context(), "configure public subnet for %s...", resName)

	zones, err := GetAvailabilityZones(ctx, input.AccountConfig, input.Provider)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get AZs for %q", resName)
	}
	if len(zones.Names) == 0 {
		return nil, errors.Errorf("AZs list is empty for %q", resName)
	}
	vpcName := fmt.Sprintf("%s-vpc", resName)

	// Create a VPC
	params.Log.Info(ctx.Context(), "configure VPC for %s...", resName)
	vpc, err := ec2.NewVpc(ctx, vpcName, &ec2.VpcArgs{
		CidrBlock: sdk.String("172.31.0.0/16"),
	}, opts...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create vpc for %q", resName)
	}

	res := MultiStaticEgressIPOut{
		VPC:     vpc,
		Subnets: make(map[string]*ec2.Subnet),
	}

	type publicGateway struct {
		zoneName   string
		igw        *ec2.InternetGateway
		natGw      *ec2.NatGateway
		routeTable *ec2.RouteTable
		subnet     *ec2.Subnet
	}

	natGatewaysList, err := util.MapErr(zones.Names, func(zoneName string, index int) (*publicGateway, error) {
		pubSubnetName := fmt.Sprintf("%s-public-subnet-%s", resName, zoneName)
		cidrBlock := fmt.Sprintf("172.31.%d.0/24", index)
		publicSubnet, err := ec2.NewSubnet(ctx, pubSubnetName, &ec2.SubnetArgs{
			VpcId:            vpc.ID(),
			CidrBlock:        sdk.String(cidrBlock),
			AvailabilityZone: sdk.StringPtr(zoneName),
		}, opts...)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to provision public subnet for %q (zone %s)", resName, zoneName)
		}

		// Create an Elastic IP for the NAT Gateway
		params.Log.Info(ctx.Context(), "configure elastic IP address for %s (az %s)...", resName, zoneName)
		eipName := fmt.Sprintf("%s-eip-%s", resName, zoneName)
		eip, err := ec2.NewEip(ctx, eipName, nil, opts...)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to provision elastic IP for %q (az %s)", resName, zoneName)
		}

		// Create a NAT Gateway in the public subnet
		params.Log.Info(ctx.Context(), "configure NAT gateway for public subnet of %s (zone %s)...", resName, zoneName)
		natGwName := fmt.Sprintf("%s-nat-gateway-%s", resName, zoneName)
		natGateway, err := ec2.NewNatGateway(ctx, natGwName, &ec2.NatGatewayArgs{
			SubnetId:     publicSubnet.ID(),
			AllocationId: eip.ID(),
		}, opts...)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to provision elastic IP for %q", resName)
		}

		params.Log.Info(ctx.Context(), "configure internet gateway for %s (zone %s)...", resName, zoneName)
		igwName := fmt.Sprintf("%s-igw-%s", resName, zoneName)
		igw, err := ec2.NewInternetGateway(ctx, igwName, &ec2.InternetGatewayArgs{
			VpcId: vpc.ID(),
		}, opts...)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to provision internet gateway for %q (az %s)", resName, zoneName)
		}

		// Create a route table for the public subnet
		params.Log.Info(ctx.Context(), "configure public route table for %s...", resName)
		routeTableName := fmt.Sprintf("%s-public-route-table-%s", resName, zoneName)
		routeTable, err := ec2.NewRouteTable(ctx, routeTableName, &ec2.RouteTableArgs{
			VpcId: vpc.ID(),
			Routes: ec2.RouteTableRouteArray{
				&ec2.RouteTableRouteArgs{
					CidrBlock: sdk.String("0.0.0.0/0"),
					GatewayId: natGateway.ID(),
				},
			},
		}, opts...)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to provision route table for %q default vpc", resName)
		}

		return lo.ToPtr(publicGateway{
			zoneName:   zoneName,
			igw:        igw,
			natGw:      natGateway,
			routeTable: routeTable,
			subnet:     publicSubnet,
		}), nil
	})
	if err != nil {
		return nil, err
	}

	natGateways := lo.Associate(natGatewaysList, func(natGw *publicGateway) (string, *publicGateway) {
		return natGw.zoneName, natGw
	})

	for _, zoneName := range zones.Names {
		//privateSubnetName := fmt.Sprintf("%s-private-subnet-%s", resName, zoneName)
		//privateSubnet, err := ec2.NewSubnet(ctx, privateSubnetName, &ec2.SubnetArgs{
		//	VpcId:            vpc.ID(),
		//	CidrBlock:        sdk.String(fmt.Sprintf("172.31.%d.0/24", subnetIdx+len(zones.Names)-1)),
		//	AvailabilityZone: sdk.StringPtr(zoneName),
		//}, opts...)
		//if err != nil {
		//	return nil, errors.Wrapf(err, "failed to provision public subnet for %q (zone %s)", resName, zoneName)
		//}
		natGateway := natGateways[zoneName]

		// Create a route table for the private subnet and a default route through the NAT Gateway
		//params.Log.Info(ctx.Context(), "configure private route table for %s (zone %s)...", resName, zoneName)
		//privateRouteTableName := fmt.Sprintf("%s-private-route-table-%s", resName, zoneName)
		//privateRouteTable, err := ec2.NewRouteTable(ctx, privateRouteTableName, &ec2.RouteTableArgs{
		//	VpcId: vpc.ID(),
		//	Routes: ec2.RouteTableRouteArray{
		//		&ec2.RouteTableRouteArgs{
		//			CidrBlock:    sdk.String("0.0.0.0/0"),
		//			NatGatewayId: natGateway.natGw.ID(),
		//		},
		//	},
		//}, opts...)
		//if err != nil {
		//	return nil, errors.Wrapf(err, "failed to provision private route table for %q (zone %s)", resName, zoneName)
		//}

		// Associate the private subnet with the route table
		//params.Log.Info(ctx.Context(), "configure private route table association for %s (zone %s)...", resName, zoneName)
		//privateRTAssocName := fmt.Sprintf("%s-route-table-association-%s", resName, zoneName)
		//_, err = ec2.NewRouteTableAssociation(ctx, privateRTAssocName, &ec2.RouteTableAssociationArgs{
		//	SubnetId:     privateSubnet.ID(),
		//	RouteTableId: privateRouteTable.ID(),
		//}, opts...gin)
		//if err != nil {
		//	return nil, errors.Wrapf(err, "failed to provision private route table association for %q (zone %s)", resName, zoneName)
		//}

		params.Log.Info(ctx.Context(), "configure security group for %s...", resName)
		securityGroupName := fmt.Sprintf("%s-ipgw-sg-%s", resName, zoneName)
		securityGroup, err := ec2.NewSecurityGroup(ctx, securityGroupName, &ec2.SecurityGroupArgs{
			VpcId:   vpc.ID(),
			Ingress: ec2.SecurityGroupIngressArray{},
			Egress: ec2.SecurityGroupEgressArray{
				&ec2.SecurityGroupEgressArgs{
					Description:    sdk.String("Allow ALL outbound traffic"),
					Protocol:       sdk.String("tcp"),
					FromPort:       sdk.Int(0),
					ToPort:         sdk.Int(65535),
					CidrBlocks:     sdk.StringArray{sdk.String("0.0.0.0/0")},
					Ipv6CidrBlocks: sdk.StringArray{sdk.String("::/0")},
				},
			},
		}, opts...)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to crate security group for %q", resName)
		}

		res.SecurityGroupIDs = append(res.SecurityGroupIDs, securityGroup.ID())
		res.SecurityGroups = append(res.SecurityGroups, securityGroup)
		res.Subnets[zoneName] = natGateway.subnet
	}

	return &res, nil
}

func provisionVpcWithStaticEgress(ctx *sdk.Context, resName string, input *StaticEgressIPIn, opts ...sdk.ResourceOption) (*StaticEgressIPOut, error) {
	zones, err := GetAvailabilityZones(ctx, input.AccountConfig, input.Provider)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get AZs for %q", resName)
	}
	if len(zones.Names) == 0 {
		return nil, errors.Errorf("AZs list is empty for %q", resName)
	}

	vpcName := fmt.Sprintf("%s-vpc", resName)
	zoneName := zones.Names[0]
	vpcCidrBlock := "10.0.0.0/16"
	publicSubnetCidrBlock := "10.0.1.0/24"
	privateSubnetCidrBlock := "10.0.2.0/24"

	params := input.Params

	// Create a VPC
	params.Log.Info(ctx.Context(), "configure VPC for %s...", resName)
	vpc, err := ec2.NewVpc(ctx, vpcName, &ec2.VpcArgs{
		CidrBlock: sdk.String(vpcCidrBlock),
	}, opts...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create vpc for %q", resName)
	}

	params.Log.Info(ctx.Context(), "configure public subnet for %s...", resName)
	pubSubnetName := fmt.Sprintf("%s-public-subnet", resName)

	publicSubnet, err := ec2.NewSubnet(ctx, pubSubnetName, &ec2.SubnetArgs{
		VpcId:            vpc.ID(),
		CidrBlock:        sdk.String(publicSubnetCidrBlock),
		AvailabilityZone: sdk.String(zoneName),
	}, opts...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to provision public subnet for %q", resName)
	}

	params.Log.Info(ctx.Context(), "configure private subnet for %s...", resName)
	privSubnetName := fmt.Sprintf("%s-private-subnet", resName)
	privateSubnet, err := ec2.NewSubnet(ctx, privSubnetName, &ec2.SubnetArgs{
		VpcId:            vpc.ID(),
		CidrBlock:        sdk.String(privateSubnetCidrBlock),
		AvailabilityZone: sdk.String(zoneName),
	}, opts...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to provision private subnet for %q", resName)
	}

	params.Log.Info(ctx.Context(), "configure internet gateway for %s...", resName)
	igwName := fmt.Sprintf("%s-igw", resName)
	igw, err := ec2.NewInternetGateway(ctx, igwName, &ec2.InternetGatewayArgs{
		VpcId: vpc.ID(),
	}, opts...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to provision internet gateway for %q", resName)
	}

	// Create a route table for the public subnet
	params.Log.Info(ctx.Context(), "configure public route table for %s...", resName)
	routeTableName := fmt.Sprintf("%s-route-table", resName)
	routeTable, err := ec2.NewRouteTable(ctx, routeTableName, &ec2.RouteTableArgs{
		VpcId: vpc.ID(),
		Routes: ec2.RouteTableRouteArray{
			&ec2.RouteTableRouteArgs{
				CidrBlock: sdk.String("0.0.0.0/0"),
				GatewayId: igw.ID(),
			},
		},
	}, opts...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to provision route table for lambda's %q vpc", resName)
	}

	// Associate the public subnet with the route table
	params.Log.Info(ctx.Context(), "configure public route table association for %s...", resName)
	pubSubnetRouteAssocName := fmt.Sprintf("%s-pub-subnet-route-assoc", resName)
	_, err = ec2.NewRouteTableAssociation(ctx, pubSubnetRouteAssocName, &ec2.RouteTableAssociationArgs{
		SubnetId:     publicSubnet.ID(),
		RouteTableId: routeTable.ID(),
	}, opts...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to provision route table association for lambda's %q public subnet %q", resName, pubSubnetName)
	}

	// Create an Elastic IP for the NAT Gateway
	params.Log.Info(ctx.Context(), "configure elastic IP address for %s...", resName)
	eipName := fmt.Sprintf("%s-eip", resName)
	eip, err := ec2.NewEip(ctx, eipName, nil, opts...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to provision elastic IP for %q", resName)
	}

	// Create a NAT Gateway in the public subnet
	params.Log.Info(ctx.Context(), "configure NAT gateway for %s...", resName)
	natGwName := fmt.Sprintf("%s-nat-gateway", resName)
	natGateway, err := ec2.NewNatGateway(ctx, natGwName, &ec2.NatGatewayArgs{
		SubnetId:     publicSubnet.ID(),
		AllocationId: eip.ID(),
	}, opts...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to provision elastic IP for %q", resName)
	}

	// Create a route table for the private subnet and a default route through the NAT Gateway
	params.Log.Info(ctx.Context(), "configure private route table for %s...", resName)
	privateRouteTableName := fmt.Sprintf("%s-private-route-table", resName)
	privateRouteTable, err := ec2.NewRouteTable(ctx, privateRouteTableName, &ec2.RouteTableArgs{
		VpcId: vpc.ID(),
		Routes: ec2.RouteTableRouteArray{
			&ec2.RouteTableRouteArgs{
				CidrBlock:    sdk.String("0.0.0.0/0"),
				NatGatewayId: natGateway.ID(),
			},
		},
	}, opts...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to provision private route table for %q", resName)
	}

	// Associate the private subnet with the route table
	params.Log.Info(ctx.Context(), "configure private route table association for %s...", resName)
	privateRTAssocName := fmt.Sprintf("%s-private-route-table-association", resName)
	_, err = ec2.NewRouteTableAssociation(ctx, privateRTAssocName, &ec2.RouteTableAssociationArgs{
		SubnetId:     privateSubnet.ID(),
		RouteTableId: privateRouteTable.ID(),
	}, opts...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to provision private route table association for %q", resName)
	}

	params.Log.Info(ctx.Context(), "configure security group for %s...", resName)
	securityGroupName := fmt.Sprintf("%s-ipgw-sg", resName)
	ingressRule := ec2.SecurityGroupIngressArgs{
		Description:    sdk.String("Allow ALL inbound traffic"),
		Protocol:       sdk.String("tcp"),
		FromPort:       sdk.Int(0),
		ToPort:         sdk.Int(65535),
		CidrBlocks:     sdk.StringArray{sdk.String("0.0.0.0/0")},
		Ipv6CidrBlocks: sdk.StringArray{sdk.String("::/0")},
	}
	if input.SecurityGroup != nil {
		ingressRule, err = processIngressSGArgs(&ingressRule, *input.SecurityGroup, []Subnet{
			{
				LookedupSubnet: LookedupSubnet{
					id:            privateSubnet.ID(),
					arn:           privateSubnet.Arn,
					cidrBlock:     fromStringPtrOutputToStringOutput(privateSubnet.CidrBlock),
					ipv6CidrBlock: fromStringPtrOutputToStringOutput(privateSubnet.Ipv6CidrBlock),
				},
				resource: privateSubnet,
			},
		})
		if err != nil {
			return nil, errors.Wrapf(err, "failed to apply security group configuration from cloud extras for %q", resName)
		}
	}

	securityGroup, err := ec2.NewSecurityGroup(ctx, securityGroupName, &ec2.SecurityGroupArgs{
		VpcId: vpc.ID(),
		Ingress: ec2.SecurityGroupIngressArray{
			&ingressRule,
		},
		Egress: ec2.SecurityGroupEgressArray{
			&ec2.SecurityGroupEgressArgs{
				Description:    sdk.String("Allow ALL outbound traffic"),
				Protocol:       sdk.String("tcp"),
				FromPort:       sdk.Int(0),
				ToPort:         sdk.Int(65535),
				CidrBlocks:     sdk.StringArray{sdk.String("0.0.0.0/0")},
				Ipv6CidrBlocks: sdk.StringArray{sdk.String("::/0")},
			},
		},
	}, opts...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to crate security group for %q", resName)
	}

	return &StaticEgressIPOut{
		Subnet:          privateSubnet,
		SecurityGroup:   securityGroup,
		SubnetID:        privateSubnet.ID(),
		SecurityGroupID: securityGroup.ID(),
	}, nil
}

func fromStringPtrOutputToStringOutput(stringPtrOutput sdk.StringPtrOutput) sdk.StringOutput {
	return stringPtrOutput.ApplyT(func(v *string) (string, error) {
		if v == nil {
			return "", nil // Handle the nil case if needed
		}
		return *v, nil
	}).(sdk.StringOutput)
}
