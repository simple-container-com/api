package handler

import (
	"encoding/json"
	"net/http"
)

// HealthHandler handles GET /health liveness probes.
type HealthHandler struct{}

// NewHealthHandler constructs a HealthHandler.
func NewHealthHandler() *HealthHandler { return &HealthHandler{} }

// Health responds with 200 {"status":"ok"}.
func (h *HealthHandler) Health(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
