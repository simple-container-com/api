package reporting

import (
	"fmt"
	"time"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/simple-container-com/api/pkg/security/sbom"
	"github.com/simple-container-com/api/pkg/security/scan"
	"github.com/simple-container-com/api/pkg/security/signing"
)

// WorkflowSummary tracks the results of all security operations
type WorkflowSummary struct {
	ImageRef         string
	StartTime        time.Time
	EndTime          time.Time
	SBOMResult       *SBOMSummary
	ScanResults      []*ScanSummary
	MergedResult     *ScanSummary
	SigningResult    *SigningSummary
	ProvenanceResult *ProvenanceSummary
	UploadResults    []*UploadSummary
}

// SBOMSummary tracks SBOM generation results
type SBOMSummary struct {
	Success      bool
	Error        error
	PackageCount int
	Format       string
	Generator    string
	Attached     bool
	Signed       bool
	Duration     time.Duration
	OutputPath   string
}

// ScanSummary tracks vulnerability scan results
type ScanSummary struct {
	Tool        scan.ScanTool
	Success     bool
	Error       error
	ScanResult  *scan.ScanResult
	Duration    time.Duration
	ToolVersion string
}

// SigningSummary tracks signing results
type SigningSummary struct {
	Success  bool
	Error    error
	Keyless  bool
	SignedAt time.Time
	Duration time.Duration
}

// ProvenanceSummary tracks provenance generation results
type ProvenanceSummary struct {
	Success  bool
	Error    error
	Format   string
	Duration time.Duration
	Attached bool
}

// UploadSummary tracks report upload results
type UploadSummary struct {
	Target   string // "defectdojo"
	Success  bool
	Error    error
	URL      string
	Duration time.Duration
}

// NewWorkflowSummary creates a new workflow summary
func NewWorkflowSummary(imageRef string) *WorkflowSummary {
	return &WorkflowSummary{
		ImageRef:  imageRef,
		StartTime: time.Now(),
	}
}

// RecordSBOM records SBOM generation result
func (w *WorkflowSummary) RecordSBOM(result *sbom.SBOM, err error, duration time.Duration, outputPath string) {
	packageCount := 0
	format := ""
	if result != nil {
		format = string(result.Format)
		if result.Metadata != nil {
			packageCount = int(result.Metadata.PackageCount)
		}
	}

	w.SBOMResult = &SBOMSummary{
		Success:      err == nil,
		Error:        err,
		PackageCount: packageCount,
		Format:       format,
		Generator:    "syft",
		Duration:     duration,
		OutputPath:   outputPath,
	}
}

// RecordScan records a scan result
func (w *WorkflowSummary) RecordScan(tool scan.ScanTool, result *scan.ScanResult, err error, duration time.Duration, toolVersion string) {
	summary := &ScanSummary{
		Tool:        tool,
		Success:     err == nil,
		Error:       err,
		ScanResult:  result,
		Duration:    duration,
		ToolVersion: toolVersion,
	}
	w.ScanResults = append(w.ScanResults, summary)
}

// RecordMergedScan records merged scan result
func (w *WorkflowSummary) RecordMergedScan(result *scan.ScanResult) {
	if result == nil {
		return
	}

	w.MergedResult = &ScanSummary{
		Tool:       scan.ScanToolAll,
		Success:    true,
		ScanResult: result,
	}
}

// RecordSigning records signing result
func (w *WorkflowSummary) RecordSigning(result *signing.SignResult, err error, duration time.Duration) {
	if err != nil {
		w.SigningResult = &SigningSummary{
			Success:  false,
			Error:    err,
			Duration: duration,
		}
		return
	}

	keyless := result.Signature != "" // Simple heuristic
	w.SigningResult = &SigningSummary{
		Success:  true,
		Keyless:  keyless,
		SignedAt: time.Now(),
		Duration: duration,
	}
}

// RecordProvenance records provenance generation result
func (w *WorkflowSummary) RecordProvenance(format string, err error, duration time.Duration, attached bool) {
	w.ProvenanceResult = &ProvenanceSummary{
		Success:  err == nil,
		Error:    err,
		Format:   format,
		Duration: duration,
		Attached: attached,
	}
}

// RecordUpload records a report upload result
func (w *WorkflowSummary) RecordUpload(target string, err error, url string, duration time.Duration) {
	upload := &UploadSummary{
		Target:   target,
		Success:  err == nil,
		Error:    err,
		URL:      url,
		Duration: duration,
	}
	w.UploadResults = append(w.UploadResults, upload)
}

// Finalize marks the workflow as complete
func (w *WorkflowSummary) Finalize() {
	w.EndTime = time.Now()
}

// Display prints a formatted summary to stdout
func (w *WorkflowSummary) Display() {
	w.Finalize()

	fmt.Println()
	fmt.Println("╔══════════════════════════════════════════════════════════════════╗")
	fmt.Println("║                    SECURITY WORKFLOW SUMMARY                      ║")
	fmt.Println("╠══════════════════════════════════════════════════════════════════╣")
	fmt.Printf("║ Image: %-52s ║\n", truncate(w.ImageRef, 52))
	fmt.Printf("║ Duration: %-48s ║\n", w.Duration().Round(time.Millisecond))
	fmt.Println("╠══════════════════════════════════════════════════════════════════╣")

	// SBOM Section
	w.displaySBOM()

	// Scanning Section
	w.displayScanning()

	// Signing Section
	w.displaySigning()

	// Provenance Section
	w.displayProvenance()

	// Uploads Section
	w.displayUploads()

	fmt.Println("╚══════════════════════════════════════════════════════════════════╝")
	fmt.Println()
}

// displaySBOM displays SBOM results
func (w *WorkflowSummary) displaySBOM() {
	fmt.Println("║ 📋 SBOM Generation                                                 ║")

	if w.SBOMResult == nil {
		fmt.Println("║   Status: ⏭️  SKIPPED                                            ║")
	} else if w.SBOMResult.Success {
		fmt.Printf("║   Status: ✅ SUCCESS                                             ║\n")
		fmt.Printf("║   Packages: %-49d ║\n", w.SBOMResult.PackageCount)
		fmt.Printf("║   Format: %-51s ║\n", w.SBOMResult.Format)
		fmt.Printf("║   Duration: %-48s ║\n", w.SBOMResult.Duration.Round(time.Millisecond))
	} else {
		fmt.Printf("║   Status: ❌ FAILED                                             ║\n")
		fmt.Printf("║   Error: %-50s ║\n", truncate(w.SBOMResult.Error.Error(), 50))
	}

	fmt.Println("╠══════════════════════════════════════════════════════════════════╣")
}

// displayScanning displays scanning results
func (w *WorkflowSummary) displayScanning() {
	fmt.Println("║ 🔍 Vulnerability Scanning                                            ║")

	if len(w.ScanResults) == 0 {
		fmt.Println("║   Status: ⏭️  SKIPPED                                            ║")
	} else {
		// Display individual tool results
		for _, sr := range w.ScanResults {
			if sr.Success {
				fmt.Printf("║   %s: %-50s ║\n",
					cases.Title(language.English).String(string(sr.Tool)),
					sr.ScanResult.Summary.String())
			} else {
				fmt.Printf("║   %s: ❌ FAILED                                           ║\n",
					cases.Title(language.English).String(string(sr.Tool)))
			}
		}

		// Display merged result if available
		if w.MergedResult != nil && w.MergedResult.ScanResult != nil {
			fmt.Printf("║   Merged: %-50s ║\n",
				w.MergedResult.ScanResult.Summary.String())
		}
	}

	fmt.Println("╠══════════════════════════════════════════════════════════════════╣")
}

// displaySigning displays signing results
func (w *WorkflowSummary) displaySigning() {
	fmt.Println("║ 🔐 Image Signing                                                    ║")

	if w.SigningResult == nil {
		fmt.Println("║   Status: ⏭️  SKIPPED                                            ║")
	} else if w.SigningResult.Success {
		fmt.Printf("║   Status: ✅ SUCCESS                                             ║\n")
		if w.SigningResult.Keyless {
			fmt.Printf("║   Method: Keyless (OIDC)                                       ║\n")
		}
		fmt.Printf("║   Duration: %-48s ║\n", w.SigningResult.Duration.Round(time.Millisecond))
	} else {
		fmt.Printf("║   Status: ❌ FAILED                                             ║\n")
		fmt.Printf("║   Error: %-50s ║\n", truncate(w.SigningResult.Error.Error(), 50))
	}

	fmt.Println("╠══════════════════════════════════════════════════════════════════╣")
}

// displayProvenance displays provenance results
func (w *WorkflowSummary) displayProvenance() {
	fmt.Println("║ 📜 Provenance Generation                                            ║")

	if w.ProvenanceResult == nil {
		fmt.Println("║   Status: ⏭️  SKIPPED                                            ║")
	} else if w.ProvenanceResult.Success {
		fmt.Printf("║   Status: ✅ SUCCESS                                             ║\n")
		fmt.Printf("║   Format: %-51s ║\n", w.ProvenanceResult.Format)
		fmt.Printf("║   Duration: %-48s ║\n", w.ProvenanceResult.Duration.Round(time.Millisecond))
	} else {
		fmt.Printf("║   Status: ❌ FAILED                                             ║\n")
		fmt.Printf("║   Error: %-50s ║\n", truncate(w.ProvenanceResult.Error.Error(), 50))
	}

	fmt.Println("╠══════════════════════════════════════════════════════════════════╣")
}

// displayUploads displays upload results
func (w *WorkflowSummary) displayUploads() {
	fmt.Println("║ 📤 Report Uploads                                                   ║")

	if len(w.UploadResults) == 0 {
		fmt.Println("║   Status: ⏭️  SKIPPED                                            ║")
	} else {
		for _, ur := range w.UploadResults {
			if ur.Success {
				fmt.Printf("║   %s: ✅ %-48s ║\n",
					cases.Title(language.English).String(ur.Target),
					"uploaded")
				if ur.URL != "" {
					fmt.Printf("║   URL: %-52s ║\n", truncate(ur.URL, 52))
				}
			} else {
				fmt.Printf("║   %s: ❌ FAILED                                           ║\n",
					cases.Title(language.English).String(ur.Target))
			}
		}
	}
}

// Duration returns the total workflow duration
func (w *WorkflowSummary) Duration() time.Duration {
	if w.EndTime.IsZero() {
		return time.Since(w.StartTime)
	}
	return w.EndTime.Sub(w.StartTime)
}

// HasFailures returns true if any operation failed
func (w *WorkflowSummary) HasFailures() bool {
	if w.SBOMResult != nil && !w.SBOMResult.Success {
		return true
	}
	for _, sr := range w.ScanResults {
		if !sr.Success {
			return true
		}
	}
	if w.SigningResult != nil && !w.SigningResult.Success {
		return true
	}
	if w.ProvenanceResult != nil && !w.ProvenanceResult.Success {
		return true
	}
	for _, ur := range w.UploadResults {
		if !ur.Success {
			return true
		}
	}
	return false
}

// truncate truncates a string to a maximum length
func truncate(s string, maxLen int) string {
	if maxLen < 4 || len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
