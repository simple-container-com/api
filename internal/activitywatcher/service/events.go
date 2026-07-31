package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/simple-container-com/api/internal/activitywatcher/model"
	"github.com/simple-container-com/api/internal/activitywatcher/repository"
)

// EventService encapsulates business logic for activity events.
type EventService struct {
	repo repository.EventRepository
}

// NewEventService constructs a service with the given repository.
func NewEventService(repo repository.EventRepository) *EventService {
	return &EventService{repo: repo}
}

// CreateEvent validates input, assigns a UUID, and persists the event.
func (s *EventService) CreateEvent(ctx context.Context, input *model.EventInput) (*model.Event, error) {
	if err := input.Validate(); err != nil {
		return nil, fmt.Errorf("validation: %w", err)
	}
	event := &model.Event{
		ID:         uuid.NewString(),
		UserID:     input.UserID,
		EventType:  input.EventType,
		OccurredAt: input.OccurredAt,
		Metadata:   input.Metadata,
	}
	if err := s.repo.Create(ctx, event); err != nil {
		return nil, fmt.Errorf("persist: %w", err)
	}
	return event, nil
}

// ListUserEvents returns all events for a user, newest first.
func (s *EventService) ListUserEvents(ctx context.Context, userID string) ([]*model.Event, error) {
	if userID == "" {
		return nil, fmt.Errorf("user_id is required")
	}
	return s.repo.ListByUser(ctx, userID)
}
