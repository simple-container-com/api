package security

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/simple-container-com/api/pkg/security/reporting"
	"github.com/simple-container-com/api/pkg/security/sbom"
	"github.com/simple-container-com/api/pkg/security/scan"
	"github.com/simple-container-com/api/pkg/security/signing"
)

// SecurityExecutor orchestrates all security operations for container images
type SecurityExecutor struct {
	Context *ExecutionContext
	Config  *SecurityConfig
	Summary *reporting.WorkflowSummary
}

// Note: SecurityConfig is now defined in config.go with comprehensive types

// NewSecurityExecutor creates a new security executor
func NewSecurityExecutor(ctx context.Context, config *SecurityConfig) (*SecurityExecutor, error) {
	if config == nil {
		config = &SecurityConfig{Enabled: false}
	}

	execCtx, err := NewExecutionContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("creating execution context: %w", err)
	}

	return &SecurityExecutor{
		Context: execCtx,
		Config:  config,
	}, nil
}

// NewSecurityExecutorWithSummary creates a new security executor with summary tracking
func NewSecurityExecutorWithSummary(ctx context.Context, config *SecurityConfig, imageRef string) (*SecurityExecutor, error) {
	executor, err := NewSecurityExecutor(ctx, config)
	if err != nil {
		return nil, err
	}

	executor.Summary = reporting.NewWorkflowSummary(imageRef)
	return executor, nil
}

// ExecuteScanning performs vulnerability scanning on the image
// This runs FIRST in the security workflow (fail-fast pattern)
func (e *SecurityExecutor) ExecuteScanning(ctx context.Context, imageRef string) (*scan.ScanResult, error) {
	if !e.Config.Enabled || e.Config.Scan == nil || !e.Config.Scan.Enabled {
		return nil, nil // Scanning disabled
	}

	// Validate scan configuration
	if err := e.Config.Scan.Validate(); err != nil {
		if e.Config.Scan.Required {
			return nil, fmt.Errorf("scan validation failed: %w", err)
		}
		// Fail-open: log warning and continue
		fmt.Printf("Warning: scan validation failed, skipping: %v\n", err)
		return nil, nil
	}

	var results []*scan.ScanResult

	// Run each configured scanner
	for _, toolConfig := range e.Config.Scan.Tools {
		// Convert ScanToolConfig to ScanTool string
		toolName := scan.ScanTool(toolConfig.Name)

		// Handle "all" tool
		if toolName == scan.ScanToolAll {
			toolName = scan.ScanToolGrype
		}

		scanner, err := scan.NewScanner(toolName)
		if err != nil {
			if e.Config.Scan.Required {
				return nil, fmt.Errorf("creating scanner %s: %w", toolName, err)
			}
			fmt.Printf("Warning: failed to create scanner %s, skipping: %v\n", toolName, err)
			if e.Summary != nil {
				e.Summary.RecordScan(toolName, nil, err, 0, "")
			}
			continue
		}

		// Check if scanner is installed
		if err := scanner.CheckInstalled(ctx); err != nil {
			if e.Config.Scan.Required {
				return nil, fmt.Errorf("scanner %s not installed: %w", toolName, err)
			}
			fmt.Printf("Warning: scanner %s not installed, skipping: %v\n", toolName, err)
			if e.Summary != nil {
				e.Summary.RecordScan(toolName, nil, err, 0, "")
			}
			continue
		}

		// Run scan with timing
		fmt.Printf("Running %s vulnerability scan on %s...\n", toolName, imageRef)
		startTime := time.Now()
		result, err := scanner.Scan(ctx, imageRef)
		duration := time.Since(startTime)

		if err != nil {
			if e.Config.Scan.Required {
				return nil, fmt.Errorf("scan with %s failed: %w", toolName, err)
			}
			fmt.Printf("Warning: scan with %s failed, continuing: %v\n", toolName, err)
			if e.Summary != nil {
				e.Summary.RecordScan(toolName, nil, err, duration, "")
			}
			continue
		}

		fmt.Printf("%s scan complete: %s\n", toolName, result.Summary.String())
		results = append(results, result)

		// Record in summary
		if e.Summary != nil {
			e.Summary.RecordScan(toolName, result, nil, duration, "")
		}
	}

	if len(results) == 0 {
		if e.Config.Scan.Required {
			return nil, fmt.Errorf("no scanners produced results")
		}
		fmt.Println("Warning: no scan results available")
		return nil, nil
	}

	// Merge results if multiple scanners were used
	var finalResult *scan.ScanResult
	if len(results) > 1 {
		finalResult = scan.MergeResults(results...)
		fmt.Printf("Merged scan results (deduplicated by CVE ID): %s\n", finalResult.Summary.String())
	} else {
		finalResult = results[0]
	}

	// Record merged result in summary
	if e.Summary != nil && len(results) > 1 {
		e.Summary.RecordMergedScan(finalResult)
	}

	// Enforce policy
	if e.Config.Scan.FailOn != "" {
		// Convert our ScanConfig to scan.Config for the policy enforcer
		scanCfg := e.convertToScanConfig()
		enforcer := scan.NewPolicyEnforcer(scanCfg)
		if err := enforcer.Enforce(finalResult); err != nil {
			// Policy violation - this should block deployment
			return nil, fmt.Errorf("vulnerability policy violation: %w", err)
		}
		fmt.Printf("✓ Vulnerability policy check passed (failOn: %s)\n", e.Config.Scan.FailOn)
	}

	// Save locally if configured
	if e.shouldSaveScanLocal() {
		if err := e.saveScanLocal(finalResult); err != nil {
			if e.Config.Scan.Required {
				return nil, fmt.Errorf("saving scan results locally: %w", err)
			}
			fmt.Printf("Warning: failed to save scan results locally: %v\n", err)
		}
	}

	return finalResult, nil
}

// shouldSaveScanLocal returns true if local output is configured
func (e *SecurityExecutor) shouldSaveScanLocal() bool {
	return e.Config.Scan != nil && len(e.Config.Scan.Tools) > 0 && e.getScanOutputPath() != ""
}

// getScanOutputPath returns the output path from the first tool config that has one
func (e *SecurityExecutor) getScanOutputPath() string {
	if e.Config.Scan == nil {
		return ""
	}
	// For now, we'll use a default path if tools exist
	// In a real implementation, each tool config could have its own output path
	return "./scan-results.json"
}

// convertToScanConfig converts our ScanConfig to scan.Config
func (e *SecurityExecutor) convertToScanConfig() *scan.Config {
	if e.Config.Scan == nil {
		return nil
	}

	// Convert tools from []ScanToolConfig to []ScanTool
	var tools []scan.ScanTool
	for _, tc := range e.Config.Scan.Tools {
		tools = append(tools, scan.ScanTool(tc.Name))
	}

	return &scan.Config{
		Enabled:  e.Config.Scan.Enabled,
		Tools:    tools,
		FailOn:   scan.Severity(e.Config.Scan.FailOn),
		WarnOn:   scan.Severity(e.Config.Scan.WarnOn),
		Required: e.Config.Scan.Required,
		Output: &scan.OutputConfig{
			Local: e.getScanOutputPath(),
		},
	}
}

// saveScanLocal saves scan results to local file
func (e *SecurityExecutor) saveScanLocal(result *scan.ScanResult) error {
	outputPath := e.getScanOutputPath()

	// Create directory if needed
	dir := filepath.Dir(outputPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	// Marshal to JSON
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling scan results: %w", err)
	}

	// Write to file
	if err := os.WriteFile(outputPath, data, 0o644); err != nil {
		return fmt.Errorf("writing scan results file: %w", err)
	}

	fmt.Printf("Scan results saved to: %s\n", outputPath)
	return nil
}

// ExecuteSigning performs signing operations on the image
func (e *SecurityExecutor) ExecuteSigning(ctx context.Context, imageRef string) (*signing.SignResult, error) {
	if !e.Config.Enabled || e.Config.Signing == nil || !e.Config.Signing.Enabled {
		return nil, nil // Signing disabled
	}

	// Validate signing configuration
	if err := e.Config.Signing.Validate(); err != nil {
		if e.Config.Signing.Required {
			return nil, fmt.Errorf("signing validation failed: %w", err)
		}
		// Fail-open: log warning and continue
		fmt.Printf("Warning: signing validation failed, skipping: %v\n", err)
		if e.Summary != nil {
			e.Summary.RecordSigning(nil, err, 0)
		}
		return nil, nil
	}

	// Create signer
	signer, err := e.Config.Signing.CreateSigner(e.Context.OIDCToken)
	if err != nil {
		if e.Config.Signing.Required {
			return nil, fmt.Errorf("creating signer: %w", err)
		}
		// Fail-open: log warning and continue
		fmt.Printf("Warning: failed to create signer, skipping: %v\n", err)
		if e.Summary != nil {
			e.Summary.RecordSigning(nil, err, 0)
		}
		return nil, nil
	}

	// Execute signing with timing
	startTime := time.Now()
	result, err := signer.Sign(ctx, imageRef)
	duration := time.Since(startTime)

	if err != nil {
		if e.Config.Signing.Required {
			return nil, fmt.Errorf("signing image: %w", err)
		}
		// Fail-open: log warning and continue
		fmt.Printf("Warning: signing failed, continuing: %v\n", err)
		if e.Summary != nil {
			e.Summary.RecordSigning(nil, err, duration)
		}
		return nil, nil
	}

	// Record in summary
	if e.Summary != nil {
		e.Summary.RecordSigning(result, nil, duration)
	}

	return result, nil
}

// ExecuteSBOM generates and optionally attaches SBOM for the image
func (e *SecurityExecutor) ExecuteSBOM(ctx context.Context, imageRef string) (*sbom.SBOM, error) {
	if !e.Config.Enabled || e.Config.SBOM == nil || !e.Config.SBOM.Enabled {
		return nil, nil // SBOM disabled
	}

	// Validate SBOM configuration
	if err := e.Config.SBOM.Validate(); err != nil {
		if e.Config.SBOM.Required {
			return nil, fmt.Errorf("SBOM validation failed: %w", err)
		}
		// Fail-open: log warning and continue
		fmt.Printf("Warning: SBOM validation failed, skipping: %v\n", err)
		if e.Summary != nil {
			e.Summary.RecordSBOM(nil, err, 0, "")
		}
		return nil, nil
	}

	// Create generator
	generator := sbom.NewSyftGenerator()

	// Parse format
	format := sbom.FormatCycloneDXJSON // default
	if e.Config.SBOM.Format != "" {
		parsedFormat, err := sbom.ParseFormat(e.Config.SBOM.Format)
		if err == nil {
			format = parsedFormat
		}
	}

	// Generate SBOM with timing
	fmt.Printf("Generating %s SBOM for %s...\n", format, imageRef)
	startTime := time.Now()
	generatedSBOM, err := generator.Generate(ctx, imageRef, format)
	duration := time.Since(startTime)

	if err != nil {
		if e.Config.SBOM.Required {
			return nil, fmt.Errorf("generating SBOM: %w", err)
		}
		// Fail-open: log warning and continue
		fmt.Printf("Warning: SBOM generation failed, continuing: %v\n", err)
		if e.Summary != nil {
			e.Summary.RecordSBOM(nil, err, duration, "")
		}
		return nil, nil
	}

	outputPath := ""
	// Save locally if configured
	if e.Config.SBOM.Output != nil && e.Config.SBOM.Output.Local != "" {
		outputPath = e.Config.SBOM.Output.Local
		if err := e.saveSBOMLocal(generatedSBOM); err != nil {
			if e.Config.SBOM.Required {
				return nil, fmt.Errorf("saving SBOM locally: %w", err)
			}
			fmt.Printf("Warning: failed to save SBOM locally: %v\n", err)
		}
	}

	// Record in summary
	if e.Summary != nil {
		e.Summary.RecordSBOM(generatedSBOM, nil, duration, outputPath)
	}

	// Attach as attestation if configured
	shouldAttach := e.Config.SBOM.Attach != nil && e.Config.SBOM.Attach.Enabled
	shouldAttachToRegistry := e.Config.SBOM.Output != nil && e.Config.SBOM.Output.Registry

	if (shouldAttach || shouldAttachToRegistry) && e.Config.Signing != nil && e.Config.Signing.Enabled {
		if err := e.attachSBOM(ctx, generatedSBOM, imageRef); err != nil {
			if e.Config.SBOM.Required {
				return nil, fmt.Errorf("attaching SBOM: %w", err)
			}
			fmt.Printf("Warning: failed to attach SBOM, continuing: %v\n", err)
		}
	}

	return generatedSBOM, nil
}

// saveSBOMLocal saves SBOM to local file
func (e *SecurityExecutor) saveSBOMLocal(sbomObj *sbom.SBOM) error {
	outputPath := e.Config.SBOM.Output.Local

	// Create directory if needed
	dir := filepath.Dir(outputPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	// Write SBOM to file
	if err := os.WriteFile(outputPath, sbomObj.Content, 0o644); err != nil {
		return fmt.Errorf("writing SBOM file: %w", err)
	}

	fmt.Printf("SBOM saved to: %s\n", outputPath)
	return nil
}

// attachSBOM attaches SBOM as signed attestation
func (e *SecurityExecutor) attachSBOM(ctx context.Context, sbomObj *sbom.SBOM, imageRef string) error {
	// Create attacher with signing config
	attacher := sbom.NewAttacher(e.Config.Signing)

	// Attach SBOM
	fmt.Printf("Attaching SBOM as attestation to %s...\n", imageRef)
	if err := attacher.Attach(ctx, sbomObj, imageRef); err != nil {
		return err
	}

	fmt.Printf("SBOM attestation attached successfully\n")
	return nil
}

// ValidateConfig validates the security configuration
func (e *SecurityExecutor) ValidateConfig() error {
	if e.Config == nil {
		return nil
	}

	// Use the comprehensive validation from config.go
	return e.Config.Validate()
}

// UploadReports uploads scan results to configured reporting systems
func (e *SecurityExecutor) UploadReports(ctx context.Context, result *scan.ScanResult, imageRef string) error {
	if e.Config.Reporting == nil {
		return nil // No reporting configured
	}

	// Upload to DefectDojo if configured
	if e.Config.Reporting.DefectDojo != nil && e.Config.Reporting.DefectDojo.Enabled && result != nil {
		startTime := time.Now()
		err := e.uploadToDefectDojo(ctx, result, imageRef)
		duration := time.Since(startTime)

		if e.Summary != nil {
			url := ""
			if err == nil {
				url = fmt.Sprintf("%s/engagement/%d", e.Config.Reporting.DefectDojo.URL, e.Config.Reporting.DefectDojo.EngagementID)
			}
			e.Summary.RecordUpload("defectdojo", err, url, duration)
		}

		if err != nil {
			fmt.Printf("Warning: failed to upload to DefectDojo: %v\n", err)
		}
	}

	// Upload to GitHub Security if configured
	if e.Config.Reporting.GitHub != nil && e.Config.Reporting.GitHub.Enabled && result != nil {
		startTime := time.Now()
		err := e.uploadToGitHub(ctx, result, imageRef)
		duration := time.Since(startTime)

		if e.Summary != nil {
			url := ""
			if err == nil {
				url = fmt.Sprintf("https://github.com/%s/security/code-scanning", e.Config.Reporting.GitHub.Repository)
			}
			e.Summary.RecordUpload("github", err, url, duration)
		}

		if err != nil {
			fmt.Printf("Warning: failed to upload to GitHub Security: %v\n", err)
		}
	}

	return nil
}

// uploadToDefectDojo uploads scan results to DefectDojo
func (e *SecurityExecutor) uploadToDefectDojo(ctx context.Context, result *scan.ScanResult, imageRef string) error {
	config := e.Config.Reporting.DefectDojo

	// Create DefectDojo client
	client := reporting.NewDefectDojoClient(config.URL, config.APIKey)

	// Create uploader config
	uploaderConfig := &reporting.DefectDojoUploaderConfig{
		EngagementID:   config.EngagementID,
		EngagementName: config.EngagementName,
		ProductID:      config.ProductID,
		ProductName:    config.ProductName,
		TestType:       config.TestType,
		Tags:           config.Tags,
		Environment:    config.Environment,
		AutoCreate:     config.AutoCreate,
	}

	// Upload
	fmt.Printf("Uploading scan results to DefectDojo at %s...\n", config.URL)
	importResp, err := client.UploadScanResult(ctx, result, imageRef, uploaderConfig)
	if err != nil {
		return err
	}

	fmt.Printf("✓ Successfully uploaded to DefectDojo (test ID: %d, %d findings)\n",
		importResp.ID, importResp.NumberOfFindings)
	return nil
}

// uploadToGitHub uploads scan results to GitHub Security tab
func (e *SecurityExecutor) uploadToGitHub(ctx context.Context, result *scan.ScanResult, imageRef string) error {
	config := e.Config.Reporting.GitHub

	// Create uploader config
	uploaderConfig := &reporting.GitHubUploaderConfig{
		Repository: config.Repository,
		Token:      config.Token,
		CommitSHA:  config.CommitSHA,
		Ref:        config.Ref,
		Workspace:  config.Workspace,
	}

	// Upload
	fmt.Printf("Uploading scan results to GitHub Security (%s)...\n", config.Repository)
	err := reporting.UploadToGitHub(ctx, result, imageRef, uploaderConfig)
	if err != nil {
		return err
	}

	fmt.Printf("✓ Successfully uploaded to GitHub Security\n")
	return nil
}
