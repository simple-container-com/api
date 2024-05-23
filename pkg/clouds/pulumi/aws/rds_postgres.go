package aws

import (
	"fmt"

	"github.com/pkg/errors"
	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/ec2"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/rds"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/samber/lo"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/aws"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

type RdsPostgresOutput struct{}

func RdsPostgres(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	if input.Descriptor.Type != aws.ResourceTypeRdsPostgres {
		return nil, errors.Errorf("unsupported resource type %q", input.Descriptor.Type)
	}

	postgresCfg, ok := input.Descriptor.Config.Config.(*aws.PostgresConfig)
	if !ok {
		return nil, errors.Errorf("failed to convert postgres config for %q", input.Descriptor.Type)
	}

	accountConfig := &aws.AccountConfig{}
	err := api.ConvertAuth(&postgresCfg.AccountConfig, accountConfig)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to convert aws account config")
	}
	postgresCfg.AccountConfig = *accountConfig

	opts := []sdk.ResourceOption{
		sdk.Provider(params.Provider),
	}

	subnets, err := getOrCreateDefaultSubnetsInRegion(ctx, postgresCfg.AccountConfig, params)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get or create default subnets in region")
	}
	params.Log.Info(ctx.Context(), "found %d default subnets in region %s", len(subnets), postgresCfg.AccountConfig.Region)

	postgresResName := lo.If(postgresCfg.Name == "", input.Descriptor.Name).Else(postgresCfg.Name)
	postgresName := toRdsPostgresName(postgresResName, input.StackParams.Environment)
	params.Log.Info(ctx.Context(), "configure postgres RDS cluster %q for %q in %q",
		postgresName, input.StackParams.StackName, input.StackParams.Environment)

	// Create a new VPC for our ECS tasks.
	params.Log.Info(ctx.Context(), "configure VPC for rds postgres cluster %s...", postgresName)
	vpcName := fmt.Sprintf("%s-vpc", postgresName)
	vpc, err := ec2.NewDefaultVpc(ctx, vpcName, nil, opts...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create default vpc for rds postgres cluster %q", postgresName)
	}

	// Create a Security Group for RDS to allow PostgreSQL traffic.
	securityGroupName := fmt.Sprintf("%s-sg", postgresName)
	params.Log.Info(ctx.Context(), "configure security group for rds postgres cluster %s...", securityGroupName)
	rdsSg, err := ec2.NewSecurityGroup(ctx, securityGroupName, &ec2.SecurityGroupArgs{
		Name:  sdk.String(securityGroupName),
		VpcId: vpc.ID(),
		Ingress: &ec2.SecurityGroupIngressArray{
			&ec2.SecurityGroupIngressArgs{
				Protocol:       sdk.String("tcp"),
				FromPort:       sdk.Int(5432),
				ToPort:         sdk.Int(5432),
				CidrBlocks:     sdk.StringArray{sdk.String("0.0.0.0/0")},
				Ipv6CidrBlocks: sdk.StringArray{sdk.String("::/0")},
			},
		},
	}, opts...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create security group for postgres cluster %q", postgresName)
	}

	// Create a Subnet Group for RDS.
	subnetGroupName := fmt.Sprintf("%s-subnet-group", postgresName)
	params.Log.Info(ctx.Context(), "configure subnet group for rds postgres cluster %s...", subnetGroupName)
	subnetGroup, err := rds.NewSubnetGroup(ctx, subnetGroupName, &rds.SubnetGroupArgs{
		Name: sdk.String(subnetGroupName),
		SubnetIds: sdk.StringArray(lo.Map(subnets, func(subnet *ec2.DefaultSubnet, _ int) sdk.StringInput {
			return subnet.ID()
		})),
	}, opts...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create subnet group for rds postgres cluster")
	}

	// Create an RDS Postgres instance.
	params.Log.Info(ctx.Context(), "configure rds postgres instance %s...", postgresName)
	var dbName sdk.StringPtrInput
	if postgresCfg.DatabaseName != nil {
		dbName = sdk.StringPtr(*postgresCfg.DatabaseName)
	}
	postgresInstance, err := rds.NewInstance(ctx, postgresName, &rds.InstanceArgs{
		DbName:            dbName,
		InstanceClass:     sdk.String(lo.If(postgresCfg.InstanceClass != "", postgresCfg.InstanceClass).Else("db.t3.micro")),
		AllocatedStorage:  sdk.Int(lo.If(postgresCfg.AllocateStorage != nil, lo.FromPtr(postgresCfg.AllocateStorage)).Else(20)),
		Engine:            sdk.String("postgres"),
		EngineVersion:     sdk.String(lo.If(postgresCfg.EngineVersion != "", postgresCfg.EngineVersion).Else("16")),
		DbSubnetGroupName: subnetGroup.Name,
		VpcSecurityGroupIds: sdk.StringArray{
			rdsSg.ID(),
		},
		Username:          sdk.String(lo.If(postgresCfg.Username != "", postgresCfg.Username).Else("postgres")),
		Password:          sdk.String(lo.If(postgresCfg.Password != "", postgresCfg.Password).Else("postgres")),
		SkipFinalSnapshot: sdk.Bool(true),
	}, opts...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create rds postgres instance")
	}

	ctx.Export(toPostgresInstanceArnExport(postgresName), postgresInstance.Arn)
	ctx.Export(toPostgresInstanceEndpointExport(postgresName), postgresInstance.Endpoint)

	return &api.ResourceOutput{Ref: nil}, nil
}

func toRdsPostgresName(name string, env string) string {
	return fmt.Sprintf("%s-%s", name, env)
}

func toPostgresInstanceArnExport(postgresName string) string {
	return fmt.Sprintf("%s-arn", postgresName)
}

func toPostgresInstanceEndpointExport(postgresName string) string {
	return fmt.Sprintf("%s-endpoint", postgresName)
}
