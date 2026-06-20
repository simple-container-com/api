// SPDX-License-Identifier: MIT
// Copyright (c) 2025 Simple Container

package security

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/security/scan"
)

// mockDefectDojoServer answers the minimal DefectDojo REST surface exercised by
// UploadScanResult when EngagementID is already set:
//   - GET  /api/v2/engagements/{id}/  -> 200 (engagement exists)
//   - GET  /api/v2/tests/...          -> 200 empty page (no existing test)
//   - POST /api/v2/import-scan/       -> 201 with test + findings populated
//     (a fully-populated body short-circuits enrichImportScanResponse so no
//     follow-up GETs are needed).
//
// It records the test_title multipart field for assertion.
func mockDefectDojoServer(t *testing.T, gotTestTitle *string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v2/engagements/"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":7}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v2/tests/":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"results":[],"next":null}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v2/import-scan/":
			if err := r.ParseMultipartForm(1 << 20); err == nil && gotTestTitle != nil {
				*gotTestTitle = r.FormValue("test_title")
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":11,"test":11,"engagement":7,"number_of_findings":3}`))
		default:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"results":[]}`))
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestUploadToDefectDojoSingleTool(t *testing.T) {
	RegisterTestingT(t)
	ctx := context.Background()

	var gotTitle string
	srv := mockDefectDojoServer(t, &gotTitle)

	e, err := NewSecurityExecutorWithSummary(ctx, &SecurityConfig{
		Enabled: true,
		Reporting: &ReportingConfig{
			DefectDojo: &DefectDojoConfig{
				Enabled:      true,
				URL:          srv.URL,
				APIKey:       "k",
				EngagementID: 7,
			},
		},
	}, "registry.example.com/demo@sha256:abc")
	Expect(err).ToNot(HaveOccurred())

	result := &scan.ScanResult{
		Tool:        scan.ScanToolGrype,
		ImageDigest: "sha256:abc",
		Summary:     scan.VulnerabilitySummary{High: 1, Total: 1},
		Vulnerabilities: []scan.Vulnerability{
			{ID: "CVE-1", Severity: scan.SeverityHigh, Package: "p", Version: "1"},
		},
	}

	resp, err := e.uploadToDefectDojo(ctx, result, "registry.example.com/demo@sha256:abc")
	Expect(err).ToNot(HaveOccurred())
	Expect(resp).ToNot(BeNil())
	Expect(resp.Test).To(Equal(11))
	Expect(resp.NumberOfFindings).To(Equal(3))
	Expect(resp.Engagement).To(Equal(7))

	// Default test type with the single tool name appended.
	Expect(gotTitle).To(ContainSubstring("Container Image Scan (grype)"))
}

func TestUploadToDefectDojoMergedToolsTestType(t *testing.T) {
	RegisterTestingT(t)
	ctx := context.Background()

	var gotTitle string
	srv := mockDefectDojoServer(t, &gotTitle)

	e, err := NewSecurityExecutorWithSummary(ctx, &SecurityConfig{
		Enabled: true,
		Reporting: &ReportingConfig{
			DefectDojo: &DefectDojoConfig{
				Enabled:      true,
				URL:          srv.URL,
				APIKey:       "k",
				EngagementID: 7,
				TestType:     "My Custom Scan",
			},
		},
	}, "img@sha256:abc")
	Expect(err).ToNot(HaveOccurred())

	// A merged result carries mergedTools in metadata; uploadToDefectDojo
	// joins them into the test type.
	result := &scan.ScanResult{
		Tool:        scan.ScanToolAll,
		ImageDigest: "sha256:abc",
		Summary:     scan.VulnerabilitySummary{Total: 0},
		Metadata: map[string]interface{}{
			"mergedTools": []scan.ScanTool{scan.ScanToolGrype, scan.ScanToolTrivy},
		},
	}

	resp, err := e.uploadToDefectDojo(ctx, result, "img@sha256:abc")
	Expect(err).ToNot(HaveOccurred())
	Expect(resp).ToNot(BeNil())
	// Custom test type prefix + joined merged tool names.
	Expect(gotTitle).To(ContainSubstring("My Custom Scan (grype, trivy)"))
}

func TestUploadReportsDefectDojoSuccessRecordsSummary(t *testing.T) {
	RegisterTestingT(t)
	ctx := context.Background()

	srv := mockDefectDojoServer(t, nil)

	e, err := NewSecurityExecutorWithSummary(ctx, &SecurityConfig{
		Enabled: true,
		Reporting: &ReportingConfig{
			DefectDojo: &DefectDojoConfig{
				Enabled:      true,
				URL:          srv.URL,
				APIKey:       "k",
				EngagementID: 7,
			},
		},
	}, "img@sha256:abc")
	Expect(err).ToNot(HaveOccurred())

	result := &scan.ScanResult{
		Tool:        scan.ScanToolGrype,
		ImageDigest: "sha256:abc",
		Summary:     scan.VulnerabilitySummary{Total: 0},
	}

	Expect(e.UploadReports(ctx, result, "img@sha256:abc")).To(Succeed())

	// UploadReports records a defectdojo upload (engagement>0 => URL built).
	Expect(e.Summary.UploadResults).To(HaveLen(1))
	ur := e.Summary.UploadResults[0]
	Expect(ur.Target).To(Equal("defectdojo"))
	Expect(ur.Success).To(BeTrue())
	Expect(ur.URL).To(ContainSubstring("/engagement/7"))
}

func TestUploadReportsDefectDojoErrorIsNonFatal(t *testing.T) {
	RegisterTestingT(t)
	ctx := context.Background()

	// Server returns 500 for the engagement existence check => upload fails,
	// but UploadReports must not return an error (warning-only path).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom"))
	}))
	t.Cleanup(srv.Close)

	e, err := NewSecurityExecutorWithSummary(ctx, &SecurityConfig{
		Enabled: true,
		Reporting: &ReportingConfig{
			DefectDojo: &DefectDojoConfig{
				Enabled:      true,
				URL:          srv.URL,
				APIKey:       "k",
				EngagementID: 7,
			},
		},
	}, "img@sha256:abc")
	Expect(err).ToNot(HaveOccurred())

	result := &scan.ScanResult{Tool: scan.ScanToolGrype, Summary: scan.VulnerabilitySummary{Total: 0}}
	Expect(e.UploadReports(ctx, result, "img@sha256:abc")).To(Succeed())

	// Upload recorded with an error and no URL.
	Expect(e.Summary.UploadResults).To(HaveLen(1))
	Expect(e.Summary.UploadResults[0].Success).To(BeFalse())
	Expect(e.Summary.UploadResults[0].URL).To(BeEmpty())
}

func TestUploadReportsBothDefectDojoAndPRComment(t *testing.T) {
	RegisterTestingT(t)
	ctx := context.Background()

	srv := mockDefectDojoServer(t, nil)
	dir := t.TempDir()
	prPath := dir + "/comment.md"

	e, err := NewSecurityExecutorWithSummary(ctx, &SecurityConfig{
		Enabled: true,
		Reporting: &ReportingConfig{
			DefectDojo: &DefectDojoConfig{Enabled: true, URL: srv.URL, APIKey: "k", EngagementID: 7},
			PRComment:  &PRCommentConfig{Enabled: true, Output: prPath},
		},
	}, "img@sha256:abc")
	Expect(err).ToNot(HaveOccurred())

	result := &scan.ScanResult{
		Tool:        scan.ScanToolGrype,
		ImageDigest: "sha256:abc",
		Summary:     scan.VulnerabilitySummary{Total: 0},
	}

	Expect(e.UploadReports(ctx, result, "img@sha256:abc")).To(Succeed())
	Expect(e.Summary.UploadResults).To(HaveLen(1))

	// Give the FS a beat (write is synchronous; this is belt-and-suspenders).
	_ = time.Millisecond
}
