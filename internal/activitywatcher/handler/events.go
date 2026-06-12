package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/simple-container-com/api/internal/activitywatcher/model"
	"github.com/simple-container-com/api/internal/activitywatcher/service"
)

// EventHandler handles HTTP requests for activity events.
type EventHandler struct {
	svc    *service.EventService
	logger *slog.Logger
}

// NewEventHandler constructs an EventHandler.
func NewEventHandler(svc *service.EventService, logger *slog.Logger) *EventHandler {
	return &EventHandler{svc: svc, logger: logger}
}

// Create handles POST /events.
// TODO: auth — add API-key middleware in Phase 2.
func (h *EventHandler) Create(w http.ResponseWriter, r *http.Request) {
	var input model.EventInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	event, err := h.svc.CreateEvent(r.Context(), &input)
	if err != nil {
		// Validation errors begin with "validation:"
		h.logger.ErrorContext(r.Context(), "create event", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": event.ID})
}

// ListByUser handles GET /users/{user_id}/events.
func (h *EventHandler) ListByUser(w http.ResponseWriter, r *http.Request) {
	userID := r.PathValue("user_id")
	events, err := h.svc.ListUserEvents(r.Context(), userID)
	if err != nil {
		h.logger.ErrorContext(r.Context(), "list events", "user_id", userID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list events")
		return
	}
	if events == nil {
		events = []*model.Event{}
	}
	writeJSON(w, http.StatusOK, events)
}

func writeJSON(w http.ResponseWriter, status int, body interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
