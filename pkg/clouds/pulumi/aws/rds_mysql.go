package aws

import (
	"fmt"

	"github.com/pkg/errors"
	"github.com/samber/lo"

	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/ec2"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/rds"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/aws"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

type RdsMysqlOutput struct{}

func RdsMysql(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	if input.Descriptor.Type != aws.ResourceTypeRdsMysql {
		return nil, errors.Errorf("unsupported resource type %q", input.Descriptor.Type)
	}

	mysqlCfg, ok := input.Descriptor.Config.Config.(*aws.MysqlConfig)
	if !ok {
		return nil, errors.Errorf("failed to convert mysql config for %q", input.Descriptor.Type)
	}

	accountConfig := &aws.AccountConfig{}
	err := api.ConvertAuth(&mysqlCfg.AccountConfig, accountConfig)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to convert aws account config")
	}
	mysqlCfg.AccountConfig = *accountConfig

	opts := []sdk.ResourceOption{
		sdk.Provider(params.Provider),
	}

	subnets, err := createDefaultSubnetsInRegionV5(ctx, mysqlCfg.AccountConfig, input.StackParams.Environment, params)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get or create default subnets in region")
	}
	opts = append(opts, sdk.DependsOn(subnets.Resources()))

	params.Log.Info(ctx.Context(), "found %d default subnets in region %s", len(subnets), mysqlCfg.AccountConfig.Region)

	dbConfig := mysqlCfg
	mysqlResName := lo.If(dbConfig.Name == "", input.Descriptor.Name).Else(dbConfig.Name)
	mysqlName := toRdsMysqlName(mysqlResName, input.StackParams.Environment)
	params.Log.Info(ctx.Context(), "configure mysql RDS cluster %q for %q in %q",
		mysqlName, input.StackParams.StackName, input.StackParams.Environment)

	params.Log.Info(ctx.Context(), "configure VPC for rds mysql cluster %s...", mysqlName)
	vpcName := fmt.Sprintf("%s-vpc", mysqlName)
	vpc, err := ec2.NewDefaultVpc(ctx, vpcName, nil, opts...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create default vpc for rds mysql cluster %q", mysqlName)
	}

	securityGroupName := fmt.Sprintf("%s-mysql-sg", mysqlName)
	params.Log.Info(ctx.Context(), "configure security group for rds mysql cluster %s...", securityGroupName)
	sgIngressArgs := ec2.SecurityGroupIngressArgs{
		Protocol:       sdk.String("tcp"),
		FromPort:       sdk.Int(3306),
		ToPort:         sdk.Int(3306),
		CidrBlocks:     sdk.StringArray{sdk.String("0.0.0.0/0")},
		Ipv6CidrBlocks: sdk.StringArray{sdk.String("::/0")},
	}
	sgIngressArgs, err = processIngressSGArgs(&sgIngressArgs, aws.SecurityGroup{}, subnets)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to apply security group configuration for mysql rds cluster %q", mysqlName)
	}

	rdsSg, err := ec2.NewSecurityGroup(ctx, securityGroupName, &ec2.SecurityGroupArgs{
		Name:  sdk.String(securityGroupName),
		VpcId: vpc.ID(),
		Ingress: &ec2.SecurityGroupIngressArray{
			&sgIngressArgs,
		},
	}, opts...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create security group for mysql cluster %q", mysqlName)
	}

	// Create a Subnet Group for RDS.
	subnetGroupName := fmt.Sprintf("%s-mysql-subnet-group", mysqlName)
	params.Log.Info(ctx.Context(), "configure subnet group for rds mysql cluster %s...", subnetGroupName)
	subnetGroup, err := rds.NewSubnetGroup(ctx, subnetGroupName, &rds.SubnetGroupArgs{
		Name:      sdk.String(subnetGroupName),
		SubnetIds: subnets.Ids(),
	}, opts...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create subnet group for rds postgres cluster")
	}

	// Create an RDS Postgres instance.
	params.Log.Info(ctx.Context(), "configure rds mysql instance %s...", mysqlName)
	var dbName sdk.StringPtrInput
	if dbConfig.DatabaseName != nil {
		dbName = sdk.StringPtr(*dbConfig.DatabaseName)
	}
	mysqlInstance, err := rds.NewInstance(ctx, mysqlName, &rds.InstanceArgs{
		DbName:            dbName,
		InstanceClass:     sdk.String(lo.If(dbConfig.InstanceClass != "", dbConfig.InstanceClass).Else("db.t3.micro")),
		AllocatedStorage:  sdk.Int(lo.If(dbConfig.AllocateStorage != nil, lo.FromPtr(dbConfig.AllocateStorage)).Else(20)),
		Engine:            sdk.String(lo.If(mysqlCfg.EngineName != nil, lo.FromPtr(mysqlCfg.EngineName)).Else("mysql")),
		EngineVersion:     sdk.String(lo.If(dbConfig.EngineVersion != "", dbConfig.EngineVersion).Else("8.0")),
		DbSubnetGroupName: subnetGroup.Name,
		VpcSecurityGroupIds: sdk.StringArray{
			rdsSg.ID(),
		},
		Username:          sdk.String(lo.If(dbConfig.Username != "", dbConfig.Username).Else("root")),
		Password:          sdk.String(lo.If(dbConfig.Password != "", dbConfig.Password).Else("root")),
		SkipFinalSnapshot: sdk.Bool(true),
	}, opts...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create rds mysql instance")
	}

	ctx.Export(toMysqlInstanceArnExport(mysqlName), mysqlInstance.Arn)
	ctx.Export(toMysqlInstanceEndpointExport(mysqlName), mysqlInstance.Endpoint)
	ctx.Export(toMysqlInstanceUsernameExport(mysqlName), mysqlInstance.Username)
	ctx.Export(toMysqlInstancePasswordExport(mysqlName), sdk.ToSecret(mysqlInstance.Password))

	return &api.ResourceOutput{Ref: nil}, nil
}

func toRdsMysqlName(name string, env string) string {
	return fmt.Sprintf("%s-%s", name, env)
}

func toMysqlInstanceArnExport(postgresName string) string {
	return fmt.Sprintf("%s-arn", postgresName)
}

func toMysqlInstanceUsernameExport(postgresName string) string {
	return fmt.Sprintf("%s-username", postgresName)
}

func toMysqlInstancePasswordExport(postgresName string) string {
	return fmt.Sprintf("%s-password", postgresName)
}

func toMysqlInstanceEndpointExport(postgresName string) string {
	return fmt.Sprintf("%s-endpoint", postgresName)
}
