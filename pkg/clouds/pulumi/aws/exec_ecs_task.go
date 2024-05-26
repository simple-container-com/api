package aws

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	awsImpl "github.com/pulumi/pulumi-aws/sdk/v6/go/aws"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/ec2"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/ecs"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/iam"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/samber/lo"

	"github.com/simple-container-com/api/pkg/clouds/aws"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

type ecsTaskConfig struct {
	name    string
	account aws.AccountConfig
	params  pApi.ProvisionParams
	image   string
	command string
	env     map[string]string
}

type EcsEnvVar struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

func execEcsTask(ctx *sdk.Context, config ecsTaskConfig) error {
	name := config.name
	accountConfig := config.account
	params := config.params

	opts := []sdk.ResourceOption{
		sdk.Provider(params.Provider),
		sdk.DependsOn(params.ComputeContext.Dependencies()),
	}

	params.Log.Info(ctx.Context(), "configure exec role for %q", name)
	execRoleName := fmt.Sprintf("%s-exec-role", name)
	taskExecRole, err := iam.NewRole(ctx, execRoleName, &iam.RoleArgs{
		Name: sdk.String(execRoleName),
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
		return errors.Wrapf(err, "failed to create IAM role for %q", name)
	}

	execPolicyName := fmt.Sprintf("%s-exec-policy", name)
	params.Log.Info(ctx.Context(), "configure exec policy attachment for %q", name)
	_, err = iam.NewRolePolicyAttachment(ctx, execPolicyName, &iam.RolePolicyAttachmentArgs{
		Role:      taskExecRole.Name,
		PolicyArn: sdk.String("arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"),
	}, opts...)
	if err != nil {
		return errors.Wrapf(err, "failed to create role policy attachment for %q", name)
	}

	envsBytes, err := json.Marshal(lo.Map(lo.Entries(config.env), func(e lo.Entry[string, string], _ int) EcsEnvVar {
		return EcsEnvVar{
			Name:  e.Key,
			Value: e.Value,
		}
	}))
	if err != nil {
		return errors.Wrapf(err, "failed to marshal env variables for %q", name)
	}

	serviceName := fmt.Sprintf("%s-service", name)
	logGroupName := fmt.Sprintf("/ecs/%s", serviceName)

	taskDefName := fmt.Sprintf("%s-task-def", name)
	params.Log.Info(ctx.Context(), "configure task definition for %q", name)
	taskDef, err := ecs.NewTaskDefinition(ctx, taskDefName, &ecs.TaskDefinitionArgs{
		Family:      sdk.String(name),
		NetworkMode: sdk.String("awsvpc"),
		RequiresCompatibilities: sdk.StringArray{
			sdk.String("FARGATE"),
		},
		Cpu:              sdk.String("256"),
		Memory:           sdk.String("512"),
		ExecutionRoleArn: taskExecRole.Arn,
		ContainerDefinitions: sdk.Sprintf(`[
				{
					"name": "%s",
					"image": "%s",
					"command": ["/bin/sh", "-c", "%s"],
					"environment": %s,
					"logConfiguration": {
					 	"logDriver": "awslogs",
					 	"options": {
							"awslogs-create-group":  "true",
					 		"awslogs-group": "%s",
							"awslogs-region": "%s",
					 		"awslogs-stream-prefix": "ecs"
                        }
					},
					"cpu": 256,
					"memory": 512
				}
			]`, config.name, config.image, config.command, string(envsBytes), logGroupName, accountConfig.Region),
	}, opts...)
	if err != nil {
		return errors.Wrapf(err, "failed to create task definition for %q", name)
	}

	ecsClusterName := fmt.Sprintf("%s-cluster", name)
	params.Log.Info(ctx.Context(), "configure ECS cluster for %q", name)
	cluster, err := ecs.NewCluster(ctx, ecsClusterName, &ecs.ClusterArgs{}, opts...)
	if err != nil {
		return err
	}

	vpcName := fmt.Sprintf("%s-vpc", name)
	params.Log.Info(ctx.Context(), "getting default VPC for %q", name)
	vpc, err := ec2.NewDefaultVpc(ctx, vpcName, nil, opts...)
	if err != nil {
		return errors.Wrapf(err, "failed to create default vpc for ECS cluster %q", ecsClusterName)
	}

	params.Log.Info(ctx.Context(), "getting default subnets for %q", name)
	subnets, err := lookupDefaultSubnetsInRegionV5(ctx, accountConfig, params)
	if err != nil {
		return errors.Wrapf(err, "failed to get or create default subnets in region")
	}

	params.Log.Info(ctx.Context(), "found %d default subnets in region %s", len(subnets), accountConfig.Region)

	securityGroupName := fmt.Sprintf("%s-sg", name)
	params.Log.Info(ctx.Context(), "configure security group for %q", name)
	securityGroup, err := ec2.NewSecurityGroup(ctx, securityGroupName, &ec2.SecurityGroupArgs{
		VpcId: vpc.ID(),
		Egress: ec2.SecurityGroupEgressArray{
			&ec2.SecurityGroupEgressArgs{
				Description:    sdk.String("Allow ALL outbound TCP traffic"),
				Protocol:       sdk.String("tcp"),
				FromPort:       sdk.Int(0),
				ToPort:         sdk.Int(65535),
				CidrBlocks:     sdk.StringArray{sdk.String("0.0.0.0/0")},
				Ipv6CidrBlocks: sdk.StringArray{sdk.String("::/0")},
			},
			&ec2.SecurityGroupEgressArgs{
				Description:    sdk.String("Allow ALL outbound UDP traffic"),
				Protocol:       sdk.String("udp"),
				FromPort:       sdk.Int(0),
				ToPort:         sdk.Int(65535),
				CidrBlocks:     sdk.StringArray{sdk.String("0.0.0.0/0")},
				Ipv6CidrBlocks: sdk.StringArray{sdk.String("::/0")},
			},
		},
	}, opts...)
	if err != nil {
		return errors.Wrapf(err, "failed to create security group for %q", name)
	}

	securityGroupNames := sdk.StringArray{
		securityGroup.ID(),
	}

	ccPolicyName := fmt.Sprintf("%s-policy", name)
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
		return errors.Wrapf(err, "failed to create policy for %q", name)
	}
	execPolicyAttachmentName := fmt.Sprintf("%s-p-exec", name)
	_, err = iam.NewRolePolicyAttachment(ctx, execPolicyAttachmentName, &iam.RolePolicyAttachmentArgs{
		Role:      taskExecRole.Name,
		PolicyArn: ccPolicy.Arn,
	}, opts...)
	if err != nil {
		return errors.Wrapf(err, "failed to create policy attachment for %q", name)
	}
	params.Log.Info(ctx.Context(), "configure ECS task execution %q", name)
	sdk.All(cluster.Name, securityGroupNames, subnets.Ids(), taskDef.Arn).ApplyT(func(in []any) error {
		clusterName := in[0].(string)
		secGroups := in[1].([]string)
		subnetIds := in[2].([]string)
		taskDefArn := in[3].(string)
		_, err = ecs.GetTaskExecution(ctx, &ecs.GetTaskExecutionArgs{
			Cluster:      clusterName,
			DesiredCount: lo.ToPtr(1),
			LaunchType:   lo.ToPtr("FARGATE"),
			NetworkConfiguration: &ecs.GetTaskExecutionNetworkConfiguration{
				AssignPublicIp: lo.ToPtr(true),
				SecurityGroups: secGroups,
				Subnets:        subnetIds,
			},
			TaskDefinition: taskDefArn,
		}, []sdk.InvokeOption{sdk.Provider(params.Provider)}...)
		if err != nil {
			return err
		}
		taskExecRole.Arn.ApplyT(func(taskRoleArn string) (*iam.RolePolicyAttachment, error) {
			role := awsImpl.GetArnOutput(ctx, awsImpl.GetArnOutputArgs{
				Arn: sdk.String(taskRoleArn),
			}, sdk.Provider(params.Provider))
			ccPolicyAttachmentName := fmt.Sprintf("%s-p-cc", name)
			return iam.NewRolePolicyAttachment(ctx, ccPolicyAttachmentName, &iam.RolePolicyAttachmentArgs{
				PolicyArn: ccPolicy.Arn,
				Role: role.Resource().ApplyT(func(roleResource string) string {
					roleName := roleResource[strings.Index(roleResource, "/")+1:]
					params.Log.Info(ctx.Context(), "attaching policy %q to role %q", ccPolicyName, roleName)
					return roleName
				}),
			}, opts...)
		})

		return nil
	})

	return nil
}
