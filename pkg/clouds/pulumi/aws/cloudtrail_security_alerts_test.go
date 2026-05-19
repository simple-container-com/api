package aws

import (
	"strings"
	"testing"

	. "github.com/onsi/gomega"

	awsApi "github.com/simple-container-com/api/pkg/clouds/aws"
)

// totalDetectors is the count of built-in security detectors. Update when adding new ones.
// Composition: 14 CIS CloudWatch.1-14 + 8 beyond-CIS additions.
const totalDetectors = 22

func TestEnabledAlerts_AllEnabled(t *testing.T) {
	RegisterTestingT(t)

	selectors := awsApi.CloudTrailAlertSelectors{
		// CIS
		RootAccountUsage:       true,
		UnauthorizedApiCalls:   true,
		ConsoleLoginWithoutMfa: true,
		IamPolicyChanges:       true,
		CloudTrailTampering:    true,
		FailedConsoleLogins:    true,
		KmsKeyDeletion:         true,
		S3BucketPolicyChanges:  true,
		ConfigChanges:          true,
		SecurityGroupChanges:   true,
		NaclChanges:            true,
		NetworkGatewayChanges:  true,
		RouteTableChanges:      true,
		VpcChanges:             true,
		// Beyond-CIS
		GuardDutyDisabled:     true,
		SecurityHubDisabled:   true,
		AccessKeyCreation:     true,
		S3PublicAccessChanges: true,
		LambdaUrlPublic:       true,
		KmsKeyPolicyChanges:   true,
		OrganizationsChanges:  true,
		AnonymousProbes:       true,
	}

	alerts := enabledAlerts(selectors)
	Expect(alerts).To(HaveLen(totalDetectors))

	names := make(map[string]bool)
	for _, a := range alerts {
		names[a.name] = true
		Expect(a.description).ToNot(BeEmpty())
		Expect(a.filterPattern).ToNot(BeEmpty())
	}

	for _, expected := range []string{
		"ct-root-account-usage",
		"ct-unauthorized-api-calls",
		"ct-console-login-no-mfa",
		"ct-iam-policy-changes",
		"ct-cloudtrail-tampering",
		"ct-failed-console-logins",
		"ct-kms-key-deletion",
		"ct-s3-bucket-policy-changes",
		"ct-config-changes",
		"ct-security-group-changes",
		"ct-nacl-changes",
		"ct-network-gateway-changes",
		"ct-route-table-changes",
		"ct-vpc-changes",
		"ct-guardduty-disabled",
		"ct-securityhub-disabled",
		"ct-access-key-creation",
		"ct-s3-public-access-changes",
		"ct-lambda-url-public",
		"ct-kms-key-policy-changes",
		"ct-organizations-changes",
		"ct-anonymous-probes",
	} {
		Expect(names).To(HaveKey(expected))
	}
}

func TestConsoleLoginWithoutMfa_RestrictedToIAMUser(t *testing.T) {
	// The CIS CloudWatch.3 pattern fires on AWS Identity Center sessions by default
	// because MFAUsed=No (MFA happens upstream). Constraining to IAMUser type avoids
	// this well-known false positive.
	RegisterTestingT(t)
	Expect(securityAlerts["consoleLoginWithoutMfa"].filterPattern).To(
		ContainSubstring(`$.userIdentity.type = "IAMUser"`))
}

func TestAnonymousProbes_DefaultThreshold(t *testing.T) {
	// Single anonymous probe is not actionable; default threshold 10 in 5min reflects
	// "sustained reconnaissance" rather than one-off enumeration.
	RegisterTestingT(t)
	Expect(securityAlerts["anonymousProbes"].threshold).To(Equal(float64(10)))
}

func TestEnabledAlerts_PartialEnabled(t *testing.T) {
	RegisterTestingT(t)

	selectors := awsApi.CloudTrailAlertSelectors{
		RootAccountUsage:    true,
		CloudTrailTampering: true,
		FailedConsoleLogins: true,
	}

	alerts := enabledAlerts(selectors)
	Expect(alerts).To(HaveLen(3))
}

func TestEnabledAlerts_NoneEnabled(t *testing.T) {
	RegisterTestingT(t)

	selectors := awsApi.CloudTrailAlertSelectors{}
	alerts := enabledAlerts(selectors)
	Expect(alerts).To(BeEmpty())
}

func TestEnabledAlerts_Deterministic(t *testing.T) {
	RegisterTestingT(t)

	selectors := awsApi.CloudTrailAlertSelectors{
		VpcChanges:           true,
		RootAccountUsage:     true,
		FailedConsoleLogins:  true,
		NaclChanges:          true,
		UnauthorizedApiCalls: true,
	}

	// Run multiple times to verify deterministic ordering
	for i := 0; i < 10; i++ {
		alerts := enabledAlerts(selectors)
		Expect(alerts).To(HaveLen(5))
		Expect(alerts[0].name).To(Equal("ct-failed-console-logins"))
		Expect(alerts[1].name).To(Equal("ct-nacl-changes"))
		Expect(alerts[2].name).To(Equal("ct-root-account-usage"))
		Expect(alerts[3].name).To(Equal("ct-unauthorized-api-calls"))
		Expect(alerts[4].name).To(Equal("ct-vpc-changes"))
	}
}

func TestSecurityAlertDefinitions(t *testing.T) {
	RegisterTestingT(t)

	Expect(securityAlerts).To(HaveLen(totalDetectors))

	for key, def := range securityAlerts {
		t.Run(key, func(t *testing.T) {
			RegisterTestingT(t)
			Expect(def.name).ToNot(BeEmpty(), "alert %q missing name", key)
			Expect(def.description).ToNot(BeEmpty(), "alert %q missing description", key)
			Expect(def.filterPattern).ToNot(BeEmpty(), "alert %q missing filterPattern", key)
			Expect(def.filterPattern[0]).To(Equal(byte('{')), "alert %q filter pattern should start with {", key)
		})
	}
}

func TestApplyOverride_Exclusions(t *testing.T) {
	RegisterTestingT(t)

	base := securityAlerts["iamPolicyChanges"]
	out := applyOverride(base, awsApi.CloudTrailAlertOverride{
		ExcludeUserNames: []string{"integrail-deployer-bot"},
	})

	// Base pattern is preserved (wrapped in parens), exclusion is AND'd on
	Expect(out.filterPattern).To(HavePrefix(`{ (`))
	Expect(out.filterPattern).To(HaveSuffix(` }`))
	Expect(out.filterPattern).To(ContainSubstring(`PutRolePolicy`)) // base predicate intact
	Expect(out.filterPattern).To(ContainSubstring(`($.userIdentity.userName != "integrail-deployer-bot")`))
}

func TestApplyOverride_MultipleExclusionFields(t *testing.T) {
	RegisterTestingT(t)

	out := applyOverride(securityAlerts["unauthorizedApiCalls"], awsApi.CloudTrailAlertOverride{
		ExcludeUserNames: []string{"prowler-readonly"},
		ExcludeUserTypes: []string{"AWSService", "AWSAccount"},
		ExcludeInvokedBy: []string{"s3.amazonaws.com", "lambda.amazonaws.com"},
		ExcludeUserArnGlobs: []string{
			"arn:aws:sts::*:assumed-role/AWSServiceRoleFor*/*",
		},
	})

	Expect(out.filterPattern).To(ContainSubstring(`($.userIdentity.userName != "prowler-readonly")`))
	Expect(out.filterPattern).To(ContainSubstring(`($.userIdentity.type != "AWSService")`))
	Expect(out.filterPattern).To(ContainSubstring(`($.userIdentity.type != "AWSAccount")`))
	Expect(out.filterPattern).To(ContainSubstring(`($.userIdentity.invokedBy != "s3.amazonaws.com")`))
	Expect(out.filterPattern).To(ContainSubstring(`($.userIdentity.invokedBy != "lambda.amazonaws.com")`))
	Expect(out.filterPattern).To(ContainSubstring(`($.userIdentity.arn != "arn:aws:sts::*:assumed-role/AWSServiceRoleFor*/*")`))
}

func TestApplyOverride_Deterministic(t *testing.T) {
	// Re-ordered input lists must produce the same filter pattern so Pulumi sees no diff.
	RegisterTestingT(t)

	ovA := awsApi.CloudTrailAlertOverride{
		ExcludeUserNames: []string{"alpha", "beta", "gamma"},
		ExcludeUserTypes: []string{"AWSService", "AWSAccount"},
	}
	ovB := awsApi.CloudTrailAlertOverride{
		ExcludeUserNames: []string{"gamma", "alpha", "beta"},
		ExcludeUserTypes: []string{"AWSAccount", "AWSService"},
	}

	a := applyOverride(securityAlerts["iamPolicyChanges"], ovA).filterPattern
	b := applyOverride(securityAlerts["iamPolicyChanges"], ovB).filterPattern
	Expect(a).To(Equal(b))
}

func TestApplyOverride_ThresholdAndPeriod(t *testing.T) {
	RegisterTestingT(t)

	out := applyOverride(securityAlerts["unauthorizedApiCalls"], awsApi.CloudTrailAlertOverride{
		Threshold:         10,
		Period:            600,
		EvaluationPeriods: 2,
	})
	Expect(out.threshold).To(Equal(float64(10)))
	Expect(out.period).To(Equal(600))
	Expect(out.evaluationPeriods).To(Equal(2))
}

func TestApplyOverride_EmptyOverrideIsNoop(t *testing.T) {
	RegisterTestingT(t)

	base := securityAlerts["iamPolicyChanges"]
	out := applyOverride(base, awsApi.CloudTrailAlertOverride{})
	Expect(out.filterPattern).To(Equal(base.filterPattern))
	Expect(out.threshold).To(Equal(base.threshold))
}

func TestApplyOverride_EmptyStringsSkipped(t *testing.T) {
	// Empty list entries (common YAML mistake: trailing `-`) must not produce
	// nonsense clauses like `($.x != "")`.
	RegisterTestingT(t)

	out := applyOverride(securityAlerts["iamPolicyChanges"], awsApi.CloudTrailAlertOverride{
		ExcludeUserNames: []string{"", "real-bot", ""},
	})
	// Should appear exactly once, not three times
	Expect(strings.Count(out.filterPattern, `$.userIdentity.userName`)).To(Equal(1))
	Expect(out.filterPattern).To(ContainSubstring(`"real-bot"`))
}

func TestEnabledAlerts_OverrideApplied(t *testing.T) {
	RegisterTestingT(t)

	selectors := awsApi.CloudTrailAlertSelectors{
		IamPolicyChanges: true,
		Overrides: map[string]awsApi.CloudTrailAlertOverride{
			"iamPolicyChanges": {
				ExcludeUserNames: []string{"integrail-deployer-bot"},
			},
		},
	}
	alerts := enabledAlerts(selectors)
	Expect(alerts).To(HaveLen(1))
	Expect(alerts[0].name).To(Equal("ct-iam-policy-changes"))
	Expect(alerts[0].filterPattern).To(ContainSubstring(`"integrail-deployer-bot"`))
}

func TestSecurityAlertDefinitions_UniqueNames(t *testing.T) {
	RegisterTestingT(t)

	names := make(map[string]string)
	for key, def := range securityAlerts {
		existing, collision := names[def.name]
		Expect(collision).To(BeFalse(), "alert name %q used by both %q and %q", def.name, existing, key)
		names[def.name] = key
	}
}
