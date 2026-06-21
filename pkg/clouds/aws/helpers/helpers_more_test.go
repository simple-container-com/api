// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package helpers

import (
	"context"
	"errors"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/logger"
)

// withCleanAlertEnv unsets every alert-related env var the cloudwatch handler
// reads, so a test starts from a known baseline regardless of the host
// environment (CI runners may inject AWS_* or SIMPLE_CONTAINER_* values). It
// uses t.Setenv so the originals are restored at test end.
func withCleanAlertEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		api.ComputeEnv.StackName,
		api.ComputeEnv.StackEnv,
		api.ComputeEnv.AlertName,
		api.ComputeEnv.AlertDescription,
		api.ComputeEnv.DiscordWebhookUrl,
		api.ComputeEnv.SlackWebhookUrl,
		api.ComputeEnv.TelegramChatID,
		api.ComputeEnv.TelegramToken,
		api.ComputeEnv.CtLogGroupName,
		api.ComputeEnv.CtLogGroupRegion,
		api.ComputeEnv.CtFilterPattern,
	} {
		t.Setenv(k, "")
	}
	// Keep the SDK from probing the EC2 instance-metadata endpoint during any
	// (lazy) AWS config resolution triggered transitively by the handler.
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
}

// alarmEventMap returns a minimal but valid CloudWatch alarm event payload as
// the map[string]any shape the Lambda runtime hands the handler. `value` sets
// the alarm state ("ALARM" or "OK").
func alarmEventMap(value string) map[string]any {
	return map[string]any{
		"accountId": "471112843480",
		"region":    "eu-central-1",
		"alarmArn":  "arn:aws:cloudwatch:eu-central-1:471112843480:alarm:demo",
		"alarmData": map[string]any{
			"alarmName": "demo-alarm",
			"state": map[string]any{
				"reason":    "Threshold Crossed",
				"value":     value,
				"timestamp": "2026-04-22T14:32:30.123+0000",
			},
			"configuration": map[string]any{"description": "demo"},
		},
	}
}

func TestCloudwatchHandler_EventTypeErrors(t *testing.T) {
	RegisterTestingT(t)

	l := &cloudwatchEventsLambda{log: logger.New()}
	ctx := context.Background()

	t.Run("non-map event is rejected", func(t *testing.T) {
		RegisterTestingT(t)
		// The handler type-asserts event.(map[string]any); a bare string fails it.
		err := l.handler(ctx, "not-a-map")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("event is not of type map[string]any"))
	})

	t.Run("nil event is rejected (also not a map)", func(t *testing.T) {
		RegisterTestingT(t)
		err := l.handler(ctx, nil)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("event is not of type map[string]any"))
	})

	t.Run("map with unmarshallable value fails JSON conversion", func(t *testing.T) {
		RegisterTestingT(t)
		// ToObjectViaJson marshals the map first; a chan value is not JSON-encodable,
		// so the conversion returns an error which the handler wraps and returns.
		err := l.handler(ctx, map[string]any{"alarmData": make(chan int)})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to convert incoming event to *AlarmEvent"))
	})
}

func TestCloudwatchHandler_NoNotifiersConfigured(t *testing.T) {
	RegisterTestingT(t)
	withCleanAlertEnv(t)
	// Populate the descriptive env vars so the constructed Alert is well-formed,
	// but leave all notifier secrets unset so every "not configured" branch is
	// taken and no AWS Secrets Manager / webhook call is attempted.
	t.Setenv(api.ComputeEnv.StackName, "payments")
	t.Setenv(api.ComputeEnv.StackEnv, "prod")
	t.Setenv(api.ComputeEnv.AlertName, "cpu-high")
	t.Setenv(api.ComputeEnv.AlertDescription, "CPU exceeded threshold")

	l := &cloudwatchEventsLambda{log: logger.New()}

	t.Run("ALARM state with no notifiers and no enrichment env returns nil", func(t *testing.T) {
		RegisterTestingT(t)
		// AlertTriggered branch entered, but with CtLogGroupName/CtFilterPattern
		// unset the enrichment lookup is skipped (no AWS call).
		Expect(l.handler(context.Background(), alarmEventMap("ALARM"))).To(Succeed())
	})

	t.Run("OK state takes the resolved path and skips enrichment entirely", func(t *testing.T) {
		RegisterTestingT(t)
		// value=OK => AlertType=RESOLVED => the `if AlertTriggered` enrichment
		// block is not entered at all.
		Expect(l.handler(context.Background(), alarmEventMap("OK"))).To(Succeed())
	})
}

func TestCloudwatchHandler_EnrichmentRequiresBothVars(t *testing.T) {
	RegisterTestingT(t)
	withCleanAlertEnv(t)
	l := &cloudwatchEventsLambda{log: logger.New()}

	// Setting only ONE of the two enrichment vars keeps the inner
	// `LogGroupName != "" && FilterPattern != ""` guard false, so the handler
	// enters the AlertTriggered block but does NOT call lookupTriggeringEvents
	// (which would require AWS). This proves the guard is an AND, not an OR.
	t.Run("only log group set -> no AWS lookup, handler succeeds", func(t *testing.T) {
		RegisterTestingT(t)
		t.Setenv(api.ComputeEnv.CtLogGroupName, "/aws/cloudtrail/security")
		t.Setenv(api.ComputeEnv.CtFilterPattern, "")
		Expect(l.handler(context.Background(), alarmEventMap("ALARM"))).To(Succeed())
	})

	t.Run("only filter pattern set -> no AWS lookup, handler succeeds", func(t *testing.T) {
		RegisterTestingT(t)
		t.Setenv(api.ComputeEnv.CtLogGroupName, "")
		t.Setenv(api.ComputeEnv.CtFilterPattern, "{ $.eventName = AttachRolePolicy }")
		Expect(l.handler(context.Background(), alarmEventMap("ALARM"))).To(Succeed())
	})
}

func TestCloudwatchLambda_SetLoggerAndConstructor(t *testing.T) {
	RegisterTestingT(t)

	t.Run("SetLogger stores the logger", func(t *testing.T) {
		RegisterTestingT(t)
		l := &cloudwatchEventsLambda{}
		log := logger.New()
		l.SetLogger(log)
		Expect(l.log).To(Equal(log))
	})

	t.Run("NewCloudwatchLambdaHelper applies options and returns a CloudHelper", func(t *testing.T) {
		RegisterTestingT(t)
		h, err := NewCloudwatchLambdaHelper(api.WithLogger(logger.New()))
		Expect(err).ToNot(HaveOccurred())
		Expect(h).ToNot(BeNil())
		// SetLogger via WithLogger must have landed on the concrete type.
		cw, ok := h.(*cloudwatchEventsLambda)
		Expect(ok).To(BeTrue())
		Expect(cw.log).ToNot(BeNil())
	})

	t.Run("NewCloudwatchLambdaHelper with no options succeeds", func(t *testing.T) {
		RegisterTestingT(t)
		h, err := NewCloudwatchLambdaHelper()
		Expect(err).ToNot(HaveOccurred())
		Expect(h).ToNot(BeNil())
	})

	t.Run("NewCloudwatchLambdaHelper propagates option errors", func(t *testing.T) {
		RegisterTestingT(t)
		boom := errors.New("boom")
		failing := func(c api.CloudHelper) error { return boom }
		h, err := NewCloudwatchLambdaHelper(failing)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to apply option on lambda helper"))
		Expect(err.Error()).To(ContainSubstring("boom"))
		Expect(h).To(BeNil())
	})
}
