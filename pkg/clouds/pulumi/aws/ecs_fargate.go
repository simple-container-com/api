package aws

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/pkg/errors"
	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/ec2"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/ecr"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/ecs"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/iam"
	"github.com/pulumi/pulumi-awsx/sdk/go/awsx/lb"
	"github.com/pulumi/pulumi-docker/sdk/v3/go/docker"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/samber/lo"
	"github.com/simple-container-com/api/pkg/util"
	"strconv"
	"strings"

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
	Images           []*EcsFargateImage
	ExecRole         *iam.Role
	TaskDefinition   *ecs.TaskDefinition
	PolicyAttachment *iam.RolePolicyAttachment
	Service          *ecs.Service
	Cluster          *ecs.Cluster
	LoadBalancer     *lb.ApplicationLoadBalancer
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
	Name         string              `json:"name"`
	Image        string              `json:"image"`
	Cpu          int                 `json:"cpu"`
	Memory       int                 `json:"memory"`
	Essential    bool                `json:"essential"`
	Environment  []EcsContainerEnv   `json:"environment"`
	PortMappings []EcsContainerPorts `json:"portMappings"`
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

	return output, nil
}

func createEcsFargateCluster(ctx *sdk.Context, stack api.Stack, params pApi.ProvisionParams, deployParams api.DeployParams, crInput *aws.EcsFargateInput, ref *EcsFargateOutput) error {
	ecsClusterName := awsResName(fmt.Sprintf("%s-%s", stack.Name, deployParams.Environment), "ecs")
	// Create an ECS task execution IAM role
	taskExecRole, err := iam.NewRole(ctx, fmt.Sprintf("%s-exec-role", ecsClusterName), &iam.RoleArgs{
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
	}, sdk.Provider(params.Provider))
	if err != nil {
		return errors.Wrapf(err, "failed to create IAM role for stack %q in %q", stack.Name, deployParams.Environment)
	}
	ref.ExecRole = taskExecRole
	ctx.Export(fmt.Sprintf("%s-exec-role-arn", ecsClusterName), taskExecRole.Arn)

	// Attach the task execution role policy to the IAM role
	policyAttachment, err := iam.NewRolePolicyAttachment(ctx, fmt.Sprintf("%s-policy-attachment", ecsClusterName), &iam.RolePolicyAttachmentArgs{
		Role:      taskExecRole.Name,
		PolicyArn: sdk.String("arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"),
	}, sdk.Provider(params.Provider))
	if err != nil {
		return errors.Wrapf(err, "failed to create policy attachment stack %q in %q", stack.Name, deployParams.Environment)
	}
	ref.PolicyAttachment = policyAttachment
	ctx.Export(fmt.Sprintf("%s-policy-arn", ecsClusterName), policyAttachment.PolicyArn)

	containerDefs := sdk.All(lo.Map(ref.Images, func(image *EcsFargateImage, _ int) any {
		return image.Image.ImageName
	})...).ApplyT(func(imageNames []any) (string, error) {
		containers := lo.Map(ref.Images, func(image *EcsFargateImage, i int) EcsContainerDef {
			return EcsContainerDef{
				Name:      image.Container.Name,
				Image:     imageNames[i].(string),
				Cpu:       lo.If(crInput.Config.Cpu == 0, 256).Else(crInput.Config.Cpu),
				Memory:    lo.If(crInput.Config.Memory == 0, 512).Else(crInput.Config.Memory),
				Essential: true,
				Environment: lo.MapToSlice(image.Container.Env, func(key string, value string) EcsContainerEnv {
					return EcsContainerEnv{
						Name:  key,
						Value: value,
					}
				}),
				PortMappings: []EcsContainerPorts{
					{
						ContainerPort: image.Container.Port,
						HostPort:      image.Container.Port,
					},
				},
			}
		})
		bytes, err := json.Marshal(containers)
		return string(bytes), err
	}).(sdk.StringOutput)

	// Create ECS Fargate task definition
	taskDef, err := ecs.NewTaskDefinition(ctx, fmt.Sprintf("%s-%s-task-def", ecsClusterName, deployParams.Environment), &ecs.TaskDefinitionArgs{
		RequiresCompatibilities: sdk.StringArray{
			sdk.String("FARGATE"),
		},
		Family:               sdk.String(fmt.Sprintf("%s-%s", stack.Name, deployParams.Environment)),
		NetworkMode:          sdk.String("awsvpc"),
		Cpu:                  sdk.String(lo.If(crInput.Config.Cpu == 0, "256").Else(strconv.Itoa(crInput.Config.Cpu))),
		Memory:               sdk.String(lo.If(crInput.Config.Memory == 0, "512").Else(strconv.Itoa(crInput.Config.Memory))),
		ExecutionRoleArn:     taskExecRole.Arn,
		ContainerDefinitions: containerDefs,
	}, sdk.Provider(params.Provider))
	if err != nil {
		return errors.Wrapf(err, "failed to create ecs task definition for stack %q in %q", stack.Name, deployParams.Environment)
	}
	ref.TaskDefinition = taskDef
	ctx.Export(fmt.Sprintf("%s-task-arn", ecsClusterName), taskDef.Arn)

	// Create an ECS cluster
	cluster, err := ecs.NewCluster(ctx, ecsClusterName, &ecs.ClusterArgs{
		Name: sdk.String(ecsClusterName),
	}, sdk.Provider(params.Provider))
	if err != nil {
		return errors.Wrapf(err, "failed to create ecs cluster for stack %q in %q", stack.Name, deployParams.Environment)
	}
	ref.Cluster = cluster
	ctx.Export(fmt.Sprintf("%s-cluster-arn", ecsClusterName), cluster.Arn)
	ctx.Export(fmt.Sprintf("%s-cluster-name", ecsClusterName), cluster.Name)

	params.Log.Info(ctx.Context(), "creating application loadbalancer for %q in %q...", stack.Name, deployParams.Environment)
	loadBalancerName := fmt.Sprintf("%s-%s-alb", stack.Name, deployParams.Environment)
	loadBalancer, err := lb.NewApplicationLoadBalancer(ctx, loadBalancerName, &lb.ApplicationLoadBalancerArgs{
		Name: sdk.String(loadBalancerName),
		DefaultTargetGroup: &lb.TargetGroupArgs{
			Name: sdk.Sprintf("%s-tg", strings.ReplaceAll(ecsClusterName, "_", "--")),
		},
	}, sdk.Provider(params.Provider))
	if err != nil {
		return errors.Wrapf(err, "failed to create application loadbalancer %q in %q", stack.Name, deployParams.Environment)
	}
	ref.LoadBalancer = loadBalancer
	ctx.Export(fmt.Sprintf("%s-alb-arn", ecsClusterName), loadBalancer.LoadBalancer.Arn())
	ctx.Export(fmt.Sprintf("%s-alb-name", ecsClusterName), loadBalancer.LoadBalancer.Name())

	iContainer := crInput.Containers[0] // TODO: find ingress container

	params.Log.Info(ctx.Context(), "creating ECS Fargate service for %q in %q with ingress container %q...",
		stack.Name, deployParams.Environment, iContainer.Name)
	service, err := ecs.NewService(ctx, fmt.Sprintf("%s-service", ecsClusterName), &ecs.ServiceArgs{
		Cluster:        cluster.Arn,
		DesiredCount:   sdk.Int(crInput.Scale.Min),
		LaunchType:     sdk.String("FARGATE"),
		TaskDefinition: taskDef.Arn,
		LoadBalancers: ecs.ServiceLoadBalancerArray{
			ecs.ServiceLoadBalancerArgs{
				ContainerName:  sdk.String(iContainer.Name),
				ContainerPort:  sdk.Int(iContainer.Port),
				TargetGroupArn: loadBalancer.DefaultTargetGroup.Arn(),
			},
		},
		NetworkConfiguration: &ecs.ServiceNetworkConfigurationArgs{
			AssignPublicIp: sdk.BoolPtr(true),
		},
	}, sdk.Provider(params.Provider))
	if err != nil {
		return errors.Wrapf(err, "failed to create ecs service for stack %q in %q", stack.Name, deployParams.Environment)
	}
	ref.Service = service
	ctx.Export(fmt.Sprintf("%s-service-name", ecsClusterName), service.Name)

	return nil
}

func buildAndPushImages(ctx *sdk.Context, stack api.Stack, params pApi.ProvisionParams, deployParams api.DeployParams, crInput *aws.EcsFargateInput, ref *EcsFargateOutput) error {
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
		})
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

func createEcrRegistry(ctx *sdk.Context, stack api.Stack, params pApi.ProvisionParams, deployParams api.DeployParams, imageName string) (EcsFargateRepository, error) {
	res := EcsFargateRepository{}
	ecrRepoName := awsResName(fmt.Sprintf("%s-%s", stack.Name, imageName), "ecr")
	params.Log.Info(ctx.Context(), "provisioning ECR repository %q for stack %q in %q...", ecrRepoName, stack.Name, deployParams.Environment)
	ecrRepo, err := ecr.NewRepository(ctx, ecrRepoName, &ecr.RepositoryArgs{
		ForceDelete: sdk.BoolPtr(true),
		Name:        sdk.String(ecrRepoName),
	}, sdk.Provider(params.Provider))
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

// getVpcDefaultSubnetIds searches for default subnets in the default VPC and returns their IDs
func getVpcDefaultSubnetIds(ctx *sdk.Context, params pApi.ProvisionParams) ([]string, error) {
	if vpc, err := ec2.LookupVpc(ctx, &ec2.LookupVpcArgs{Default: lo.ToPtr(true)}, sdk.Provider(params.Provider)); err != nil {
		return nil, err
	} else if ids, err := ec2.GetSubnetIds(ctx, &ec2.GetSubnetIdsArgs{VpcId: vpc.Id}, sdk.Provider(params.Provider)); err != nil {
		return nil, err
	} else {
		return ids.Ids, nil
	}
}
