package aws

import (
	"testing"

	. "github.com/onsi/gomega"

	awsApi "github.com/simple-container-com/api/pkg/clouds/aws"
)

func TestEnabledAlerts_AllEnabled(t *testing.T) {
	RegisterTestingT(t)

	selectors := awsApi.CloudTrailAlertSelectors{
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
	}

	alerts := enabledAlerts(selectors)
	Expect(alerts).To(HaveLen(14))

	names := make(map[string]bool)
	for _, a := range alerts {
		names[a.name] = true
		Expect(a.description).ToNot(BeEmpty())
		Expect(a.filterPattern).ToNot(BeEmpty())
	}

	Expect(names).To(HaveKey("ct-root-account-usage"))
	Expect(names).To(HaveKey("ct-unauthorized-api-calls"))
	Expect(names).To(HaveKey("ct-console-login-no-mfa"))
	Expect(names).To(HaveKey("ct-iam-policy-changes"))
	Expect(names).To(HaveKey("ct-cloudtrail-tampering"))
	Expect(names).To(HaveKey("ct-failed-console-logins"))
	Expect(names).To(HaveKey("ct-kms-key-deletion"))
	Expect(names).To(HaveKey("ct-s3-bucket-policy-changes"))
	Expect(names).To(HaveKey("ct-config-changes"))
	Expect(names).To(HaveKey("ct-security-group-changes"))
	Expect(names).To(HaveKey("ct-nacl-changes"))
	Expect(names).To(HaveKey("ct-network-gateway-changes"))
	Expect(names).To(HaveKey("ct-route-table-changes"))
	Expect(names).To(HaveKey("ct-vpc-changes"))
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

	Expect(securityAlerts).To(HaveLen(14))

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

func TestSecurityAlertDefinitions_UniqueNames(t *testing.T) {
	RegisterTestingT(t)

	names := make(map[string]string)
	for key, def := range securityAlerts {
		existing, collision := names[def.name]
		Expect(collision).To(BeFalse(), "alert name %q used by both %q and %q", def.name, existing, key)
		names[def.name] = key
	}
}
