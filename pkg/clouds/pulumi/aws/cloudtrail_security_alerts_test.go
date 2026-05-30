package aws

import (
	"reflect"
	"strings"
	"testing"

	. "github.com/onsi/gomega"

	awsApi "github.com/simple-container-com/api/pkg/clouds/aws"
)

// totalDetectors is the count of built-in security detectors. Update when adding new ones.
// Composition: 14 CIS CloudWatch.1-14 + 9 beyond-CIS additions
// (kmsKeyPolicy + kmsKeyGrants count as two; they were split from the prior kmsKeyPolicyChanges
// because the events have different signal density and warrant different thresholds).
const totalDetectors = 23

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
		KmsKeyPolicy:          true,
		KmsKeyGrants:          true,
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
		"ct-kms-key-policy",
		"ct-kms-key-grants",
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

func TestUnauthorizedApiCalls_DefaultThresholdIsTen(t *testing.T) {
	// CIS Benchmark's "1 event = alert" is too noisy for any active AWS account;
	// production data showed 10/300s absorbs natural permission-evaluation noise
	// (eventual consistency, mistyped CLI commands, optional-service self-probes)
	// without missing real bursts.
	RegisterTestingT(t)
	Expect(securityAlerts["unauthorizedApiCalls"].threshold).To(Equal(float64(10)))
}

func TestUnauthorizedApiCalls_DefaultExcludesAWSService(t *testing.T) {
	// AWS service-linked roles continuously probe optional services for capability
	// discovery (Macie scanning S3 buckets that aren't enrolled, etc.). 60% of
	// historical events in our production sample were AWSService-type AccessDenied
	// — pure noise that adds zero security signal in every AWS account. Excluding
	// at the BASE filter (not just via consumer override) makes the detector quiet
	// out-of-the-box. NOT EXISTS guard preserves events where the field is absent.
	RegisterTestingT(t)
	p := securityAlerts["unauthorizedApiCalls"].filterPattern
	Expect(p).To(ContainSubstring(`($.userIdentity.type NOT EXISTS)`))
	Expect(p).To(ContainSubstring(`$.userIdentity.type != "AWSService"`))
}

func TestUnauthorizedApiCalls_DefaultExcludesAWSAccount(t *testing.T) {
	// AWSAccount cross-account AccessDenied events are covered by the dedicated
	// anonymousProbes detector (same threshold). Excluding here avoids double-paging
	// the same event class. Same NOT EXISTS guard.
	RegisterTestingT(t)
	p := securityAlerts["unauthorizedApiCalls"].filterPattern
	Expect(p).To(ContainSubstring(`$.userIdentity.type != "AWSAccount"`))
}

func TestKmsKeyPolicy_HighSignalDefault(t *testing.T) {
	// kmsKeyPolicy is the structural "who can use this key" change detector. Default
	// threshold 1 — page on any PutKeyPolicy. Scoped to PutKeyPolicy only; grants
	// live in the separate kmsKeyGrants detector.
	RegisterTestingT(t)
	def := securityAlerts["kmsKeyPolicy"]
	Expect(def.filterPattern).To(ContainSubstring(`$.eventName = "PutKeyPolicy"`))
	Expect(def.filterPattern).ToNot(ContainSubstring(`CreateGrant`),
		"kmsKeyPolicy must NOT include grants — they belong in kmsKeyGrants")
	Expect(def.filterPattern).ToNot(ContainSubstring(`RetireGrant`))
	Expect(def.filterPattern).ToNot(ContainSubstring(`RevokeGrant`))
	Expect(def.threshold).To(Equal(float64(0)),
		"kmsKeyPolicy uses the default threshold (1) which is encoded as zero in the def")
}

func TestKmsKeyGrants_HighVolumeDefault(t *testing.T) {
	// kmsKeyGrants is the high-volume detector. Default threshold 10/300s because
	// any IaC tool issues a CreateGrant per resource that needs KMS — at typical
	// deploy cadence ~25/hour from one bot.
	RegisterTestingT(t)
	def := securityAlerts["kmsKeyGrants"]
	Expect(def.filterPattern).To(ContainSubstring(`CreateGrant`))
	Expect(def.filterPattern).To(ContainSubstring(`RetireGrant`))
	Expect(def.filterPattern).To(ContainSubstring(`RevokeGrant`))
	Expect(def.filterPattern).ToNot(ContainSubstring(`PutKeyPolicy`),
		"kmsKeyGrants must NOT include PutKeyPolicy — it belongs in kmsKeyPolicy")
	Expect(def.threshold).To(Equal(float64(10)))
}

func TestKmsKeyPolicyChanges_OldNameRemoved(t *testing.T) {
	// The old aggregate name kmsKeyPolicyChanges was split into kmsKeyPolicy +
	// kmsKeyGrants. The old name must be gone from securityAlerts so that
	// validateOverrides flags any consumer YAML still using it.
	RegisterTestingT(t)
	Expect(securityAlerts).ToNot(HaveKey("kmsKeyPolicyChanges"))
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

	// Single-value exclusion form: (NOT EXISTS) || (!= "value")
	// The NOT EXISTS guard keeps events where the field is absent (e.g.,
	// AssumedRole events lacking top-level $.userIdentity.userName) IN the
	// detector; without it, every assumed-role event would silently bypass
	// the alarm.
	Expect(out.filterPattern).To(ContainSubstring(`(($.userIdentity.userName NOT EXISTS) || ($.userIdentity.userName != "integrail-deployer-bot"))`))
}

func TestApplyOverride_NotExistsGuard_MultipleValues(t *testing.T) {
	// Multi-value form uses inner-AND: De Morgan'd "NOT (v1 OR v2)" = "!= v1 AND != v2".
	// The OR with NOT EXISTS still keeps absent-field events flowing through.
	RegisterTestingT(t)

	out := applyOverride(securityAlerts["iamPolicyChanges"], awsApi.CloudTrailAlertOverride{
		ExcludeUserNames: []string{"alpha", "beta", "gamma"},
	})
	Expect(out.filterPattern).To(ContainSubstring(
		`(($.userIdentity.userName NOT EXISTS) || (($.userIdentity.userName != "alpha") && ($.userIdentity.userName != "beta") && ($.userIdentity.userName != "gamma")))`,
	))
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

	// Each exclusion field gets its own NOT EXISTS guard.
	Expect(out.filterPattern).To(ContainSubstring(`($.userIdentity.userName NOT EXISTS)`))
	Expect(out.filterPattern).To(ContainSubstring(`($.userIdentity.type NOT EXISTS)`))
	Expect(out.filterPattern).To(ContainSubstring(`($.userIdentity.invokedBy NOT EXISTS)`))
	Expect(out.filterPattern).To(ContainSubstring(`($.userIdentity.arn NOT EXISTS)`))
	// Values still wired up correctly:
	Expect(out.filterPattern).To(ContainSubstring(`"prowler-readonly"`))
	Expect(out.filterPattern).To(ContainSubstring(`"AWSService"`))
	Expect(out.filterPattern).To(ContainSubstring(`"AWSAccount"`))
	Expect(out.filterPattern).To(ContainSubstring(`"s3.amazonaws.com"`))
	Expect(out.filterPattern).To(ContainSubstring(`"lambda.amazonaws.com"`))
	Expect(out.filterPattern).To(ContainSubstring(`"arn:aws:sts::*:assumed-role/AWSServiceRoleFor*/*"`))
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

// TestApplyOverride_WorstCaseBasePattern guards the trim-and-rewrap logic that wraps
// the base pattern in parens before AND'ing exclusions. The shape we have to preserve:
//   - leading/trailing whitespace inside the braces (idiomatic indentation),
//   - internal parentheses from existing OR-groups,
//   - mixed AND/OR/&& at the top level.
//
// If a future contributor writes a pattern with leading whitespace or extra braces,
// the wrap output should still be syntactically valid CloudWatch.
func TestApplyOverride_WorstCaseBasePattern(t *testing.T) {
	RegisterTestingT(t)

	// Synthetic worst-case: leading whitespace, internal OR-group, trailing whitespace,
	// AND with a sub-expression.
	worst := securityAlertDef{
		name:          "worst-case",
		description:   "synthetic",
		filterPattern: `{   ($.eventName = "A") || ($.eventName = "B") && ($.eventSource = "x.amazonaws.com")   }`,
	}
	out := applyOverride(worst, awsApi.CloudTrailAlertOverride{
		ExcludeUserNames: []string{"bot"},
	})

	// Output must still be a single braced expression
	Expect(out.filterPattern).To(HavePrefix("{ "))
	Expect(out.filterPattern).To(HaveSuffix(" }"))
	// Base predicate atoms intact
	Expect(out.filterPattern).To(ContainSubstring(`($.eventName = "A")`))
	Expect(out.filterPattern).To(ContainSubstring(`($.eventName = "B")`))
	Expect(out.filterPattern).To(ContainSubstring(`($.eventSource = "x.amazonaws.com")`))
	// Exclusion present
	Expect(out.filterPattern).To(ContainSubstring(`"bot"`))
	// Top-level structure: base wrapped in parens, then `&&` to exclusion
	Expect(out.filterPattern).To(MatchRegexp(`^\{ \(.*\) && \(.*\) \}$`))
}

func TestApplyOverride_EmptyStringsSkipped(t *testing.T) {
	// Empty list entries (common YAML mistake: trailing `-`) must not produce
	// nonsense clauses like `($.x != "")`. Each non-empty value still contributes
	// a NOT-EXISTS-guarded clause: in single-value form we get the field name twice
	// (once for NOT EXISTS, once for !=).
	RegisterTestingT(t)

	out := applyOverride(securityAlerts["iamPolicyChanges"], awsApi.CloudTrailAlertOverride{
		ExcludeUserNames: []string{"", "real-bot", ""},
	})
	// Single-value clause shape: (($.field NOT EXISTS) || ($.field != "real-bot"))
	// — two occurrences of $.userIdentity.userName, no empty-string match.
	Expect(strings.Count(out.filterPattern, `$.userIdentity.userName`)).To(Equal(2))
	Expect(out.filterPattern).To(ContainSubstring(`"real-bot"`))
	Expect(out.filterPattern).ToNot(ContainSubstring(`!= ""`))
}

func TestApplyOverride_DeDupesValues(t *testing.T) {
	// User pastes the same exclusion twice — generated filter must dedupe so we
	// don't emit a redundant `&& ($.field != "v") && ($.field != "v")`.
	RegisterTestingT(t)

	out := applyOverride(securityAlerts["iamPolicyChanges"], awsApi.CloudTrailAlertOverride{
		ExcludeUserNames: []string{"bot", "bot", "bot"},
	})
	Expect(strings.Count(out.filterPattern, `"bot"`)).To(Equal(1))
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

// TestSelectorChecksWireUpAllDetectors guards against the regression where a contributor
// adds a new bool to CloudTrailAlertSelectors + a new entry to securityAlerts but forgets
// to wire it through selectorChecks. Without this test, the detector would never appear
// in enabledAlerts and the new toggle would be silently dead.
func TestSelectorChecksWireUpAllDetectors(t *testing.T) {
	RegisterTestingT(t)

	// Flip every bool selector on via reflection. selectorChecks then enumerates
	// the (key, enabled) pairs; we assert each securityAlerts key appears exactly once.
	sel := awsApi.CloudTrailAlertSelectors{}
	v := reflect.ValueOf(&sel).Elem()
	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i)
		if f.Kind() == reflect.Bool {
			f.SetBool(true)
		}
	}

	checkedKeys := map[string]int{}
	for _, c := range selectorChecks(sel) {
		Expect(c.enabled).To(BeTrue(), "selectorChecks did not pick up the bool for %q (likely missing wireup)", c.key)
		checkedKeys[c.key]++
	}

	// Bidirectional: every securityAlerts key must be in selectorChecks
	for key := range securityAlerts {
		Expect(checkedKeys).To(HaveKey(key), "securityAlerts has %q but selectorChecks doesn't wire it up", key)
		Expect(checkedKeys[key]).To(Equal(1), "selectorChecks lists %q more than once", key)
	}
	// And every selectorChecks key must be in securityAlerts
	for key := range checkedKeys {
		Expect(securityAlerts).To(HaveKey(key), "selectorChecks lists %q but securityAlerts has no entry", key)
	}
	// And the count must equal totalDetectors (catches half-applied additions)
	Expect(checkedKeys).To(HaveLen(totalDetectors))
}

func TestValidateOverrides_Empty(t *testing.T) {
	RegisterTestingT(t)
	Expect(validateOverrides(awsApi.CloudTrailAlertSelectors{})).To(Succeed())
}

func TestValidateOverrides_KnownKey(t *testing.T) {
	RegisterTestingT(t)
	err := validateOverrides(awsApi.CloudTrailAlertSelectors{
		Overrides: map[string]awsApi.CloudTrailAlertOverride{
			"iamPolicyChanges": {ExcludeUserNames: []string{"bot"}},
		},
	})
	Expect(err).To(Succeed())
}

func TestValidateOverrides_UnknownKeyIsLoudError(t *testing.T) {
	// Common YAML mistake: misspell the detector key. Without validation the override
	// is silently dropped and the operator gets no signal — they just keep seeing the
	// alarm fire and wonder why their exclusion didn't take. Fail at deploy time instead.
	RegisterTestingT(t)
	err := validateOverrides(awsApi.CloudTrailAlertSelectors{
		Overrides: map[string]awsApi.CloudTrailAlertOverride{
			"unauthorizedApiCall": {ExcludeUserNames: []string{"bot"}}, // missing trailing 's'
		},
	})
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("unauthorizedApiCall"))
	// Should hint at what the user can use
	Expect(err.Error()).To(ContainSubstring("known"))
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
