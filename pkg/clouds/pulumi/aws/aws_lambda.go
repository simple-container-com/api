package aws

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/samber/lo"

	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/apigatewayv2"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/cloudwatch"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/iam"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/lambda"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/aws"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	"github.com/simple-container-com/api/pkg/util"
)

type LambdaOutput struct {
	sdk.Output
}

func Lambda(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	if input.Descriptor.Type != aws.TemplateTypeAwsLambda {
		return nil, errors.Errorf("unsupported template type %q", input.Descriptor.Type)
	}
	if input.StackParams == nil {
		return nil, errors.Errorf("missing deploy params for %q in stack %q", input.Descriptor.Type, stack.Name)
	}
	deployParams := *input.StackParams

	ref := &LambdaOutput{}
	output := &api.ResourceOutput{Ref: ref}

	crInput, ok := input.Descriptor.Config.Config.(*aws.LambdaInput)
	if !ok {
		return output, errors.Errorf("failed to convert aws-lambda config for %q in stack %q in %q", input.Descriptor.Type, stack.Name, deployParams.Environment)
	}
	if err := api.ConvertAuth(crInput, &crInput.AccountConfig); err != nil {
		return nil, errors.Wrapf(err, "failed to convert auth config to aws.AccountConfig")
	}
	stackConfig := crInput.StackConfig

	awsCloudExtras := &aws.CloudExtras{}
	if stackConfig.CloudExtras != nil {
		var err error
		awsCloudExtras, err = api.ConvertDescriptor(stackConfig.CloudExtras, awsCloudExtras)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to convert cloudExtras field to AWS Cloud extras format")
		}
	}

	cloudExtras := lo.FromPtr(awsCloudExtras)
	lambdaRoutingType := cloudExtras.LambdaRoutingType
	if lambdaRoutingType == "" {
		lambdaRoutingType = aws.LambdaRoutingApiGw
	}
	params.Log.Info(ctx.Context(), "lambda will use routing type: %q", lambdaRoutingType)

	opts := []sdk.ResourceOption{
		sdk.Provider(params.Provider),
		sdk.DependsOn(params.ComputeContext.Dependencies()),
	}

	image, err := buildAndPushDockerImageV2(ctx, stack, params, deployParams, dockerImage{
		name:       stack.Name,
		dockerfile: stackConfig.Image.Dockerfile,
		context:    stackConfig.Image.Context,
		args:       lo.FromPtr(stackConfig.Image.Build).Args,
		version:    lo.If(deployParams.Version != "", deployParams.Version).Else("latest"),
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to build and push image for lambda in stack %q env %q", stack.Name, deployParams.Environment)
	}
	opts = append(opts, image.addOpts...)

	secretEnvVariables := lo.Filter(params.ComputeContext.SecretEnvVariables(), func(s pApi.ComputeEnvVariable, _ int) bool {
		return stackConfig.Secrets[s.Name] == ""
	})
	contextEnvVariables := lo.Filter(params.ComputeContext.EnvVariables(), func(v pApi.ComputeEnvVariable, _ int) bool {
		return stackConfig.Env[v.Name] == ""
	})

	// Create IAM Role for Lambda Function
	lambdaExecutionRoleName := fmt.Sprintf("%s-execution-role", stack.Name)
	params.Log.Info(ctx.Context(), "configure lambda execution role %q for %q in %q...", lambdaExecutionRoleName, stack.Name, deployParams.Environment)
	lambdaExecutionRole, err := iam.NewRole(ctx, lambdaExecutionRoleName, &iam.RoleArgs{
		AssumeRolePolicy: sdk.String(`{
			"Version": "2012-10-17",
			"Statement": [{
				"Effect": "Allow",
				"Principal": {
					"Service": "lambda.amazonaws.com"
				},
				"Action": "sts:AssumeRole"
			}]
		}`),
	}, opts...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create iam role")
	}

	// Attach the necessary AWS managed policies to the role created
	rolePolicyAttachmentName := fmt.Sprintf("%s-policy-attachment", stack.Name)
	params.Log.Info(ctx.Context(), "configure role policy attachment %q for %q in %q...", rolePolicyAttachmentName, stack.Name, deployParams.Environment)
	_, err = iam.NewRolePolicyAttachment(ctx, rolePolicyAttachmentName, &iam.RolePolicyAttachmentArgs{
		Role:      lambdaExecutionRole.Name,
		PolicyArn: sdk.String("arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole"),
	}, opts...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create iam policy attachment")
	}

	// Custom policy allowing to read secrets
	lambdaRoles := []string{
		"secretsmanager:GetSecretValue",
		"secretsmanager:DescribeSecret",
		"logs:CreateLogStream",
		"logs:CreateLogGroup",
		"logs:DescribeLogStreams",
		"logs:PutLogEvents",
		"ec2:DescribeNetworkInterfaces",
		"ec2:CreateNetworkInterface",
		"ec2:DeleteNetworkInterface",
		"ec2:DescribeInstances",
		"ec2:AttachNetworkInterface",
	}
	params.Log.Info(ctx.Context(), "adding extra roles %q for lambda %q...", strings.Join(awsCloudExtras.AwsRoles, ","), stack.Name)
	lambdaRoles = append(lambdaRoles, awsCloudExtras.AwsRoles...)
	extraPolicyName := fmt.Sprintf("%s-xpolicy", stack.Name)
	params.Log.Info(ctx.Context(), "configure extra policy %q for %q in %q...", extraPolicyName, stack.Name, deployParams.Environment)
	extraPolicy, err := iam.NewPolicy(ctx, extraPolicyName, &iam.PolicyArgs{
		Description: sdk.String(fmt.Sprintf("Allows reading secrets in lambda for stack %s", stack.Name)),
		Name:        sdk.String(extraPolicyName),
		Policy: sdk.All().ApplyT(func(args []interface{}) (sdk.StringOutput, error) {
			policy := map[string]interface{}{
				"Version": "2012-10-17",
				"Statement": []map[string]any{
					{
						"Effect":   "Allow",
						"Resource": "*",
						"Action":   lambdaRoles,
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
		return nil, errors.Wrapf(err, "failed to create extra policy for lambda role")
	}

	extraPolicyAttachmentName := fmt.Sprintf("%s-xp-attach", stack.Name)
	params.Log.Info(ctx.Context(), "configure IAM policy attachment %q for %q in %q...", extraPolicyAttachmentName, stack.Name, deployParams.Environment)
	_, err = iam.NewRolePolicyAttachment(ctx, extraPolicyAttachmentName, &iam.RolePolicyAttachmentArgs{
		Role:      lambdaExecutionRole.Name,
		PolicyArn: extraPolicy.Arn,
	}, opts...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create extra policy attachment for lambda role")
	}

	// SECRETS
	var secrets []*CreatedSecret
	ctxSecrets, err := util.MapErr(secretEnvVariables, func(v pApi.ComputeEnvVariable, _ int) (*CreatedSecret, error) {
		return createSecret(ctx, toSecretName(deployParams, v.ResourceType, v.ResourceName, v.Name, stackConfig.Version), v.Name, v.Value, opts...)
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create context secrets for stack %q in %q", stack.Name, deployParams.Environment)
	}
	secrets = append(secrets, ctxSecrets...)
	for name, value := range stackConfig.Secrets {
		s, err := createSecret(ctx, toSecretName(deployParams, "values", "", name, stackConfig.Version), name, value, opts...)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create secret")
		}
		secrets = append(secrets, s)
	}
	params.Log.Info(ctx.Context(), "configure secrets in SecretsManager for %d secrets in stack %q in %q...", len(secrets), stack.Name, deployParams.Environment)

	lambdaSizeMb := lo.If(stackConfig.MaxMemory == nil, 128).Else(lo.FromPtr(stackConfig.MaxMemory))

	invokeMode := cloudExtras.LambdaInvokeMode
	if invokeMode == "" {
		invokeMode = aws.LambdaInvokeModeBuffered
	}

	// ENV VARIABLES
	envVariables := sdk.StringMap{
		api.ComputeEnv.StackName:                   sdk.String(stack.Name),
		api.ComputeEnv.StackEnv:                    sdk.String(deployParams.Environment),
		api.ComputeEnv.StackVersion:                sdk.String(deployParams.Version),
		"SIMPLE_CONTAINER_AWS_LAMBDA_ROUTING_TYPE": sdk.String(lambdaRoutingType),
		"SIMPLE_CONTAINER_AWS_LAMBDA_SIZE_MB":      sdk.String(strconv.Itoa(lambdaSizeMb)),
		"SIMPLE_CONTAINER_AWS_LAMBDA_INVOKE_MODE":  sdk.String(invokeMode),
	}
	for envVar, envVal := range params.BaseEnvVariables {
		envVariables[envVar] = sdk.String(envVal)
	}
	for _, envVar := range contextEnvVariables {
		envVariables[envVar.Name] = sdk.String(envVar.Value)
	}

	for _, secret := range secrets {
		envVariables[secret.EnvVar] = secret.Secret.Arn
	}
	for name, value := range params.BaseEnvVariables {
		envVariables[name] = sdk.String(value)
	}
	for k := range lo.Assign(stackConfig.Env) {
		if _, found := lo.Find(secrets, func(s *CreatedSecret) bool {
			return s.EnvVar == k
		}); found {
			delete(stackConfig.Env, k)
		}
	}
	for name, value := range stackConfig.Env {
		envVariables[name] = sdk.String(value)
	}

	accessLogGroupName := fmt.Sprintf("%s-access-logs", stack.Name)
	params.Log.Info(ctx.Context(), "configure cloudwatch access log group for %q in %q...", stack.Name, deployParams.Environment)
	logGroup, err := cloudwatch.NewLogGroup(ctx, accessLogGroupName, &cloudwatch.LogGroupArgs{
		Name: sdk.String(accessLogGroupName),
	}, opts...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create access logs group for api gateway")
	}

	lambdaName := fmt.Sprintf("%s-callback", stack.Name)
	lambdaFuncArgs := lambda.FunctionArgs{
		PackageType: sdk.String("Image"),
		Role:        lambdaExecutionRole.Arn,
		ImageUri:    image.image.ImageName,
		MemorySize:  sdk.IntPtr(lambdaSizeMb),
		Timeout:     sdk.IntPtr(lo.If(stackConfig.Timeout != nil, lo.FromPtr(stackConfig.Timeout)).Else(10)),
		EphemeralStorage: lambda.FunctionEphemeralStorageArgs{
			Size: sdk.IntPtr(lo.If(stackConfig.MaxEphemeralStorage != nil, lo.FromPtr(stackConfig.MaxEphemeralStorage)).Else(1024)),
		},
		LoggingConfig: lambda.FunctionLoggingConfigArgs{
			LogFormat:      sdk.String("JSON"),
			LogGroup:       sdk.String(accessLogGroupName),
			SystemLogLevel: sdk.String("DEBUG"),
		},
		Environment: lambda.FunctionEnvironmentArgs{
			Variables: envVariables,
		},
	}

	if lo.FromPtr(stackConfig.StaticEgressIP) {
		staticEgressOut, err := provisionVpcWithStaticEgress(ctx, lambdaName, &StaticEgressIPIn{
			Params:        params,
			Provider:      params.Provider,
			AccountConfig: crInput.AccountConfig,
			SecurityGroup: cloudExtras.SecurityGroup,
		}, opts...)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to provision static egress IP for lambda")
		}

		params.Log.Info(ctx.Context(), "configure private route table association for lambda %s...", lambdaName)
		lambdaFuncArgs.VpcConfig = lambda.FunctionVpcConfigArgs{
			SubnetIds:        sdk.StringArray{staticEgressOut.SubnetID},
			SecurityGroupIds: sdk.StringArray{staticEgressOut.SecurityGroupID},
		}
	}

	params.Log.Info(ctx.Context(), "configure lambda function for %q in %q...", stack.Name, deployParams.Environment)
	lambdaFunc, err := lambda.NewFunction(ctx, lambdaName, &lambdaFuncArgs, opts...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create lambda function")
	}
	ctx.Export(fmt.Sprintf("%s-%s-lambda-arn", stack.Name, deployParams.Environment), lambdaFunc.Arn)
	opts = append(opts, sdk.DependsOn([]sdk.Resource{lambdaFunc}))

	if lambdaRoutingType == aws.LambdaRoutingApiGw {
		// Create an HTTP API Gateway for the Lambda Function
		params.Log.Info(ctx.Context(), "configure API gateway for %q in %q...", stack.Name, deployParams.Environment)
		apiGwName := fmt.Sprintf("%s-api-gw", stack.Name)
		apiGw, err := apigatewayv2.NewApi(ctx, apiGwName, &apigatewayv2.ApiArgs{
			Name: sdk.String(apiGwName),
			// RouteKey:     sdk.String("$default"), // TODO: figure out whether this will work
			ProtocolType: sdk.String("HTTP"),
		}, opts...)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create API gateway for lambda")
		}

		// Create an integration between the HTTP API Gateway and the Lambda Function
		params.Log.Info(ctx.Context(), "configure API gateway lambda integration for %q in %q...", stack.Name, deployParams.Environment)
		integration, err := apigatewayv2.NewIntegration(ctx, fmt.Sprintf("%s-api-lambda-integration", stack.Name),
			&apigatewayv2.IntegrationArgs{
				ApiId:           apiGw.ID(),
				IntegrationType: sdk.String("AWS_PROXY"),
				IntegrationUri:  lambdaFunc.InvokeArn,
			}, opts...)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create API gateway lambda integration")
		}

		// Create a route for the HTTP API Gateway for invoking the Lambda Function
		params.Log.Info(ctx.Context(), "configure API gateway route for %q in %q...", stack.Name, deployParams.Environment)
		routeName := fmt.Sprintf("%s-route", stack.Name)
		route, err := apigatewayv2.NewRoute(ctx, routeName, &apigatewayv2.RouteArgs{
			ApiId:         apiGw.ID(),
			OperationName: sdk.String("ANY"),
			RouteKey:      sdk.String("ANY /{proxy+}"), // Define the catch-all route
			Target: integration.ID().ApplyT(func(id string) string {
				return fmt.Sprintf("integrations/%s", id)
			}).(sdk.StringOutput),
		}, opts...)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create API gateway route for lambda")
		}

		// Grant the API Gateway permission to invoke the Lambda function
		_, err = lambda.NewPermission(ctx, fmt.Sprintf("%s-permission", stack.Name), &lambda.PermissionArgs{
			Action:    sdk.String("lambda:InvokeFunction"),
			Function:  lambdaFunc.Arn,
			Principal: sdk.String("apigateway.amazonaws.com"),
			SourceArn: sdk.All(apiGw.ExecutionArn, route.RouteKey).ApplyT(func(args []any) string {
				executionArn, _ := args[0], args[1]
				return fmt.Sprintf("%s/*/*/{proxy+}", executionArn)
			}).(sdk.StringOutput),
		}, opts...)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create permission for api gateway to invoke lambda")
		}

		// Define the stage. This is the URL path where your API will be accessible
		params.Log.Info(ctx.Context(), "configure API gateway stage for %q in %q...", stack.Name, deployParams.Environment)
		_, err = apigatewayv2.NewStage(ctx, fmt.Sprintf("%s-http-stage", stack.Name), &apigatewayv2.StageArgs{
			ApiId: apiGw.ID(),
			Name:  sdk.String(lo.If(stackConfig.BasePath == "", "api").Else(stackConfig.BasePath)),
			Description: route.ID().ApplyT(func(routeId string) string {
				return fmt.Sprintf("stage for route %s", routeId)
			}).(sdk.StringOutput),
			AutoDeploy: sdk.Bool(true),
			AccessLogSettings: apigatewayv2.StageAccessLogSettingsArgs{
				DestinationArn: logGroup.Arn,
				Format:         sdk.String(`{ "requestId":"$context.requestId", "ip": "$context.identity.sourceIp", "caller":"$context.identity.caller", "user":"$context.identity.user", "requestTime":"$context.requestTime", "httpMethod":"$context.httpMethod", "resourcePath":"$context.resourcePath", "status":"$context.status", "protocol":"$context.protocol", "responseLength":"$context.responseLength", "integrationError":"$context.integrationErrorMessage"}`),
			},
		}, opts...)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create API gateway stage")
		}

		ctx.Export(fmt.Sprintf("%s-%s-function-url", stack.Name, deployParams.Environment), apiGw.ApiEndpoint)
		if stackConfig.Domain != "" {
			_, err := provisionDNSForLambda(ctx, stack, params, lambdaName, stackConfig.Domain, apiGw.ApiEndpoint)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to provision DNS for lambda")
			}
		}
	} else if lambdaRoutingType == aws.LambdaRoutingFunctionUrl {
		functionUrlName := fmt.Sprintf("%s-url", lambdaName)
		params.Log.Info(ctx.Context(), "configure lambda function url for %q in %q...", stack.Name, deployParams.Environment)
		functionUrl, err := lambda.NewFunctionUrl(ctx, functionUrlName, &lambda.FunctionUrlArgs{
			AuthorizationType: sdk.String("NONE"),
			FunctionName:      lambdaFunc.Name,
			InvokeMode:        sdk.String(invokeMode),
		}, opts...)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create lambda function url")
		}

		// Add permission to allow anonymous invocation via function URL
		// This is required for AuthorizationType: "NONE" to work properly
		// AWS now requires FunctionUrlAuthType to be specified for public access
		urlPermissionName := fmt.Sprintf("%s-url-permission", lambdaName)
		_, err = lambda.NewPermission(ctx, urlPermissionName, &lambda.PermissionArgs{
			Action:              sdk.String("lambda:InvokeFunctionUrl"),
			Function:            lambdaFunc.Name,
			Principal:           sdk.String("*"),
			FunctionUrlAuthType: sdk.String("NONE"),
		}, opts...)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create lambda function url permission")
		}
		ctx.Export(fmt.Sprintf("%s-%s-function-url", stack.Name, deployParams.Environment), functionUrl.FunctionUrl)
		if stackConfig.Domain != "" {
			_, err := provisionDNSForLambda(ctx, stack, params, lambdaName, stackConfig.Domain, functionUrl.FunctionUrl)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to provision DNS for lambda")
			}
		}
	}

	schedules := make([]aws.LambdaSchedule, 0)
	if cloudExtras.LambdaSchedule != nil {
		schedules = append(schedules, *cloudExtras.LambdaSchedule)
	}
	schedules = append(schedules, cloudExtras.LambdaSchedules...)
	schedulesByName := lo.GroupBy(schedules, func(s aws.LambdaSchedule) string {
		return s.Name
	})
	if _, found := lo.Find(lo.Entries(schedulesByName), func(s lo.Entry[string, []aws.LambdaSchedule]) bool {
		return len(s.Value) > 1
	}); found {
		return nil, errors.Errorf("schedules must have unique names")
	}
	for _, schedule := range schedules {
		if err := provisionScheduleForLambda(ctx, stack, params, lambdaName, lambdaFunc, schedule, opts); err != nil {
			return nil, errors.Wrapf(err, "failed to provision schedule %q for lambda", schedule.Name)
		}
	}

	return output, nil
}

func provisionDNSForLambda(ctx *sdk.Context, stack api.Stack, params pApi.ProvisionParams, lambdaName, domain string, endpointUrl sdk.StringOutput) (*api.ResourceOutput, error) {
	params.Log.Info(ctx.Context(), "configure CNAME DNS record %q for %q in %q...", domain, stack.Name)

	endpointHost := endpointUrl.ApplyT(func(epUrl string) (string, error) {
		parsed, err := url.Parse(epUrl)
		if err != nil {
			return "", errors.Wrapf(err, "failed to parse URL %q", epUrl)
		}
		return parsed.Host, nil
	}).(sdk.StringOutput)
	record, err := params.Registrar.NewRecord(ctx, api.DnsRecord{
		Name:     domain,
		Type:     "CNAME",
		ValueOut: endpointHost,
		Proxied:  true,
	})
	if err != nil {
		params.Log.Error(ctx.Context(), "failed to create DNS record %q: %s", domain, err.Error())
		return nil, errors.Wrapf(err, "failed to create DNS record %q", domain)
	}
	_, err = params.Registrar.NewOverrideHeaderRule(ctx, stack, pApi.OverrideHeaderRule{
		Name:     lambdaName,
		FromHost: domain,
		ToHost:   endpointHost,
	})
	if err != nil {
		params.Log.Error(ctx.Context(), "failed to create override header rule for %q", domain)
		return nil, errors.Wrapf(err, "failed to create override host rule from %q", domain)
	}
	return record, nil
}

func provisionScheduleForLambda(ctx *sdk.Context, stack api.Stack, params pApi.ProvisionParams,
	lambdaName string, lambdaFunc *lambda.Function, schedule aws.LambdaSchedule, opts []sdk.ResourceOption,
) error {
	if schedule.Expression == "" {
		return errors.Errorf("cron expression must be specified for schedule %q", schedule.Name)
	}
	if schedule.Request == "" {
		return errors.Errorf("API Gateway request must be specified for schedule %q to work properly", schedule.Name)
	}

	expression := schedule.Expression
	params.Log.Info(ctx.Context(), "configure cron schedule for lambda %s with expression %q...", lambdaName, expression)
	scheduleName := fmt.Sprintf("%s%s-schedule", lambdaName, lo.If(schedule.Name != "", fmt.Sprintf("-%s", schedule.Name)).Else(""))
	scheduleRule, err := cloudwatch.NewEventRule(ctx, scheduleName, &cloudwatch.EventRuleArgs{
		ScheduleExpression: sdk.String(expression),
	}, opts...)
	if err != nil {
		return errors.Wrapf(err, "failed to create aws lambda schedule")
	}

	scheduleTargetName := fmt.Sprintf("%s-target", scheduleName)
	params.Log.Info(ctx.Context(), "configure cloudwatch event target to trigger lambda %s on schedule: %q...", lambdaName, scheduleTargetName)
	_, err = cloudwatch.NewEventTarget(ctx, scheduleTargetName, &cloudwatch.EventTargetArgs{
		Rule:     scheduleRule.Name,
		Arn:      lambdaFunc.Arn,
		Input:    sdk.String(schedule.Request),
		TargetId: sdk.String(scheduleTargetName),
	}, opts...)
	if err != nil {
		return errors.Wrapf(err, "failed to create schedule target for lambda")
	}

	params.Log.Info(ctx.Context(), "configure permission for schedule to invoke lambda %s...", lambdaName)
	permissionName := fmt.Sprintf("%s-permission", scheduleName)
	_, err = lambda.NewPermission(ctx, permissionName, &lambda.PermissionArgs{
		Action:    sdk.String("lambda:InvokeFunction"),
		Function:  lambdaFunc.Arn,
		Principal: sdk.String("events.amazonaws.com"),
		SourceArn: scheduleRule.Arn,
	}, opts...)
	if err != nil {
		return errors.Wrapf(err, "failed to create permission for schedule to invoke lambda")
	}
	return nil
}
