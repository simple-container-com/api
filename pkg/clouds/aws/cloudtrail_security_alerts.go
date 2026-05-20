package aws

import "github.com/simple-container-com/api/pkg/api"

const ResourceTypeCloudTrailSecurityAlerts = "aws-cloudtrail-security-alerts"

// CloudTrailSecurityAlertsConfig defines CloudWatch metric filters and alarms
// for security-relevant CloudTrail events (SOC2 CC6.1/CC7.1, ISO 27001 A.8.2/A.8.15/A.8.16).
type CloudTrailSecurityAlertsConfig struct {
	AccountConfig  `json:",inline" yaml:",inline"`
	LogGroupName   string                   `json:"logGroupName" yaml:"logGroupName"`
	LogGroupRegion string                   `json:"logGroupRegion,omitempty" yaml:"logGroupRegion,omitempty"`
	Slack          *api.SlackCfg            `json:"slack,omitempty" yaml:"slack,omitempty"`
	Discord        *api.DiscordCfg          `json:"discord,omitempty" yaml:"discord,omitempty"`
	Telegram       *api.TelegramCfg         `json:"telegram,omitempty" yaml:"telegram,omitempty"`
	Email          *api.EmailCfg            `json:"email,omitempty" yaml:"email,omitempty"`
	Alerts         CloudTrailAlertSelectors `json:"alerts" yaml:"alerts"`
	// Trail pre-flight check (SOC2 CC7.1 / ISO 27001 A.8.15): when TrailName
	// is set, SC inspects the referenced CloudTrail trail at provision time
	// and refuses to deploy unless log-file validation is enabled. This
	// prevents a silent gap where the metric filters + alarms happily match
	// events in a trail whose log files are not tamper-detectable.
	//
	// TrailName is the friendly name of the CloudTrail trail (not the ARN).
	// AWS accepts either, but friendly name is what `aws cloudtrail
	// describe-trails` returns by default and what users actually see in
	// the console.
	//
	// When TrailName is empty the check is skipped (back-compat for
	// deployments that don't want SC touching the trail). Set explicitly to
	// opt into the guardrail.
	TrailName string `json:"trailName,omitempty" yaml:"trailName,omitempty"`
	// When true (default, if TrailName is set) the provisioner FAILS deploy
	// if log-file validation is off. When false, SC only logs a warning
	// and continues — use during incremental rollout, not as a permanent
	// escape hatch.
	RequireLogFileValidation *bool `json:"requireLogFileValidation,omitempty" yaml:"requireLogFileValidation,omitempty"`
}

// RequiresTrailValidation reports whether the config has opted into the
// pre-flight check. True iff TrailName is set AND RequireLogFileValidation
// is either unset (default true) or explicitly true.
func (c CloudTrailSecurityAlertsConfig) RequiresTrailValidation() bool {
	if c.TrailName == "" {
		return false
	}
	if c.RequireLogFileValidation == nil {
		return true
	}
	return *c.RequireLogFileValidation
}

// CloudTrailAlertSelectors controls which security alerts are enabled.
// Each field maps to a CloudWatch metric filter + alarm on the CloudTrail log group.
// Built-in detectors track AWS Security Hub/CIS CloudWatch controls (CloudWatch.1-14)
// plus a set of high-value additions covering attacker-blinding moves and exposure paths
// that CIS does not cover (GuardDuty/SecurityHub disable, IAM access keys, S3 Block Public
// Access, public Lambda Function URLs, KMS key policy edits, AWS Organizations changes,
// anonymous external probes).
type CloudTrailAlertSelectors struct {
	// CIS CloudWatch.1-14
	RootAccountUsage       bool `json:"rootAccountUsage,omitempty" yaml:"rootAccountUsage,omitempty"`             // CIS CloudWatch.1
	UnauthorizedApiCalls   bool `json:"unauthorizedApiCalls,omitempty" yaml:"unauthorizedApiCalls,omitempty"`     // CIS CloudWatch.2
	ConsoleLoginWithoutMfa bool `json:"consoleLoginWithoutMfa,omitempty" yaml:"consoleLoginWithoutMfa,omitempty"` // CIS CloudWatch.3
	IamPolicyChanges       bool `json:"iamPolicyChanges,omitempty" yaml:"iamPolicyChanges,omitempty"`             // CIS CloudWatch.4
	CloudTrailTampering    bool `json:"cloudTrailTampering,omitempty" yaml:"cloudTrailTampering,omitempty"`       // CIS CloudWatch.5
	FailedConsoleLogins    bool `json:"failedConsoleLogins,omitempty" yaml:"failedConsoleLogins,omitempty"`       // CIS CloudWatch.6
	KmsKeyDeletion         bool `json:"kmsKeyDeletion,omitempty" yaml:"kmsKeyDeletion,omitempty"`                 // CIS CloudWatch.7
	S3BucketPolicyChanges  bool `json:"s3BucketPolicyChanges,omitempty" yaml:"s3BucketPolicyChanges,omitempty"`   // CIS CloudWatch.8
	ConfigChanges          bool `json:"configChanges,omitempty" yaml:"configChanges,omitempty"`                   // CIS CloudWatch.9
	SecurityGroupChanges   bool `json:"securityGroupChanges,omitempty" yaml:"securityGroupChanges,omitempty"`     // CIS CloudWatch.10
	NaclChanges            bool `json:"naclChanges,omitempty" yaml:"naclChanges,omitempty"`                       // CIS CloudWatch.11
	NetworkGatewayChanges  bool `json:"networkGatewayChanges,omitempty" yaml:"networkGatewayChanges,omitempty"`   // CIS CloudWatch.12
	RouteTableChanges      bool `json:"routeTableChanges,omitempty" yaml:"routeTableChanges,omitempty"`           // CIS CloudWatch.13
	VpcChanges             bool `json:"vpcChanges,omitempty" yaml:"vpcChanges,omitempty"`                         // CIS CloudWatch.14

	// Beyond-CIS detectors. Default off so existing deployments don't gain new alerts on plugin upgrade.
	GuardDutyDisabled     bool `json:"guardDutyDisabled,omitempty" yaml:"guardDutyDisabled,omitempty"`         // GuardDuty disabled/detector deleted
	SecurityHubDisabled   bool `json:"securityHubDisabled,omitempty" yaml:"securityHubDisabled,omitempty"`     // Security Hub disabled or standards turned off
	AccessKeyCreation     bool `json:"accessKeyCreation,omitempty" yaml:"accessKeyCreation,omitempty"`         // CreateAccessKey on any IAM user
	S3PublicAccessChanges bool `json:"s3PublicAccessChanges,omitempty" yaml:"s3PublicAccessChanges,omitempty"` // Block Public Access toggled at account or bucket scope
	LambdaUrlPublic       bool `json:"lambdaUrlPublic,omitempty" yaml:"lambdaUrlPublic,omitempty"`             // Lambda Function URL created/updated with AuthType=NONE
	// KMS-related detectors are split into two by signal density:
	//   KmsKeyPolicy  — PutKeyPolicy: rare, real signal, default threshold 1. The structural
	//                   change to "who can use this key" — page on any occurrence.
	//   KmsKeyGrants  — CreateGrant / RetireGrant / RevokeGrant: high-volume in any IaC-driven
	//                   environment (Pulumi/Terraform issue a grant per encrypted resource).
	//                   Default off; turn on only with the override.threshold tuned to your
	//                   deploy cadence. Splitting was driven by production data showing
	//                   ~25 CreateGrant/hour from one Pulumi bot account that would have
	//                   buried a single PutKeyPolicy signal.
	KmsKeyPolicy         bool `json:"kmsKeyPolicy,omitempty" yaml:"kmsKeyPolicy,omitempty"`                 // PutKeyPolicy (rare; default threshold 1)
	KmsKeyGrants         bool `json:"kmsKeyGrants,omitempty" yaml:"kmsKeyGrants,omitempty"`                 // CreateGrant / RetireGrant / RevokeGrant (high-volume; default threshold 10)
	OrganizationsChanges bool `json:"organizationsChanges,omitempty" yaml:"organizationsChanges,omitempty"` // SCP / account-membership churn in AWS Organizations
	AnonymousProbes      bool `json:"anonymousProbes,omitempty" yaml:"anonymousProbes,omitempty"`           // userIdentity.type=AWSAccount AccessDenied probes from public IPs

	// Overrides allows per-detector tuning without forking the plugin. Keyed by detector
	// selector name (e.g. "iamPolicyChanges"). An override can:
	//   - bake exclusion clauses into the CloudWatch metric filter pattern (preferred
	//     for governed automation that contributes nothing but noise — Pulumi CI bots,
	//     known scanners, AWS service-linked roles);
	//   - raise the alarm threshold so a single matched event no longer trips the alarm
	//     (CIS Benchmark guidance for unauthorized-api-calls);
	//   - adjust the evaluation window.
	//
	// Suppression happens at the metric-filter layer, not in the Lambda enrichment
	// step — excluded events never increment the metric, the alarm never trips, and
	// no Slack notification is sent. The CW alarm dashboard therefore reflects real
	// signal rather than known-good noise (preserves SOC2/ISO audit clarity).
	Overrides map[string]CloudTrailAlertOverride `json:"overrides,omitempty" yaml:"overrides,omitempty"`
}

// CloudTrailAlertOverride tunes a single detector. All fields are optional; zero values
// mean "use plugin default."
type CloudTrailAlertOverride struct {
	// Threshold raises the alarm threshold (events per period). 0 = use the plugin default
	// for this detector (1 for most, 5 for unauthorizedApiCalls, 10 for anonymousProbes).
	Threshold float64 `json:"threshold,omitempty" yaml:"threshold,omitempty"`
	// Period in seconds. 0 = 300 (5 min). CloudWatch supports 60, 300, 3600.
	Period int `json:"period,omitempty" yaml:"period,omitempty"`
	// EvaluationPeriods is how many consecutive periods must breach. 0 = 1.
	EvaluationPeriods int `json:"evaluationPeriods,omitempty" yaml:"evaluationPeriods,omitempty"`

	// ExcludeUserNames excludes events where $.userIdentity.userName matches the given
	// names exactly. Useful for IAMUser-type principals like CI bot accounts.
	ExcludeUserNames []string `json:"excludeUserNames,omitempty" yaml:"excludeUserNames,omitempty"`
	// ExcludePrincipalIds excludes by $.userIdentity.principalId (AIDA..., AROA..., AKIA...).
	ExcludePrincipalIds []string `json:"excludePrincipalIds,omitempty" yaml:"excludePrincipalIds,omitempty"`
	// ExcludeUserArns excludes by exact $.userIdentity.arn match.
	ExcludeUserArns []string `json:"excludeUserArns,omitempty" yaml:"excludeUserArns,omitempty"`
	// ExcludeUserArnGlobs excludes by $.userIdentity.arn glob (CloudWatch metric filter
	// patterns support * within string values, e.g.
	// "arn:aws:sts::*:assumed-role/AWSServiceRoleFor*/*"). One glob per list entry.
	ExcludeUserArnGlobs []string `json:"excludeUserArnGlobs,omitempty" yaml:"excludeUserArnGlobs,omitempty"`
	// ExcludeUserTypes excludes by $.userIdentity.type (e.g. "AWSService", "AWSAccount",
	// "AssumedRole"). Useful for stripping AWS internal plumbing from unauthorized-api-calls.
	ExcludeUserTypes []string `json:"excludeUserTypes,omitempty" yaml:"excludeUserTypes,omitempty"`
	// ExcludeInvokedBy excludes by $.userIdentity.invokedBy (e.g. "s3.amazonaws.com").
	// This is the canonical way to strip AWS service self-probes.
	ExcludeInvokedBy []string `json:"excludeInvokedBy,omitempty" yaml:"excludeInvokedBy,omitempty"`
}

func ReadCloudTrailSecurityAlertsConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &CloudTrailSecurityAlertsConfig{})
}
