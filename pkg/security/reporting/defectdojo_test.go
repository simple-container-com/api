package reporting

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestImportScanEnrichesMissingResponseFields(t *testing.T) {
	imageRef := "registry.example.com/demo@sha256:1234"
	config := &DefectDojoUploaderConfig{TestType: "Container Scan"}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v2/import-scan/":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"message":"success"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v2/tests/":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"results": []map[string]interface{}{
					{"id": 99, "title": "Container Scan - " + imageRef, "engagement": 42},
					{"id": 98, "title": "older", "engagement": 42},
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v2/findings/":
			if got := r.URL.Query().Get("test"); got != "99" {
				t.Fatalf("findings test filter = %q, want %q", got, "99")
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"count": 24})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client := NewDefectDojoClient(server.URL, "secret")
	resp, err := client.importScan(context.Background(), 42, []byte(`{}`), imageRef, config)
	if err != nil {
		t.Fatalf("importScan() error = %v", err)
	}

	if got, want := resp.Engagement, 42; got != want {
		t.Fatalf("Engagement = %d, want %d", got, want)
	}
	if got, want := resp.Test, 99; got != want {
		t.Fatalf("Test = %d, want %d", got, want)
	}
	if got, want := resp.NumberOfFindings, 24; got != want {
		t.Fatalf("NumberOfFindings = %d, want %d", got, want)
	}
}

func TestCreateEngagementUsesCICDType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v2/products/":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"results": []map[string]interface{}{}})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v2/products/":
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"id": 8, "name": "demo"})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v2/engagements/":
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("reading request body: %v", err)
			}
			if !strings.Contains(string(body), `"engagement_type":"CI/CD"`) {
				t.Fatalf("request body = %s, want CI/CD engagement type", string(body))
			}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"id": 42})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client := NewDefectDojoClient(server.URL, "secret")
	id, err := client.createEngagement(context.Background(), &DefectDojoUploaderConfig{
		ProductName:    "demo",
		EngagementName: "staging",
		AutoCreate:     true,
	})
	if err != nil {
		t.Fatalf("createEngagement() error = %v", err)
	}
	if got, want := id, 42; got != want {
		t.Fatalf("engagement ID = %d, want %d", got, want)
	}
}
