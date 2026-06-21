// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package helpers

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/pkg/errors"

	"github.com/simple-container-com/api/pkg/api/logger"
)

// Per-region config cache. Lambda containers may handle many sequential
// invocations across their ~5-15 minute lifetime; reusing the resolved
// aws.Config (credentials provider, HTTP client) avoids redoing credential
// resolution on every alarm. Keyed by region because different CloudTrail log
// groups may live in different regions.
//
// aws-sdk-go-v2 replaces v1's *session.Session with aws.Config, which is a
// value type — we cache it by value rather than by pointer.
var (
	configCacheMu sync.Mutex
	configCache   = map[string]aws.Config{}
)

func configForRegion(ctx context.Context, region string) (aws.Config, error) {
	// Empty region → AWS SDK resolves from AWS_REGION / AWS_DEFAULT_REGION
	// (set automatically in Lambda). We still cache under a sentinel key
	// so repeat empty-region callers share one config.
	key := region
	if key == "" {
		key = "__default__"
	}
	configCacheMu.Lock()
	defer configCacheMu.Unlock()
	if c, ok := configCache[key]; ok {
		return c, nil
	}
	opts := []func(*config.LoadOptions) error{}
	if region != "" {
		opts = append(opts, config.WithRegion(region))
	}
	c, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return aws.Config{}, err
	}
	configCache[key] = c
	return c, nil
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

// Enrichment budget constants. Chosen to keep the Lambda's 10s handler
// timeout mostly available for the Slack/Discord/Telegram send calls that
// follow — if FilterLogEvents gets slow, we bail out with whatever we have
// (or nothing) rather than letting it swallow the entire budget and drop
// the alert.
const (
	// enrichmentTimeout caps the total wall-clock of the CloudTrail lookup.
	// Must leave room for ~3 webhook POSTs after it.
	enrichmentTimeout = 3 * time.Second
	// maxPages caps how many FilterLogEvents pages we'll read before
	// stopping. Prevents pathological cases (thousands of matching records)
	// from dominating the Lambda budget.
	maxPages = 5
	// perPageLimit is the Limit parameter sent to FilterLogEvents. Combined
	// with maxPages, the hard ceiling per invocation is 250 events fetched.
	// Typed int32 because aws-sdk-go-v2 narrowed the field from *int64 (v1)
	// to *int32 (v2) — the over-the-wire ceiling is 10_000, so int32 is safe.
	perPageLimit int32 = 50
)

// lookupTriggeringEvents calls CloudWatch Logs FilterLogEvents over the given
// log group with the metric-filter pattern that fed the alarm, scoped to the
// time window [alarmFiredAt - lookback, alarmFiredAt + buffer]. Each returned
// log event's message is a CloudTrail JSON record; we parse only the subset
// of fields we care about (ctEvent).
//
// Returns at most `limit` events (sorted newest-first) plus the total count
// of matched events across all pages we actually fetched. When more pages
// exist beyond our page cap the returned total is suffixed externally (see
// formatEventsForNotification's '+' decoration) so the "showing X of Y"
// header doesn't silently under-report during high-burst incidents.
//
// The call is bounded by enrichmentTimeout so a slow CloudWatch Logs
// endpoint can't consume the Lambda's 10s handler budget and starve the
// webhook-send step that runs afterward.
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

	awsCfg, err := configForRegion(ctx, cfg.LogGroupRegion)
	if err != nil {
		return nil, 0, errors.Wrapf(err, "failed to load AWS config for %q", cfg.LogGroupRegion)
	}
	client := cloudwatchlogs.NewFromConfig(awsCfg)

	const lookback = 10 * time.Minute
	const buffer = 1 * time.Minute
	start := alarmFiredAt.Add(-lookback).UnixMilli()
	end := alarmFiredAt.Add(buffer).UnixMilli()

	// Budget the entire lookup — connection + all pages — so a slow endpoint
	// can't drop the alert.
	lookupCtx, cancel := context.WithTimeout(ctx, enrichmentTimeout)
	defer cancel()

	pageLimit := perPageLimit
	input := &cloudwatchlogs.FilterLogEventsInput{
		LogGroupName:  aws.String(cfg.LogGroupName),
		FilterPattern: aws.String(cfg.FilterPattern),
		StartTime:     aws.Int64(start),
		EndTime:       aws.Int64(end),
		Limit:         &pageLimit,
	}

	events := make([]ctEvent, 0, perPageLimit)
	pages := 0
	truncated := false
	for {
		// aws-sdk-go-v2 folds the WithContext suffix into the canonical
		// method signature; ctx is the first argument.
		out, err := client.FilterLogEvents(lookupCtx, input)
		if err != nil {
			return nil, 0, errors.Wrapf(err, "FilterLogEvents on %q (page %d)", cfg.LogGroupName, pages+1)
		}
		// v2 returns []types.FilteredLogEvent (value slice) where v1 returned
		// []*FilteredLogEvent; the element itself can no longer be nil, so we
		// only need to guard against an absent Message pointer.
		for _, e := range out.Events {
			if e.Message == nil {
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
		pages++
		if out.NextToken == nil || *out.NextToken == "" {
			break
		}
		if pages >= maxPages {
			// More pages available, but we've hit the cap. Flag it so the
			// "total" reported to the caller is explicitly a floor.
			truncated = true
			break
		}
		input.NextToken = out.NextToken
	}
	total := len(events)

	sort.Slice(events, func(i, j int) bool { return events[i].EventTime.After(events[j].EventTime) })
	if len(events) > limit {
		events = events[:limit]
	}
	// When we stopped short of iterating every page, encode that in the
	// returned total: negate it. formatEventsForNotification interprets a
	// negative value as "≥|n| matches" to surface the truncation honestly
	// instead of claiming a definite total that we don't actually know.
	if truncated {
		total = -total
	}
	return events, total, nil
}

// formatEventsForNotification renders a list of CloudTrail events as a Slack/
// Discord/Telegram-friendly text block. Safe to append to an existing alert
// description. Empty list → empty string.
//
// The `total` argument encodes two cases:
//   - positive: the exact count of matching events (we saw every page).
//   - negative: we stopped at the page cap; |total| is a lower bound and
//     we render the count as "≥|n|" to avoid claiming a precise total we
//     don't actually know.
func formatEventsForNotification(events []ctEvent, total int) string {
	if len(events) == 0 {
		return ""
	}
	var b strings.Builder
	switch {
	case total < 0:
		// Truncated — we saw ≥|total| events, but there were more pages.
		b.WriteString(fmt.Sprintf("\n\n*Recent matching events* (showing %d of ≥%d):\n", len(events), -total))
	case total > len(events):
		b.WriteString(fmt.Sprintf("\n\n*Recent matching events* (showing %d of %d):\n", len(events), total))
	default:
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
