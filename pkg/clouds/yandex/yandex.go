// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package yandex

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"

	"github.com/simple-container-com/api/pkg/api"
)

// TemplateConfig is the parent-stack half of a Yandex Cloud template: the
// credentials plus the knobs an operator sets once for every service deployed
// under this template. Per-service overrides live in the client stack's
// cloudExtras (see CloudExtras).
type TemplateConfig struct {
	AccountConfig `json:",inline" yaml:",inline"`

	// RegistryID is the Container Registry images are pushed to and pulled from.
	RegistryID string `json:"registryId,omitempty" yaml:"registryId,omitempty"`
	// ServiceAccountID is the identity deployed containers run as. It needs
	// lockbox.payloadViewer to read secrets and container-registry.images.puller
	// to start at all.
	ServiceAccountID string `json:"serviceAccountId,omitempty" yaml:"serviceAccountId,omitempty"`
}

func ReadTemplateConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &TemplateConfig{})
}

// CloudExtras is the Yandex Cloud decoding of a client stack's `cloudExtras`
// block (api.StackConfigSingleImage.CloudExtras, which is a *any decoded
// per-provider with api.ConvertDescriptor).
//
// Schedules live HERE, in client.yaml, and not as a resource in server.yaml: a
// schedule is a property of one service, not of the shared infrastructure its
// parent stack provisions. This mirrors aws.CloudExtras.LambdaSchedules, which is
// how every fleet service already declares its ticks.
type CloudExtras struct {
	// Schedules become Yandex Serverless Timer Triggers invoking the container.
	Schedules []ContainerSchedule `json:"schedules,omitempty" yaml:"schedules,omitempty"`
	// ServiceAccountID overrides the template's container identity for this
	// service only.
	ServiceAccountID string `json:"serviceAccountId,omitempty" yaml:"serviceAccountId,omitempty"`
	// Roles are extra IAM roles bound to the container's service account, the
	// analogue of aws.CloudExtras.AwsRoles.
	Roles []string `json:"roles,omitempty" yaml:"roles,omitempty"`
	// Concurrency is the number of simultaneous requests one container instance
	// handles. YC's own default is 1.
	Concurrency *int `json:"concurrency,omitempty" yaml:"concurrency,omitempty"`
	// ProvisionedInstances keeps N instances warm, the cold-start escape hatch.
	ProvisionedInstances *int `json:"provisionedInstances,omitempty" yaml:"provisionedInstances,omitempty"`
}

// ContainerSchedule is one Yandex Serverless Timer Trigger.
//
// Name/Expression/Request are deliberately field-compatible with
// aws.LambdaSchedule so a service moving between clouds renames the surrounding
// key rather than rewriting each schedule. RetryAttempts/RetryInterval/DLQ have
// no EventBridge equivalent — on AWS those are properties of the target, not of
// the rule.
type ContainerSchedule struct {
	Name       string `json:"name" yaml:"name"`
	Expression string `json:"expression" yaml:"expression"`
	// Request is the HTTP request payload delivered to the container, the same
	// JSON envelope the AWS schedules use.
	Request string `json:"request" yaml:"request"`
	// RetryAttempts is how many times YC retries a failed invocation, 1..5.
	RetryAttempts *int `json:"retryAttempts,omitempty" yaml:"retryAttempts,omitempty"`
	// RetryInterval is the delay between retries as a Go duration, 10s..60s.
	RetryInterval string `json:"retryInterval,omitempty" yaml:"retryInterval,omitempty"`
	// DLQ names a Message Queue that receives invocations exhausted by retries.
	DLQ string `json:"dlq,omitempty" yaml:"dlq,omitempty"`
}

// Yandex Cloud's documented limits for a timer trigger. They are hard API
// rejections, which is why they are enforced at config-parse time rather than in
// the provisioner: a provisioner-only check never runs for an already-provisioned
// stack, so a bad value would sit unnoticed until the next rebuild.
const (
	MinRetryAttempts = 1
	MaxRetryAttempts = 5

	MinRetryInterval = 10 * time.Second
	MaxRetryInterval = 60 * time.Second

	// MaxRequestPayloadLen is YC's cap on the trigger payload, in characters.
	MaxRequestPayloadLen = 4096

	// CronFieldCount is the number of fields in a YC cron expression:
	// minutes, hours, day-of-month, month, day-of-week, year.
	CronFieldCount = 6
)

// triggerNameRegexp is YC's own constraint on a trigger name: 3-63 characters,
// lowercase Latin letters, digits and hyphens, starting with a letter and not
// ending with a hyphen.
var triggerNameRegexp = regexp.MustCompile(`^[a-z][a-z0-9-]{1,61}[a-z0-9]$`)

// cronWrapperRegexp matches the AWS `cron(...)` wrapper. YC takes the bare
// expression; accepting and stripping the wrapper is what lets one schedule
// definition be copied between an AWS and a YC client stack unchanged.
var cronWrapperRegexp = regexp.MustCompile(`^cron\((.*)\)$`)

// NormalizedExpression returns the bare YC cron expression, stripping an AWS-style
// `cron(...)` wrapper if present.
func (s *ContainerSchedule) NormalizedExpression() string {
	expr := strings.TrimSpace(s.Expression)
	if m := cronWrapperRegexp.FindStringSubmatch(expr); m != nil {
		return strings.TrimSpace(m[1])
	}
	return expr
}

// EffectiveRetryInterval parses RetryInterval, returning zero when unset.
func (s *ContainerSchedule) EffectiveRetryInterval() (time.Duration, error) {
	if s.RetryInterval == "" {
		return 0, nil
	}
	// A bare integer is a common slip and means seconds nowhere in Go; reject it
	// with the fix in the message rather than with time.ParseDuration's wording.
	if _, err := strconv.Atoi(s.RetryInterval); err == nil {
		return 0, errors.Errorf("retryInterval %q must carry a unit, e.g. %qs", s.RetryInterval, s.RetryInterval)
	}
	d, err := time.ParseDuration(s.RetryInterval)
	if err != nil {
		return 0, errors.Wrapf(err, "invalid retryInterval %q", s.RetryInterval)
	}
	return d, nil
}

// Validate checks one schedule against Yandex Cloud's documented trigger limits.
func (s *ContainerSchedule) Validate() error {
	if s.Name == "" {
		return errors.Errorf("schedule name must not be empty")
	}
	if !triggerNameRegexp.MatchString(s.Name) {
		return errors.Errorf("schedule name %q is not a valid Yandex trigger name: 3-63 characters, "+
			"lowercase letters, digits and hyphens, starting with a letter and not ending with a hyphen", s.Name)
	}

	expr := s.NormalizedExpression()
	if expr == "" {
		return errors.Errorf("schedule %q has an empty cron expression", s.Name)
	}
	fields := strings.Fields(expr)
	if len(fields) != CronFieldCount {
		return errors.Errorf("schedule %q cron expression %q has %d fields; Yandex requires %d "+
			"(minutes hours day-of-month month day-of-week year)", s.Name, expr, len(fields), CronFieldCount)
	}
	// YC, like EventBridge, refuses an expression that constrains both day-of-month
	// and day-of-week; exactly one of them must be '?'. This is the single most
	// common way a copied crontab line is rejected.
	dom, dow := fields[2], fields[4]
	if (dom == "?") == (dow == "?") {
		return errors.Errorf("schedule %q cron expression %q must use '?' for exactly one of "+
			"day-of-month and day-of-week (got %q and %q)", s.Name, expr, dom, dow)
	}

	if l := len([]rune(s.Request)); l > MaxRequestPayloadLen {
		return errors.Errorf("schedule %q request payload is %d characters; Yandex allows at most %d",
			s.Name, l, MaxRequestPayloadLen)
	}

	if s.RetryAttempts != nil {
		if a := *s.RetryAttempts; a < MinRetryAttempts || a > MaxRetryAttempts {
			return errors.Errorf("schedule %q retryAttempts is %d; Yandex allows %d..%d",
				s.Name, a, MinRetryAttempts, MaxRetryAttempts)
		}
	}

	interval, err := s.EffectiveRetryInterval()
	if err != nil {
		return errors.Wrapf(err, "schedule %q", s.Name)
	}
	if interval != 0 && (interval < MinRetryInterval || interval > MaxRetryInterval) {
		return errors.Errorf("schedule %q retryInterval is %s; Yandex allows %s..%s",
			s.Name, interval, MinRetryInterval, MaxRetryInterval)
	}

	return nil
}

// Validate checks every schedule and rejects duplicate names, which YC would
// otherwise accept as two triggers racing over the same tick.
func (c *CloudExtras) Validate() error {
	seen := make(map[string]struct{}, len(c.Schedules))
	for i := range c.Schedules {
		s := &c.Schedules[i]
		if err := s.Validate(); err != nil {
			return err
		}
		if _, dup := seen[s.Name]; dup {
			return errors.Errorf("duplicate schedule name %q", s.Name)
		}
		seen[s.Name] = struct{}{}
	}
	if c.Concurrency != nil && *c.Concurrency < 1 {
		return errors.Errorf("concurrency must be at least 1, got %d", *c.Concurrency)
	}
	if c.ProvisionedInstances != nil && *c.ProvisionedInstances < 0 {
		return errors.Errorf("provisionedInstances must not be negative, got %d", *c.ProvisionedInstances)
	}
	return nil
}

// ReadCloudExtras decodes and validates a client stack's cloudExtras block. It
// returns an empty (valid) CloudExtras when the block is absent, so callers do not
// have to nil-check before reading Schedules.
func ReadCloudExtras(cloudExtras *any) (*CloudExtras, error) {
	res := &CloudExtras{}
	if cloudExtras == nil {
		return res, nil
	}
	res, err := api.ConvertDescriptor(cloudExtras, res)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to convert cloudExtras to yandex.CloudExtras")
	}
	if err := res.Validate(); err != nil {
		return nil, err
	}
	return res, nil
}
