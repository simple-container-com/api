package reporting

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	. "github.com/onsi/gomega"
)

func TestImportScanEnrichesMissingResponseFields(t *testing.T) {
	RegisterTestingT(t)

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
			Expect(r.URL.Query().Get("test")).To(Equal("99"), "findings test filter")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"count": 24})
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.String())
			http.Error(w, "unexpected", http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	client := NewDefectDojoClient(server.URL, "secret")
	resp, err := client.importScan(context.Background(), 42, []byte(`{}`), imageRef, config)
	Expect(err).ToNot(HaveOccurred())

	Expect(resp.Engagement).To(Equal(42))
	Expect(resp.Test).To(Equal(99))
	Expect(resp.NumberOfFindings).To(Equal(24))
}

func TestCreateEngagementUsesCICDType(t *testing.T) {
	RegisterTestingT(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v2/products/":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"results": []map[string]interface{}{}})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v2/products/":
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"id": 8, "name": "demo"})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v2/engagements/":
			body, err := io.ReadAll(r.Body)
			Expect(err).ToNot(HaveOccurred())
			Expect(string(body)).To(ContainSubstring(`"engagement_type":"CI/CD"`))
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
	Expect(err).ToNot(HaveOccurred())
	Expect(id).To(Equal(42))
}

func TestTestTitle(t *testing.T) {
	RegisterTestingT(t)

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
		Expect(got).To(Equal(tt.want), "testTitle(type=%q, product=%q)", tt.testType, tt.productName)
	}
}

func TestFindLatestTestByTitle(t *testing.T) {
	RegisterTestingT(t)

	tests := []DefectDojoTest{
		{ID: 1, Title: "Container Scan - everworker"},
		{ID: 5, Title: "Other Test"},
		{ID: 10, Title: "Container Scan - everworker"},
		{ID: 3, Title: "Container Scan - everworker"},
	}

	match := findLatestTestByTitle(tests, "Container Scan - everworker")
	Expect(match).ToNot(BeNil())
	Expect(match.ID).To(Equal(10))

	noMatch := findLatestTestByTitle(tests, "nonexistent")
	Expect(noMatch).To(BeNil())
}
