package mongodb

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/pkg/errors"
	"github.com/samber/lo"

	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/ec2"
	"github.com/pulumi/pulumi-mongodbatlas/sdk/v3/go/mongodbatlas"
	"github.com/pulumi/pulumi-random/sdk/v4/go/random"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/aws"
	"github.com/simple-container-com/api/pkg/clouds/mongodb"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	pAws "github.com/simple-container-com/api/pkg/clouds/pulumi/aws"
	"github.com/simple-container-com/api/pkg/util"
)

type ClusterOutput struct {
	DbUsers                    sdk.Output
	Cluster                    *mongodbatlas.Cluster
	Project                    *mongodbatlas.Project
	PrivateLinkEndpointService *mongodbatlas.PrivateLinkEndpointService
}

func Cluster(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	if input.Descriptor.Type != mongodb.ResourceTypeMongodbAtlas {
		return nil, errors.Errorf("unsupported mongodb-atlas type %q", input.Descriptor.Type)
	}

	out := &ClusterOutput{}

	atlasCfg, ok := input.Descriptor.Config.Config.(*mongodb.AtlasConfig)
	if !ok {
		return nil, errors.Errorf("failed to convert mongodb atlas config for %q", input.Descriptor.Type)
	}

	projectName := toProjectName(stack.Name, input)
	clusterName := toClusterName(stack.Name, input)

	var projectId sdk.StringOutput
	opts := []sdk.ResourceOption{
		sdk.Provider(params.Provider),
	}
	if atlasCfg.ProjectId == "" {
		projName := lo.If(atlasCfg.ProjectName != "", atlasCfg.ProjectName).Else(projectName)

		params.Log.Info(ctx.Context(), "configure MongoDB Atlas project %q for stack %q in %q", projName, input.StackParams.StackName, input.StackParams.Environment)
		project, err := mongodbatlas.NewProject(ctx, projectName, &mongodbatlas.ProjectArgs{
			Name:  sdk.String(projName),
			OrgId: sdk.String(atlasCfg.OrgId),
		}, opts...)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create mongodb project for stack %q", stack.Name)
		}
		out.Project = project
		projectId = project.ID().ToStringOutput()
		opts = append(opts, sdk.DependsOn([]sdk.Resource{project}))
	} else {
		projectRes, err := mongodbatlas.LookupProject(ctx, &mongodbatlas.LookupProjectArgs{
			ProjectId: &atlasCfg.ProjectId,
		}, sdk.Provider(params.Provider))
		if err != nil {
			return nil, errors.Wrapf(err, "failed to lookup mongodb project for stack %q", stack.Name)
		}
		projectId = sdk.String(*projectRes.ProjectId).ToStringOutput()
	}
	ctx.Export(toProjectIdExport(projectName), projectId)

	sharedInstanceSizes := []string{"M0", "M2", "M5"}
	_, isSharedInstanceSize := lo.Find(sharedInstanceSizes, func(size string) bool {
		return size == atlasCfg.InstanceSize
	})

	params.Log.Info(ctx.Context(), "configure MongoDB Atlas cluster %q for stack %q in %q", clusterName, input.StackParams.StackName, input.StackParams.Environment)
	cluster, err := mongodbatlas.NewCluster(ctx, fmt.Sprintf("%s-cluster", clusterName), &mongodbatlas.ClusterArgs{
		Name:                     sdk.StringPtr(clusterName),
		ProviderRegionName:       sdk.StringPtr(atlasCfg.Region),
		ProviderInstanceSizeName: sdk.String(atlasCfg.InstanceSize),
		ClusterType:              sdk.StringPtr("REPLICASET"),
		ProjectId:                projectId,
		BackingProviderName:      sdk.StringPtrFromPtr(lo.If(isSharedInstanceSize, &atlasCfg.CloudProvider).Else(nil)),
		ProviderName:             sdk.String(lo.If(isSharedInstanceSize, "TENANT").Else(atlasCfg.CloudProvider)),
		CloudBackup:              sdk.BoolPtr(lo.If(atlasCfg.Backup != nil, true).Else(false)),
	}, opts...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create mongodb cluster for stack %q", stack.Name)
	}
	out.Cluster = cluster

	if atlasCfg.Backup != nil {
		// Configure the backup schedule
		backupArgs := &mongodbatlas.CloudBackupScheduleArgs{
			ProjectId:       projectId,
			ClusterName:     cluster.Name,
			UpdateSnapshots: sdk.Bool(true),
		}
		every, err := time.ParseDuration(atlasCfg.Backup.Every)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to parse schedule %q", atlasCfg.Backup.Every)
		}
		retention, err := time.ParseDuration(atlasCfg.Backup.Retention)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to parse retention %q", atlasCfg.Backup.Retention)
		}
		if every.Hours() < 24 {
			backupArgs.PolicyItemHourly = &mongodbatlas.CloudBackupSchedulePolicyItemHourlyArgs{
				FrequencyInterval: sdk.Int(every.Hours()),
				RetentionUnit:     sdk.String("days"),
				RetentionValue:    sdk.Int(retention.Hours() / 24),
			}
		} else if every.Hours()/24/7 > 0 {
			backupArgs.PolicyItemWeeklies = mongodbatlas.CloudBackupSchedulePolicyItemWeeklyArray{
				&mongodbatlas.CloudBackupSchedulePolicyItemWeeklyArgs{
					FrequencyInterval: sdk.Int(every.Hours() / 24 / 7),
					RetentionUnit:     sdk.String("days"),
					RetentionValue:    sdk.Int(retention.Hours() / 24),
				},
			}
		} else if every.Hours() > 24 {
			backupArgs.PolicyItemDaily = &mongodbatlas.CloudBackupSchedulePolicyItemDailyArgs{
				FrequencyInterval: sdk.Int(every.Hours() / 24),
				RetentionUnit:     sdk.String("days"),
				RetentionValue:    sdk.Int(retention.Hours() / 24),
			}
		}
		_, err = mongodbatlas.NewCloudBackupSchedule(ctx, fmt.Sprintf("%s-backups-schedule", clusterName), backupArgs, opts...)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create mongodb backups schedule for stack %q", stack.Name)
		}
	}

	networkConfig := atlasCfg.NetworkConfig
	if networkConfig != nil {
		if networkConfig.PrivateLinkEndpoint != nil {
			privateLink := lo.FromPtr(networkConfig.PrivateLinkEndpoint)

			depProvider, depFound := params.DependencyProviders[privateLink.ProviderName]
			if !depFound {
				return nil, errors.Errorf("%q provider is not configured in Atlas configuration's extraProviders", privateLink.ProviderName)
			}

			var awsAccountConfig *aws.AccountConfig
			if awsAccount, ok := depProvider.Config.Config.(*aws.AccountConfig); !ok {
				return nil, errors.Errorf("failed to convert dep provider config to *aws.AccountConfig for private vpc endpoint for cluster %q in stack %q in %q",
					clusterName, input.StackParams.StackName, input.StackParams.Environment)
			} else {
				err := api.ConvertAuth(awsAccount, &awsAccount)
				if err != nil {
					return nil, errors.Wrapf(err, "failed to convert aws account config")
				}
				awsAccountConfig = awsAccount
			}

			providerType := awsAccountConfig.ProviderType()
			if providerType != aws.ProviderType {
				return nil, errors.Errorf("unsupported provider type %q for private vpc endpoint for cluster %q in stack %q in %q",
					providerType, clusterName, input.StackParams.StackName, input.StackParams.Environment)
			}

			vpcEndpointName := toPrivateVpcExport(clusterName)
			linkEndpointName := toPrivateLinkEndpointExport(clusterName)
			linkEndpointServiceName := toPrivateLinkEndpointServiceExport(clusterName)

			params.Log.Info(ctx.Context(), "configure MongoDB Atlas private link endpoint for cluster %q in stack %q in %q",
				clusterName, input.StackParams.StackName, input.StackParams.Environment)
			linkEndpoint, err := mongodbatlas.NewPrivateLinkEndpoint(ctx, linkEndpointName, &mongodbatlas.PrivateLinkEndpointArgs{
				ProjectId:    projectId,
				ProviderName: sdk.String(privateLink.ProviderName),
				Region:       sdk.String(awsAccountConfig.Region),
			}, opts...)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to create private link endpoint for MongoDB Atlas cluster %q", clusterName)
			}
			ctx.Export(linkEndpointName, linkEndpoint.PrivateEndpoints)
			privateLinkId := linkEndpoint.PrivateLinkId

			params.Log.Info(ctx.Context(), "configure aws private endpoint for MongoDB cluster %q in stack %q in %q",
				clusterName, input.StackParams.StackName, input.StackParams.Environment)
			vpcEndpoint, err := createAwsVpcEndpoint(ctx, vpcEndpointInput{
				clusterName:      clusterName,
				vpcEndpointName:  vpcEndpointName,
				endpointService:  linkEndpoint.EndpointServiceName,
				input:            input,
				awsAccountConfig: awsAccountConfig,
				provider:         depProvider.Provider,
				params:           params,
				cluster:          cluster,
			})
			if err != nil {
				return nil, errors.Wrapf(err, "failed to create AWS VPC Endpoint for MongoDB cluster %q", clusterName)
			}
			ctx.Export(vpcEndpointName, vpcEndpoint.DnsEntries)

			params.Log.Info(ctx.Context(), "configure MongoDB Atlas private link endpoint service for cluster %q in stack %q in %q",
				clusterName, input.StackParams.StackName, input.StackParams.Environment)
			linkEndpointService, err := mongodbatlas.NewPrivateLinkEndpointService(ctx, linkEndpointServiceName, &mongodbatlas.PrivateLinkEndpointServiceArgs{
				EndpointServiceId: vpcEndpoint.ID(),
				PrivateLinkId:     privateLinkId,
				ProjectId:         projectId,
				ProviderName:      sdk.String(privateLink.ProviderName),
			}, opts...)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to create private link endpoint service for MongoDB Atlas cluster %q", clusterName)
			}
			out.PrivateLinkEndpointService = linkEndpointService
			ctx.Export(linkEndpointServiceName, linkEndpointService.ToPrivateLinkEndpointServiceOutput())

		} else {
			return nil, errors.Errorf("network configuration for MongoDB Atlas cluster %q is provided but not supported", clusterName)
		}
	}

	// if public access was requested or network config wasn't provided
	if networkConfig == nil || lo.FromPtr(networkConfig.AllowAllIps) {
		params.Log.Info(ctx.Context(), "configure MongoDB Atlas ip access list for cluster %q in stack %q in %q",
			clusterName, input.StackParams.StackName, input.StackParams.Environment)
		_, err := mongodbatlas.NewProjectIpAccessList(ctx, fmt.Sprintf("%s-ip-access-list", clusterName), &mongodbatlas.ProjectIpAccessListArgs{
			CidrBlock: sdk.StringPtr("0.0.0.0/0"),
			Comment:   sdk.StringPtr("Allow all access to the cluster (TODO: restrict to our cluster only)"),
			ProjectId: projectId,
		}, opts...)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create mongodb ip access list for stack %q", stack.Name)
		}
	} else if networkConfig != nil && len(lo.FromPtr(networkConfig.AllowCidrs)) > 0 {
		for _, cidr := range lo.FromPtr(networkConfig.AllowCidrs) {
			params.Log.Info(ctx.Context(), "configure MongoDB Atlas access cidr %q for cluster %q in stack %q in %q",
				cidr, clusterName, input.StackParams.StackName, input.StackParams.Environment)
			_, err := mongodbatlas.NewProjectIpAccessList(ctx, fmt.Sprintf("%s-ip-access-cidr-%q", clusterName, cidr), &mongodbatlas.ProjectIpAccessListArgs{
				CidrBlock: sdk.StringPtr(cidr),
				Comment:   sdk.StringPtr(fmt.Sprintf("Allow access to the cluster from %q", cidr)),
				ProjectId: projectId,
			}, opts...)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to create mongodb cidr access from %q for cluster %q", cidr, clusterName)
			}
		}
	}

	ctx.Export(toClusterIdExport(clusterName), cluster.ClusterId)

	ctx.Export(toMongoUriExport(clusterName), cluster.MongoUri)
	if networkConfig != nil && networkConfig.PrivateLinkEndpoint != nil {
		params.Log.Info(ctx.Context(), "Looking up for private endpoint connection string for MongoDB cluster %q for stack %q in %q",
			clusterName, input.StackParams.StackName, input.StackParams.Environment)

		ctx.Export(toMongoUriWithOptionsExport(clusterName), cluster.ConnectionStrings.ApplyT(func(connStrings []mongodbatlas.ClusterConnectionString) (string, error) {
			for _, cs := range connStrings {
				if len(cs.PrivateEndpoints) == 1 {
					// TODO: support more private endpoints
					return lo.FromPtr(cs.PrivateEndpoints[0].SrvConnectionString), nil
				}
			}
			return "", errors.Errorf("failed to detect private network connection string for MongoDB cluster %q", clusterName)
		}))
	} else {
		ctx.Export(toMongoUriWithOptionsExport(clusterName), cluster.MongoUriWithOptions)
	}

	usersOutput := sdk.All(projectId).ApplyT(func(args []any) any {
		return createDatabaseUsers(ctx, cluster, atlasCfg, params)
	})
	ctx.Export(fmt.Sprintf("%s-users", projectName), usersOutput)

	out.DbUsers = usersOutput

	return &api.ResourceOutput{Ref: out}, nil
}

func toPrivateLinkEndpointServiceExport(clusterName string) string {
	return fmt.Sprintf("%s-private-link-endpoint-svc", clusterName)
}

func toPrivateLinkEndpointExport(clusterName string) string {
	return fmt.Sprintf("%s-private-link-endpoint", clusterName)
}

func toPrivateVpcExport(clusterName string) string {
	return fmt.Sprintf("%s-vpc-endpoint", clusterName)
}

func toMongoUriExport(clusterName string) string {
	return fmt.Sprintf("%s-mongo-uri", clusterName)
}

func toMongoUriWithOptionsExport(clusterName string) string {
	return fmt.Sprintf("%s-mongo-uri-options", clusterName)
}

func toClusterIdExport(clusterName string) string {
	return fmt.Sprintf("%s-cluster-id", clusterName)
}

func toProjectIdExport(projectName string) string {
	return fmt.Sprintf("%s-id", projectName)
}

func toProjectName(stackName string, input api.ResourceInput) string {
	return input.ToResName(fmt.Sprintf("%s--%s", stackName, input.Descriptor.Name))
}

func toClusterName(stackName string, input api.ResourceInput) string {
	projectName := toProjectName(stackName, input)
	return util.TrimStringMiddle(projectName, 21, "--") //  Atlas truncates cluster names to 23 characters
}

type dbRole struct {
	dbName string
	role   string
}

type dbUserInput struct {
	projectId   string
	clusterName string
	dbUri       string
	userName    string
	roles       []dbRole
	dependency  sdk.Resource
}

type DbUserOutput struct {
	UserName string `json:"userName" yaml:"userName"`
	Password string `json:"password" yaml:"password"`
	DbUri    string `json:"dbUri" yaml:"dbUri"`
}

func (o DbUserOutput) ToJson() string {
	res, _ := json.Marshal(o)
	return string(res)
}

type vpcEndpointInput struct {
	clusterName      string
	endpointService  sdk.StringInput
	input            api.ResourceInput
	awsAccountConfig *aws.AccountConfig
	provider         sdk.ProviderResource
	params           pApi.ProvisionParams
	cluster          *mongodbatlas.Cluster
	vpcEndpointName  string
}

func createAwsVpcEndpoint(ctx *sdk.Context, opts vpcEndpointInput) (*ec2.VpcEndpoint, error) {
	clusterName, input, params := opts.clusterName, opts.input, opts.params
	subnets, err := pAws.LookupSubnetsInAccount(ctx, *opts.awsAccountConfig, opts.provider)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get or create default subnets in region for MongoDB cluster %q in stack %q in %q",
			clusterName, input.StackParams.StackName, input.StackParams.Environment)
	}
	vpcName := fmt.Sprintf("%s-vpc", clusterName)
	params.Log.Info(ctx.Context(), "getting default aws VPC for MongoDB cluster %q", clusterName)
	awsOpts := []sdk.ResourceOption{
		sdk.Provider(opts.provider),
		sdk.DependsOn([]sdk.Resource{opts.cluster}),
	}
	vpc, err := pAws.NewVpcInAccount(ctx, vpcName, awsOpts...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create default aws vpc for MongoDB cluster %q", clusterName)
	}

	// Create Security Group for the Endpoint
	sg, err := ec2.NewSecurityGroup(ctx, fmt.Sprintf("%s-aws-endpoint-sg", clusterName), &ec2.SecurityGroupArgs{
		VpcId: vpc.ID(),
		Ingress: ec2.SecurityGroupIngressArray{
			&ec2.SecurityGroupIngressArgs{
				Description:    sdk.String("Allow ALL inbound traffic"),
				Protocol:       sdk.String("tcp"),
				FromPort:       sdk.Int(0),
				ToPort:         sdk.Int(65535),
				CidrBlocks:     sdk.StringArray{sdk.String("0.0.0.0/0")},
				Ipv6CidrBlocks: sdk.StringArray{sdk.String("::/0")},
			},
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
	}, awsOpts...)
	if err != nil {
		return nil, err
	}

	// Create the VPC Endpoint
	vpcEndpoint, err := ec2.NewVpcEndpoint(ctx, opts.vpcEndpointName, &ec2.VpcEndpointArgs{
		VpcId:            vpc.ID(),
		ServiceName:      opts.endpointService,
		VpcEndpointType:  sdk.String("Interface"),
		SubnetIds:        subnets.Ids(),
		SecurityGroupIds: sdk.StringArray{sg.ID()},
	}, awsOpts...)
	if err != nil {
		return nil, err
	}
	return vpcEndpoint, nil
}

func createDatabaseUser(ctx *sdk.Context, user dbUserInput, params pApi.ProvisionParams) (any, error) {
	// Generate a random password for the MongoDB Atlas database user.
	passwordName := fmt.Sprintf("%s-%s-password", user.projectId, user.userName)
	password, err := random.NewRandomPassword(ctx, passwordName, &random.RandomPasswordArgs{
		Length:  sdk.Int(20),
		Special: sdk.Bool(false),
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to generate random password for mongodb for user %q", user.userName)
	}
	ctx.Export(passwordName, password.Result)

	userObjectName := fmt.Sprintf("%s-%s-user", user.clusterName, user.userName)
	roles := mongodbatlas.DatabaseUserRoleArray{}

	for _, role := range user.roles {
		roles = append(roles, mongodbatlas.DatabaseUserRoleArgs{
			RoleName:     sdk.String(role.role),
			DatabaseName: sdk.String(role.dbName),
		})
	}
	opts := []sdk.ResourceOption{
		sdk.Provider(params.Provider),
	}
	if user.dependency != nil {
		opts = append(opts, sdk.DependsOn([]sdk.Resource{user.dependency}))
	}
	dbUser, err := mongodbatlas.NewDatabaseUser(ctx, userObjectName, &mongodbatlas.DatabaseUserArgs{
		AuthDatabaseName: sdk.String("admin"),
		Password:         password.Result,
		ProjectId:        sdk.String(user.projectId),
		Roles:            roles,
		Username:         sdk.String(user.userName),
	}, opts...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create database user %q", user.userName)
	}
	return sdk.All(dbUser.Username, dbUser.Password).ApplyT(func(args []any) (any, error) {
		username := args[0].(string)
		password := args[1].(*string)
		return DbUserOutput{
			UserName: username,
			Password: *password,
			DbUri:    user.dbUri,
		}.ToJson(), nil
	}), nil
}

func createDatabaseUsers(ctx *sdk.Context, cluster *mongodbatlas.Cluster, cfg *mongodb.AtlasConfig, params pApi.ProvisionParams) any {
	return sdk.All(cluster.Name, cluster.ProjectId, cluster.MongoUriWithOptions, cluster.ConnectionStrings).ApplyT(func(args []any) (any, error) {
		res := make(map[string]any)
		clusterName := args[0].(string)
		projectId := args[1].(string)
		mongoUri := args[2].(string)

		for _, usr := range cfg.Admins {
			dbUser, err := createDatabaseUser(ctx, dbUserInput{
				dependency:  cluster,
				clusterName: clusterName,
				projectId:   projectId,
				dbUri:       mongoUri,
				userName:    usr,
				roles: []dbRole{
					{dbName: "admin", role: "readWriteAnyDatabase"},
					{dbName: "local", role: "read"},
				},
			}, params)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to create mongodb user %q", usr)
			}
			res[usr] = dbUser
		}
		for _, usr := range cfg.Developers {
			dbUser, err := createDatabaseUser(ctx, dbUserInput{
				dependency:  cluster,
				clusterName: clusterName,
				projectId:   projectId,
				dbUri:       mongoUri,
				userName:    usr,
				roles: []dbRole{
					{dbName: "admin", role: "readAnyDatabase"},
					{dbName: "local", role: "read"},
				},
			}, params)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to create mongodb user %q", usr)
			}
			res[usr] = dbUser
		}
		return sdk.ToMapOutput(lo.MapValues(res, func(value any, key string) sdk.Output {
			return value.(sdk.Output)
		})), nil
	})
}
