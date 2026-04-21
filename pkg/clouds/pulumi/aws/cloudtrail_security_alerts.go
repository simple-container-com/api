package aws

import (
	"fmt"
	"sort"

	"github.com/pkg/errors"

	sdkAws "github.com/pulumi/pulumi-aws/sdk/v6/go/aws"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/cloudwatch"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/sns"
	"github.com/pulumi/pulumi-docker/sdk/v4/go/docker"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	awsApi "github.com/simple-container-com/api/pkg/clouds/aws"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	"github.com/simple-container-com/api/pkg/util"
)

// securityAlertDef defines a CloudTrail security alert with its metric filter pattern.
type securityAlertDef struct {
	name          string
	description   string
	filterPattern string
	threshold     float64 // default 1 if 0
}

// securityAlerts maps CloudTrailAlertSelectors field names to their definitions.
// Filter patterns follow AWS Security Hub/CIS CloudWatch controls (CloudWatch.1-14).
// Reference: https://docs.aws.amazon.com/securityhub/latest/userguide/cloudwatch-controls.html
var securityAlerts = map[string]securityAlertDef{
	// CloudWatch.1 — Root account usage
	"rootAccountUsage": {
		name:          "ct-root-account-usage",
		description:   "Root account API call detected — investigate immediately",
		filterPattern: `{ $.userIdentity.type = "Root" && $.userIdentity.invokedBy NOT EXISTS && $.eventType != "AwsServiceEvent" }`,
	},
	// CloudWatch.2 — Unauthorized API calls (threshold 5 to reduce noise from normal permission probing)
	"unauthorizedApiCalls": {
		name:        "ct-unauthorized-api-calls",
		description: "Spike in unauthorized API calls — possible credential compromise or misconfiguration",
		filterPattern: `{ ($.errorCode = "AccessDenied") || ($.errorCode = "AccessDeniedException") || ` +
			`($.errorCode = "UnauthorizedAccess") || ($.errorCode = "Client.UnauthorizedAccess") || ` +
			`($.errorCode = "Client.UnauthorizedOperation") || ($.errorCode = "UnauthorizedOperation") }`,
		threshold: 5,
	},
	// CloudWatch.3 — Console login without MFA (successful only)
	"consoleLoginWithoutMfa": {
		name:          "ct-console-login-no-mfa",
		description:   "Successful console login without MFA detected",
		filterPattern: `{ ($.eventName = "ConsoleLogin") && ($.additionalEventData.MFAUsed != "Yes") && ($.responseElements.ConsoleLogin = "Success") }`,
	},
	// CloudWatch.4 — IAM policy changes
	"iamPolicyChanges": {
		name:        "ct-iam-policy-changes",
		description: "IAM policy or role modified — verify authorization",
		filterPattern: `{ ($.eventName = "DeleteGroupPolicy") || ($.eventName = "DeleteRolePolicy") || ` +
			`($.eventName = "DeleteUserPolicy") || ($.eventName = "PutGroupPolicy") || ` +
			`($.eventName = "PutRolePolicy") || ($.eventName = "PutUserPolicy") || ` +
			`($.eventName = "CreatePolicy") || ($.eventName = "DeletePolicy") || ` +
			`($.eventName = "CreatePolicyVersion") || ($.eventName = "DeletePolicyVersion") || ` +
			`($.eventName = "AttachRolePolicy") || ($.eventName = "DetachRolePolicy") || ` +
			`($.eventName = "AttachUserPolicy") || ($.eventName = "DetachUserPolicy") || ` +
			`($.eventName = "AttachGroupPolicy") || ($.eventName = "DetachGroupPolicy") }`,
	},
	// CloudWatch.5 — CloudTrail configuration changes
	"cloudTrailTampering": {
		name:        "ct-cloudtrail-tampering",
		description: "CloudTrail logging modified or stopped — potential compromise indicator",
		filterPattern: `{ ($.eventName = "CreateTrail") || ($.eventName = "UpdateTrail") || ` +
			`($.eventName = "DeleteTrail") || ($.eventName = "StartLogging") || ($.eventName = "StopLogging") }`,
	},
	// CloudWatch.6 — Failed console logins
	"failedConsoleLogins": {
		name:          "ct-failed-console-logins",
		description:   "Console login failures detected — possible brute force attempt",
		filterPattern: `{ ($.eventName = "ConsoleLogin") && ($.errorMessage = "Failed authentication") }`,
	},
	// CloudWatch.7 — KMS key deletion/disable
	"kmsKeyDeletion": {
		name:          "ct-kms-key-deletion",
		description:   "KMS encryption key disabled or scheduled for deletion — may cause data loss",
		filterPattern: `{ ($.eventSource = "kms.amazonaws.com") && (($.eventName = "DisableKey") || ($.eventName = "ScheduleKeyDeletion")) }`,
	},
	// CloudWatch.8 — S3 bucket policy changes
	"s3BucketPolicyChanges": {
		name:        "ct-s3-bucket-policy-changes",
		description: "S3 bucket policy or ACL modified — check for public exposure",
		filterPattern: `{ ($.eventSource = "s3.amazonaws.com") && (($.eventName = "PutBucketAcl") || ` +
			`($.eventName = "PutBucketPolicy") || ($.eventName = "PutBucketCors") || ` +
			`($.eventName = "PutBucketLifecycle") || ($.eventName = "PutBucketReplication") || ` +
			`($.eventName = "DeleteBucketPolicy") || ($.eventName = "DeleteBucketCors") || ` +
			`($.eventName = "DeleteBucketLifecycle") || ($.eventName = "DeleteBucketReplication")) }`,
	},
	// CloudWatch.9 — AWS Config changes
	"configChanges": {
		name:        "ct-config-changes",
		description: "AWS Config recorder or delivery channel modified",
		filterPattern: `{ ($.eventSource = "config.amazonaws.com") && (($.eventName = "StopConfigurationRecorder") || ` +
			`($.eventName = "DeleteDeliveryChannel") || ($.eventName = "PutDeliveryChannel") || ($.eventName = "PutConfigurationRecorder")) }`,
	},
	// CloudWatch.10 — Security group changes
	"securityGroupChanges": {
		name:        "ct-security-group-changes",
		description: "Security group rules modified — verify network exposure",
		filterPattern: `{ ($.eventName = "AuthorizeSecurityGroupIngress") || ` +
			`($.eventName = "AuthorizeSecurityGroupEgress") || ` +
			`($.eventName = "RevokeSecurityGroupIngress") || ` +
			`($.eventName = "RevokeSecurityGroupEgress") || ` +
			`($.eventName = "CreateSecurityGroup") || ` +
			`($.eventName = "DeleteSecurityGroup") }`,
	},
	// CloudWatch.11 — NACL changes
	"naclChanges": {
		name:        "ct-nacl-changes",
		description: "Network ACL modified — verify subnet-level network exposure",
		filterPattern: `{ ($.eventName = "CreateNetworkAcl") || ($.eventName = "CreateNetworkAclEntry") || ` +
			`($.eventName = "DeleteNetworkAcl") || ($.eventName = "DeleteNetworkAclEntry") || ` +
			`($.eventName = "ReplaceNetworkAclEntry") || ($.eventName = "ReplaceNetworkAclAssociation") }`,
	},
	// CloudWatch.12 — Network gateway changes
	"networkGatewayChanges": {
		name:        "ct-network-gateway-changes",
		description: "Network gateway created, modified, or deleted",
		filterPattern: `{ ($.eventName = "CreateCustomerGateway") || ($.eventName = "DeleteCustomerGateway") || ` +
			`($.eventName = "AttachInternetGateway") || ($.eventName = "CreateInternetGateway") || ` +
			`($.eventName = "DeleteInternetGateway") || ($.eventName = "DetachInternetGateway") }`,
	},
	// CloudWatch.13 — Route table changes
	"routeTableChanges": {
		name:        "ct-route-table-changes",
		description: "Route table modified — verify network routing",
		filterPattern: `{ ($.eventName = "CreateRoute") || ($.eventName = "CreateRouteTable") || ` +
			`($.eventName = "ReplaceRoute") || ($.eventName = "ReplaceRouteTableAssociation") || ` +
			`($.eventName = "DeleteRouteTable") || ($.eventName = "DeleteRoute") || ` +
			`($.eventName = "DisassociateRouteTable") }`,
	},
	// CloudWatch.14 — VPC changes
	"vpcChanges": {
		name:        "ct-vpc-changes",
		description: "VPC created, modified, or deleted",
		filterPattern: `{ ($.eventName = "CreateVpc") || ($.eventName = "DeleteVpc") || ` +
			`($.eventName = "ModifyVpcAttribute") || ($.eventName = "AcceptVpcPeeringConnection") || ` +
			`($.eventName = "CreateVpcPeeringConnection") || ($.eventName = "DeleteVpcPeeringConnection") || ` +
			`($.eventName = "RejectVpcPeeringConnection") || ($.eventName = "AttachClassicLinkVpc") || ` +
			`($.eventName = "DetachClassicLinkVpc") || ($.eventName = "DisableVpcClassicLink") || ` +
			`($.eventName = "EnableVpcClassicLink") }`,
	},
}

// enabledAlerts returns the alert definitions that are enabled in the selector config.
// Results are sorted by name for deterministic Pulumi resource ordering.
func enabledAlerts(selectors awsApi.CloudTrailAlertSelectors) []securityAlertDef {
	var result []securityAlertDef
	checks := []struct {
		key     string
		enabled bool
	}{
		{"cloudTrailTampering", selectors.CloudTrailTampering},
		{"configChanges", selectors.ConfigChanges},
		{"consoleLoginWithoutMfa", selectors.ConsoleLoginWithoutMfa},
		{"failedConsoleLogins", selectors.FailedConsoleLogins},
		{"iamPolicyChanges", selectors.IamPolicyChanges},
		{"kmsKeyDeletion", selectors.KmsKeyDeletion},
		{"naclChanges", selectors.NaclChanges},
		{"networkGatewayChanges", selectors.NetworkGatewayChanges},
		{"rootAccountUsage", selectors.RootAccountUsage},
		{"routeTableChanges", selectors.RouteTableChanges},
		{"s3BucketPolicyChanges", selectors.S3BucketPolicyChanges},
		{"securityGroupChanges", selectors.SecurityGroupChanges},
		{"unauthorizedApiCalls", selectors.UnauthorizedApiCalls},
		{"vpcChanges", selectors.VpcChanges},
	}
	for _, c := range checks {
		if c.enabled {
			if def, ok := securityAlerts[c.key]; ok {
				result = append(result, def)
			}
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].name < result[j].name })
	return result
}

// CloudTrailSecurityAlerts provisions CloudWatch metric filters and alarms
// for security-relevant CloudTrail events. Each enabled alert creates:
//   - A LogMetricFilter on the CloudTrail log group
//   - A MetricAlarm that triggers when the filter matches >= 1 event in 5 minutes
//   - Notification via SNS topic (email) and/or Lambda (Slack/Discord/Telegram)
func CloudTrailSecurityAlerts(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	if input.Descriptor.Type != awsApi.ResourceTypeCloudTrailSecurityAlerts {
		return nil, errors.Errorf("unsupported resource type %q", input.Descriptor.Type)
	}

	cfg, ok := input.Descriptor.Config.Config.(*awsApi.CloudTrailSecurityAlertsConfig)
	if !ok {
		return nil, errors.Errorf("failed to convert config for %q", input.Descriptor.Type)
	}

	// Rehydrate credentials from the ${auth:...} reference flow
	accountConfig := &awsApi.AccountConfig{}
	if err := api.ConvertAuth(&cfg.AccountConfig, accountConfig); err != nil {
		return nil, errors.Wrapf(err, "failed to convert aws account config")
	}
	cfg.AccountConfig = *accountConfig

	if cfg.LogGroupName == "" {
		return nil, errors.New("logGroupName is required for CloudTrail security alerts")
	}

	resPrefix := input.ToResName(input.Descriptor.Name)
	alerts := enabledAlerts(cfg.Alerts)

	params.Log.Info(ctx.Context(), "configuring %d CloudTrail security alerts for log group %q", len(alerts), cfg.LogGroupName)

	if len(alerts) == 0 {
		params.Log.Info(ctx.Context(), "no security alerts enabled — skipping")
		return &api.ResourceOutput{}, nil
	}

	// If logGroupRegion differs from the default provider region, create a region-specific provider.
	// CloudTrail log groups are often in a different region than the main deployment.
	// Carry over AWS credentials from AccountConfig so non-ambient auth works.
	provider := params.Provider
	if cfg.LogGroupRegion != "" {
		providerArgs := &sdkAws.ProviderArgs{
			Region: sdk.String(cfg.LogGroupRegion),
		}
		if cfg.AccessKey != "" {
			providerArgs.AccessKey = sdk.String(cfg.AccessKey)
		}
		if cfg.SecretAccessKey != "" {
			providerArgs.SecretKey = sdk.String(cfg.SecretAccessKey)
		}
		regionProvider, err := sdkAws.NewProvider(ctx, fmt.Sprintf("%s-region-provider", resPrefix), providerArgs)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create region-specific provider for %q", cfg.LogGroupRegion)
		}
		provider = regionProvider
	}

	opts := []sdk.ResourceOption{
		sdk.Provider(provider),
	}

	var tags sdk.StringMap
	if input.StackParams != nil {
		tags = pApi.BuildTagsFromStackParams(*input.StackParams).ToAWSTags()
	}

	// Create SNS topic for email notifications (if email config provided)
	var snsTopic *sns.Topic
	if cfg.Email != nil && len(cfg.Email.Addresses) > 0 {
		var err error
		snsTopic, err = createSNSTopicForAlerts(ctx, fmt.Sprintf("%s-security-alerts", resPrefix), tags, opts...)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create SNS topic for security alerts")
		}
		if err := createSNSEmailSubscriptions(ctx, snsTopic, cfg.Email.Addresses, fmt.Sprintf("%s-security", resPrefix), opts...); err != nil {
			return nil, errors.Wrapf(err, "failed to create SNS email subscriptions")
		}
	}

	// Push helpers Lambda image to ECR if Slack/Discord/Telegram webhooks are configured.
	// The image contains the alert-formatting Lambda that delivers to webhook endpoints.
	// Lambda must pull from an ECR repo in the same region, so when logGroupRegion differs
	// from the stack's default region, we push to a region-specific ECR via a params copy
	// whose Provider matches the Lambda's region.
	hasWebhooks := cfg.Slack != nil || cfg.Discord != nil || cfg.Telegram != nil
	var helpersImage *docker.Image
	if hasWebhooks {
		if input.StackParams == nil {
			return nil, errors.New("input.StackParams is required to provision webhook-based security alerts")
		}
		helperParams := params
		helperParams.Provider = provider
		// Namespace BOTH the Pulumi resource (imageName) AND the ECR repo name so this
		// resource can coexist with compute-stack ALB alerts (which use cloud-helpers)
		// and with other aws-cloudtrail-security-alerts instances in the same stack.
		img, err := pushHelpersImageToECR(ctx, helperCfg{
			imageName:       fmt.Sprintf("%s-security-helpers", resPrefix),
			ecrRepoName:     fmt.Sprintf("%s-security-helpers", resPrefix),
			opts:            opts,
			provisionParams: helperParams,
			stack:           stack,
			deployParams:    *input.StackParams,
		})
		if err != nil {
			return nil, errors.Wrapf(err, "failed to push cloud-helpers image for security alerts")
		}
		helpersImage = img
		opts = append(opts, sdk.DependsOn([]sdk.Resource{helpersImage}))
	}

	// Create metric filter + alarm for each enabled alert
	metricNamespace := fmt.Sprintf("SC/SecurityAlerts/%s", resPrefix)

	for _, alertDef := range alerts {
		metricName := alertDef.name

		// Create CloudWatch Log Metric Filter
		filterName := fmt.Sprintf("%s-%s-filter", resPrefix, alertDef.name)
		_, err := cloudwatch.NewLogMetricFilter(ctx, filterName, &cloudwatch.LogMetricFilterArgs{
			LogGroupName: sdk.String(cfg.LogGroupName),
			Pattern:      sdk.String(alertDef.filterPattern),
			MetricTransformation: cloudwatch.LogMetricFilterMetricTransformationArgs{
				Name:      sdk.String(metricName),
				Namespace: sdk.String(metricNamespace),
				Value:     sdk.String("1"),
			},
		}, opts...)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create metric filter for %q", alertDef.name)
		}

		threshold := alertDef.threshold
		if threshold == 0 {
			threshold = 1
		}
		// createAlert suffixes cfg.name with "-execution-role" (15 chars) and Pulumi adds
		// an 8-char random suffix when it auto-names the IAM role. Cap the base so the
		// resulting physical role name stays within AWS's 64-char limit (with headroom).
		alertBaseName := util.TrimStringMiddle(fmt.Sprintf("%s-%s", resPrefix, alertDef.name), 38, "-")
		alarmArgs := cloudwatch.MetricAlarmArgs{
			AlarmDescription:   sdk.String(alertDef.description),
			MetricName:         sdk.String(metricName),
			Namespace:          sdk.String(metricNamespace),
			Statistic:          sdk.String("Sum"),
			Period:             sdk.Int(300),
			EvaluationPeriods:  sdk.Int(1),
			Threshold:          sdk.Float64(threshold),
			ComparisonOperator: sdk.String("GreaterThanOrEqualToThreshold"),
			TreatMissingData:   sdk.String("notBreaching"),
		}

		if hasWebhooks {
			// Webhook path: createAlert creates Lambda + MetricAlarm wired to Lambda (+ optional SNS email).
			if err := createAlert(ctx, alertCfg{
				name:            alertBaseName,
				description:     alertDef.description,
				slackConfig:     cfg.Slack,
				discordConfig:   cfg.Discord,
				telegramConfig:  cfg.Telegram,
				deployParams:    *input.StackParams,
				secretSuffix:    resPrefix,
				helpersImage:    helpersImage,
				snsTopic:        snsTopic,
				opts:            opts,
				tags:            tags,
				metricAlarmArgs: alarmArgs,
			}); err != nil {
				return nil, errors.Wrapf(err, "failed to create alert %q", alertDef.name)
			}
		} else {
			// Email-only (or unnotified) path: create the alarm directly, wired to SNS if present.
			if snsTopic != nil {
				actions := sdk.Array{snsTopic.Arn}
				alarmArgs.AlarmActions = actions
				alarmArgs.OkActions = actions
			}
			alarmArgs.Tags = tags
			if _, err := cloudwatch.NewMetricAlarm(ctx, fmt.Sprintf("%s-alarm", alertBaseName), &alarmArgs, opts...); err != nil {
				return nil, errors.Wrapf(err, "failed to create alarm for %q", alertDef.name)
			}
		}

		params.Log.Info(ctx.Context(), "  created security alert: %s", alertDef.name)
	}

	params.Log.Info(ctx.Context(), "CloudTrail security alerts configured: %d alerts active (webhooks=%v, email=%v)",
		len(alerts), hasWebhooks, snsTopic != nil)

	return &api.ResourceOutput{}, nil
}
