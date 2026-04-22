package helpers

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudwatchlogs"
	"github.com/pkg/errors"

	"github.com/simple-container-com/api/pkg/api/logger"
)

// Per-region session cache. Lambda containers may handle many sequential
// invocations across their ~5-15 minute lifetime; reusing the session and its
// underlying HTTP client avoids redoing TLS handshake + credential resolution
// on every alarm. Keyed by region because different CloudTrail log groups may
// live in different regions.
var (
	sessionCacheMu sync.Mutex
	sessionCache   = map[string]*session.Session{}
)

func sessionForRegion(region string) (*session.Session, error) {
	// Empty region → AWS SDK resolves from AWS_REGION / AWS_DEFAULT_REGION
	// (set automatically in Lambda). We still cache under a sentinel key
	// so repeat empty-region callers share one session.
	key := region
	if key == "" {
		key = "__default__"
	}
	sessionCacheMu.Lock()
	defer sessionCacheMu.Unlock()
	if s, ok := sessionCache[key]; ok {
		return s, nil
	}
	cfg := &aws.Config{}
	if region != "" {
		cfg.Region = aws.String(region)
	}
	s, err := session.NewSession(cfg)
	if err != nil {
		return nil, err
	}
	sessionCache[key] = s
	return s, nil
}

// Subset of CloudTrail event schema — only the fields we surface in Slack/Discord/Telegram
// notifications. Everything else in the payload is ignored so schema drift on unused
// fields doesn't break us.
type ctEvent struct {
	EventTime       time.Time `json:"-"`
	EventTimeRaw    string    `json:"eventTime"`
	EventName       string    `json:"eventName"`
	SourceIPAddress string    `json:"sourceIPAddress"`
	UserIdentity    struct {
		Type      string `json:"type"`
		ARN       string `json:"arn"`
		UserName  string `json:"userName"`
		AccountId string `json:"accountId"`
		// AssumedRole / FederatedUser identities carry the human-readable role
		// name under sessionContext.sessionIssuer.userName.
		SessionContext struct {
			SessionIssuer struct {
				Type     string `json:"type"`
				UserName string `json:"userName"`
				ARN      string `json:"arn"`
			} `json:"sessionIssuer"`
		} `json:"sessionContext"`
	} `json:"userIdentity"`
	ErrorCode string `json:"errorCode,omitempty"`
}

// Actor returns the most-specific human-readable identity of the caller, falling
// back through sessionIssuer.userName (for assumed roles, the typical case for
// SC CI deploys) → userIdentity.userName → userIdentity.arn → "unknown".
func (e ctEvent) Actor() string {
	if n := e.UserIdentity.SessionContext.SessionIssuer.UserName; n != "" {
		return n
	}
	if e.UserIdentity.UserName != "" {
		return e.UserIdentity.UserName
	}
	if e.UserIdentity.ARN != "" {
		return e.UserIdentity.ARN
	}
	return "unknown"
}

// enrichmentConfig holds the three env-var values needed to look up matching
// CloudTrail events for a given CloudWatch alarm firing.
type enrichmentConfig struct {
	LogGroupName   string
	LogGroupRegion string
	FilterPattern  string
}

// lookupTriggeringEvents calls CloudWatch Logs FilterLogEvents over the given
// log group with the metric-filter pattern that fed the alarm, scoped to the
// time window [alarmFiredAt - lookback, alarmFiredAt + buffer]. Each returned
// log event's message is a CloudTrail JSON record; we parse only the subset
// of fields we care about (ctEvent).
//
// Returns at most `limit` events (sorted newest-first) plus the total count
// of matched events before truncation — so the caller can render an accurate
// "showing X of Y" header when the window was busy.
//
// The lookback covers the alarm's evaluation period (5 min in CloudTrailSecurity
// Alerts) plus a small safety margin; the trailing buffer protects against
// clock skew between CloudTrail publishing and the alarm firing.
func lookupTriggeringEvents(
	ctx context.Context,
	log logger.Logger,
	cfg enrichmentConfig,
	alarmFiredAt time.Time,
	limit int,
) ([]ctEvent, int, error) {
	if cfg.LogGroupName == "" || cfg.FilterPattern == "" {
		return nil, 0, nil
	}
	if limit <= 0 {
		limit = 5
	}

	sess, err := sessionForRegion(cfg.LogGroupRegion)
	if err != nil {
		return nil, 0, errors.Wrapf(err, "failed to create AWS session for %q", cfg.LogGroupRegion)
	}
	client := cloudwatchlogs.New(sess)

	const lookback = 10 * time.Minute
	const buffer = 1 * time.Minute
	start := alarmFiredAt.Add(-lookback).UnixMilli()
	end := alarmFiredAt.Add(buffer).UnixMilli()

	// We only render `limit` events (default 5) but fetch a few more so we
	// have something to choose the newest from. Capped to keep cold-path
	// memory + network bounded even under an alarm storm. 50 is a
	// comfortable ceiling: at full payload (~1 KB per CloudTrail record)
	// that's ≤50 KB transferred and parsed.
	fetchCap := int64(limit * 5)
	if fetchCap < 50 {
		fetchCap = 50
	}

	out, err := client.FilterLogEventsWithContext(ctx, &cloudwatchlogs.FilterLogEventsInput{
		LogGroupName:  aws.String(cfg.LogGroupName),
		FilterPattern: aws.String(cfg.FilterPattern),
		StartTime:     aws.Int64(start),
		EndTime:       aws.Int64(end),
		Limit:         aws.Int64(fetchCap),
	})
	if err != nil {
		return nil, 0, errors.Wrapf(err, "FilterLogEvents on %q", cfg.LogGroupName)
	}

	events := make([]ctEvent, 0, len(out.Events))
	for _, e := range out.Events {
		if e == nil || e.Message == nil {
			continue
		}
		var ce ctEvent
		if err := json.Unmarshal([]byte(*e.Message), &ce); err != nil {
			// A malformed record in the log group shouldn't abort enrichment —
			// log and skip the one event.
			log.Warn(ctx, "failed to parse CloudTrail record: %v", err)
			continue
		}
		if t, perr := time.Parse(time.RFC3339, ce.EventTimeRaw); perr == nil {
			ce.EventTime = t
		}
		events = append(events, ce)
	}
	total := len(events)

	sort.Slice(events, func(i, j int) bool { return events[i].EventTime.After(events[j].EventTime) })
	if len(events) > limit {
		events = events[:limit]
	}
	return events, total, nil
}

// formatEventsForNotification renders a list of CloudTrail events as a Slack/
// Discord/Telegram-friendly text block. Safe to append to an existing alert
// description. Empty list → empty string.
func formatEventsForNotification(events []ctEvent, total int) string {
	if len(events) == 0 {
		return ""
	}
	var b strings.Builder
	if total > len(events) {
		b.WriteString(fmt.Sprintf("\n\n*Recent matching events* (showing %d of %d):\n", len(events), total))
	} else {
		b.WriteString(fmt.Sprintf("\n\n*Recent matching events* (%d):\n", len(events)))
	}
	for _, e := range events {
		ts := e.EventTimeRaw
		if !e.EventTime.IsZero() {
			ts = e.EventTime.UTC().Format("15:04:05 UTC")
		}
		line := fmt.Sprintf("• `%s` by `%s`", e.EventName, e.Actor())
		if e.SourceIPAddress != "" {
			line += fmt.Sprintf(" from `%s`", e.SourceIPAddress)
		}
		if ts != "" {
			line += fmt.Sprintf(" at %s", ts)
		}
		if e.ErrorCode != "" {
			line += fmt.Sprintf(" (errorCode: `%s`)", e.ErrorCode)
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	return b.String()
}
