package model

import (
	"encoding/json"
	"fmt"
	"time"
)

// Event is the core domain object persisted by the activity watcher.
type Event struct {
	ID         string          `json:"id"`
	UserID     string          `json:"user_id"`
	EventType  string          `json:"event_type"`
	OccurredAt time.Time       `json:"occurred_at"`
	Metadata   json.RawMessage `json:"metadata"`
	CreatedAt  time.Time       `json:"created_at"`
}

// EventInput is the inbound payload for POST /events.
type EventInput struct {
	UserID     string          `json:"user_id"`
	EventType  string          `json:"event_type"`
	OccurredAt time.Time       `json:"occurred_at"`
	Metadata   json.RawMessage `json:"metadata"`
}

const (
	maxFieldLen      = 255
	maxOccurredAtAge = 30 * 24 * time.Hour
)

// Validate returns an error if any field fails the agreed validation rules.
func (e *EventInput) Validate() error {
	if e.UserID == "" {
		return fmt.Errorf("user_id is required")
	}
	if len(e.UserID) > maxFieldLen {
		return fmt.Errorf("user_id must be at most %d characters", maxFieldLen)
	}
	if e.EventType == "" {
		return fmt.Errorf("event_type is required")
	}
	if len(e.EventType) > maxFieldLen {
		return fmt.Errorf("event_type must be at most %d characters", maxFieldLen)
	}
	if e.OccurredAt.IsZero() {
		return fmt.Errorf("occurred_at is required")
	}
	now := time.Now().UTC()
	if e.OccurredAt.After(now) {
		return fmt.Errorf("occurred_at must not be in the future")
	}
	if now.Sub(e.OccurredAt) > maxOccurredAtAge {
		return fmt.Errorf("occurred_at must not be more than 30 days in the past")
	}
	if len(e.Metadata) == 0 {
		return fmt.Errorf("metadata is required")
	}
	var obj map[string]interface{}
	if err := json.Unmarshal(e.Metadata, &obj); err != nil {
		return fmt.Errorf("metadata must be a valid JSON object")
	}
	return nil
}
