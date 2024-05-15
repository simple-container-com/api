package aws

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/ec2"
	ecsV5 "github.com/pulumi/pulumi-aws/sdk/v5/go/aws/ecs"
	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/efs"
	lbV5 "github.com/pulumi/pulumi-aws/sdk/v5/go/aws/lb"
	awsImpl "github.com/pulumi/pulumi-aws/sdk/v6/go/aws"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/appautoscaling"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/cloudwatch"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/ecr"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/iam"
	"github.com/pulumi/pulumi-awsx/sdk/go/awsx/awsx"
	"github.com/pulumi/pulumi-awsx/sdk/go/awsx/ecs"
	"github.com/pulumi/pulumi-awsx/sdk/go/awsx/lb"
	"github.com/pulumi/pulumi-docker/sdk/v3/go/docker"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/samber/lo"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/aws"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	"github.com/simple-container-com/api/pkg/util"
)

type EcsFargateRepository struct {
	Repository *ecr.Repository
	Password   sdk.StringOutput
}

type EcsFargateImage struct {
	Container  aws.EcsFargateContainer
	ImageName  sdk.StringOutput
	Repository EcsFargateRepository
}

type EcsFargateOutput struct {
	Images               []*EcsFargateImage
	ExecRole             *iam.Role
	ExecPolicyAttachment *iam.RolePolicyAttachment
	Service              *ecs.FargateService
	LoadBalancer         *lb.ApplicationLoadBalancer
	MainDnsRecord        sdk.AnyOutput
	Cluster              *ecsV5.Cluster
	Policy               *iam.Policy
	Secrets              []*CreatedSecret
}

type EcsContainerEnv struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type EcsContainerPorts struct {
	ContainerPort int `json:"containerPort"`
	HostPort      int `json:"hostPort"`
}
type EcsContainerDef struct {
	Name string `json:"name"`
	ecs.TaskDefinitionContainerDefinitionArgs
}

func EcsFargate(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	if input.Descriptor.Type != aws.TemplateTypeEcsFargate {
		return nil, errors.Errorf("unsupported template type %q", input.Descriptor.Type)
	}
	if input.StackParams == nil {
		return nil, errors.Errorf("missing deploy params for %q in stack %q", input.Descriptor.Type, stack.Name)
	}
	deployParams := *input.StackParams

	ref := &EcsFargateOutput{}
	output := &api.ResourceOutput{Ref: ref}

	crInput, ok := input.Descriptor.Config.Config.(*aws.EcsFargateInput)
	if !ok {
		return output, errors.Errorf("failed to convert ecs_fargate config for %q in stack %q in %q", input.Descriptor.Type, stack.Name, deployParams.Environment)
	}
	if err := api.ConvertAuth(crInput, &crInput.AccountConfig); err != nil {
		return nil, errors.Wrapf(err, "failed to convert auth config to aws.AccountConfig")
	}

	params.Log.Debug(ctx.Context(), "configure ECS Fargate for stack %q in %q: %q...", stack.Name, deployParams.Environment, crInput)

	err := buildAndPushImages(ctx, stack, params, deployParams, crInput, ref)
	if err != nil {
		return output, errors.Wrapf(err, "failed to build and push images for stack %q in %q", stack.Name, deployParams.Environment)
	}

	err = createEcsFargateCluster(ctx, stack, params, deployParams, crInput, ref)
	if err != nil {
		return output, errors.Wrapf(err, "failed to create ECS Fargate cluster for stack %q in %q", stack.Name, deployParams.Environment)
	}

	params.Log.Info(ctx.Context(), "configure CNAME DNS record %q for %q in %q...", crInput.Domain, stack.Name, deployParams.Environment)
	mainRecord := ref.LoadBalancer.LoadBalancer.DnsName().ApplyT(func(endpoint string) (*api.ResourceOutput, error) {
		return params.Registrar.NewRecord(ctx, api.DnsRecord{
			Name:    crInput.Domain,
			Type:    "CNAME",
			Value:   endpoint,
			Proxied: true,
		})
	}).(sdk.AnyOutput)
	ref.MainDnsRecord = mainRecord
	ctx.Export(fmt.Sprintf("%s-%s-dns-record", stack.Name, deployParams.Environment), mainRecord)

	return output, nil
}

func createEcsFargateCluster(ctx *sdk.Context, stack api.Stack, params pApi.ProvisionParams, deployParams api.StackParams, crInput *aws.EcsFargateInput, ref *EcsFargateOutput) error {
	opts := []sdk.ResourceOption{
		sdk.Provider(params.Provider),
		sdk.DependsOn(params.ComputeContext.Dependencies()),
	}

	iContainer := crInput.IngressContainer

	contextEnvVariables := params.ComputeContext.EnvVariables()

	var secrets []*CreatedSecret
	ctxSecrets, err := util.MapErr(contextEnvVariables, func(v pApi.ComputeEnvVariable, _ int) (*CreatedSecret, error) {
		return createSecret(ctx, toSecretName(deployParams, v.ResourceType, v.ResourceName, v.Name, crInput.Config.Version), v.Name, v.Value, opts...)
	})
	if err != nil {
		return errors.Wrapf(err, "failed to create context secrets for stack %q in %q", stack.Name, deployParams.Environment)
	}
	secrets = append(secrets, ctxSecrets...)
	for name, value := range crInput.Secrets {
		s, err := createSecret(ctx, toSecretName(deployParams, "values", "", name, crInput.Config.Version), name, value, opts...)
		if err != nil {
			return errors.Wrapf(err, "failed to create secret")
		}
		secrets = append(secrets, s)
	}
	params.Log.Info(ctx.Context(), "configure secrets in SecretsManager for %d secrets in stack %q in %q...", len(secrets), stack.Name, deployParams.Environment)
	ref.Secrets = secrets

	ecsSimpleClusterName := fmt.Sprintf("%s-%s", stack.Name, deployParams.Environment)

	// Create a new VPC for our ECS tasks.
	params.Log.Info(ctx.Context(), "configure VPC for ECS cluster %s...", ecsSimpleClusterName)
	vpcName := fmt.Sprintf("%s-vpc", ecsSimpleClusterName)
	vpc, err := ec2.NewDefaultVpc(ctx, vpcName, nil, opts...)
	if err != nil {
		return errors.Wrapf(err, "failed to create default vpc for ECS cluster %q", ecsSimpleClusterName)
	}

	params.Log.Info(ctx.Context(), "configure security group for ECS cluster %s...", ecsSimpleClusterName)
	securityGroupName := fmt.Sprintf("%s-sg", ecsSimpleClusterName)
	securityGroup, err := ec2.NewSecurityGroup(ctx, securityGroupName, &ec2.SecurityGroupArgs{
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
			&ec2.SecurityGroupEgressArgs{
				Description:    sdk.String("Allow NFS outbound traffic"),
				Protocol:       sdk.String("tcp"),
				FromPort:       sdk.Int(2049),
				ToPort:         sdk.Int(2049),
				CidrBlocks:     sdk.StringArray{sdk.String("0.0.0.0/0")},
				Ipv6CidrBlocks: sdk.StringArray{sdk.String("::/0")},
			},
		},
	}, opts...)
	if err != nil {
		return errors.Wrapf(err, "failed to crate security group for ECS cluster %q", ecsSimpleClusterName)
	}

	// Create an ECS task execution IAM role
	roleName := fmt.Sprintf("%s-exec-role", ecsSimpleClusterName)
	taskExecRole, err := iam.NewRole(ctx, roleName, &iam.RoleArgs{
		Name: sdk.String(ecsSimpleClusterName),
		AssumeRolePolicy: sdk.String(`{
                "Version": "2012-10-17",
                "Statement": [{
                    "Action": "sts:AssumeRole",
                    "Effect": "Allow",
                    "Principal": {
                        "Service": "ecs-tasks.amazonaws.com"
                    }
                }]
            }`),
	}, opts...)
	if err != nil {
		return errors.Wrapf(err, "failed to create IAM role for stack %q in %q", stack.Name, deployParams.Environment)
	}
	ref.ExecRole = taskExecRole
	ctx.Export(fmt.Sprintf("%s-exec-role-arn", ecsSimpleClusterName), taskExecRole.Arn)

	subnets, err := getOrCreateDefaultSubnetsInRegion(ctx, crInput.AccountConfig, params)
	if err != nil {
		return errors.Wrapf(err, "failed to get or create default subnets in region")
	}
	params.Log.Info(ctx.Context(), "found %d default subnets in region %s", len(subnets), crInput.AccountConfig.Region)
	var volumes ecsV5.TaskDefinitionVolumeArray
	for _, v := range crInput.Volumes {
		efsName := fmt.Sprintf("%s-%s-fs", ecsSimpleClusterName, v.Name)
		params.Log.Info(ctx.Context(), "configure efs file system %s for volume %s...", efsName, v.Name)
		fs, err := efs.NewFileSystem(ctx, efsName, &efs.FileSystemArgs{}, opts...)
		if err != nil {
			return errors.Wrapf(err, "failed to create file system for persistent volume %s of stack %s", v.Name, stack.Name)
		}
		_, err = util.MapErr(subnets, func(subnet *ec2.DefaultSubnet, i int) (*efs.MountTarget, error) {
			mountTargetName := fmt.Sprintf("%s-%s-mt-%d", ecsSimpleClusterName, v.Name, i)
			params.Log.Info(ctx.Context(), "configure mount target %s for volume %s for efs...", mountTargetName, v.Name)
			return efs.NewMountTarget(ctx, mountTargetName, &efs.MountTargetArgs{
				FileSystemId: fs.ID(),
				SubnetId:     subnet.ID(),
				SecurityGroups: sdk.StringArray{
					securityGroup.ID(),
				},
			}, opts...)
		})
		if err != nil {
			return errors.Wrapf(err, "failed to create mount targets for efs %s for volume %s of stack %s", efsName, v.Name, stack.Name)
		}
		volumes = append(volumes, ecsV5.TaskDefinitionVolumeArgs{
			Name: sdk.String(v.Name),
			EfsVolumeConfiguration: ecsV5.TaskDefinitionVolumeEfsVolumeConfigurationArgs{
				FileSystemId:      fs.ID(),
				RootDirectory:     sdk.String("/"),
				TransitEncryption: sdk.String("ENABLED"),
				AuthorizationConfig: ecsV5.TaskDefinitionVolumeEfsVolumeConfigurationAuthorizationConfigArgs{
					Iam: sdk.String("ENABLED"),
				},
			},
		})
	}
	logGroupName := fmt.Sprintf("/ecs/%s", ecsSimpleClusterName)
	containers := lo.MapValues(lo.GroupBy(lo.Map(
		ref.Images,
		func(image *EcsFargateImage, index int) EcsContainerDef {
			hostPort := image.Container.Port
			envVariables := ecs.TaskDefinitionKeyValuePairArray{}
			for k := range lo.Assign(image.Container.Env) {
				if _, found := lo.Find(secrets, func(s *CreatedSecret) bool {
					return s.EnvVar == k
				}); found {
					delete(image.Container.Env, k)
				}
			}
			envVariables = append(envVariables, lo.MapToSlice(image.Container.Env, func(key string, value string) ecs.TaskDefinitionKeyValuePairInput {
				return ecs.TaskDefinitionKeyValuePairArgs{
					Name:  sdk.StringPtr(key),
					Value: sdk.StringPtr(value),
				}
			})...)
			envVariables = append(envVariables, lo.Map(lo.Entries(params.BaseEnvVariables), func(e lo.Entry[string, string], index int) ecs.TaskDefinitionKeyValuePairInput {
				return ecs.TaskDefinitionKeyValuePairArgs{
					Name:  sdk.StringPtr(e.Key),
					Value: sdk.StringPtr(e.Value),
				}
			})...)
			secretsVariables := ecs.TaskDefinitionSecretArray{}
			secretsVariables = append(secretsVariables, lo.Map(secrets, func(item *CreatedSecret, _ int) ecs.TaskDefinitionSecretInput {
				return ecs.TaskDefinitionSecretArgs{
					Name:      sdk.String(item.EnvVar),
					ValueFrom: item.Secret.Arn,
				}
			})...)

			var mountPoints ecs.TaskDefinitionMountPointArray
			lo.ForEach(image.Container.MountPoints, func(v aws.EcsFargateMountPoint, _ int) {
				mountPoints = append(mountPoints, ecs.TaskDefinitionMountPointArgs{
					ContainerPath: sdk.String(v.ContainerPath),
					ReadOnly:      sdk.BoolPtr(v.ReadOnly),
					SourceVolume:  sdk.String(v.SourceVolume),
				})
			})
			cpu := lo.If(image.Container.Cpu == 0, 256).Else(image.Container.Cpu)

			var dependsOn ecs.TaskDefinitionContainerDependencyArray
			lo.ForEach(image.Container.DependsOn, func(dep aws.EcsFargateDependsOn, _ int) {
				dependsOn = append(dependsOn, ecs.TaskDefinitionContainerDependencyArgs{
					Condition:     sdk.String(dep.Condition),
					ContainerName: sdk.String(dep.Container),
				})
			})
			memory := lo.If(image.Container.Memory == 0, 512).Else(image.Container.Memory)
			cDef := EcsContainerDef{
				TaskDefinitionContainerDefinitionArgs: ecs.TaskDefinitionContainerDefinitionArgs{
					Name:        sdk.String(image.Container.Name),
					Image:       image.ImageName,
					Cpu:         sdk.IntPtr(cpu),
					Memory:      sdk.IntPtr(memory),
					Essential:   sdk.BoolPtr(true),
					Environment: envVariables,
					Secrets:     secretsVariables,
					MountPoints: mountPoints,
					DependsOn:   dependsOn,
					LogConfiguration: ecs.TaskDefinitionLogConfigurationArgs{
						LogDriver: sdk.String("awslogs"),
						Options: sdk.StringMap{
							"awslogs-create-group":  sdk.String("true"),
							"awslogs-group":         sdk.String(logGroupName),
							"awslogs-region":        sdk.String(crInput.Config.Region),
							"awslogs-stream-prefix": sdk.String("ecs"),
						},
						// `secretOptions` is omitted since it's null in the provided configuration.
					},
					PortMappings: ecs.TaskDefinitionPortMappingArray{
						ecs.TaskDefinitionPortMappingArgs{
							ContainerPort: sdk.IntPtr(image.Container.Port),
							HostPort:      sdk.IntPtr(hostPort),
						},
					},
				},
				Name: image.Container.Name,
			}
			if image.Container.LivenessProbe.HttpGet.Port != 0 {
				// TODO
			} else if len(image.Container.LivenessProbe.Command) > 0 {
				cDef.HealthCheck = ecs.TaskDefinitionHealthCheckArgs{
					Command: sdk.ToStringArray(image.Container.LivenessProbe.Command),
				}
			}
			return cDef
		}),
		func(container EcsContainerDef) string {
			return container.Name
		}),
		func(value []EcsContainerDef, key string) ecs.TaskDefinitionContainerDefinitionArgs {
			return value[0].TaskDefinitionContainerDefinitionArgs
		})

	params.Log.Info(ctx.Context(), "configure application loadbalancer for %q in %q...", stack.Name, deployParams.Environment)
	loadBalancerName := util.TrimStringMiddle(fmt.Sprintf("%s-%s-alb%s", stack.Name, deployParams.Environment, crInput.Config.Version), 30, "-")
	targetGroupName := util.TrimStringMiddle(fmt.Sprintf("%s-%s-tg%s", stack.Name, deployParams.Environment, crInput.Config.Version), 30, "-")

	var lbHC *lbV5.TargetGroupHealthCheckArgs
	if iContainer.LivenessProbe.HttpGet.Path != "" || iContainer.LivenessProbe.HttpGet.SuccessCodes != "" {
		lbHC = &lbV5.TargetGroupHealthCheckArgs{
			Port:    sdk.StringPtr(strconv.Itoa(iContainer.Port)),
			Path:    sdk.StringPtr(iContainer.LivenessProbe.HttpGet.Path),
			Matcher: sdk.StringPtr(iContainer.LivenessProbe.HttpGet.SuccessCodes),
		}
	}
	loadBalancer, err := lb.NewApplicationLoadBalancer(ctx, loadBalancerName, &lb.ApplicationLoadBalancerArgs{
		Name: sdk.String(loadBalancerName),
		DefaultTargetGroup: &lb.TargetGroupArgs{
			Name:        sdk.String(targetGroupName),
			HealthCheck: lbHC,
		},
	}, opts...)
	if err != nil {
		return errors.Wrapf(err, "failed to create application loadbalancer for %q in %q", stack.Name, deployParams.Environment)
	}
	ref.LoadBalancer = loadBalancer
	ctx.Export(fmt.Sprintf("%s-alb-arn", ecsSimpleClusterName), loadBalancer.LoadBalancer.Arn())
	ctx.Export(fmt.Sprintf("%s-alb-name", ecsSimpleClusterName), loadBalancer.LoadBalancer.Name())

	params.Log.Info(ctx.Context(), "configure ECS Fargate cluster for %q in %q with ingress container %q...",
		stack.Name, deployParams.Environment, iContainer.Name)
	ecsClusterName := awsResName(ecsSimpleClusterName, "cluster")
	cluster, err := ecsV5.NewCluster(ctx, ecsSimpleClusterName, &ecsV5.ClusterArgs{
		Name: sdk.String(ecsClusterName),
		Configuration: ecsV5.ClusterConfigurationArgs{
			ExecuteCommandConfiguration: ecsV5.ClusterConfigurationExecuteCommandConfigurationArgs{
				Logging: sdk.String("DEFAULT"),
			},
		},
	}, opts...)
	if err != nil {
		return errors.Wrapf(err, "failed to create ECS cluster for %q in %q", stack.Name, deployParams.Environment)
	}
	ref.Cluster = cluster
	ctx.Export(fmt.Sprintf("%s-arn", ecsSimpleClusterName), cluster.Arn)
	ctx.Export(fmt.Sprintf("%s-name", ecsSimpleClusterName), cluster.Name)

	ccPolicyName := fmt.Sprintf("%s-policy", ecsSimpleClusterName)
	ccPolicy, err := iam.NewPolicy(ctx, ccPolicyName, &iam.PolicyArgs{
		Description: sdk.String("Allows CreateControlChannel operation and reading secrets"),
		Name:        sdk.String(ccPolicyName),
		Policy: sdk.All().ApplyT(func(args []interface{}) (sdk.StringOutput, error) {
			policy := map[string]interface{}{
				"Version": "2012-10-17",
				"Statement": []map[string]any{
					{
						"Effect":   "Allow",
						"Resource": "*",
						"Action": []string{
							"ssm:StartSession",
							"ssmmessages:CreateControlChannel",
							"ssmmessages:CreateDataChannel",
							"ssmmessages:OpenControlChannel",
							"ssmmessages:OpenDataChannel",
							"secretsmanager:GetSecretValue",
							"ecr:GetAuthorizationToken",
							"ecr:DescribeImages",
							"ecr:DescribeRepositories",
							"ecr:BatchGetImage",
							"ecr:BatchCheckLayerAvailability",
							"ecr:GetDownloadUrlForLayer",
							"logs:CreateLogStream",
							"logs:CreateLogGroup",
							"logs:DescribeLogStreams",
							"logs:PutLogEvents",
							"elasticfilesystem:ClientMount",
							"elasticfilesystem:ClientWrite",
							"elasticfilesystem:DescribeMountTargets",
							"elasticfilesystem:DescribeFileSystems",
						},
					},
				},
			}
			policyJSON, err := json.Marshal(policy)
			if err != nil {
				return sdk.StringOutput{}, err
			}
			return sdk.String(policyJSON).ToStringOutput(), nil
		}).(sdk.StringOutput),
	}, opts...)
	if err != nil {
		return errors.Wrapf(err, "failed to create policy for stack %q in %q", stack.Name, deployParams.Environment)
	}
	ref.Policy = ccPolicy
	ctx.Export(fmt.Sprintf("%s-policy", ecsSimpleClusterName), ccPolicy.Arn)

	params.Log.Info(ctx.Context(), "configure Fargate service for %q in %q with ingress container %q...",
		stack.Name, deployParams.Environment, iContainer.Name)
	ecsServiceName := awsResName(ecsSimpleClusterName, "svc")
	service, err := ecs.NewFargateService(ctx, fmt.Sprintf("%s-service", ecsSimpleClusterName), &ecs.FargateServiceArgs{
		Cluster:                         cluster.Arn,
		Name:                            sdk.String(ecsServiceName),
		DesiredCount:                    sdk.Int(lo.If(crInput.Scale.Min == 0, 1).Else(crInput.Scale.Min)),
		DeploymentMaximumPercent:        sdk.IntPtr(lo.If(crInput.Scale.Update.MaxPercent == 0, 200).Else(crInput.Scale.Update.MaxPercent)),
		DeploymentMinimumHealthyPercent: sdk.IntPtr(lo.If(crInput.Scale.Update.MinHealthyPercent == 0, 100).Else(crInput.Scale.Update.MinHealthyPercent)),
		ContinueBeforeSteadyState:       sdk.BoolPtr(false),
		TaskDefinitionArgs: &ecs.FargateServiceTaskDefinitionArgs{
			Family:     sdk.String(fmt.Sprintf("%s-%s", stack.Name, deployParams.Environment)),
			Cpu:        sdk.String(lo.If(crInput.Config.Cpu == 0, "256").Else(strconv.Itoa(crInput.Config.Cpu))),
			Memory:     sdk.String(lo.If(crInput.Config.Memory == 0, "512").Else(strconv.Itoa(crInput.Config.Memory))),
			Containers: containers,
			Volumes:    volumes,
			ExecutionRole: &awsx.DefaultRoleWithPolicyArgs{
				RoleArn: taskExecRole.Arn,
			},
			TaskRole: &awsx.DefaultRoleWithPolicyArgs{
				RoleArn: taskExecRole.Arn,
			},
		},
		ForceNewDeployment:   sdk.BoolPtr(true),
		EnableExecuteCommand: sdk.BoolPtr(true),
		Tags: sdk.StringMap{
			"deployTime": sdk.String(time.Now().Format(time.RFC3339)),
		},
		NetworkConfiguration: ecsV5.ServiceNetworkConfigurationArgs{
			AssignPublicIp: sdk.BoolPtr(true),
			SecurityGroups: sdk.StringArray{
				securityGroup.ID(),
			},
			Subnets: sdk.StringArray(lo.Map(subnets, func(subnet *ec2.DefaultSubnet, _ int) sdk.StringInput {
				return subnet.ID()
			})),
		},
		LoadBalancers: ecsV5.ServiceLoadBalancerArray{
			ecsV5.ServiceLoadBalancerArgs{
				ContainerName:  sdk.String(iContainer.Name),
				ContainerPort:  sdk.Int(iContainer.Port),
				TargetGroupArn: loadBalancer.DefaultTargetGroup.Arn(),
			},
		},
	}, opts...)
	if err != nil {
		return errors.Wrapf(err, "failed to create ecs service for stack %q in %q", stack.Name, deployParams.Environment)
	}
	ref.Service = service
	ctx.Export(fmt.Sprintf("%s-service-name", ecsSimpleClusterName), service.Service.Name())

	execPolicyAttachmentName := fmt.Sprintf("%s-p-exec", ecsSimpleClusterName)
	execPolicyAttachment, err := iam.NewRolePolicyAttachment(ctx, execPolicyAttachmentName, &iam.RolePolicyAttachmentArgs{
		Role:      taskExecRole.Name,
		PolicyArn: ccPolicy.Arn,
	}, opts...)
	if err != nil {
		return errors.Wrapf(err, "failed to create policy attachment stack %q in %q", stack.Name, deployParams.Environment)
	}
	ref.ExecPolicyAttachment = execPolicyAttachment
	ctx.Export(fmt.Sprintf("%s-p-exec-arn", ecsSimpleClusterName), execPolicyAttachment.PolicyArn)

	service.TaskDefinition.ApplyT(func(td *ecsV5.TaskDefinition) any {
		return td.TaskRoleArn.ApplyT(func(taskRoleArn *string) (*iam.RolePolicyAttachment, error) {
			role := awsImpl.GetArnOutput(ctx, awsImpl.GetArnOutputArgs{
				Arn: sdk.String(lo.FromPtr(taskRoleArn)),
			}, sdk.Provider(params.Provider))
			ccPolicyAttachmentName := fmt.Sprintf("%s-p-cc", ecsSimpleClusterName)
			return iam.NewRolePolicyAttachment(ctx, ccPolicyAttachmentName, &iam.RolePolicyAttachmentArgs{
				PolicyArn: ccPolicy.Arn,
				Role: role.Resource().ApplyT(func(roleResource string) string {
					roleName := roleResource[strings.Index(roleResource, "/")+1:]
					params.Log.Info(ctx.Context(), "attaching policy %q to role %q", ccPolicyName, roleName)
					return roleName
				}),
			}, opts...)
		})
	})

	params.Log.Info(ctx.Context(), "configure Cloudwatch dashboard for ecs cluster %q...", ecsClusterName)
	if err := createEcsCloudwatchDashboard(ctx, ecsCloudwatchDashboardCfg{
		ecsClusterName: ecsClusterName,
		ecsServiceName: ecsServiceName,
		logGroupName:   logGroupName,
		stackName:      stack.Name,
		region:         crInput.Region,
	}, params); err != nil {
		return errors.Wrapf(err, "failed to create ECS cw dashboard")
	}

	if crInput.Scale.Policy != nil {
		err = attachAutoScalingPolicy(ctx, stack, params, crInput, cluster, service)
		if err != nil {
			return errors.Wrapf(err, "failed to attach auto scaling policy to service %q/%q", ecsSimpleClusterName, fmt.Sprintf("%s-service", ecsSimpleClusterName))
		}
	}

	if crInput.Alerts != nil {
		cluster.Name.ApplyT(func(clusterName string) any {
			return createEcsAlerts(ctx, clusterName, ecsServiceName, stack, crInput, deployParams, params, opts...)
		})
	}
	return nil
}

func createEcsAlerts(ctx *sdk.Context, clusterName, serviceName string, stack api.Stack, crInput *aws.EcsFargateInput, deployParams api.StackParams, params pApi.ProvisionParams, opts ...sdk.ResourceOption) error {
	alerts := crInput.Alerts

	helpersImage, err := pushHelpersImageToECR(ctx, helperCfg{
		imageName:       "sc-cloud-helpers",
		opts:            opts,
		provisionParams: params,
		stack:           stack,
		deployParams:    deployParams,
	})
	if err != nil {
		return errors.Wrapf(err, "failed to push cloud-helpers image")
	}

	if alerts.MaxCPU != nil {
		if err := createAlert(ctx, alertCfg{
			name:           alerts.MaxCPU.AlertName,
			description:    alerts.MaxCPU.Description,
			telegramConfig: alerts.Telegram,
			discordConfig:  alerts.Discord,
			deployParams:   deployParams,
			helpersImage:   helpersImage,
			secretSuffix:   crInput.Config.Version,
			opts:           opts,
			metricAlarmArgs: cloudwatch.MetricAlarmArgs{
				ComparisonOperator: sdk.String("GreaterThanThreshold"),
				EvaluationPeriods:  sdk.Int(1),
				MetricName:         sdk.String("CPUUtilization"),
				Namespace:          sdk.String("AWS/ECS"),
				Threshold:          sdk.Float64(alerts.MaxCPU.Threshold),
				Period:             sdk.Int(lo.If(alerts.MaxCPU.PeriodSec == 0, 60).Else(alerts.MaxCPU.PeriodSec)),
				Statistic:          sdk.String("Average"),
				Dimensions: sdk.StringMap{
					"ClusterName": sdk.String(clusterName),
					"ServiceName": sdk.String(serviceName),
				},
				AlarmDescription: sdk.String(alerts.MaxCPU.Description),
				TreatMissingData: sdk.String("missing"),
			},
		}); err != nil {
			return errors.Wrapf(err, "failed to create max CPU alert")
		}
	}
	if alerts.MaxMemory != nil {
		if err := createAlert(ctx, alertCfg{
			name:           alerts.MaxMemory.AlertName,
			description:    alerts.MaxMemory.Description,
			telegramConfig: alerts.Telegram,
			discordConfig:  alerts.Discord,
			deployParams:   deployParams,
			secretSuffix:   crInput.Config.Version,
			helpersImage:   helpersImage,
			opts:           opts,
			metricAlarmArgs: cloudwatch.MetricAlarmArgs{
				ComparisonOperator: sdk.String("GreaterThanThreshold"),
				EvaluationPeriods:  sdk.Int(1),
				MetricName:         sdk.String("MemoryUtilization"),
				Namespace:          sdk.String("AWS/ECS"),
				Threshold:          sdk.Float64(alerts.MaxMemory.Threshold),
				Period:             sdk.Int(lo.If(alerts.MaxMemory.PeriodSec == 0, 60).Else(alerts.MaxMemory.PeriodSec)),
				Statistic:          sdk.String("Average"),
				Dimensions: sdk.StringMap{
					"ClusterName": sdk.String(clusterName),
					"ServiceName": sdk.String(serviceName),
				},
				AlarmDescription: sdk.String(alerts.MaxMemory.Description),
				TreatMissingData: sdk.String("missing"),
			},
		}); err != nil {
			return errors.Wrapf(err, "failed to create max memory alert")
		}
	}
	return nil
}

func buildAndPushImages(ctx *sdk.Context, stack api.Stack, params pApi.ProvisionParams, deployParams api.StackParams, crInput *aws.EcsFargateInput, ref *EcsFargateOutput) error {
	images, err := util.MapErr(crInput.Containers, func(container aws.EcsFargateContainer, _ int) (*EcsFargateImage, error) {
		dockerfile := container.Image.Dockerfile
		if dockerfile == "" && container.Image.Context == "" {
			// do not build and return right away
			return &EcsFargateImage{
				Container: container,
				ImageName: sdk.String(container.Image.Name).ToStringOutput(),
			}, nil
		}
		if !filepath.IsAbs(dockerfile) {
			dockerfile = filepath.Join(crInput.ComposeDir, dockerfile)
		}

		imageName := fmt.Sprintf("%s/%s", stack.Name, container.Name)
		version := "latest" // TODO: support versioning
		repository, err := createEcrRegistry(ctx, stack, params, deployParams, container.Name)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create ecr repository")
		}

		imageFullUrl := repository.Repository.RepositoryUrl.ApplyT(func(repoUri string) string {
			return fmt.Sprintf("%s:%s", repoUri, version)
		}).(sdk.StringOutput)
		params.Log.Info(ctx.Context(), "building and pushing docker image %q (from %q) for service %q in stack %q env %q",
			imageName, container.Image.Context, container.Name, stack.Name, deployParams.Environment)
		image, err := docker.NewImage(ctx, imageName, &docker.ImageArgs{
			Build: &docker.DockerBuildArgs{
				Context:    sdk.String(container.Image.Context),
				Dockerfile: sdk.String(dockerfile),
				Args: map[string]sdk.StringInput{
					"VERSION": sdk.String(version),
				},
			},
			ImageName: imageFullUrl,
			Registry: docker.ImageRegistryArgs{
				Server:   repository.Repository.RepositoryUrl,
				Username: sdk.String("AWS"), // Use 'AWS' for ECR registry authentication
				Password: repository.Password,
			},
		}, sdk.DependsOn(params.ComputeContext.Dependencies()))
		if err != nil {
			return nil, errors.Wrapf(err, "failed to build and push image for container %q in stack %q env %q", container.Name, stack.Name, deployParams.Environment)
		}
		return &EcsFargateImage{
			Container:  container,
			ImageName:  image.ImageName,
			Repository: repository,
		}, nil
	})
	if err != nil {
		return err
	}
	ref.Images = images
	for _, image := range images {
		if image != nil {
			ctx.Export(fmt.Sprintf("%s--%s--%s--image", stack.Name, deployParams.Environment, image.Container.Name), image.ImageName)
		}
	}
	return nil
}

func attachAutoScalingPolicy(ctx *sdk.Context, stack api.Stack, params pApi.ProvisionParams, crInput *aws.EcsFargateInput, cluster *ecsV5.Cluster, service *ecs.FargateService) error {
	scalePolicyName := fmt.Sprintf("%s-ecs-scale", stack.Name)

	// Register the ECS service as a scalable target
	scalableTarget, err := appautoscaling.NewTarget(ctx, scalePolicyName, &appautoscaling.TargetArgs{
		MaxCapacity: sdk.Int(crInput.Scale.Max),
		MinCapacity: sdk.Int(crInput.Scale.Min),
		ResourceId: sdk.All(cluster.Name, service.Service.Name()).ApplyT(func(args []any) (string, error) {
			clusterName := args[0].(string)
			svcName := args[1].(string)
			return fmt.Sprintf("service/%s/%s", clusterName, svcName), nil
		}).(sdk.StringOutput),
		ScalableDimension: sdk.String("ecs:service:DesiredCount"),
		ServiceNamespace:  sdk.String("ecs"),
	}, sdk.Provider(params.Provider))
	if err != nil {
		return errors.Wrapf(err, "failed to create autoscaling target for ecs service in %q", stack.Name)
	}
	ctx.Export(fmt.Sprintf("%s-ecs-autoscale-target-arn", stack.Name), scalableTarget.Arn)
	if crInput.Scale.Policy.Type == aws.ScaleCpu {
		// Create an autoscaling policy for the target based on CPU utilization
		policy, err := appautoscaling.NewPolicy(ctx, scalePolicyName, &appautoscaling.PolicyArgs{
			PolicyType:        sdk.String("TargetTrackingScaling"),
			ResourceId:        scalableTarget.ResourceId,
			ScalableDimension: scalableTarget.ScalableDimension,
			ServiceNamespace:  scalableTarget.ServiceNamespace,
			TargetTrackingScalingPolicyConfiguration: appautoscaling.PolicyTargetTrackingScalingPolicyConfigurationArgs{
				TargetValue:      sdk.Float64(lo.If(crInput.Scale.Policy.TargetValue != 0, float32(crInput.Scale.Policy.TargetValue)).Else(70.0)),
				ScaleInCooldown:  sdk.Int(lo.If(crInput.Scale.Policy.ScaleInCooldown != 0, crInput.Scale.Policy.ScaleInCooldown).Else(60)),
				ScaleOutCooldown: sdk.Int(lo.If(crInput.Scale.Policy.ScaleOutCooldown != 0, crInput.Scale.Policy.ScaleOutCooldown).Else(60)),
				PredefinedMetricSpecification: appautoscaling.PolicyTargetTrackingScalingPolicyConfigurationPredefinedMetricSpecificationArgs{
					PredefinedMetricType: sdk.String("ECSServiceAverageCPUUtilization"),
				},
			},
		}, sdk.Provider(params.Provider))
		if err != nil {
			return errors.Wrapf(err, "failed to create autoscaling policy for ecs service in %q", stack.Name)
		}
		ctx.Export(fmt.Sprintf("%s-ecs-autoscale-policy-arn", stack.Name), policy.Arn)
	}

	return nil
}

func createEcrRegistry(ctx *sdk.Context, stack api.Stack, params pApi.ProvisionParams, deployParams api.StackParams, imageName string) (EcsFargateRepository, error) {
	res := EcsFargateRepository{}
	ecrRepoName := fmt.Sprintf("%s-%s", stack.Name, imageName)
	params.Log.Info(ctx.Context(), "configure ECR repository %q for stack %q in %q...", ecrRepoName, stack.Name, deployParams.Environment)
	ecrRepo, err := ecr.NewRepository(ctx, ecrRepoName, &ecr.RepositoryArgs{
		ForceDelete: sdk.BoolPtr(true),
		Name:        sdk.String(awsResName(ecrRepoName, "ecr")),
	}, sdk.Provider(params.Provider), sdk.DependsOn(params.ComputeContext.Dependencies()))
	if err != nil {
		return res, errors.Wrapf(err, "failed to provision ECR repository %q for stack %q in %q", ecrRepoName, stack.Name, deployParams.Environment)
	}
	res.Repository = ecrRepo
	ctx.Export(fmt.Sprintf("%s-url", ecrRepoName), ecrRepo.RepositoryUrl)
	ctx.Export(fmt.Sprintf("%s-id", ecrRepoName), ecrRepo.RegistryId)

	lifecyclePolicyDocument, err := json.Marshal(map[string][]map[string]interface{}{
		"rules": {
			{
				"rulePriority": 1,
				"description":  "Keep only 3 last images",
				"selection": map[string]interface{}{
					"tagStatus":   "any",
					"countType":   "imageCountMoreThan",
					"countNumber": 3,
				},
				"action": map[string]interface{}{
					"type": "expire",
				},
			},
		},
	})
	if err != nil {
		return res, errors.Wrapf(err, "failed to marshal ecr lifecycle policy for ecr registry %s", ecrRepoName)
	}

	// Apply the lifecycle policy to the ECR repository.
	_, err = ecr.NewLifecyclePolicy(ctx, fmt.Sprintf("%s-lc-policy", ecrRepoName), &ecr.LifecyclePolicyArgs{
		Repository: ecrRepo.Name,
		Policy:     sdk.String(lifecyclePolicyDocument),
	}, sdk.Provider(params.Provider), sdk.DependsOn(params.ComputeContext.Dependencies()))
	if err != nil {
		return res, errors.Wrapf(err, "failed to create ecr lifecycle policy for ecr registry %s", ecrRepoName)
	}

	registryPassword := ecrRepo.RegistryId.ApplyT(func(registryId string) (string, error) {
		// Fetch the auth token for the registry
		creds, err := ecr.GetCredentials(ctx, &ecr.GetCredentialsArgs{
			RegistryId: registryId,
		}, sdk.Provider(params.Provider))
		if err != nil {
			return "", err
		}

		decodedCreds, err := base64.StdEncoding.DecodeString(creds.AuthorizationToken)
		if err != nil {
			return "", errors.Wrapf(err, "failed to decode auth token for ECR registry %q", ecrRepoName)
		}
		return strings.TrimPrefix(string(decodedCreds), "AWS:"), nil
	}).(sdk.StringOutput)

	res.Password = registryPassword
	ctx.Export(fmt.Sprintf("%s-password", ecrRepoName), sdk.ToSecret(registryPassword))

	return res, nil
}

func awsResName(name string, suffix string) string {
	return strings.ReplaceAll(fmt.Sprintf("%s-%s", name, suffix), "--", "_")
}

func getOrCreateDefaultSubnetsInRegion(ctx *sdk.Context, account aws.AccountConfig, params pApi.ProvisionParams) ([]*ec2.DefaultSubnet, error) {
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
	subnets, err := util.MapErr(availabilityZones.Names, func(zone string, _ int) (*ec2.DefaultSubnet, error) {
		subnetName := fmt.Sprintf("default-subnet-%s", zone)
		subnet, err := ec2.NewDefaultSubnet(ctx, subnetName, &ec2.DefaultSubnetArgs{
			AvailabilityZone: sdk.String(zone),
		}, sdk.Provider(params.Provider))
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create default subnet %s in %q", subnetName, account.Region)
		}
		return subnet, nil
	})
	return subnets, err
}
