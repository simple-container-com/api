package aws

import (
	"fmt"

	"github.com/pkg/errors"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/cloudwatch"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/iam"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/lambda"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	awsApi "github.com/simple-container-com/api/pkg/clouds/aws"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

type alertCfg struct {
	name            string
	description     string
	deployParams    api.StackParams
	provisionParams pApi.ProvisionParams
	eventRule       *cloudwatch.EventRule
	discordConfig   *api.DiscordCfg
	telegramConfig  *api.TelegramCfg
	secretSuffix    string
	opts            []sdk.ResourceOption
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

	envVariables := sdk.StringMap{
		api.ScCloudHelperTypeEnvVariable:     sdk.String(awsApi.CloudHelperLambda),
		api.CloudHelpersEnv.AlertName:        sdk.String(cfg.name),
		api.CloudHelpersEnv.AlertDescription: sdk.String(cfg.description),
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
		ImageUri:    pulumi.String(cfg.provisionParams.HelpersImage),
		Environment: lambda.FunctionEnvironmentArgs{
			Variables: envVariables,
		},
	}, cfg.opts...)
	if err != nil {
		return errors.Wrapf(err, "failed to create lambda function")
	}

	// Set up Lambda as the target for the CloudWatch Event Rule
	_, err = cloudwatch.NewEventTarget(ctx, fmt.Sprintf("%s-event-target", cfg.name), &cloudwatch.EventTargetArgs{
		Rule: cfg.eventRule.Name,
		Arn:  lambdaFunc.Arn,
	}, cfg.opts...)
	if err != nil {
		return errors.Wrapf(err, "failed to create event target")
	}

	// Give CloudWatch Events permission to invoke the Lambda function
	_, err = lambda.NewPermission(ctx, fmt.Sprintf("%s-lambda-permission", cfg.name), &lambda.PermissionArgs{
		Action:    pulumi.String("lambda:InvokeFunction"),
		Function:  lambdaFunc.Name,
		Principal: pulumi.String("events.amazonaws.com"),
		SourceArn: cfg.eventRule.Arn,
	}, cfg.opts...)
	if err != nil {
		return err
	}

	// Output the Lambda function's ARN
	ctx.Export(fmt.Sprintf("%s-lambda-arn", cfg.name), lambdaFunc.Arn)
	return nil
}

type eventRuleCfg struct {
	name           string
	ecsClusterName string
	description    string
	metricsJson    string
	opts           []sdk.ResourceOption
}

func createEcsEventRule(ctx *pulumi.Context, cfg eventRuleCfg) (*cloudwatch.EventRule, error) {
	return cloudwatch.NewEventRule(ctx, fmt.Sprintf("%s-eventRule", cfg.name), &cloudwatch.EventRuleArgs{
		Description: pulumi.String(cfg.description),
		EventPattern: pulumi.String(fmt.Sprintf(`{
				"source": ["aws.ecs"],
				"detail-type": ["ECS Container Instance State Change"],
				"detail": {
					"clusterArn": ["arn:aws:ecs:region:account-id:cluster/%s"],
					"metrics": {
						%s
					}
				}
			}`, cfg.ecsClusterName, cfg.metricsJson)),
	}, cfg.opts...)
}
