package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/internal/activitywatcher/handler"
	"github.com/simple-container-com/api/internal/activitywatcher/repository"
	"github.com/simple-container-com/api/internal/activitywatcher/service"
)

func newTestServer(t *testing.T) http.Handler {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "test-events-*.db")
	if err != nil {
		t.Fatalf("create temp db: %v", err)
	}
	f.Close()

	repo, err := repository.NewSQLiteRepository(f.Name())
	if err != nil {
		t.Fatalf("open repo: %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })

	svc := service.NewEventService(repo)
	evtH := handler.NewEventHandler(svc, newTestLogger())
	healthH := handler.NewHealthHandler()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", healthH.Health)
	mux.HandleFunc("POST /events", evtH.Create)
	mux.HandleFunc("GET /users/{user_id}/events", evtH.ListByUser)
	return mux
}

func TestHealth(t *testing.T) {
	RegisterTestingT(t)
	srv := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	Expect(rec.Code).To(Equal(http.StatusOK))
	var body map[string]string
	Expect(json.NewDecoder(rec.Body).Decode(&body)).To(Succeed())
	Expect(body["status"]).To(Equal("ok"))
}

func TestCreateEvent_Valid(t *testing.T) {
	RegisterTestingT(t)
	srv := newTestServer(t)

	payload := map[string]interface{}{
		"user_id":     "user-1",
		"event_type":  "page_view",
		"occurred_at": time.Now().Add(-time.Minute).UTC().Format(time.RFC3339),
		"metadata":    map[string]string{"page": "/home"},
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/events", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	Expect(rec.Code).To(Equal(http.StatusCreated))
	var resp map[string]string
	Expect(json.NewDecoder(rec.Body).Decode(&resp)).To(Succeed())
	Expect(resp["id"]).ToNot(BeEmpty())
}

func TestCreateEvent_MissingFields(t *testing.T) {
	RegisterTestingT(t)
	srv := newTestServer(t)

	tests := []struct {
		name    string
		payload map[string]interface{}
	}{
		{"empty body", map[string]interface{}{}},
		{"missing user_id", map[string]interface{}{
			"event_type": "click",
			"occurred_at": time.Now().Add(-time.Minute).Format(time.RFC3339),
			"metadata":    map[string]string{},
		}},
		{"missing event_type", map[string]interface{}{
			"user_id":     "u1",
			"occurred_at": time.Now().Add(-time.Minute).Format(time.RFC3339),
			"metadata":    map[string]string{},
		}},
		{"future occurred_at", map[string]interface{}{
			"user_id":     "u1",
			"event_type":  "click",
			"occurred_at": time.Now().Add(time.Hour).Format(time.RFC3339),
			"metadata":    map[string]string{},
		}},
		{"occurred_at too old", map[string]interface{}{
			"user_id":     "u1",
			"event_type":  "click",
			"occurred_at": time.Now().Add(-31 * 24 * time.Hour).Format(time.RFC3339),
			"metadata":    map[string]string{},
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			body, _ := json.Marshal(tt.payload)
			req := httptest.NewRequest(http.MethodPost, "/events", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			srv.ServeHTTP(rec, req)
			Expect(rec.Code).To(Equal(http.StatusBadRequest), "case: "+tt.name)
		})
	}
}

func TestListUserEvents(t *testing.T) {
	RegisterTestingT(t)
	srv := newTestServer(t)

	// Ingest two events for user-42.
	for i, ts := range []time.Time{
		time.Now().Add(-2 * time.Minute),
		time.Now().Add(-1 * time.Minute),
	} {
		payload := map[string]interface{}{
			"user_id":     "user-42",
			"event_type":  "click",
			"occurred_at": ts.UTC().Format(time.RFC3339),
			"metadata":    map[string]int{"seq": i},
		}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, "/events", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		Expect(rec.Code).To(Equal(http.StatusCreated))
	}

	req := httptest.NewRequest(http.MethodGet, "/users/user-42/events", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	Expect(rec.Code).To(Equal(http.StatusOK))
	var events []map[string]interface{}
	Expect(json.NewDecoder(rec.Body).Decode(&events)).To(Succeed())
	Expect(events).To(HaveLen(2))
	Expect(events[0]["user_id"]).To(Equal("user-42"))
	// First result should be the newest (occurred_at DESC).
	t0, _ := time.Parse(time.RFC3339, events[0]["occurred_at"].(string))
	t1, _ := time.Parse(time.RFC3339, events[1]["occurred_at"].(string))
	Expect(t0.After(t1) || t0.Equal(t1)).To(BeTrue())
}

func TestListUserEvents_Empty(t *testing.T) {
	RegisterTestingT(t)
	srv := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/users/nobody/events", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	Expect(rec.Code).To(Equal(http.StatusOK))
	var events []map[string]interface{}
	Expect(json.NewDecoder(rec.Body).Decode(&events)).To(Succeed())
	Expect(events).To(HaveLen(0))
}
