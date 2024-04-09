package aws

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/ec2"
	legacyEcs "github.com/pulumi/pulumi-aws/sdk/v5/go/aws/ecs"
	awsImpl "github.com/pulumi/pulumi-aws/sdk/v6/go/aws"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/ecr"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/iam"
	"github.com/pulumi/pulumi-awsx/sdk/go/awsx/ecs"
	"github.com/pulumi/pulumi-awsx/sdk/go/awsx/lb"
	"github.com/pulumi/pulumi-docker/sdk/v3/go/docker"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/samber/lo"
	"github.com/simple-container-com/api/pkg/util"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/aws"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

type EcsFargateRepository struct {
	Repository *ecr.Repository
	Password   sdk.StringOutput
}

type EcsFargateImage struct {
	Container  aws.EcsFargateContainer
	Image      *docker.Image
	Repository EcsFargateRepository
}
type EcsFargateOutput struct {
	Images               []*EcsFargateImage
	ExecRole             *iam.Role
	ExecPolicyAttachment *iam.RolePolicyAttachment
	Service              *ecs.FargateService
	LoadBalancer         *lb.ApplicationLoadBalancer
	MainDnsRecord        sdk.AnyOutput
	Cluster              *legacyEcs.Cluster
	Policy               *iam.Policy
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

func ProvisionEcsFargate(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	if input.Descriptor.Type != aws.TemplateTypeEcsFargate {
		return nil, errors.Errorf("unsupported template type %q", input.Descriptor.Type)
	}
	if input.DeployParams == nil {
		return nil, errors.Errorf("missing deploy params for %q in stack %q", input.Descriptor.Type, stack.Name)
	}
	deployParams := *input.DeployParams

	ref := &EcsFargateOutput{}
	output := &api.ResourceOutput{Ref: ref}

	crInput, ok := input.Descriptor.Config.Config.(*aws.EcsFargateInput)
	if !ok {
		return output, errors.Errorf("failed to convert ecs_fargate config for %q in stack %q in %q", input.Descriptor.Type, stack.Name, deployParams.Environment)
	}
	params.Log.Debug(ctx.Context(), "provisioning ECS Fargate for stack %q in %q: %q...", stack.Name, deployParams.Environment, crInput)

	err := buildAndPushImages(ctx, stack, params, deployParams, crInput, ref)
	if err != nil {
		return output, errors.Wrapf(err, "failed to build and push images for stack %q in %q", stack.Name, deployParams.Environment)
	}

	err = createEcsFargateCluster(ctx, stack, params, deployParams, crInput, ref)
	if err != nil {
		return output, errors.Wrapf(err, "failed to create ECS Fargate cluster for stack %q in %q", stack.Name, deployParams.Environment)
	}

	params.Log.Info(ctx.Context(), "provisioning CNAME DNS record %q for %q in %q...", crInput.Domain, stack.Name, deployParams.Environment)
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
	dependsOnOpt := sdk.DependsOn(params.ComputeContext.Dependencies())

	ecsClusterName := fmt.Sprintf("%s-%s", stack.Name, deployParams.Environment)
	// Create an ECS task execution IAM role
	roleName := fmt.Sprintf("%s-exec-role", ecsClusterName)
	taskExecRole, err := iam.NewRole(ctx, roleName, &iam.RoleArgs{
		Name: sdk.String(ecsClusterName),
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
	}, sdk.Provider(params.Provider), dependsOnOpt)
	if err != nil {
		return errors.Wrapf(err, "failed to create IAM role for stack %q in %q", stack.Name, deployParams.Environment)
	}
	ref.ExecRole = taskExecRole
	ctx.Export(fmt.Sprintf("%s-exec-role-arn", ecsClusterName), taskExecRole.Arn)

	containers := lo.MapValues(lo.GroupBy(lo.Map(
		ref.Images,
		func(image *EcsFargateImage, index int) EcsContainerDef {
			// get env variables from resources defined for stack
			image.Container.Env = lo.Assign(image.Container.Env, params.ComputeContext.EnvVariables())
			envVariables := append(ecs.TaskDefinitionKeyValuePairArray{}, lo.MapToSlice(image.Container.Env, func(key string, value string) ecs.TaskDefinitionKeyValuePairInput {
				return ecs.TaskDefinitionKeyValuePairArgs{
					Name:  sdk.StringPtr(key),
					Value: sdk.StringPtr(value),
				}
			})...)
			return EcsContainerDef{
				TaskDefinitionContainerDefinitionArgs: ecs.TaskDefinitionContainerDefinitionArgs{
					Name:        sdk.String(image.Container.Name),
					Image:       image.Image.ImageName,
					Cpu:         sdk.IntPtr(lo.If(crInput.Config.Cpu == 0, 256).Else(crInput.Config.Cpu)),
					Memory:      sdk.IntPtr(lo.If(crInput.Config.Memory == 0, 512).Else(crInput.Config.Memory)),
					Essential:   sdk.BoolPtr(true),
					Environment: envVariables,
					PortMappings: ecs.TaskDefinitionPortMappingArray{
						ecs.TaskDefinitionPortMappingArgs{
							ContainerPort: sdk.IntPtr(image.Container.Port),
							HostPort:      sdk.IntPtr(image.Container.Port),
						},
					},
				},
				Name: image.Container.Name,
			}
		}),
		func(container EcsContainerDef) string {
			return container.Name
		}),
		func(value []EcsContainerDef, key string) ecs.TaskDefinitionContainerDefinitionArgs {
			return value[0].TaskDefinitionContainerDefinitionArgs
		})

	params.Log.Info(ctx.Context(), "creating application loadbalancer for %q in %q...", stack.Name, deployParams.Environment)
	loadBalancerName := util.TrimStringMiddle(fmt.Sprintf("%s-%s-alb", stack.Name, deployParams.Environment), 30, "-")
	targetGroupName := util.TrimStringMiddle(fmt.Sprintf("%s-%s-tg", stack.Name, deployParams.Environment), 30, "-")
	loadBalancer, err := lb.NewApplicationLoadBalancer(ctx, loadBalancerName, &lb.ApplicationLoadBalancerArgs{
		Name: sdk.String(loadBalancerName),
		DefaultTargetGroup: &lb.TargetGroupArgs{
			Name: sdk.String(targetGroupName),
		},
	}, sdk.Provider(params.Provider), dependsOnOpt)
	if err != nil {
		return errors.Wrapf(err, "failed to create application loadbalancer for %q in %q", stack.Name, deployParams.Environment)
	}
	ref.LoadBalancer = loadBalancer
	ctx.Export(fmt.Sprintf("%s-alb-arn", ecsClusterName), loadBalancer.LoadBalancer.Arn())
	ctx.Export(fmt.Sprintf("%s-alb-name", ecsClusterName), loadBalancer.LoadBalancer.Name())

	iContainer := crInput.IngressContainer

	params.Log.Info(ctx.Context(), "creating ECS Fargate cluster for %q in %q with ingress container %q...",
		stack.Name, deployParams.Environment, iContainer.Name)
	cluster, err := legacyEcs.NewCluster(ctx, ecsClusterName, &legacyEcs.ClusterArgs{
		Name: sdk.String(awsResName(ecsClusterName, "cluster")),
		Configuration: legacyEcs.ClusterConfigurationArgs{
			ExecuteCommandConfiguration: legacyEcs.ClusterConfigurationExecuteCommandConfigurationArgs{
				Logging: sdk.String("DEFAULT"),
			},
		},
	}, sdk.Provider(params.Provider), dependsOnOpt)
	if err != nil {
		return errors.Wrapf(err, "failed to create ECS cluster for %q in %q", stack.Name, deployParams.Environment)
	}
	ref.Cluster = cluster
	ctx.Export(fmt.Sprintf("%s-arn", ecsClusterName), cluster.Arn)
	ctx.Export(fmt.Sprintf("%s-name", ecsClusterName), cluster.Name)

	params.Log.Info(ctx.Context(), "creating Fargate service for %q in %q with ingress container %q...",
		stack.Name, deployParams.Environment, iContainer.Name)
	service, err := ecs.NewFargateService(ctx, fmt.Sprintf("%s-service", ecsClusterName), &ecs.FargateServiceArgs{
		Cluster:      cluster.Arn,
		Name:         sdk.String(awsResName(ecsClusterName, "svc")),
		DesiredCount: sdk.Int(lo.If(crInput.Scale.Min == 0, 1).Else(crInput.Scale.Min)),
		TaskDefinitionArgs: &ecs.FargateServiceTaskDefinitionArgs{
			Family:     sdk.String(fmt.Sprintf("%s-%s", stack.Name, deployParams.Environment)),
			Cpu:        sdk.String(lo.If(crInput.Config.Cpu == 0, "256").Else(strconv.Itoa(crInput.Config.Cpu))),
			Memory:     sdk.String(lo.If(crInput.Config.Memory == 0, "512").Else(strconv.Itoa(crInput.Config.Memory))),
			Containers: containers,
		},
		ForceNewDeployment:   sdk.BoolPtr(true),
		EnableExecuteCommand: sdk.BoolPtr(true),
		Tags: sdk.StringMap{
			"deployTime": sdk.String(time.Now().Format(time.RFC3339)),
		},
		AssignPublicIp: sdk.BoolPtr(true),
		LoadBalancers: legacyEcs.ServiceLoadBalancerArray{
			legacyEcs.ServiceLoadBalancerArgs{
				ContainerName:  sdk.String(iContainer.Name),
				ContainerPort:  sdk.Int(iContainer.Port),
				TargetGroupArn: loadBalancer.DefaultTargetGroup.Arn(),
			},
		},
	}, sdk.Provider(params.Provider), dependsOnOpt)
	if err != nil {
		return errors.Wrapf(err, "failed to create ecs service for stack %q in %q", stack.Name, deployParams.Environment)
	}
	ref.Service = service
	ctx.Export(fmt.Sprintf("%s-service-name", ecsClusterName), service.Service.Name())

	ccPolicyName := fmt.Sprintf("%s-policy", ecsClusterName)
	ccPolicy, err := iam.NewPolicy(ctx, ccPolicyName, &iam.PolicyArgs{
		Description: sdk.String("Allows CreateControlChannel operation"),
		Name:        sdk.String(ccPolicyName),
		Policy: sdk.All().ApplyT(func(args []interface{}) (sdk.StringOutput, error) {
			policy := map[string]interface{}{
				"Version": "2012-10-17",
				"Statement": []map[string]any{
					{
						"Effect":   "Allow",
						"Resource": "*",
						"Action": []string{
							"ssmmessages:CreateControlChannel",
							"ssmmessages:CreateDataChannel",
							"ssmmessages:OpenControlChannel",
							"ssmmessages:OpenDataChannel",
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
	}, sdk.Provider(params.Provider), dependsOnOpt)
	if err != nil {
		return errors.Wrapf(err, "failed to create policy for stack %q in %q", stack.Name, deployParams.Environment)
	}
	ref.Policy = ccPolicy
	ctx.Export(fmt.Sprintf("%s-policy", ecsClusterName), ccPolicy.Arn)

	execPolicyAttachmentName := fmt.Sprintf("%s-p-exec", ecsClusterName)
	execPolicyAttachment, err := iam.NewRolePolicyAttachment(ctx, execPolicyAttachmentName, &iam.RolePolicyAttachmentArgs{
		Role:      taskExecRole.Name,
		PolicyArn: sdk.String("arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"),
	}, sdk.Provider(params.Provider), dependsOnOpt)
	if err != nil {
		return errors.Wrapf(err, "failed to create policy attachment stack %q in %q", stack.Name, deployParams.Environment)
	}
	ref.ExecPolicyAttachment = execPolicyAttachment
	ctx.Export(fmt.Sprintf("%s-p-exec-arn", ecsClusterName), execPolicyAttachment.PolicyArn)

	service.TaskDefinition.ApplyT(func(td *legacyEcs.TaskDefinition) any {
		return td.TaskRoleArn.ApplyT(func(taskRoleArn *string) (*iam.RolePolicyAttachment, error) {
			role := awsImpl.GetArnOutput(ctx, awsImpl.GetArnOutputArgs{
				Arn: sdk.String(lo.FromPtr(taskRoleArn)),
			}, sdk.Provider(params.Provider))
			ccPolicyAttachmentName := fmt.Sprintf("%s-p-cc", ecsClusterName)
			return iam.NewRolePolicyAttachment(ctx, ccPolicyAttachmentName, &iam.RolePolicyAttachmentArgs{
				PolicyArn: ccPolicy.Arn,
				Role: role.Resource().ApplyT(func(roleResource string) string {
					roleName := roleResource[strings.Index(roleResource, "/")+1:]
					params.Log.Info(ctx.Context(), "attaching policy %q to role %q", ccPolicyName, roleName)
					return roleName
				}),
			}, sdk.Provider(params.Provider), dependsOnOpt)
		})
	})

	return nil
}

func buildAndPushImages(ctx *sdk.Context, stack api.Stack, params pApi.ProvisionParams, deployParams api.StackParams, crInput *aws.EcsFargateInput, ref *EcsFargateOutput) error {
	images, err := util.MapErr(crInput.Containers, func(container aws.EcsFargateContainer, _ int) (*EcsFargateImage, error) {
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
				Dockerfile: sdk.String(container.Image.Dockerfile),
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
			Image:      image,
			Repository: repository,
		}, nil
	})
	if err != nil {
		return err
	}
	ref.Images = images
	for _, image := range images {
		if image != nil {
			ctx.Export(fmt.Sprintf("%s--%s--%s--image", stack.Name, deployParams.Environment, image.Container.Name), image.Image.ImageName)
		}
	}
	return nil
}

func createEcrRegistry(ctx *sdk.Context, stack api.Stack, params pApi.ProvisionParams, deployParams api.StackParams, imageName string) (EcsFargateRepository, error) {
	res := EcsFargateRepository{}
	ecrRepoName := fmt.Sprintf("%s-%s", stack.Name, imageName)
	params.Log.Info(ctx.Context(), "provisioning ECR repository %q for stack %q in %q...", ecrRepoName, stack.Name, deployParams.Environment)
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
		})
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create default subnet %s in %q", subnetName, account.Region)
		}
		return subnet, nil
	})
	return subnets, err
}
