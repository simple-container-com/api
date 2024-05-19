package aws

import (
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/pkg/errors"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/apigatewayv2"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/iam"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/lambda"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/samber/lo"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/aws"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	"github.com/simple-container-com/api/pkg/util"
)

type LambdaOutput struct {
	MainDnsRecord sdk.AnyOutput
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

	opts := []sdk.ResourceOption{
		sdk.Provider(params.Provider),
		sdk.DependsOn(params.ComputeContext.Dependencies()),
	}

	image, err := buildAndPushDockerImage(ctx, stack, params, deployParams, dockerImage{
		name:       deployParams.StackName,
		dockerfile: stackConfig.Image.Dockerfile,
		context:    stackConfig.Image.Context,
		version:    "latest", // TODO: support versioning
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to build and push image for lambda in stack %q env %q", stack.Name, deployParams.Environment)
	}

	contextEnvVariables := params.ComputeContext.EnvVariables()

	// Create IAM Role for Lambda Function
	lambdaExecutionRoleName := fmt.Sprintf("%s-execution-role", deployParams.StackName)
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
	rolePolicyAttachmentName := fmt.Sprintf("%s-policy-attachment", deployParams.StackName)
	params.Log.Info(ctx.Context(), "configure role policy attachment %q for %q in %q...", rolePolicyAttachmentName, stack.Name, deployParams.Environment)
	_, err = iam.NewRolePolicyAttachment(ctx, rolePolicyAttachmentName, &iam.RolePolicyAttachmentArgs{
		Role:      lambdaExecutionRole.Name,
		PolicyArn: sdk.String("arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole"),
	}, opts...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create iam policy attachment")
	}

	// Custom policy allowing to read secrets
	extraPolicyName := fmt.Sprintf("%s-xpolicy", deployParams.StackName)
	params.Log.Info(ctx.Context(), "configure extra policy %q for %q in %q...", extraPolicyName, stack.Name, deployParams.Environment)
	extraPolicy, err := iam.NewPolicy(ctx, extraPolicyName, &iam.PolicyArgs{
		Description: sdk.String(fmt.Sprintf("Allows reading secrets in lambda for stack %s", deployParams.StackName)),
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
	}, opts...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create extra policy for lambda role")
	}

	extraPolicyAttachmentName := fmt.Sprintf("%s-xp-attach", deployParams.StackName)
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
	ctxSecrets, err := util.MapErr(contextEnvVariables, func(v pApi.ComputeEnvVariable, _ int) (*CreatedSecret, error) {
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

	// ENV VARIABLES
	envVariables := sdk.StringMap{
		api.CloudHelpersEnv.StackName: sdk.String(deployParams.StackName),
		api.CloudHelpersEnv.StackEnv:  sdk.String(deployParams.Environment),
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

	params.Log.Info(ctx.Context(), "configure lambda callback for %q in %q...", stack.Name, deployParams.Environment)
	lambdaFunc, err := lambda.NewFunction(ctx, fmt.Sprintf("%s-callback", deployParams.StackName), &lambda.FunctionArgs{
		PackageType: sdk.String("Image"),
		Role:        lambdaExecutionRole.Arn,
		ImageUri:    image.ImageName,
		Timeout:     sdk.IntPtr(lo.If(stackConfig.Timeout != nil, lo.FromPtr(stackConfig.Timeout)).Else(10)),
		Environment: lambda.FunctionEnvironmentArgs{
			Variables: envVariables,
		},
	}, opts...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create lambda function")
	}

	// Create an HTTP API Gateway for the Lambda Function
	params.Log.Info(ctx.Context(), "configure API gateway for %q in %q...", stack.Name, deployParams.Environment)
	apiGw, err := apigatewayv2.NewApi(ctx, fmt.Sprintf("%s-api-gw", deployParams.StackName), &apigatewayv2.ApiArgs{
		ProtocolType: sdk.String("HTTP"),
	}, opts...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create API gateway for lambda")
	}

	// Create an integration between the HTTP API Gateway and the Lambda Function
	params.Log.Info(ctx.Context(), "configure API gateway lambda integration for %q in %q...", stack.Name, deployParams.Environment)
	integration, err := apigatewayv2.NewIntegration(ctx, fmt.Sprintf("%s-api-lambda-integration", deployParams.StackName),
		&apigatewayv2.IntegrationArgs{
			ApiId:           apiGw.ID(),
			IntegrationType: sdk.String("AWS_PROXY"),
			IntegrationUri:  lambdaFunc.InvokeArn,
		}, opts...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create API gateway lambda integration")
	}

	// Grant the API Gateway permission to invoke the Lambda function
	_, err = lambda.NewPermission(ctx, fmt.Sprintf("%s-permission", deployParams.StackName), &lambda.PermissionArgs{
		Action:    sdk.String("lambda:InvokeFunction"),
		Function:  lambdaFunc.Arn,
		Principal: sdk.String("apigateway.amazonaws.com"),
		SourceArn: apiGw.ExecutionArn,
	}, opts...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create permission for api gateway to invoke lambda")
	}

	// Create a route for the HTTP API Gateway for invoking the Lambda Function
	params.Log.Info(ctx.Context(), "configure API gateway route for %q in %q...", deployParams.StackName, deployParams.Environment)
	route, err := apigatewayv2.NewRoute(ctx, fmt.Sprintf("%s-route", deployParams.StackName), &apigatewayv2.RouteArgs{
		ApiId:    apiGw.ID(),
		RouteKey: sdk.String("ANY /{proxy+}"), // Define the catch-all route
		Target: integration.ID().ApplyT(func(id string) string {
			return fmt.Sprintf("integrations/%s", id)
		}).(sdk.StringOutput),
	}, opts...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create API gateway route for lambda")
	}

	params.Log.Info(ctx.Context(), "configure API gateway deployment for %q in %q...", stack.Name, deployParams.Environment)
	deployment, err := apigatewayv2.NewDeployment(ctx, fmt.Sprintf("%s-deployment", deployParams.StackName), &apigatewayv2.DeploymentArgs{
		ApiId: apiGw.ID(),
		Description: route.ID().ApplyT(func(routeId any) string {
			return fmt.Sprintf("Deployment for route ID %s", routeId)
		}).(sdk.StringOutput),
		Triggers: sdk.StringMap{
			"redeployment": sdk.String("changes-in-the-blueprint"),
		},
	}, opts...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create deployment of API gateway")
	}

	// Define the stage. This is the URL path where your API will be accessible
	params.Log.Info(ctx.Context(), "configure API gateway stage for %q in %q...", deployParams.StackName, deployParams.Environment)
	_, err = apigatewayv2.NewStage(ctx, fmt.Sprintf("%s-http-stage", deployParams.StackName), &apigatewayv2.StageArgs{
		ApiId:        apiGw.ID(),
		AutoDeploy:   sdk.Bool(true),
		DeploymentId: deployment.ID(),
	}, opts...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create API gateway stage")
	}

	params.Log.Info(ctx.Context(), "configure CNAME DNS record %q for %q in %q...", stackConfig.Domain, stack.Name, deployParams.Environment)
	mainRecord := sdk.All(apiGw.ApiEndpoint).ApplyT(func(vals []any) (*api.ResourceOutput, error) {
		apiEndpointUrl, err := url.Parse(fmt.Sprintf("%s", vals[0]))
		if err != nil {
			return nil, errors.Wrapf(err, "failed to parse URL %q", vals[0])
		}

		return params.Registrar.NewRecord(ctx, api.DnsRecord{
			Name:    stackConfig.Domain,
			Type:    "CNAME",
			Value:   apiEndpointUrl.Host,
			Proxied: true,
		})
	}).(sdk.AnyOutput)
	ref.MainDnsRecord = mainRecord
	ctx.Export(fmt.Sprintf("%s-%s-dns-record", stack.Name, deployParams.Environment), mainRecord)
	ctx.Export(fmt.Sprintf("%s-%s-lambda-arn", stack.Name, deployParams.Environment), lambdaFunc.Arn)

	return output, nil
}
