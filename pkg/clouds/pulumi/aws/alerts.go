package aws

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/cloudwatch"
	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/iam"
	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/lambda"
	"github.com/pulumi/pulumi-docker/sdk/v3/go/docker"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/samber/lo"

	"github.com/simple-container-com/api/pkg/api"
	awsApi "github.com/simple-container-com/api/pkg/clouds/aws/helpers"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

type alertCfg struct {
	name            string
	description     string
	deployParams    api.StackParams
	discordConfig   *api.DiscordCfg
	telegramConfig  *api.TelegramCfg
	secretSuffix    string
	opts            []sdk.ResourceOption
	metricAlarmArgs cloudwatch.MetricAlarmArgs
	helpersImage    *docker.Image
}

type helperCfg struct {
	imageName       string
	opts            []sdk.ResourceOption
	provisionParams pApi.ProvisionParams
	stack           api.Stack
	deployParams    api.StackParams
}

func pushHelpersImageToECR(ctx *sdk.Context, cfg helperCfg) (*docker.Image, error) {
	ecrRepo, err := createEcrRegistry(ctx, cfg.stack, cfg.provisionParams, cfg.deployParams, "cloud-helpers")
	if err != nil {
		return nil, errors.Wrapf(err, "failed to provision ECR repository for cloud-helpers")
	}

	// Pull the existing image from Docker Hub.
	helpersImageName := fmt.Sprintf("%s-image", cfg.imageName)
	chImage, err := docker.NewRemoteImage(ctx, helpersImageName, &docker.RemoteImageArgs{
		Name: pulumi.String(cfg.provisionParams.HelpersImage),
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to pull helpers image")
	}

	cfg.provisionParams.Log.Info(ctx.Context(), "creating temporary Dockerfile for cloud-helpers...")

	// hack taken from here https://github.com/pulumi/pulumi-docker/issues/54#issuecomment-772250411
	var dockerFilePath string
	if depDir, err := os.MkdirTemp(os.TempDir(), cfg.imageName); err != nil {
		return nil, errors.Wrapf(err, "failed to create tempDir")
	} else if err = os.WriteFile(filepath.Join(depDir, "Dockerfile"), []byte("ARG SOURCE_IMAGE\n\nFROM ${SOURCE_IMAGE}"), os.ModePerm); err != nil {
		return nil, errors.Wrapf(err, "failed to write temporary Dockerfile")
	} else {
		dockerFilePath = filepath.Join(depDir, "Dockerfile")
	}

	imageFullUrl := ecrRepo.Repository.RepositoryUrl.ApplyT(func(repoUri string) string {
		cfg.provisionParams.Log.Info(ctx.Context(), "preparing push for cloud-helpers image to %q...", repoUri)
		version := lo.If(cfg.deployParams.Version == "", "latest").Else(cfg.deployParams.Version)
		return fmt.Sprintf("%s:%s", repoUri, version)
	}).(sdk.StringOutput)

	cfg.provisionParams.Log.Info(ctx.Context(), "pushing cloud-helpers image...")
	ecrImage, err := docker.NewImage(ctx, helpersImageName, &docker.ImageArgs{
		Build: &docker.DockerBuildArgs{
			Context:    sdk.String("."),
			Dockerfile: sdk.String(dockerFilePath),
			Args: map[string]sdk.StringInput{
				"SOURCE_IMAGE": chImage.Name,
			},
		},
		SkipPush:  sdk.Bool(false),
		ImageName: imageFullUrl,
		Registry: docker.ImageRegistryArgs{
			Server:   ecrRepo.Repository.RepositoryUrl,
			Username: sdk.String("AWS"), // Use 'AWS' for ECR registry authentication
			Password: ecrRepo.Password,
		},
	}, cfg.opts...)
	if err != nil {
		return nil, err
	}

	return ecrImage, nil
}

func createAlert(ctx *sdk.Context, cfg alertCfg) error {
	// Create IAM Role for Lambda Function
	lambdaExecutionRole, err := iam.NewRole(ctx, fmt.Sprintf("%s-execution-role", cfg.name), &iam.RoleArgs{
		AssumeRolePolicy: pulumi.String(`{
			"Version": "2012-10-17",
			"Statement": [{
				"Effect": "Allow",
				"Principal": {
					"Service": "lambda.amazonaws.com"
				},
				"Action": "sts:AssumeRole"
			}]
		}`),
	}, cfg.opts...)
	if err != nil {
		return errors.Wrapf(err, "failed to create iam role")
	}

	// Attach the necessary AWS managed policies to the role created
	_, err = iam.NewRolePolicyAttachment(ctx, fmt.Sprintf("%s-policy-attachment", cfg.name), &iam.RolePolicyAttachmentArgs{
		Role:      lambdaExecutionRole.Name,
		PolicyArn: pulumi.String("arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole"),
	}, cfg.opts...)
	if err != nil {
		return errors.Wrapf(err, "failed to create iam policy attachment")
	}

	// Custom policy allowing to read secrets
	extraPolicyName := fmt.Sprintf("%s-xpolicy", cfg.name)
	extraPolicy, err := iam.NewPolicy(ctx, extraPolicyName, &iam.PolicyArgs{
		Description: sdk.String("Allows reading secrets for alerts cloud helper"),
		Name:        sdk.String(extraPolicyName),
		Policy: sdk.All().ApplyT(func(args []interface{}) (sdk.StringOutput, error) {
			policy := map[string]interface{}{
				"Version": "2012-10-17",
				"Statement": []map[string]any{
					{
						"Effect":   "Allow",
						"Resource": "*",
						"Action": []string{
							"secretsmanager:GetSecretValue",
							"secretsmanager:DescribeSecret",
							"logs:CreateLogStream",
							"logs:CreateLogGroup",
							"logs:DescribeLogStreams",
							"logs:PutLogEvents",
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
	}, cfg.opts...)
	if err != nil {
		return errors.Wrapf(err, "failed to create extra policy for lambda role")
	}

	extraPolicyAttachmentName := fmt.Sprintf("%s-xp-attach", cfg.name)
	_, err = iam.NewRolePolicyAttachment(ctx, extraPolicyAttachmentName, &iam.RolePolicyAttachmentArgs{
		Role:      lambdaExecutionRole.Name,
		PolicyArn: extraPolicy.Arn,
	}, cfg.opts...)
	if err != nil {
		return errors.Wrapf(err, "failed to create extra policy attachment for lambda role")
	}

	envVariables := sdk.StringMap{
		api.CloudHelpersEnv.Type:             sdk.String(awsApi.CHCloudwatchAlertLambda),
		api.CloudHelpersEnv.AlertName:        sdk.String(cfg.name),
		api.CloudHelpersEnv.AlertDescription: sdk.String(cfg.description),
		api.CloudHelpersEnv.StackName:        sdk.String(cfg.deployParams.StackName),
		api.CloudHelpersEnv.StackEnv:         sdk.String(cfg.deployParams.Environment),
	}

	if cfg.discordConfig != nil {
		if s, err := createSecret(ctx,
			toSecretName(cfg.deployParams, "alert", cfg.name, api.CloudHelpersEnv.DiscordWebhookUrl, cfg.secretSuffix),
			api.CloudHelpersEnv.DiscordWebhookUrl, cfg.discordConfig.WebhookUrl, cfg.opts...,
		); err != nil {
			return errors.Wrapf(err, "failed to create secret %q", api.CloudHelpersEnv.DiscordWebhookUrl)
		} else {
			envVariables[api.CloudHelpersEnv.DiscordWebhookUrl] = s.Secret.Arn
		}
	}

	if cfg.telegramConfig != nil {
		if s, err := createSecret(ctx,
			toSecretName(cfg.deployParams, "alert", cfg.name, api.CloudHelpersEnv.TelegramToken, cfg.secretSuffix),
			api.CloudHelpersEnv.TelegramToken, cfg.discordConfig.WebhookUrl, cfg.opts...,
		); err != nil {
			return errors.Wrapf(err, "failed to create secret %q", api.CloudHelpersEnv.TelegramToken)
		} else {
			envVariables[api.CloudHelpersEnv.TelegramToken] = s.Secret.Arn
			envVariables[api.CloudHelpersEnv.TelegramChatID] = sdk.String(cfg.telegramConfig.ChatID)
		}
	}

	lambdaFunc, err := lambda.NewFunction(ctx, fmt.Sprintf("%s-callback", cfg.name), &lambda.FunctionArgs{
		PackageType: pulumi.String("Image"),
		Role:        lambdaExecutionRole.Arn,
		ImageUri:    cfg.helpersImage.ImageName,
		Timeout:     sdk.IntPtr(10),
		Environment: lambda.FunctionEnvironmentArgs{
			Variables: envVariables,
		},
	}, cfg.opts...)
	if err != nil {
		return errors.Wrapf(err, "failed to create lambda function")
	}

	cfg.metricAlarmArgs.AlarmActions = sdk.Array{
		lambdaFunc.Arn,
	}
	cfg.metricAlarmArgs.OkActions = sdk.Array{
		lambdaFunc.Arn,
	}
	alarm, err := cloudwatch.NewMetricAlarm(ctx, fmt.Sprintf("%s-metric-alarm", cfg.name), &cfg.metricAlarmArgs, cfg.opts...)
	if err != nil {
		return errors.Wrapf(err, "failed to create metric alarm")
	}

	// Define the permission for CloudWatch to invoke the Lambda function
	_, err = lambda.NewPermission(ctx, fmt.Sprintf("%s-permission", cfg.name), &lambda.PermissionArgs{
		Action:    pulumi.String("lambda:InvokeFunction"),
		Function:  lambdaFunc.Name,
		Principal: pulumi.String("lambda.alarms.cloudwatch.amazonaws.com"),
		SourceArn: alarm.Arn,
	}, cfg.opts...)
	if err != nil {
		return errors.Wrapf(err, "failed to create permission")
	}

	// Output the Lambda function's ARN
	ctx.Export(fmt.Sprintf("%s-lambda-arn", cfg.name), lambdaFunc.Arn)
	return nil
}
