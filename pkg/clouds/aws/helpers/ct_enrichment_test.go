package helpers

import (
	"testing"
	"time"

	. "github.com/onsi/gomega"
)

func TestCtEventActor(t *testing.T) {
	RegisterTestingT(t)

	t.Run("assumed role uses sessionIssuer.userName", func(t *testing.T) {
		RegisterTestingT(t)
		e := ctEvent{}
		e.UserIdentity.Type = "AssumedRole"
		e.UserIdentity.ARN = "arn:aws:sts::471112843480:assumed-role/integrail-devops-bot/i-0abcdef1234567890"
		e.UserIdentity.SessionContext.SessionIssuer.UserName = "integrail-devops-bot"
		// The STS ARN in `arn` is not useful for humans; the role name is.
		Expect(e.Actor()).To(Equal("integrail-devops-bot"))
	})

	t.Run("IAM user falls back to userName", func(t *testing.T) {
		RegisterTestingT(t)
		e := ctEvent{}
		e.UserIdentity.Type = "IAMUser"
		e.UserIdentity.UserName = "dmitrii.creed"
		e.UserIdentity.ARN = "arn:aws:iam::471112843480:user/dmitrii.creed"
		Expect(e.Actor()).To(Equal("dmitrii.creed"))
	})

	t.Run("unknown identity falls back to arn", func(t *testing.T) {
		RegisterTestingT(t)
		e := ctEvent{}
		e.UserIdentity.Type = "AWSService"
		e.UserIdentity.ARN = "arn:aws:iam::471112843480:role/aws-service-role/some/SLR"
		Expect(e.Actor()).To(Equal("arn:aws:iam::471112843480:role/aws-service-role/some/SLR"))
	})

	t.Run("empty identity returns sentinel", func(t *testing.T) {
		RegisterTestingT(t)
		Expect(ctEvent{}.Actor()).To(Equal("unknown"))
	})
}

func TestFormatEventsForNotification(t *testing.T) {
	RegisterTestingT(t)

	t.Run("empty list renders as empty string so callers can safely concat", func(t *testing.T) {
		RegisterTestingT(t)
		Expect(formatEventsForNotification(nil, 0)).To(BeEmpty())
		Expect(formatEventsForNotification([]ctEvent{}, 0)).To(BeEmpty())
	})

	t.Run("full block: event name + actor + IP + parsed time", func(t *testing.T) {
		RegisterTestingT(t)
		ev := ctEvent{
			EventName:       "AttachRolePolicy",
			EventTimeRaw:    "2026-04-22T14:32:01Z",
			SourceIPAddress: "54.240.197.10",
		}
		ev.UserIdentity.SessionContext.SessionIssuer.UserName = "integrail-devops-bot"
		ev.EventTime, _ = time.Parse(time.RFC3339, ev.EventTimeRaw)
		out := formatEventsForNotification([]ctEvent{ev}, 1)
		Expect(out).To(ContainSubstring("Recent matching events"))
		Expect(out).To(ContainSubstring("`AttachRolePolicy`"))
		Expect(out).To(ContainSubstring("`integrail-devops-bot`"))
		Expect(out).To(ContainSubstring("`54.240.197.10`"))
		Expect(out).To(ContainSubstring("14:32:01 UTC"))
	})

	t.Run("truncation header notes total count when events are a subset", func(t *testing.T) {
		RegisterTestingT(t)
		ev := ctEvent{EventName: "ConsoleLogin"}
		ev.UserIdentity.UserName = "dmitrii"
		out := formatEventsForNotification([]ctEvent{ev, ev, ev}, 27)
		Expect(out).To(ContainSubstring("(showing 3 of 27)"))
	})

	t.Run("errorCode is rendered when present — useful for failed-login alerts", func(t *testing.T) {
		RegisterTestingT(t)
		ev := ctEvent{
			EventName: "ConsoleLogin",
			ErrorCode: "AccessDenied",
		}
		ev.UserIdentity.UserName = "attacker"
		out := formatEventsForNotification([]ctEvent{ev}, 1)
		Expect(out).To(ContainSubstring("errorCode: `AccessDenied`"))
	})

	t.Run("missing optional fields don't produce empty placeholders", func(t *testing.T) {
		// An event with only the required name+actor should still render cleanly,
		// not with holes like "from `` at ``".
		RegisterTestingT(t)
		ev := ctEvent{EventName: "CreatePolicy"}
		ev.UserIdentity.SessionContext.SessionIssuer.UserName = "bot"
		out := formatEventsForNotification([]ctEvent{ev}, 1)
		Expect(out).To(ContainSubstring("`CreatePolicy`"))
		Expect(out).To(ContainSubstring("`bot`"))
		Expect(out).ToNot(ContainSubstring("from ``"))
		Expect(out).ToNot(ContainSubstring("at ``"))
	})
}

func TestParseAlarmStateTimestamp(t *testing.T) {
	RegisterTestingT(t)

	t.Run("CloudWatch native format — offset without colon", func(t *testing.T) {
		RegisterTestingT(t)
		got := parseAlarmStateTimestamp("2026-04-22T14:32:30.123+0000")
		Expect(got.UTC().Format(time.RFC3339)).To(Equal("2026-04-22T14:32:30Z"))
	})

	t.Run("RFC3339 with Z", func(t *testing.T) {
		RegisterTestingT(t)
		got := parseAlarmStateTimestamp("2026-04-22T14:32:30Z")
		Expect(got.UTC().Format(time.RFC3339)).To(Equal("2026-04-22T14:32:30Z"))
	})

	t.Run("RFC3339 with offset-colon", func(t *testing.T) {
		RegisterTestingT(t)
		got := parseAlarmStateTimestamp("2026-04-22T14:32:30+02:00")
		Expect(got.UTC().Format(time.RFC3339)).To(Equal("2026-04-22T12:32:30Z"))
	})

	t.Run("empty and malformed fall back to now — lookup window still covers recent past", func(t *testing.T) {
		RegisterTestingT(t)
		// Allow a generous skew since we're comparing to wall-clock time.
		before := time.Now().UTC().Add(-2 * time.Second)
		after := time.Now().UTC().Add(2 * time.Second)
		Expect(parseAlarmStateTimestamp("")).To(BeTemporally("~", time.Now().UTC(), 2*time.Second))
		Expect(parseAlarmStateTimestamp("garbage")).To(BeTemporally(">", before))
		Expect(parseAlarmStateTimestamp("garbage")).To(BeTemporally("<", after))
	})
}

func TestLookupTriggeringEventsShortCircuits(t *testing.T) {
	// We don't exercise the AWS SDK path in unit tests (that would need
	// network or a mock). But the helper short-circuits to nil,nil when
	// required config is missing — which is worth asserting so future
	// refactors don't accidentally try to open a session with empty inputs.
	RegisterTestingT(t)

	t.Run("missing log group name — no call, no error", func(t *testing.T) {
		RegisterTestingT(t)
		events, total, err := lookupTriggeringEvents(nil, nil, enrichmentConfig{FilterPattern: "x"}, time.Now(), 5)
		Expect(err).To(BeNil())
		Expect(events).To(BeNil())
		Expect(total).To(Equal(0))
	})

	t.Run("missing filter pattern — no call, no error", func(t *testing.T) {
		RegisterTestingT(t)
		events, total, err := lookupTriggeringEvents(nil, nil, enrichmentConfig{LogGroupName: "g"}, time.Now(), 5)
		Expect(err).To(BeNil())
		Expect(events).To(BeNil())
		Expect(total).To(Equal(0))
	})
}
