// SPDX-License-Identifier: MIT
// Copyright (c) 2025 Simple Container

package reporting

import (
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/security/sbom"
	"github.com/simple-container-com/api/pkg/security/scan"
	"github.com/simple-container-com/api/pkg/security/signing"
)

// captureStdout runs fn while os.Stdout is redirected to a pipe and returns
// everything written. Display() and its helpers print directly to stdout, so
// this lets us assert real rendered output without changing the source.
func captureStdout(fn func()) string {
	orig := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	done := make(chan string, 1)
	go func() {
		data, _ := io.ReadAll(r)
		done <- string(data)
	}()

	fn()

	_ = w.Close()
	os.Stdout = orig
	return <-done
}

func TestNewWorkflowSummary(t *testing.T) {
	RegisterTestingT(t)

	w := NewWorkflowSummary("registry/app:1.0")
	Expect(w.ImageRef).To(Equal("registry/app:1.0"))
	Expect(w.StartTime.IsZero()).To(BeFalse())
	Expect(w.EndTime.IsZero()).To(BeTrue())
	Expect(w.ScanResults).To(BeNil())
}

func TestRecordSBOM(t *testing.T) {
	RegisterTestingT(t)

	t.Run("success with metadata", func(t *testing.T) {
		RegisterTestingT(t)
		w := NewWorkflowSummary("img")
		s := &sbom.SBOM{
			Format:   sbom.FormatCycloneDXJSON,
			Metadata: &sbom.Metadata{PackageCount: 42},
		}
		w.RecordSBOM(s, nil, 3*time.Second, "/tmp/sbom.json")

		Expect(w.SBOMResult).ToNot(BeNil())
		Expect(w.SBOMResult.Success).To(BeTrue())
		Expect(w.SBOMResult.PackageCount).To(Equal(42))
		Expect(w.SBOMResult.Format).To(Equal("cyclonedx-json"))
		Expect(w.SBOMResult.Generator).To(Equal("syft"))
		Expect(w.SBOMResult.OutputPath).To(Equal("/tmp/sbom.json"))
	})

	t.Run("nil result keeps zero values", func(t *testing.T) {
		RegisterTestingT(t)
		w := NewWorkflowSummary("img")
		w.RecordSBOM(nil, nil, 0, "")
		Expect(w.SBOMResult.PackageCount).To(Equal(0))
		Expect(w.SBOMResult.Format).To(Equal(""))
		Expect(w.SBOMResult.Success).To(BeTrue()) // err == nil
	})

	t.Run("result without metadata leaves package count zero", func(t *testing.T) {
		RegisterTestingT(t)
		w := NewWorkflowSummary("img")
		w.RecordSBOM(&sbom.SBOM{Format: sbom.FormatSPDXJSON}, nil, 0, "")
		Expect(w.SBOMResult.Format).To(Equal("spdx-json"))
		Expect(w.SBOMResult.PackageCount).To(Equal(0))
	})

	t.Run("error marks failure", func(t *testing.T) {
		RegisterTestingT(t)
		w := NewWorkflowSummary("img")
		w.RecordSBOM(nil, errors.New("boom"), 0, "")
		Expect(w.SBOMResult.Success).To(BeFalse())
		Expect(w.SBOMResult.Error).To(MatchError("boom"))
	})
}

func TestRecordScan(t *testing.T) {
	RegisterTestingT(t)

	w := NewWorkflowSummary("img")
	res := scan.NewScanResult("sha256:x", scan.ScanToolGrype, nil)
	w.RecordScan(scan.ScanToolGrype, res, nil, time.Second, "0.74")
	w.RecordScan(scan.ScanToolTrivy, nil, errors.New("trivy failed"), 0, "")

	Expect(w.ScanResults).To(HaveLen(2))
	Expect(w.ScanResults[0].Success).To(BeTrue())
	Expect(w.ScanResults[0].Tool).To(Equal(scan.ScanToolGrype))
	Expect(w.ScanResults[0].ToolVersion).To(Equal("0.74"))
	Expect(w.ScanResults[1].Success).To(BeFalse())
	Expect(w.ScanResults[1].Error).To(MatchError("trivy failed"))
}

func TestRecordMergedScan(t *testing.T) {
	RegisterTestingT(t)

	t.Run("nil result is ignored", func(t *testing.T) {
		RegisterTestingT(t)
		w := NewWorkflowSummary("img")
		w.RecordMergedScan(nil)
		Expect(w.MergedResult).To(BeNil())
	})

	t.Run("non-nil result recorded with all tool marker", func(t *testing.T) {
		RegisterTestingT(t)
		w := NewWorkflowSummary("img")
		res := scan.NewScanResult("sha256:x", scan.ScanToolAll, nil)
		w.RecordMergedScan(res)
		Expect(w.MergedResult).ToNot(BeNil())
		Expect(w.MergedResult.Success).To(BeTrue())
		Expect(w.MergedResult.Tool).To(Equal(scan.ScanToolAll))
		Expect(w.MergedResult.ScanResult).To(Equal(res))
	})
}

func TestRecordSigning(t *testing.T) {
	RegisterTestingT(t)

	t.Run("error path", func(t *testing.T) {
		RegisterTestingT(t)
		w := NewWorkflowSummary("img")
		w.RecordSigning(nil, errors.New("no key"), time.Second)
		Expect(w.SigningResult.Success).To(BeFalse())
		Expect(w.SigningResult.Error).To(MatchError("no key"))
		Expect(w.SigningResult.Keyless).To(BeFalse())
	})

	t.Run("success keyless when signature present", func(t *testing.T) {
		RegisterTestingT(t)
		w := NewWorkflowSummary("img")
		w.RecordSigning(&signing.SignResult{Signature: "MEUCIQ..."}, nil, time.Second)
		Expect(w.SigningResult.Success).To(BeTrue())
		Expect(w.SigningResult.Keyless).To(BeTrue())
		Expect(w.SigningResult.SignedAt.IsZero()).To(BeFalse())
	})

	t.Run("success non-keyless when signature empty", func(t *testing.T) {
		RegisterTestingT(t)
		w := NewWorkflowSummary("img")
		w.RecordSigning(&signing.SignResult{}, nil, time.Second)
		Expect(w.SigningResult.Success).To(BeTrue())
		Expect(w.SigningResult.Keyless).To(BeFalse())
	})
}

func TestRecordProvenance(t *testing.T) {
	RegisterTestingT(t)

	w := NewWorkflowSummary("img")
	w.RecordProvenance("slsa", nil, time.Second, true)
	Expect(w.ProvenanceResult.Success).To(BeTrue())
	Expect(w.ProvenanceResult.Format).To(Equal("slsa"))
	Expect(w.ProvenanceResult.Attached).To(BeTrue())

	w.RecordProvenance("slsa", errors.New("fail"), 0, false)
	Expect(w.ProvenanceResult.Success).To(BeFalse())
	Expect(w.ProvenanceResult.Error).To(MatchError("fail"))
}

func TestRecordUpload(t *testing.T) {
	RegisterTestingT(t)

	w := NewWorkflowSummary("img")
	w.RecordUpload("defectdojo", nil, "https://dd/1", time.Second)
	w.RecordUpload("s3", errors.New("denied"), "", 0)

	Expect(w.UploadResults).To(HaveLen(2))
	Expect(w.UploadResults[0].Success).To(BeTrue())
	Expect(w.UploadResults[0].URL).To(Equal("https://dd/1"))
	Expect(w.UploadResults[1].Success).To(BeFalse())
	Expect(w.UploadResults[1].Error).To(MatchError("denied"))
}

func TestFinalizeAndDuration(t *testing.T) {
	RegisterTestingT(t)

	t.Run("duration uses since start while end is zero", func(t *testing.T) {
		RegisterTestingT(t)
		w := NewWorkflowSummary("img")
		w.StartTime = time.Now().Add(-2 * time.Second)
		d := w.Duration()
		Expect(d >= 2*time.Second).To(BeTrue())
		Expect(w.EndTime.IsZero()).To(BeTrue())
	})

	t.Run("finalize sets end and duration becomes fixed span", func(t *testing.T) {
		RegisterTestingT(t)
		w := NewWorkflowSummary("img")
		w.StartTime = time.Now().Add(-5 * time.Second)
		w.Finalize()
		Expect(w.EndTime.IsZero()).To(BeFalse())
		d := w.Duration()
		Expect(d >= 5*time.Second).To(BeTrue())
		Expect(d < 6*time.Second).To(BeTrue())
	})
}

func TestHasFailures(t *testing.T) {
	RegisterTestingT(t)

	t.Run("empty summary has no failures", func(t *testing.T) {
		RegisterTestingT(t)
		Expect(NewWorkflowSummary("img").HasFailures()).To(BeFalse())
	})

	cases := []struct {
		name  string
		setup func(w *WorkflowSummary)
	}{
		{"sbom failed", func(w *WorkflowSummary) { w.RecordSBOM(nil, errors.New("x"), 0, "") }},
		{"scan failed", func(w *WorkflowSummary) { w.RecordScan(scan.ScanToolGrype, nil, errors.New("x"), 0, "") }},
		{"signing failed", func(w *WorkflowSummary) { w.RecordSigning(nil, errors.New("x"), 0) }},
		{"provenance failed", func(w *WorkflowSummary) { w.RecordProvenance("slsa", errors.New("x"), 0, false) }},
		{"upload failed", func(w *WorkflowSummary) { w.RecordUpload("dd", errors.New("x"), "", 0) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			w := NewWorkflowSummary("img")
			tc.setup(w)
			Expect(w.HasFailures()).To(BeTrue())
		})
	}

	t.Run("all success has no failures", func(t *testing.T) {
		RegisterTestingT(t)
		w := NewWorkflowSummary("img")
		w.RecordSBOM(&sbom.SBOM{Format: sbom.FormatSPDXJSON}, nil, 0, "")
		w.RecordScan(scan.ScanToolGrype, scan.NewScanResult("d", scan.ScanToolGrype, nil), nil, 0, "")
		w.RecordSigning(&signing.SignResult{Signature: "s"}, nil, 0)
		w.RecordProvenance("slsa", nil, 0, true)
		w.RecordUpload("dd", nil, "u", 0)
		Expect(w.HasFailures()).To(BeFalse())
	})
}

func TestTruncate(t *testing.T) {
	RegisterTestingT(t)

	cases := []struct {
		name   string
		input  string
		maxLen int
		want   string
	}{
		{"shorter than max unchanged", "hello", 10, "hello"},
		{"equal to max unchanged", "hello", 5, "hello"},
		{"longer than max truncated with ellipsis", "hello world", 8, "hello..."},
		{"maxLen below 4 returns original", "hello world", 3, "hello world"},
		{"empty string", "", 10, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			got := truncate(tc.input, tc.maxLen)
			Expect(got).To(Equal(tc.want))
			if tc.maxLen >= 4 {
				Expect(len(got) <= tc.maxLen || got == tc.input).To(BeTrue())
			}
		})
	}
}

func TestDisplaySkippedSections(t *testing.T) {
	RegisterTestingT(t)

	// Empty summary: every section renders SKIPPED. Display() also calls Finalize().
	w := NewWorkflowSummary("registry/app:verylongtag")
	out := captureStdout(w.Display)

	Expect(out).To(ContainSubstring("SECURITY WORKFLOW SUMMARY"))
	Expect(out).To(ContainSubstring("SBOM Generation"))
	Expect(out).To(ContainSubstring("Vulnerability Scanning"))
	Expect(out).To(ContainSubstring("Image Signing"))
	Expect(out).To(ContainSubstring("Provenance Generation"))
	Expect(out).To(ContainSubstring("Report Uploads"))
	Expect(strings.Count(out, "SKIPPED")).To(Equal(5))
	// Finalize() ran as part of Display().
	Expect(w.EndTime.IsZero()).To(BeFalse())
}

func TestDisplaySuccessSections(t *testing.T) {
	RegisterTestingT(t)

	w := NewWorkflowSummary("img:1.0")
	w.RecordSBOM(&sbom.SBOM{Format: sbom.FormatCycloneDXJSON, Metadata: &sbom.Metadata{PackageCount: 12}}, nil, time.Second, "/tmp/s.json")
	res := scan.NewScanResult("sha256:x", scan.ScanToolGrype, []scan.Vulnerability{
		{ID: "CVE-1", Severity: scan.SeverityHigh, Package: "p", Version: "1"},
	})
	w.RecordScan(scan.ScanToolGrype, res, nil, time.Second, "0.74")
	w.RecordMergedScan(res)
	w.RecordSigning(&signing.SignResult{Signature: "sig"}, nil, time.Second)
	w.RecordProvenance("slsa-v1", nil, time.Second, true)
	w.RecordUpload("defectdojo", nil, "https://dd/test/1", time.Second)

	out := captureStdout(w.Display)

	Expect(out).To(ContainSubstring("SUCCESS"))
	Expect(out).To(ContainSubstring("Packages:"))
	Expect(out).To(ContainSubstring("Method: Keyless (OIDC)"))
	Expect(out).To(ContainSubstring("Merged:"))
	// Target name is title-cased ("Defectdojo"); status word stays lowercase "uploaded".
	Expect(out).To(ContainSubstring("Defectdojo: ✅ uploaded"))
	Expect(out).To(ContainSubstring("URL:"))
	// Title-cased tool name from cases.Title.
	Expect(out).To(ContainSubstring("Grype"))
}

func TestDisplayFailureSections(t *testing.T) {
	RegisterTestingT(t)

	w := NewWorkflowSummary("img")
	w.RecordSBOM(nil, errors.New("sbom error"), 0, "")
	w.RecordScan(scan.ScanToolTrivy, nil, errors.New("scan error"), 0, "")
	w.RecordSigning(nil, errors.New("sign error"), 0)
	w.RecordProvenance("slsa", errors.New("prov error"), 0, false)
	w.RecordUpload("defectdojo", errors.New("upload error"), "", 0)

	out := captureStdout(w.Display)

	Expect(out).To(ContainSubstring("FAILED"))
	Expect(out).To(ContainSubstring("sbom error"))
	Expect(out).To(ContainSubstring("sign error"))
	Expect(out).To(ContainSubstring("prov error"))
	Expect(strings.Count(out, "FAILED")).To(BeNumerically(">=", 5))
}
