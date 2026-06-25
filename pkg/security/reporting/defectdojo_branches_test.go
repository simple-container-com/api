// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package reporting

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	. "github.com/onsi/gomega"
)

func TestDecodeImportScanResponse(t *testing.T) {
	RegisterTestingT(t)

	t.Run("empty data yields zero-value response", func(t *testing.T) {
		RegisterTestingT(t)
		resp := decodeImportScanResponse([]byte("   "))
		Expect(resp).ToNot(BeNil())
		Expect(resp.ID).To(Equal(0))
		Expect(resp.NumberOfFindings).To(Equal(0))
	})

	t.Run("invalid json yields zero-value response", func(t *testing.T) {
		RegisterTestingT(t)
		resp := decodeImportScanResponse([]byte("not json"))
		Expect(resp.ID).To(Equal(0))
	})

	t.Run("direct fields decoded", func(t *testing.T) {
		RegisterTestingT(t)
		resp := decodeImportScanResponse([]byte(`{"id":1,"test":2,"product":3,"engagement":4,"number_of_findings":5}`))
		Expect(resp.ID).To(Equal(1))
		Expect(resp.Test).To(Equal(2))
		Expect(resp.Product).To(Equal(3))
		Expect(resp.Engagement).To(Equal(4))
		Expect(resp.NumberOfFindings).To(Equal(5))
	})

	t.Run("findings_count fallback", func(t *testing.T) {
		RegisterTestingT(t)
		resp := decodeImportScanResponse([]byte(`{"findings_count":8}`))
		Expect(resp.NumberOfFindings).To(Equal(8))
	})

	t.Run("statistics fallback picks first positive key", func(t *testing.T) {
		RegisterTestingT(t)
		// after_count is zero, count is positive -> count wins.
		resp := decodeImportScanResponse([]byte(`{"statistics":{"after_count":0,"count":17}}`))
		Expect(resp.NumberOfFindings).To(Equal(17))
	})

	t.Run("string-typed numeric fields coerced", func(t *testing.T) {
		RegisterTestingT(t)
		resp := decodeImportScanResponse([]byte(`{"id":"21","number_of_findings":"6"}`))
		Expect(resp.ID).To(Equal(21))
		Expect(resp.NumberOfFindings).To(Equal(6))
	})
}

func TestGetOrCreateProduct(t *testing.T) {
	RegisterTestingT(t)

	t.Run("explicit product id short-circuits", func(t *testing.T) {
		RegisterTestingT(t)
		c := NewDefectDojoClient("https://dd", "k")
		id, err := c.getOrCreateProduct(context.Background(), &DefectDojoUploaderConfig{ProductID: 12})
		Expect(err).ToNot(HaveOccurred())
		Expect(id).To(Equal(12))
	})

	t.Run("found existing product by name", func(t *testing.T) {
		RegisterTestingT(t)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"results": []map[string]interface{}{{"id": 31, "name": "demo"}},
			})
		}))
		defer server.Close()
		c := NewDefectDojoClient(server.URL, "k")
		id, err := c.getOrCreateProduct(context.Background(), &DefectDojoUploaderConfig{ProductName: "demo"})
		Expect(err).ToNot(HaveOccurred())
		Expect(id).To(Equal(31))
	})

	t.Run("creates product when none found", func(t *testing.T) {
		RegisterTestingT(t)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodGet && r.URL.Path == "/api/v2/products/":
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"results": []map[string]interface{}{}})
			case r.Method == http.MethodPost && r.URL.Path == "/api/v2/products/":
				w.WriteHeader(http.StatusCreated)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"id": 44})
			default:
				t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
				http.Error(w, "x", http.StatusInternalServerError)
			}
		}))
		defer server.Close()
		c := NewDefectDojoClient(server.URL, "k")
		id, err := c.getOrCreateProduct(context.Background(), &DefectDojoUploaderConfig{ProductName: "new"})
		Expect(err).ToNot(HaveOccurred())
		Expect(id).To(Equal(44))
	})

	t.Run("propagates list error", func(t *testing.T) {
		RegisterTestingT(t)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "down", http.StatusInternalServerError)
		}))
		defer server.Close()
		c := NewDefectDojoClient(server.URL, "k")
		_, err := c.getOrCreateProduct(context.Background(), &DefectDojoUploaderConfig{ProductName: "demo"})
		Expect(err).To(HaveOccurred())
	})
}

func TestListProductsBadStatusAndDecode(t *testing.T) {
	RegisterTestingT(t)

	t.Run("non-200 errors", func(t *testing.T) {
		RegisterTestingT(t)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "no", http.StatusForbidden)
		}))
		defer server.Close()
		c := NewDefectDojoClient(server.URL, "k")
		_, err := c.listProducts(context.Background(), "demo")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("403"))
	})

	t.Run("invalid json body errors", func(t *testing.T) {
		RegisterTestingT(t)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("not-json"))
		}))
		defer server.Close()
		c := NewDefectDojoClient(server.URL, "k")
		_, err := c.listProducts(context.Background(), "demo")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("decoding response"))
	})
}

func TestCreateProductErrors(t *testing.T) {
	RegisterTestingT(t)

	t.Run("non-201 errors", func(t *testing.T) {
		RegisterTestingT(t)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "bad", http.StatusBadRequest)
		}))
		defer server.Close()
		c := NewDefectDojoClient(server.URL, "k")
		_, err := c.createProduct(context.Background(), &DefectDojoUploaderConfig{ProductName: "p"})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("400"))
	})

	t.Run("invalid created body errors", func(t *testing.T) {
		RegisterTestingT(t)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte("oops"))
		}))
		defer server.Close()
		c := NewDefectDojoClient(server.URL, "k")
		_, err := c.createProduct(context.Background(), &DefectDojoUploaderConfig{ProductName: "p"})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("decoding response"))
	})
}

func TestCreateEngagementErrors(t *testing.T) {
	RegisterTestingT(t)

	t.Run("product resolution failure propagates", func(t *testing.T) {
		RegisterTestingT(t)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "down", http.StatusInternalServerError)
		}))
		defer server.Close()
		c := NewDefectDojoClient(server.URL, "k")
		_, err := c.createEngagement(context.Background(), &DefectDojoUploaderConfig{ProductName: "p", EngagementName: "e"})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("getting product"))
	})

	t.Run("non-201 engagement create errors", func(t *testing.T) {
		RegisterTestingT(t)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost && r.URL.Path == "/api/v2/engagements/" {
				http.Error(w, "bad", http.StatusBadRequest)
				return
			}
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			http.Error(w, "x", http.StatusInternalServerError)
		}))
		defer server.Close()
		c := NewDefectDojoClient(server.URL, "k")
		_, err := c.createEngagement(context.Background(), &DefectDojoUploaderConfig{ProductID: 1, EngagementName: "e"})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("400"))
	})

	t.Run("invalid created engagement body errors", func(t *testing.T) {
		RegisterTestingT(t)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte("bogus"))
		}))
		defer server.Close()
		c := NewDefectDojoClient(server.URL, "k")
		_, err := c.createEngagement(context.Background(), &DefectDojoUploaderConfig{ProductID: 1, EngagementName: "e"})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("decoding response"))
	})
}

func TestImportScanBadStatus(t *testing.T) {
	RegisterTestingT(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusBadRequest)
	}))
	defer server.Close()
	c := NewDefectDojoClient(server.URL, "k")
	_, err := c.importScan(context.Background(), 1, []byte(`{}`), "img", &DefectDojoUploaderConfig{
		Environment: "Production", Tags: []string{"ci"},
	})
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("400"))
}

func TestListTests(t *testing.T) {
	RegisterTestingT(t)

	t.Run("returns results", func(t *testing.T) {
		RegisterTestingT(t)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("engagement") != "9" {
				t.Errorf("unexpected engagement filter: %s", r.URL.RawQuery)
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"results": []map[string]interface{}{{"id": 3, "title": "t"}},
			})
		}))
		defer server.Close()
		c := NewDefectDojoClient(server.URL, "k")
		tests, err := c.listTests(context.Background(), 9)
		Expect(err).ToNot(HaveOccurred())
		Expect(tests).To(HaveLen(1))
	})

	t.Run("non-200 errors", func(t *testing.T) {
		RegisterTestingT(t)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "no", http.StatusForbidden)
		}))
		defer server.Close()
		c := NewDefectDojoClient(server.URL, "k")
		_, err := c.listTests(context.Background(), 9)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("403"))
	})
}

func TestCountFindings(t *testing.T) {
	RegisterTestingT(t)

	t.Run("by test id when test query succeeds", func(t *testing.T) {
		RegisterTestingT(t)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("test") != "5" {
				t.Errorf("expected test filter 5, got %s", r.URL.RawQuery)
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"count": 14})
		}))
		defer server.Close()
		c := NewDefectDojoClient(server.URL, "k")
		count, err := c.countFindings(context.Background(), 5, 30)
		Expect(err).ToNot(HaveOccurred())
		Expect(count).To(Equal(14))
	})

	t.Run("falls back to engagement when test query fails", func(t *testing.T) {
		RegisterTestingT(t)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("test") != "" {
				http.Error(w, "no", http.StatusInternalServerError)
				return
			}
			if r.URL.Query().Get("test__engagement") != "30" {
				t.Errorf("expected engagement fallback, got %s", r.URL.RawQuery)
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"count": 7})
		}))
		defer server.Close()
		c := NewDefectDojoClient(server.URL, "k")
		count, err := c.countFindings(context.Background(), 5, 30)
		Expect(err).ToNot(HaveOccurred())
		Expect(count).To(Equal(7))
	})

	t.Run("zero test and zero engagement returns zero", func(t *testing.T) {
		RegisterTestingT(t)
		c := NewDefectDojoClient("https://dd", "k")
		count, err := c.countFindings(context.Background(), 0, 0)
		Expect(err).ToNot(HaveOccurred())
		Expect(count).To(Equal(0))
	})
}

func TestCountFindingsByQuery(t *testing.T) {
	RegisterTestingT(t)

	t.Run("non-200 errors", func(t *testing.T) {
		RegisterTestingT(t)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "no", http.StatusForbidden)
		}))
		defer server.Close()
		c := NewDefectDojoClient(server.URL, "k")
		_, err := c.countFindingsByQuery(context.Background(), "/api/v2/findings/?test=1")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("403"))
	})

	t.Run("invalid json errors", func(t *testing.T) {
		RegisterTestingT(t)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("oops"))
		}))
		defer server.Close()
		c := NewDefectDojoClient(server.URL, "k")
		_, err := c.countFindingsByQuery(context.Background(), "/api/v2/findings/?test=1")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("decoding response"))
	})
}
