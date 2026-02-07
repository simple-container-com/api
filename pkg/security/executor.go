package security

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/simple-container-com/api/pkg/security/sbom"
	"github.com/simple-container-com/api/pkg/security/scan"
	"github.com/simple-container-com/api/pkg/security/signing"
)

// SecurityExecutor orchestrates all security operations for container images
type SecurityExecutor struct {
	Context *ExecutionContext
	Config  *SecurityConfig
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
			continue
		}

		// Check if scanner is installed
		if err := scanner.CheckInstalled(ctx); err != nil {
			if e.Config.Scan.Required {
				return nil, fmt.Errorf("scanner %s not installed: %w", toolName, err)
			}
			fmt.Printf("Warning: scanner %s not installed, skipping: %v\n", toolName, err)
			continue
		}

		// Run scan
		fmt.Printf("Running %s vulnerability scan on %s...\n", toolName, imageRef)
		result, err := scanner.Scan(ctx, imageRef)
		if err != nil {
			if e.Config.Scan.Required {
				return nil, fmt.Errorf("scan with %s failed: %w", toolName, err)
			}
			fmt.Printf("Warning: scan with %s failed, continuing: %v\n", toolName, err)
			continue
		}

		fmt.Printf("%s scan complete: %s\n", toolName, result.Summary.String())
		results = append(results, result)
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
		return nil, nil
	}

	// Execute signing
	result, err := signer.Sign(ctx, imageRef)
	if err != nil {
		if e.Config.Signing.Required {
			return nil, fmt.Errorf("signing image: %w", err)
		}
		// Fail-open: log warning and continue
		fmt.Printf("Warning: signing failed, continuing: %v\n", err)
		return nil, nil
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

	// Generate SBOM
	fmt.Printf("Generating %s SBOM for %s...\n", format, imageRef)
	generatedSBOM, err := generator.Generate(ctx, imageRef, format)
	if err != nil {
		if e.Config.SBOM.Required {
			return nil, fmt.Errorf("generating SBOM: %w", err)
		}
		// Fail-open: log warning and continue
		fmt.Printf("Warning: SBOM generation failed, continuing: %v\n", err)
		return nil, nil
	}

	// Save locally if configured
	if e.Config.SBOM.Output != nil && e.Config.SBOM.Output.Local != "" {
		if err := e.saveSBOMLocal(generatedSBOM); err != nil {
			if e.Config.SBOM.Required {
				return nil, fmt.Errorf("saving SBOM locally: %w", err)
			}
			fmt.Printf("Warning: failed to save SBOM locally: %v\n", err)
		}
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
