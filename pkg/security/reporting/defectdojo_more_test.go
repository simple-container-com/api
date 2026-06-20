// SPDX-License-Identifier: MIT
// Copyright (c) 2025 Simple Container

package reporting

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/security/scan"
)

func TestNewDefectDojoClientTrimsAndDefaults(t *testing.T) {
	RegisterTestingT(t)

	t.Run("https url retained and trailing slash trimmed", func(t *testing.T) {
		RegisterTestingT(t)
		c := NewDefectDojoClient("https://dd.example.com/", "key")
		Expect(c.BaseURL).To(Equal("https://dd.example.com"))
		Expect(c.APIKey).To(Equal("key"))
		Expect(c.HTTPClient).ToNot(BeNil())
	})

	t.Run("non-https url is still accepted (warning path)", func(t *testing.T) {
		RegisterTestingT(t)
		// http:// triggers the cleartext warning to stderr; we only assert the
		// client is constructed and the URL trimmed.
		c := NewDefectDojoClient("http://dd.local///", "key")
		Expect(c.BaseURL).To(Equal("http://dd.local"))
	})
}

func TestIntValue(t *testing.T) {
	RegisterTestingT(t)

	cases := []struct {
		name string
		in   interface{}
		want int
	}{
		{"nil", nil, 0},
		{"int", 7, 7},
		{"int32", int32(8), 8},
		{"int64", int64(9), 9},
		{"float64", float64(10.9), 10},
		{"json.Number valid", json.Number("11"), 11},
		{"json.Number invalid", json.Number("not-a-number"), 0},
		{"string numeric with spaces", "  12  ", 12},
		{"string non-numeric", "abc", 0},
		{"nested map id", map[string]interface{}{"id": float64(13)}, 13},
		{"nested map without id", map[string]interface{}{"x": 1}, 0},
		{"unhandled type bool", true, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			Expect(intValue(tc.in)).To(Equal(tc.want))
		})
	}
}

func TestEngagementExists(t *testing.T) {
	RegisterTestingT(t)

	t.Run("200 means exists", func(t *testing.T) {
		RegisterTestingT(t)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()
		c := NewDefectDojoClient(server.URL, "k")
		ok, err := c.engagementExists(context.Background(), 5)
		Expect(err).ToNot(HaveOccurred())
		Expect(ok).To(BeTrue())
	})

	t.Run("404 means not exists", func(t *testing.T) {
		RegisterTestingT(t)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()
		c := NewDefectDojoClient(server.URL, "k")
		ok, err := c.engagementExists(context.Background(), 5)
		Expect(err).ToNot(HaveOccurred())
		Expect(ok).To(BeFalse())
	})

	t.Run("other status is an error", func(t *testing.T) {
		RegisterTestingT(t)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "boom", http.StatusInternalServerError)
		}))
		defer server.Close()
		c := NewDefectDojoClient(server.URL, "k")
		_, err := c.engagementExists(context.Background(), 5)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("500"))
	})
}

func TestListEngagements(t *testing.T) {
	RegisterTestingT(t)

	t.Run("returns results and forwards filters", func(t *testing.T) {
		RegisterTestingT(t)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("name") != "staging" || r.URL.Query().Get("product") != "3" {
				t.Errorf("unexpected query: %s", r.URL.RawQuery)
				http.Error(w, "bad", http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"results": []map[string]interface{}{{"id": 11, "name": "staging"}},
			})
		}))
		defer server.Close()
		c := NewDefectDojoClient(server.URL, "k")
		eng, err := c.listEngagements(context.Background(), &DefectDojoUploaderConfig{EngagementName: "staging", ProductID: 3})
		Expect(err).ToNot(HaveOccurred())
		Expect(eng).To(HaveLen(1))
		Expect(eng[0].ID).To(Equal(11))
	})

	t.Run("non-200 status returns error", func(t *testing.T) {
		RegisterTestingT(t)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "nope", http.StatusForbidden)
		}))
		defer server.Close()
		c := NewDefectDojoClient(server.URL, "k")
		_, err := c.listEngagements(context.Background(), &DefectDojoUploaderConfig{})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("403"))
	})
}

func TestEngagementLookupConfig(t *testing.T) {
	RegisterTestingT(t)

	t.Run("nil config returns nil", func(t *testing.T) {
		RegisterTestingT(t)
		c := NewDefectDojoClient("https://dd", "k")
		got, err := c.engagementLookupConfig(context.Background(), nil)
		Expect(err).ToNot(HaveOccurred())
		Expect(got).To(BeNil())
	})

	t.Run("product id already set short-circuits", func(t *testing.T) {
		RegisterTestingT(t)
		c := NewDefectDojoClient("https://dd", "k")
		cfg := &DefectDojoUploaderConfig{ProductID: 9, ProductName: "p"}
		got, err := c.engagementLookupConfig(context.Background(), cfg)
		Expect(err).ToNot(HaveOccurred())
		Expect(got.ProductID).To(Equal(9))
	})

	t.Run("no product name short-circuits", func(t *testing.T) {
		RegisterTestingT(t)
		c := NewDefectDojoClient("https://dd", "k")
		got, err := c.engagementLookupConfig(context.Background(), &DefectDojoUploaderConfig{})
		Expect(err).ToNot(HaveOccurred())
		Expect(got.ProductID).To(Equal(0))
	})

	t.Run("resolves product id from name", func(t *testing.T) {
		RegisterTestingT(t)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"results": []map[string]interface{}{{"id": 77, "name": "demo"}},
			})
		}))
		defer server.Close()
		c := NewDefectDojoClient(server.URL, "k")
		got, err := c.engagementLookupConfig(context.Background(), &DefectDojoUploaderConfig{ProductName: "demo"})
		Expect(err).ToNot(HaveOccurred())
		Expect(got.ProductID).To(Equal(77))
	})

	t.Run("propagates product lookup error", func(t *testing.T) {
		RegisterTestingT(t)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "down", http.StatusInternalServerError)
		}))
		defer server.Close()
		c := NewDefectDojoClient(server.URL, "k")
		_, err := c.engagementLookupConfig(context.Background(), &DefectDojoUploaderConfig{ProductName: "demo"})
		Expect(err).To(HaveOccurred())
	})

	t.Run("product name unmatched leaves product id zero", func(t *testing.T) {
		RegisterTestingT(t)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"results": []map[string]interface{}{}})
		}))
		defer server.Close()
		c := NewDefectDojoClient(server.URL, "k")
		got, err := c.engagementLookupConfig(context.Background(), &DefectDojoUploaderConfig{ProductName: "ghost"})
		Expect(err).ToNot(HaveOccurred())
		Expect(got.ProductID).To(Equal(0))
	})
}

func TestGetOrCreateEngagement(t *testing.T) {
	RegisterTestingT(t)

	t.Run("existing engagement id verified returns it", func(t *testing.T) {
		RegisterTestingT(t)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/v2/engagements/15/" {
				w.WriteHeader(http.StatusOK)
				return
			}
			t.Errorf("unexpected request %s", r.URL.Path)
			http.Error(w, "x", http.StatusInternalServerError)
		}))
		defer server.Close()
		c := NewDefectDojoClient(server.URL, "k")
		id, err := c.getOrCreateEngagement(context.Background(), &DefectDojoUploaderConfig{EngagementID: 15})
		Expect(err).ToNot(HaveOccurred())
		Expect(id).To(Equal(15))
	})

	t.Run("engagement id check errors are propagated", func(t *testing.T) {
		RegisterTestingT(t)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "x", http.StatusInternalServerError)
		}))
		defer server.Close()
		c := NewDefectDojoClient(server.URL, "k")
		_, err := c.getOrCreateEngagement(context.Background(), &DefectDojoUploaderConfig{EngagementID: 15})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("checking engagement existence"))
	})

	t.Run("finds engagement by name", func(t *testing.T) {
		RegisterTestingT(t)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/v2/engagements/" {
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"results": []map[string]interface{}{{"id": 21, "name": "staging"}},
				})
				return
			}
			t.Errorf("unexpected request %s", r.URL.Path)
			http.Error(w, "x", http.StatusInternalServerError)
		}))
		defer server.Close()
		c := NewDefectDojoClient(server.URL, "k")
		id, err := c.getOrCreateEngagement(context.Background(), &DefectDojoUploaderConfig{EngagementName: "staging", ProductID: 1})
		Expect(err).ToNot(HaveOccurred())
		Expect(id).To(Equal(21))
	})

	t.Run("auto-create when nothing found", func(t *testing.T) {
		RegisterTestingT(t)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodGet && r.URL.Path == "/api/v2/engagements/":
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"results": []map[string]interface{}{}})
			case r.Method == http.MethodPost && r.URL.Path == "/api/v2/engagements/":
				w.WriteHeader(http.StatusCreated)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"id": 55})
			default:
				t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
				http.Error(w, "x", http.StatusInternalServerError)
			}
		}))
		defer server.Close()
		c := NewDefectDojoClient(server.URL, "k")
		id, err := c.getOrCreateEngagement(context.Background(), &DefectDojoUploaderConfig{
			EngagementName: "staging", ProductID: 1, AutoCreate: true,
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(id).To(Equal(55))
	})

	t.Run("error when not found and auto-create disabled", func(t *testing.T) {
		RegisterTestingT(t)
		c := NewDefectDojoClient("https://dd", "k")
		_, err := c.getOrCreateEngagement(context.Background(), &DefectDojoUploaderConfig{AutoCreate: false})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("auto-create is disabled"))
	})

	t.Run("lookup config resolution error propagates", func(t *testing.T) {
		RegisterTestingT(t)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// product lookup (used by engagementLookupConfig) fails
			http.Error(w, "down", http.StatusInternalServerError)
		}))
		defer server.Close()
		c := NewDefectDojoClient(server.URL, "k")
		_, err := c.getOrCreateEngagement(context.Background(), &DefectDojoUploaderConfig{
			EngagementName: "staging", ProductName: "demo",
		})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("resolving engagement lookup config"))
	})
}

func TestReimportScan(t *testing.T) {
	RegisterTestingT(t)

	t.Run("posts to reimport endpoint and decodes response", func(t *testing.T) {
		RegisterTestingT(t)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost || r.URL.Path != "/api/v2/reimport-scan/" {
				t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
				http.Error(w, "x", http.StatusInternalServerError)
				return
			}
			if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "multipart/form-data") {
				t.Errorf("expected multipart content type, got %q", ct)
			}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"test": 7, "number_of_findings": 3})
		}))
		defer server.Close()
		c := NewDefectDojoClient(server.URL, "k")
		resp, err := c.reimportScan(context.Background(), 7, []byte(`{}`), &DefectDojoUploaderConfig{
			Environment: "Production", Tags: []string{"ci", "nightly"},
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(resp.Test).To(Equal(7))
		Expect(resp.NumberOfFindings).To(Equal(3))
	})

	t.Run("non-2xx status returns error", func(t *testing.T) {
		RegisterTestingT(t)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "bad payload", http.StatusBadRequest)
		}))
		defer server.Close()
		c := NewDefectDojoClient(server.URL, "k")
		_, err := c.reimportScan(context.Background(), 7, []byte(`{}`), &DefectDojoUploaderConfig{})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("400"))
	})
}

func TestListTestsPaginated(t *testing.T) {
	RegisterTestingT(t)

	t.Run("follows next pages and accumulates", func(t *testing.T) {
		RegisterTestingT(t)
		var server *httptest.Server
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/v2/tests/" {
				t.Errorf("unexpected path %s", r.URL.Path)
				http.Error(w, "x", http.StatusInternalServerError)
				return
			}
			// First page returns a next pointer; second page closes it.
			if r.URL.Query().Get("offset") == "200" {
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"results": []map[string]interface{}{{"id": 2, "title": "b"}},
					"next":    nil,
				})
				return
			}
			next := server.URL + "/api/v2/tests/?engagement=4&limit=200&offset=200"
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"results": []map[string]interface{}{{"id": 1, "title": "a"}},
				"next":    next,
			})
		}))
		defer server.Close()
		c := NewDefectDojoClient(server.URL, "k")
		tests, err := c.listTestsPaginated(context.Background(), 4)
		Expect(err).ToNot(HaveOccurred())
		Expect(tests).To(HaveLen(2))
		Expect(tests[0].ID).To(Equal(1))
		Expect(tests[1].ID).To(Equal(2))
	})

	t.Run("single page without next", func(t *testing.T) {
		RegisterTestingT(t)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"results": []map[string]interface{}{{"id": 9, "title": "only"}},
			})
		}))
		defer server.Close()
		c := NewDefectDojoClient(server.URL, "k")
		tests, err := c.listTestsPaginated(context.Background(), 4)
		Expect(err).ToNot(HaveOccurred())
		Expect(tests).To(HaveLen(1))
	})

	t.Run("non-200 status returns partial and error", func(t *testing.T) {
		RegisterTestingT(t)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "denied", http.StatusUnauthorized)
		}))
		defer server.Close()
		c := NewDefectDojoClient(server.URL, "k")
		_, err := c.listTestsPaginated(context.Background(), 4)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("401"))
	})
}

func TestUploadScanResult(t *testing.T) {
	RegisterTestingT(t)

	imageRef := "registry/app@sha256:abc"
	result := scan.NewScanResult("sha256:abc", scan.ScanToolGrype, []scan.Vulnerability{
		{ID: "CVE-1", Severity: scan.SeverityHigh, Package: "p", Version: "1"},
	})

	t.Run("reimports when matching test exists", func(t *testing.T) {
		RegisterTestingT(t)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodGet && r.URL.Path == "/api/v2/engagements/30/":
				w.WriteHeader(http.StatusOK)
			case r.Method == http.MethodGet && r.URL.Path == "/api/v2/tests/":
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"results": []map[string]interface{}{
						{"id": 71, "title": "Container Scan - everworker", "engagement": 30},
					},
				})
			case r.Method == http.MethodPost && r.URL.Path == "/api/v2/reimport-scan/":
				w.WriteHeader(http.StatusCreated)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"test": 71, "number_of_findings": 5})
			default:
				t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
				http.Error(w, "x", http.StatusInternalServerError)
			}
		}))
		defer server.Close()
		c := NewDefectDojoClient(server.URL, "k")
		resp, err := c.UploadScanResult(context.Background(), result, imageRef, &DefectDojoUploaderConfig{
			EngagementID: 30, ProductName: "everworker",
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(resp.Test).To(Equal(71))
		Expect(resp.NumberOfFindings).To(Equal(5))
		// reimport response gets engagement stamped from the resolved engagement id.
		Expect(resp.Engagement).To(Equal(30))
	})

	t.Run("falls through to import-scan when no matching test", func(t *testing.T) {
		RegisterTestingT(t)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodGet && r.URL.Path == "/api/v2/engagements/30/":
				w.WriteHeader(http.StatusOK)
			case r.Method == http.MethodGet && r.URL.Path == "/api/v2/tests/":
				// no test with the dedup title -> import-scan path
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"results": []map[string]interface{}{{"id": 1, "title": "unrelated", "engagement": 30}},
				})
			case r.Method == http.MethodPost && r.URL.Path == "/api/v2/import-scan/":
				w.WriteHeader(http.StatusCreated)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"test": 90, "number_of_findings": 2})
			case r.Method == http.MethodGet && r.URL.Path == "/api/v2/findings/":
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"count": 2})
			default:
				t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
				http.Error(w, "x", http.StatusInternalServerError)
			}
		}))
		defer server.Close()
		c := NewDefectDojoClient(server.URL, "k")
		resp, err := c.UploadScanResult(context.Background(), result, imageRef, &DefectDojoUploaderConfig{
			EngagementID: 30, ProductName: "everworker",
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(resp.Test).To(Equal(90))
		Expect(resp.Engagement).To(Equal(30))
	})

	t.Run("engagement resolution failure surfaces error", func(t *testing.T) {
		RegisterTestingT(t)
		c := NewDefectDojoClient("https://dd", "k")
		_, err := c.UploadScanResult(context.Background(), result, imageRef, &DefectDojoUploaderConfig{
			AutoCreate: false,
		})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("getting engagement"))
	})

	t.Run("test-list failure still falls through to import-scan", func(t *testing.T) {
		RegisterTestingT(t)
		// listTestsPaginated returns an error (500) -> warning logged, import-scan used.
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodGet && r.URL.Path == "/api/v2/engagements/30/":
				w.WriteHeader(http.StatusOK)
			case r.Method == http.MethodGet && r.URL.Path == "/api/v2/tests/":
				http.Error(w, "boom", http.StatusInternalServerError)
			case r.Method == http.MethodPost && r.URL.Path == "/api/v2/import-scan/":
				w.WriteHeader(http.StatusCreated)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"test": 100, "number_of_findings": 1})
			case r.Method == http.MethodGet && r.URL.Path == "/api/v2/findings/":
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"count": 1})
			default:
				t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
				http.Error(w, "x", http.StatusInternalServerError)
			}
		}))
		defer server.Close()
		c := NewDefectDojoClient(server.URL, "k")
		resp, err := c.UploadScanResult(context.Background(), result, imageRef, &DefectDojoUploaderConfig{
			EngagementID: 30, ProductName: "everworker",
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(resp.Test).To(Equal(100))
	})
}
