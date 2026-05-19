package aws

import (
	"fmt"
	"sort"
	"strings"

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
// Defaults (zero values) are documented per field.
type securityAlertDef struct {
	name              string
	description       string
	filterPattern     string
	threshold         float64 // default 1 when zero
	period            int     // alarm period seconds; default 300 when zero
	evaluationPeriods int     // default 1 when zero
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
	// CloudWatch.3 — Console login without MFA (successful only).
	//
	// Scoped to userIdentity.type = "IAMUser". AWS Identity Center / federated logins
	// always emit ConsoleLogin events with additionalEventData.MFAUsed = "No" because
	// MFA is enforced upstream at the IdP rather than at the AWS console step. Without
	// this scope the detector pages every SSO console session — a well-documented
	// CIS CloudWatch.3 false positive. Identity Center sessions should be audited via
	// signin.amazonaws.com UserAuthentication events (not currently provisioned by
	// this plugin; surface via a separate detector if needed).
	"consoleLoginWithoutMfa": {
		name:          "ct-console-login-no-mfa",
		description:   "Successful IAM user console login without MFA detected",
		filterPattern: `{ ($.eventName = "ConsoleLogin") && ($.additionalEventData.MFAUsed != "Yes") && ($.responseElements.ConsoleLogin = "Success") && ($.userIdentity.type = "IAMUser") }`,
	},
	// CloudWatch.4 — IAM policy changes
	// SetDefaultPolicyVersion is included: flipping a managed policy's default version
	// changes the effective permissions granted to every principal that has the policy
	// attached, which is a common privilege-escalation path.
	"iamPolicyChanges": {
		name:        "ct-iam-policy-changes",
		description: "IAM policy or role modified — verify authorization",
		filterPattern: `{ ($.eventName = "DeleteGroupPolicy") || ($.eventName = "DeleteRolePolicy") || ` +
			`($.eventName = "DeleteUserPolicy") || ($.eventName = "PutGroupPolicy") || ` +
			`($.eventName = "PutRolePolicy") || ($.eventName = "PutUserPolicy") || ` +
			`($.eventName = "CreatePolicy") || ($.eventName = "DeletePolicy") || ` +
			`($.eventName = "CreatePolicyVersion") || ($.eventName = "DeletePolicyVersion") || ` +
			`($.eventName = "SetDefaultPolicyVersion") || ` +
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

	// Beyond-CIS — GuardDuty disabled/blinded.
	// Why: CIS CloudWatch.9 (configChanges) tracks AWS Config recorder ops but not the
	// other detective controls. Disabling GuardDuty is a classic attacker-blinding move
	// because findings stop flowing and historical findings can be deleted.
	"guardDutyDisabled": {
		name:        "ct-guardduty-disabled",
		description: "GuardDuty detector disabled, deleted, or members removed — security visibility lost",
		filterPattern: `{ ($.eventSource = "guardduty.amazonaws.com") && (($.eventName = "DeleteDetector") || ` +
			`($.eventName = "UpdateDetector") || ($.eventName = "DisassociateMembers") || ` +
			`($.eventName = "DeleteMembers") || ($.eventName = "StopMonitoringMembers")) }`,
	},

	// Beyond-CIS — Security Hub disabled or standards turned off.
	// Why: same attacker-blinding category as GuardDuty.
	"securityHubDisabled": {
		name:        "ct-securityhub-disabled",
		description: "Security Hub disabled or standards/import subscriptions turned off — security visibility lost",
		filterPattern: `{ ($.eventSource = "securityhub.amazonaws.com") && (($.eventName = "DisableSecurityHub") || ` +
			`($.eventName = "BatchDisableStandards") || ($.eventName = "DisableImportFindingsForProduct") || ` +
			`($.eventName = "DeleteActionTarget") || ($.eventName = "DeleteInsight")) }`,
	},

	// Beyond-CIS — IAM access key creation.
	// Why: persistent credentials are higher-risk than short-lived STS tokens. Creation
	// without a documented rotation/expiry path is a common audit finding.
	"accessKeyCreation": {
		name:          "ct-access-key-creation",
		description:   "IAM access key created — verify rotation policy and owner attestation",
		filterPattern: `{ ($.eventSource = "iam.amazonaws.com") && ($.eventName = "CreateAccessKey") }`,
	},

	// Beyond-CIS — S3 Block Public Access (BPA) toggled at account or bucket scope.
	// Why: CIS CloudWatch.8 only catches bucket policy edits; BPA is the higher-leverage
	// gate. Turning it off is a single click that exposes previously private buckets.
	"s3PublicAccessChanges": {
		name:        "ct-s3-public-access-changes",
		description: "S3 Block Public Access settings modified — possible exposure path opened",
		filterPattern: `{ ($.eventSource = "s3.amazonaws.com") && (($.eventName = "PutAccountPublicAccessBlock") || ` +
			`($.eventName = "PutBucketPublicAccessBlock") || ($.eventName = "DeleteAccountPublicAccessBlock") || ` +
			`($.eventName = "DeleteBucketPublicAccessBlock")) }`,
	},

	// Beyond-CIS — Lambda Function URL created or updated with AuthType=NONE.
	// Why: a Function URL with AuthType=NONE is a public HTTPS endpoint with the function's
	// IAM role behind it; one click of misconfiguration exposes whatever the function can do.
	// We observed anonymous Azure-IP scanners probing GetFunctionUrlConfig in the wild — they
	// will hit a real endpoint the moment one exists.
	"lambdaUrlPublic": {
		name:          "ct-lambda-url-public",
		description:   "Lambda Function URL created or updated with AuthType=NONE — public endpoint exposed",
		filterPattern: `{ ($.eventSource = "lambda.amazonaws.com") && (($.eventName = "CreateFunctionUrlConfig") || ($.eventName = "UpdateFunctionUrlConfig")) && ($.requestParameters.authType = "NONE") }`,
	},

	// Beyond-CIS — KMS key policy / grant changes.
	// Why: CIS CloudWatch.7 catches key deletion but not policy edits. PutKeyPolicy / CreateGrant
	// can quietly hand decrypt rights to a new principal without touching the key's lifecycle.
	"kmsKeyPolicyChanges": {
		name:        "ct-kms-key-policy-changes",
		description: "KMS key policy modified or grant created — verify principal scope and conditions",
		filterPattern: `{ ($.eventSource = "kms.amazonaws.com") && (($.eventName = "PutKeyPolicy") || ` +
			`($.eventName = "PutResourcePolicy") || ($.eventName = "CreateGrant") || ` +
			`($.eventName = "RetireGrant") || ($.eventName = "RevokeGrant")) }`,
	},

	// Beyond-CIS — AWS Organizations / SCP changes.
	// Why: SCPs are the strongest preventative control in a multi-account org. Detaching
	// or deleting one widens blast radius across every account it covered.
	"organizationsChanges": {
		name:        "ct-organizations-changes",
		description: "AWS Organizations policy modified — verify SCP boundaries",
		filterPattern: `{ ($.eventSource = "organizations.amazonaws.com") && (($.eventName = "CreatePolicy") || ` +
			`($.eventName = "DeletePolicy") || ($.eventName = "UpdatePolicy") || ($.eventName = "AttachPolicy") || ` +
			`($.eventName = "DetachPolicy") || ($.eventName = "EnablePolicyType") || ($.eventName = "DisablePolicyType") || ` +
			`($.eventName = "LeaveOrganization") || ($.eventName = "RemoveAccountFromOrganization")) }`,
	},

	// Beyond-CIS — Anonymous external probes (recon).
	// Why: userIdentity.type=AWSAccount represents another AWS account (or unauthenticated
	// AWS API client) hitting our resources. We observed ~400/14d hits in the wild scanning
	// for exposed Lambda Function URLs. Default threshold=10 so individual probes don't
	// page — we only care about sustained scanning from one source.
	"anonymousProbes": {
		name:        "ct-anonymous-probes",
		description: "Anonymous external account probing AWS resources — possible reconnaissance",
		filterPattern: `{ ($.userIdentity.type = "AWSAccount") && (($.errorCode = "AccessDenied") || ` +
			`($.errorCode = "AccessDeniedException") || ($.errorCode = "UnauthorizedAccess") || ` +
			`($.errorCode = "Client.UnauthorizedAccess") || ($.errorCode = "Client.UnauthorizedOperation") || ` +
			`($.errorCode = "UnauthorizedOperation")) }`,
		threshold: 10,
	},
}

// enabledAlerts returns the alert definitions that are enabled in the selector config.
// Per-detector overrides (exclusions, threshold, period) are baked into the returned
// definitions here so downstream provisioning code sees a single resolved struct per alert.
// Results are sorted by name for deterministic Pulumi resource ordering.
func enabledAlerts(selectors awsApi.CloudTrailAlertSelectors) []securityAlertDef {
	var result []securityAlertDef
	checks := []struct {
		key     string
		enabled bool
	}{
		// CIS CloudWatch.1-14
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
		// Beyond-CIS
		{"guardDutyDisabled", selectors.GuardDutyDisabled},
		{"securityHubDisabled", selectors.SecurityHubDisabled},
		{"accessKeyCreation", selectors.AccessKeyCreation},
		{"s3PublicAccessChanges", selectors.S3PublicAccessChanges},
		{"lambdaUrlPublic", selectors.LambdaUrlPublic},
		{"kmsKeyPolicyChanges", selectors.KmsKeyPolicyChanges},
		{"organizationsChanges", selectors.OrganizationsChanges},
		{"anonymousProbes", selectors.AnonymousProbes},
	}
	for _, c := range checks {
		if !c.enabled {
			continue
		}
		def, ok := securityAlerts[c.key]
		if !ok {
			continue
		}
		if ov, hasOv := selectors.Overrides[c.key]; hasOv {
			def = applyOverride(def, ov)
		}
		result = append(result, def)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].name < result[j].name })
	return result
}

// applyOverride returns a copy of def with the override applied:
//   - exclusion clauses are appended to the filter pattern as `&& (field != "val")`;
//   - threshold/period/evaluationPeriods overrides replace the defaults when non-zero.
//
// The original filter pattern is wrapped in parentheses so the appended NOT-clauses
// AND with the entire base predicate, not just its last OR-term. Example:
//
//	base:    { ($.eventName = "PutRolePolicy") || ($.eventName = "AttachRolePolicy") }
//	out:     { ( ($.eventName = "PutRolePolicy") || ($.eventName = "AttachRolePolicy") ) && ($.userIdentity.userName != "integrail-deployer-bot") }
//
// CloudWatch metric filter pattern syntax reference:
//
//	https://docs.aws.amazon.com/AmazonCloudWatch/latest/logs/FilterAndPatternSyntax.html
func applyOverride(def securityAlertDef, ov awsApi.CloudTrailAlertOverride) securityAlertDef {
	if ov.Threshold > 0 {
		def.threshold = ov.Threshold
	}
	if ov.Period > 0 {
		def.period = ov.Period
	}
	if ov.EvaluationPeriods > 0 {
		def.evaluationPeriods = ov.EvaluationPeriods
	}

	clauses := buildExclusionClauses(ov)
	if len(clauses) == 0 {
		return def
	}

	// Strip the outer braces from the base pattern so we can wrap the predicate
	// in parens before AND'ing the exclusions. CloudWatch JSON filter patterns
	// are always braced.
	base := strings.TrimSpace(def.filterPattern)
	base = strings.TrimPrefix(base, "{")
	base = strings.TrimSuffix(base, "}")
	base = strings.TrimSpace(base)

	def.filterPattern = "{ (" + base + ") && " + strings.Join(clauses, " && ") + " }"
	return def
}

// buildExclusionClauses turns an override into a list of `(field != "val")` clauses
// in deterministic order. The list is empty when the override has no exclusions.
func buildExclusionClauses(ov awsApi.CloudTrailAlertOverride) []string {
	var clauses []string
	add := func(field string, values []string) {
		// Sort to keep the generated filter pattern deterministic across deploys
		// (so Pulumi sees a stable resource and doesn't churn the filter on every
		// run when the user reorders the YAML list).
		vs := append([]string(nil), values...)
		sort.Strings(vs)
		for _, v := range vs {
			if v == "" {
				continue
			}
			clauses = append(clauses, fmt.Sprintf(`($.%s != %q)`, field, v))
		}
	}
	add("userIdentity.userName", ov.ExcludeUserNames)
	add("userIdentity.principalId", ov.ExcludePrincipalIds)
	add("userIdentity.arn", ov.ExcludeUserArns)
	add("userIdentity.arn", ov.ExcludeUserArnGlobs) // CloudWatch supports * within the quoted value
	add("userIdentity.type", ov.ExcludeUserTypes)
	add("userIdentity.invokedBy", ov.ExcludeInvokedBy)
	return clauses
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

	// Pre-flight: if the user declared a trailName, verify the trail has
	// log-file validation turned on BEFORE we go ahead and provision metric
	// filters / alarms / Lambdas. Running the security-alerts stack on top
	// of a trail without integrity signing is a silent compliance gap —
	// refuse to deploy unless the user has explicitly downgraded the check
	// to a warning via requireLogFileValidation: false.
	if cfg.TrailName != "" {
		outcome, err := ensureTrailLogFileValidation(ctx.Context(), cfg)
		if err != nil {
			return nil, errors.Wrap(err, "CloudTrail trail pre-flight failed")
		}
		if outcome.Enabled {
			params.Log.Info(ctx.Context(), outcome.Message)
		} else {
			// Warning path: log but don't fail (user opted into soft mode).
			params.Log.Warn(ctx.Context(), outcome.Message)
		}
	}

	// resPrefix carries the SC environment suffix (e.g. `cloudtrail-security--prod`).
	// CloudTrail log groups are account-wide, so this resource should be declared in
	// exactly one environment block per AWS account — declaring it in multiple envs
	// within the same account creates duplicate metric filters that all match the same
	// events, producing duplicate notifications. Keeping the env suffix in the prefix
	// is still correct: if two environments target *different* AWS accounts, each
	// account gets its own independent filter/alarm/Lambda set.
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
		period := alertDef.period
		if period == 0 {
			period = 300
		}
		evalPeriods := alertDef.evaluationPeriods
		if evalPeriods == 0 {
			evalPeriods = 1
		}
		// createAlert suffixes cfg.name with "-execution-role" (15 chars) and Pulumi adds
		// an 8-char random suffix when it auto-names the IAM role. Cap the base so the
		// resulting physical role name stays within AWS's 64-char limit (with headroom).
		//
		// Concat order is alertDef.name-then-resPrefix so TrimStringWithHash's prefix
		// retention keeps the alert-distinguishing segment in the output. If the order
		// were flipped, a long descriptor + env prefix could eat the entire alert name
		// during truncation, leaving only the 4-char hash to disambiguate — which is a
		// ~0.15% birthday-collision surface across the 14 CIS alerts.
		alertBaseName := util.TrimStringWithHash(fmt.Sprintf("%s-%s", alertDef.name, resPrefix), 38, "-")
		alarmArgs := cloudwatch.MetricAlarmArgs{
			AlarmDescription:   sdk.String(alertDef.description),
			MetricName:         sdk.String(metricName),
			Namespace:          sdk.String(metricNamespace),
			Statistic:          sdk.String("Sum"),
			Period:             sdk.Int(period),
			EvaluationPeriods:  sdk.Int(evalPeriods),
			Threshold:          sdk.Float64(threshold),
			ComparisonOperator: sdk.String("GreaterThanOrEqualToThreshold"),
			TreatMissingData:   sdk.String("notBreaching"),
		}

		if hasWebhooks {
			// Webhook path: createAlert creates Lambda + MetricAlarm wired to Lambda (+ optional SNS email).
			// Pass the CT log-group details so the Lambda handler can look up
			// the actual events that fed the alarm and include actor/time/IP
			// in the notification — otherwise the message is just the alarm
			// description and reviewers have to click through to the console
			// for every alert.
			alertRegion := cfg.LogGroupRegion
			ctLogGroupArn := cloudTrailLogGroupArn(cfg, alertRegion)
			if err := createAlert(ctx, alertCfg{
				name:             alertBaseName,
				description:      alertDef.description,
				slackConfig:      cfg.Slack,
				discordConfig:    cfg.Discord,
				telegramConfig:   cfg.Telegram,
				deployParams:     *input.StackParams,
				secretSuffix:     resPrefix,
				helpersImage:     helpersImage,
				snsTopic:         snsTopic,
				opts:             opts,
				tags:             tags,
				metricAlarmArgs:  alarmArgs,
				ctLogGroupName:   cfg.LogGroupName,
				ctLogGroupRegion: alertRegion,
				ctFilterPattern:  alertDef.filterPattern,
				ctLogGroupArn:    ctLogGroupArn,
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

// cloudTrailLogGroupArn builds the CloudTrail log-group ARN so the alert
// Lambda's IAM policy can grant logs:FilterLogEvents scoped to just that
// group. When `region` is empty we use the IAM wildcard `*` in the region
// segment — the ARN still pins down account + log-group name, so this is
// a scoped grant, just region-agnostic. Without this fallback the same-
// region case (where users don't set `logGroupRegion` because the log
// group lives in the stack's default region) would ship a policy that
// omits the FilterLogEvents statement, the Lambda would hit AccessDenied,
// and every alert would silently lose its enrichment.
//
// Returns nil only when we genuinely can't construct an ARN (missing
// account id or log-group name). The caller then skips the CT policy
// statement, the Lambda hits AccessDenied on FilterLogEvents, and the
// handler logs a warning — alerts still go out, just without enrichment.
func cloudTrailLogGroupArn(cfg *awsApi.CloudTrailSecurityAlertsConfig, region string) sdk.StringInput {
	if cfg.AccountConfig.Account == "" || cfg.LogGroupName == "" {
		return nil
	}
	regionSeg := region
	if regionSeg == "" {
		regionSeg = "*"
	}
	return sdk.String(fmt.Sprintf("arn:aws:logs:%s:%s:log-group:%s",
		regionSeg, cfg.AccountConfig.Account, cfg.LogGroupName))
}
