package aws

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudtrail"
	"github.com/pkg/errors"

	awsApi "github.com/simple-container-com/api/pkg/clouds/aws"
)

// trailValidationOutcome is the observable result of a log-file validation
// check on an existing CloudTrail trail. Separated out so the decision logic
// in ensureTrailLogFileValidation is testable without an AWS call.
type trailValidationOutcome struct {
	// TrailFound is false when DescribeTrails returned no entry for the
	// requested name. The caller renders this as a hard failure — requiring
	// a trail that does not exist is a misconfiguration, never a soft warn.
	TrailFound bool
	// Enabled mirrors the CloudTrail LogFileValidationEnabled flag on the
	// returned trail. Meaningful only when TrailFound is true.
	Enabled bool
	// Message is a user-facing explanation rendered into the Pulumi log or
	// the returned error. Always set.
	Message string
}

// evaluateTrailValidation encapsulates the strict decision: given the result
// of DescribeTrails (one or zero trails matched) and the user's stance
// (require-on), produce either a pass outcome or an error. Isolating this
// from the AWS call makes the tricky bits — trail absent, flag absent, flag
// explicitly false — directly unit-testable.
func evaluateTrailValidation(trailName string, require bool, trails []*cloudtrail.Trail) (trailValidationOutcome, error) {
	if trailName == "" {
		return trailValidationOutcome{TrailFound: true, Enabled: true, Message: "trail check skipped (no trailName set)"}, nil
	}
	if len(trails) == 0 {
		msg := fmt.Sprintf("CloudTrail trail %q was not found — SC cannot verify log-file validation. "+
			"Fix either by creating the trail, correcting trailName in server.yaml, or removing the trailName field to skip this check.", trailName)
		return trailValidationOutcome{Message: msg}, errors.New(msg)
	}
	t := trails[0]
	enabled := t.LogFileValidationEnabled != nil && *t.LogFileValidationEnabled
	outcome := trailValidationOutcome{TrailFound: true, Enabled: enabled}
	if enabled {
		outcome.Message = fmt.Sprintf("CloudTrail trail %q has log-file validation enabled — pre-flight OK.", trailName)
		return outcome, nil
	}
	// Disabled.
	remedy := fmt.Sprintf("aws cloudtrail update-trail --name %s --enable-log-file-validation", trailName)
	if require {
		msg := fmt.Sprintf("CloudTrail trail %q has log-file validation DISABLED. SC refuses to deploy security alerts on top of an unverifiable trail. "+
			"Enable it via:\n  %s\nOr set requireLogFileValidation: false on the aws-cloudtrail-security-alerts resource to downgrade to a warning.",
			trailName, remedy)
		outcome.Message = msg
		return outcome, errors.New(msg)
	}
	outcome.Message = fmt.Sprintf("warning: CloudTrail trail %q has log-file validation DISABLED. Enable with: %s", trailName, remedy)
	return outcome, nil
}

// ensureTrailLogFileValidation runs the pre-flight check against the live
// CloudTrail API and returns the outcome. Errors are already wrapped with
// the actionable remedy message; the caller logs + returns as-is.
//
// Non-vars for credentials so callers can use ambient creds or the auth
// config on the CloudTrailSecurityAlertsConfig. Region comes from
// LogGroupRegion (the trail and its log group are always in the same
// region — trails can't deliver cross-region).
func ensureTrailLogFileValidation(ctx context.Context, cfg *awsApi.CloudTrailSecurityAlertsConfig) (trailValidationOutcome, error) {
	// Defense-in-depth: the current caller gates on cfg.TrailName != "" before
	// invoking this function, but short-circuit here too so the helper is
	// safe to call without the caller-side gate.
	if cfg.TrailName == "" {
		return trailValidationOutcome{TrailFound: true, Enabled: true, Message: "trail check skipped (no trailName set)"}, nil
	}

	awsCfg := &aws.Config{}
	if cfg.LogGroupRegion != "" {
		awsCfg.Region = aws.String(cfg.LogGroupRegion)
	} else if cfg.AccountConfig.Region != "" {
		awsCfg.Region = aws.String(cfg.AccountConfig.Region)
	}
	if cfg.AccessKey != "" && cfg.SecretAccessKey != "" {
		awsCfg.Credentials = credentials.NewStaticCredentials(cfg.AccessKey, cfg.SecretAccessKey, "")
	}
	sess, err := session.NewSession(awsCfg)
	if err != nil {
		return trailValidationOutcome{}, errors.Wrap(err, "failed to create AWS session for CloudTrail pre-flight")
	}

	client := cloudtrail.New(sess)
	out, err := client.DescribeTrailsWithContext(ctx, &cloudtrail.DescribeTrailsInput{
		TrailNameList:       []*string{aws.String(cfg.TrailName)},
		IncludeShadowTrails: aws.Bool(false),
	})
	if err != nil {
		return trailValidationOutcome{}, errors.Wrapf(err, "DescribeTrails for %q", cfg.TrailName)
	}

	return evaluateTrailValidation(cfg.TrailName, cfg.RequiresTrailValidation(), out.TrailList)
}
