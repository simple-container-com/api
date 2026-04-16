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
	config := &DefectDojoUploaderConfig{TestType: "Container Scan", ProductName: "everworker"}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v2/import-scan/":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"message":"success"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v2/tests/":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"results": []map[string]interface{}{
					{"id": 99, "title": "Container Scan - everworker", "engagement": 42},
					{"id": 98, "title": "older", "engagement": 42},
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v2/findings/":
			if got := r.URL.Query().Get("test"); got != "99" {
				t.Errorf("findings test filter = %q, want %q", got, "99")
				http.Error(w, "bad filter", http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"count": 24})
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.String())
			http.Error(w, "unexpected", http.StatusInternalServerError)
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
				t.Errorf("reading request body: %v", err)
				http.Error(w, "read error", http.StatusInternalServerError)
				return
			}
			if !strings.Contains(string(body), `"engagement_type":"CI/CD"`) {
				t.Errorf("request body = %s, want CI/CD engagement type", string(body))
			}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"id": 42})
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.String())
			http.Error(w, "unexpected", http.StatusInternalServerError)
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

func TestTestTitle(t *testing.T) {
	client := NewDefectDojoClient("https://dd.example.com", "key")
	tests := []struct {
		testType    string
		productName string
		want        string
	}{
		{"", "everworker", "Container Scan - everworker"},
		{"Custom Type", "myapp", "Custom Type - myapp"},
		{"", "", "Container Scan"},
	}
	for _, tt := range tests {
		cfg := &DefectDojoUploaderConfig{TestType: tt.testType, ProductName: tt.productName}
		got := client.testTitle(cfg, "image@sha256:abc")
		if got != tt.want {
			t.Errorf("testTitle(type=%q, product=%q) = %q, want %q", tt.testType, tt.productName, got, tt.want)
		}
	}
}

func TestFindLatestTestByTitle(t *testing.T) {
	tests := []DefectDojoTest{
		{ID: 1, Title: "Container Scan - everworker"},
		{ID: 5, Title: "Other Test"},
		{ID: 10, Title: "Container Scan - everworker"},
		{ID: 3, Title: "Container Scan - everworker"},
	}

	match := findLatestTestByTitle(tests, "Container Scan - everworker")
	if match == nil {
		t.Fatal("expected match")
	}
	if match.ID != 10 {
		t.Errorf("findLatestTestByTitle() ID = %d, want 10 (latest)", match.ID)
	}

	noMatch := findLatestTestByTitle(tests, "nonexistent")
	if noMatch != nil {
		t.Error("expected nil for nonexistent title")
	}
}
