package aws

import (
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/cloudtrail"
	. "github.com/onsi/gomega"

	awsApi "github.com/simple-container-com/api/pkg/clouds/aws"
)

func TestEvaluateTrailValidation(t *testing.T) {
	RegisterTestingT(t)

	t.Run("empty trailName — check is skipped, no error", func(t *testing.T) {
		RegisterTestingT(t)
		out, err := evaluateTrailValidation("", true, nil)
		Expect(err).To(BeNil())
		Expect(out.TrailFound).To(BeTrue())
		Expect(out.Enabled).To(BeTrue())
	})

	t.Run("trail missing — hard error regardless of require flag", func(t *testing.T) {
		// A configured trailName that doesn't resolve is always a
		// misconfiguration — the soft-warn escape hatch is for the
		// "trail exists but validation is off" case, NOT for "did you
		// type the name right".
		RegisterTestingT(t)
		out, err := evaluateTrailValidation("cloudtrail_events", false, nil)
		Expect(err).ToNot(BeNil())
		Expect(out.Message).To(ContainSubstring("not found"))
	})

	t.Run("trail found with validation enabled — pass", func(t *testing.T) {
		RegisterTestingT(t)
		trails := []*cloudtrail.Trail{{
			Name:                     aws.String("cloudtrail_events"),
			LogFileValidationEnabled: aws.Bool(true),
		}}
		out, err := evaluateTrailValidation("cloudtrail_events", true, trails)
		Expect(err).To(BeNil())
		Expect(out.Enabled).To(BeTrue())
		Expect(out.Message).To(ContainSubstring("pre-flight OK"))
	})

	t.Run("trail found with validation disabled + require=true — hard error with remedy", func(t *testing.T) {
		RegisterTestingT(t)
		trails := []*cloudtrail.Trail{{
			Name:                     aws.String("cloudtrail_events"),
			LogFileValidationEnabled: aws.Bool(false),
		}}
		out, err := evaluateTrailValidation("cloudtrail_events", true, trails)
		Expect(err).ToNot(BeNil())
		Expect(out.Enabled).To(BeFalse())
		// The error message must include the exact AWS CLI command so the
		// user can resolve this in one copy-paste — no "see docs" pointers.
		Expect(err.Error()).To(ContainSubstring("aws cloudtrail update-trail"))
		Expect(err.Error()).To(ContainSubstring("--enable-log-file-validation"))
	})

	t.Run("trail found with validation disabled + require=false — warning only", func(t *testing.T) {
		RegisterTestingT(t)
		trails := []*cloudtrail.Trail{{
			Name:                     aws.String("cloudtrail_events"),
			LogFileValidationEnabled: aws.Bool(false),
		}}
		out, err := evaluateTrailValidation("cloudtrail_events", false, trails)
		Expect(err).To(BeNil())
		Expect(out.Enabled).To(BeFalse())
		Expect(out.Message).To(ContainSubstring("warning"))
		Expect(out.Message).To(ContainSubstring("DISABLED"))
	})

	t.Run("trail found with LogFileValidationEnabled == nil — treated as disabled", func(t *testing.T) {
		// Back-compat: older trails or partially-populated DescribeTrails
		// responses may leave the flag nil. Err on the side of "assume off"
		// so we surface it rather than silently passing an indeterminate
		// trail as validated.
		RegisterTestingT(t)
		trails := []*cloudtrail.Trail{{
			Name: aws.String("cloudtrail_events"),
			// LogFileValidationEnabled intentionally omitted → nil pointer
		}}
		out, err := evaluateTrailValidation("cloudtrail_events", true, trails)
		Expect(err).ToNot(BeNil())
		Expect(out.Enabled).To(BeFalse())
	})
}

func TestCloudTrailSecurityAlertsConfig_RequiresTrailValidation(t *testing.T) {
	// Exercises the predicate that drives whether the pre-flight runs at
	// all. Tests the real method directly — the test file lives in
	// pkg/clouds/pulumi/aws which already depends on pkg/clouds/aws, so we
	// can import the config type without a circular dep.
	RegisterTestingT(t)
	trueVal := true
	falseVal := false

	cases := []struct {
		name      string
		trailName string
		require   *bool
		want      bool
	}{
		{"default (unset) with trailName set → required", "cloudtrail_events", nil, true},
		{"explicit false with trailName set → not required", "cloudtrail_events", &falseVal, false},
		{"explicit true with trailName set → required", "cloudtrail_events", &trueVal, true},
		{"trailName empty, require=true → not required", "", &trueVal, false},
		{"trailName empty, require=nil → not required", "", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			cfg := awsApi.CloudTrailSecurityAlertsConfig{
				TrailName:                tc.trailName,
				RequireLogFileValidation: tc.require,
			}
			Expect(cfg.RequiresTrailValidation()).To(Equal(tc.want))
		})
	}
}
