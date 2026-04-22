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
// Alert names reference AWS Security Hub/CIS CloudWatch controls (CloudWatch.1-14).
type CloudTrailAlertSelectors struct {
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
}

func ReadCloudTrailSecurityAlertsConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &CloudTrailSecurityAlertsConfig{})
}
