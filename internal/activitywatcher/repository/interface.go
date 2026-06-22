package repository

import (
	"context"

	"github.com/simple-container-com/api/internal/activitywatcher/model"
)

// EventRepository is the storage interface for activity events.
// Implementations can swap the backend (SQLite → Postgres) without touching handlers.
type EventRepository interface {
	// Create persists a new event and returns it with server-assigned fields populated.
	Create(ctx context.Context, event *model.Event) error
	// ListByUser returns events for the given user, newest first.
	ListByUser(ctx context.Context, userID string) ([]*model.Event, error)
}
