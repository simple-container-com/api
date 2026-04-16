package security

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/simple-container-com/api/pkg/security/provenance"
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

// scanToolOutcome holds the result of scanning with a single tool.
// It is sent over a channel from parallel goroutines to the main goroutine.
type scanToolOutcome struct {
	toolName  scan.ScanTool
	result    *scan.ScanResult
	policyErr error
	err       error
	duration  time.Duration
	version   string
	required  bool
}

// ExecuteScanning performs vulnerability scanning on the image.
// All configured scan tools run in parallel; results are merged and deduplicated.
// This runs FIRST in the security workflow (fail-fast pattern).
func (e *SecurityExecutor) ExecuteScanning(ctx context.Context, imageRef string) (*scan.ScanResult, error) {
	if !e.Config.Enabled || e.Config.Scan == nil || !e.Config.Scan.Enabled {
		return nil, nil // Scanning disabled
	}
	if err := ValidateImageRef(imageRef); err != nil {
		return nil, fmt.Errorf("invalid image ref: %w", err)
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

	toolConfigs := e.enabledScanTools()
	if len(toolConfigs) == 0 {
		if e.Config.Scan.Required {
			return nil, fmt.Errorf("no enabled scan tools configured")
		}
		fmt.Println("Warning: no enabled scan tools configured")
		return nil, nil
	}

	cache, cacheErr := e.newScanCache()
	if cacheErr != nil {
		if e.Config.Scan.Required {
			return nil, fmt.Errorf("creating scan cache: %w", cacheErr)
		}
		fmt.Printf("Warning: failed to initialize scan cache, continuing without cache: %v\n", cacheErr)
	}

	// Pre-install scanners sequentially to avoid simultaneous GitHub API calls
	// (parallel installs hit the release API at the same time and get rate-limited).
	// Goroutines below will find scanners already installed and skip re-install.
	for _, tc := range toolConfigs {
		toolName := normalizedScanToolName(tc.Name)
		s, err := scan.NewScannerWithVersion(toolName, tc.Version)
		if err != nil {
			continue
		}
		if err := s.CheckInstalled(ctx); err != nil {
			fmt.Printf("Scanner %s not found, attempting auto-install...\n", toolName)
			if installErr := s.Install(ctx); installErr != nil {
				fmt.Printf("Warning: scanner %s auto-install failed: %v\n", toolName, installErr)
			} else {
				fmt.Printf("Scanner %s installed successfully\n", toolName)
			}
		}
	}

	// Run all scanners in parallel; each goroutine sends exactly one outcome.
	outcomeCh := make(chan scanToolOutcome, len(toolConfigs))
	var wg sync.WaitGroup
	for _, tc := range toolConfigs {
		wg.Add(1)
		go func(toolConfig ScanToolConfig) {
			defer wg.Done()
			outcomeCh <- e.runScannerForTool(ctx, toolConfig, imageRef, cache)
		}(tc)
	}
	go func() {
		wg.Wait()
		close(outcomeCh)
	}()

	// Collect outcomes in the main goroutine — keeps WorkflowSummary single-threaded.
	var results []*scan.ScanResult
	var policyErr error
	for outcome := range outcomeCh {
		if e.Summary != nil {
			e.Summary.RecordScan(outcome.toolName, outcome.result, outcome.err, outcome.duration, outcome.version)
		}

		if outcome.err != nil {
			if outcome.required {
				return nil, fmt.Errorf("scan with %s failed: %w", outcome.toolName, outcome.err)
			}
			fmt.Printf("Warning: scan with %s failed, continuing: %v\n", outcome.toolName, outcome.err)
			continue
		}

		if outcome.result != nil {
			results = append(results, outcome.result)
		}

		if outcome.policyErr != nil && policyErr == nil {
			policyErr = fmt.Errorf("%s vulnerability policy violation: %w", outcome.toolName, outcome.policyErr)
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
		fmt.Printf("Merged scan results (deduplicated by vulnerability and package): %s\n", finalResult.Summary.String())
	} else {
		finalResult = results[0]
	}

	// Record merged result in summary
	if e.Summary != nil && len(results) > 1 {
		e.Summary.RecordMergedScan(finalResult)
	}

	// Enforce global policy on merged result
	if e.Config.Scan.FailOn != "" {
		scanCfg := e.convertToScanConfig()
		enforcer := scan.NewPolicyEnforcer(scanCfg)
		if err := enforcer.Enforce(finalResult); err != nil {
			if policyErr == nil {
				policyErr = fmt.Errorf("vulnerability policy violation: %w", err)
			}
		} else {
			fmt.Printf("✓ Vulnerability policy check passed (failOn: %s)\n", e.Config.Scan.FailOn)
		}
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

	return finalResult, policyErr
}

// runScannerForTool runs a single scanner and returns its outcome.
// Safe to call from a goroutine — does not touch shared state.
func (e *SecurityExecutor) runScannerForTool(ctx context.Context, toolConfig ScanToolConfig, imageRef string, cache *Cache) scanToolOutcome {
	toolName := normalizedScanToolName(toolConfig.Name)
	outcome := scanToolOutcome{
		toolName: toolName,
		required: e.isScanToolRequired(toolConfig),
	}

	scanner, err := scan.NewScannerWithVersion(toolName, toolConfig.Version)
	if err != nil {
		outcome.err = fmt.Errorf("creating scanner: %w", err)
		return outcome
	}

	// Auto-install if not present
	if err := scanner.CheckInstalled(ctx); err != nil {
		fmt.Printf("Scanner %s not found, attempting auto-install...\n", toolName)
		if installErr := scanner.Install(ctx); installErr != nil {
			outcome.err = fmt.Errorf("not installed and auto-install failed: %w (install error: %v)", err, installErr)
			return outcome
		}
		fmt.Printf("Scanner %s installed successfully\n", toolName)
	}

	toolVersion, err := scanner.Version(ctx)
	if err != nil {
		toolVersion = ""
	}
	outcome.version = toolVersion

	// Check cache
	if cache != nil {
		cachedResult, found, err := e.loadScanResultFromCache(cache, toolConfig, imageRef)
		if err != nil {
			fmt.Printf("Warning: failed to read cached %s scan result: %v\n", toolName, err)
		} else if found {
			fmt.Printf("Using cached %s vulnerability scan for %s...\n", toolName, imageRef)
			outcome.result = cachedResult
			if err := e.enforceToolPolicy(toolConfig, cachedResult); err != nil {
				outcome.policyErr = err
			}
			return outcome
		}
	}

	// Run scan
	fmt.Printf("Running %s vulnerability scan on %s...\n", toolName, imageRef)
	startTime := time.Now()
	result, err := scanner.Scan(ctx, imageRef)
	outcome.duration = time.Since(startTime)

	if err != nil {
		outcome.err = err
		return outcome
	}

	fmt.Printf("%s scan complete: %s\n", toolName, result.Summary.String())
	outcome.result = result

	if cache != nil {
		if err := e.saveScanResultToCache(cache, toolConfig, imageRef, result); err != nil {
			fmt.Printf("Warning: failed to cache %s scan result: %v\n", toolName, err)
		}
	}

	if err := e.enforceToolPolicy(toolConfig, result); err != nil {
		outcome.policyErr = err
	}

	return outcome
}

// shouldSaveScanLocal returns true if local output is configured
func (e *SecurityExecutor) shouldSaveScanLocal() bool {
	return e.Config.Scan != nil && e.getScanOutputPath() != ""
}

// getScanOutputPath returns the configured local output path for merged scan results.
func (e *SecurityExecutor) getScanOutputPath() string {
	if e.Config.Scan == nil || e.Config.Scan.Output == nil {
		return ""
	}
	return e.Config.Scan.Output.Local
}

// convertToScanConfig converts our ScanConfig to scan.Config
func (e *SecurityExecutor) convertToScanConfig() *scan.Config {
	if e.Config.Scan == nil {
		return nil
	}

	// Convert tools from []ScanToolConfig to []ScanTool
	var tools []scan.ScanTool
	for _, tc := range e.enabledScanTools() {
		tools = append(tools, normalizedScanToolName(tc.Name))
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

func (e *SecurityExecutor) enabledScanTools() []ScanToolConfig {
	if e.Config == nil || e.Config.Scan == nil {
		return nil
	}

	tools := make([]ScanToolConfig, 0, len(e.Config.Scan.Tools))
	for _, tool := range e.Config.Scan.Tools {
		if tool.Name == "" {
			continue
		}
		if scanToolEnabled(tool) {
			tools = append(tools, tool)
		}
	}

	return tools
}

func scanToolEnabled(tool ScanToolConfig) bool {
	if tool.Enabled != nil {
		return *tool.Enabled
	}
	return tool.Required || tool.FailOn != "" || tool.WarnOn != "" || tool.Name != ""
}

func normalizedScanToolName(name string) scan.ScanTool {
	toolName := scan.ScanTool(name)
	if toolName == scan.ScanToolAll {
		return scan.ScanToolGrype
	}
	return toolName
}

func (e *SecurityExecutor) isScanToolRequired(toolConfig ScanToolConfig) bool {
	return e.Config.Scan.Required || toolConfig.Required
}

func (e *SecurityExecutor) enforceToolPolicy(toolConfig ScanToolConfig, result *scan.ScanResult) error {
	failOn := toolConfig.FailOn
	if failOn == "" {
		failOn = e.Config.Scan.FailOn
	}

	warnOn := toolConfig.WarnOn
	if warnOn == "" {
		warnOn = e.Config.Scan.WarnOn
	}

	if failOn == "" && warnOn == "" {
		return nil
	}

	enforcer := scan.NewPolicyEnforcer(&scan.Config{
		Enabled: true,
		Tools:   []scan.ScanTool{result.Tool},
		FailOn:  scan.Severity(failOn),
		WarnOn:  scan.Severity(warnOn),
	})

	return enforcer.Enforce(result)
}

func (e *SecurityExecutor) newScanCache() (*Cache, error) {
	if e.Config == nil || e.Config.Scan == nil || e.Config.Scan.Cache == nil || !e.Config.Scan.Cache.Enabled {
		return nil, nil
	}

	return NewCache(e.Config.Scan.Cache.Dir)
}

func (e *SecurityExecutor) loadScanResultFromCache(cache *Cache, toolConfig ScanToolConfig, imageRef string) (*scan.ScanResult, bool, error) {
	cacheKey, err := e.scanCacheKey(toolConfig, imageRef)
	if err != nil {
		return nil, false, err
	}

	data, found, err := cache.Get(cacheKey)
	if err != nil || !found {
		return nil, found, err
	}

	var result scan.ScanResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, false, fmt.Errorf("unmarshaling cached scan result: %w", err)
	}

	return &result, true, nil
}

func (e *SecurityExecutor) saveScanResultToCache(cache *Cache, toolConfig ScanToolConfig, imageRef string, result *scan.ScanResult) error {
	cacheKey, err := e.scanCacheKey(toolConfig, imageRef)
	if err != nil {
		return err
	}

	data, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("marshaling scan result: %w", err)
	}

	return cache.SetWithTTL(cacheKey, data, e.scanCacheTTL())
}

func (e *SecurityExecutor) scanCacheKey(toolConfig ScanToolConfig, imageRef string) (CacheKey, error) {
	configHash, err := ComputeConfigHash(struct {
		ToolName string
		ImageRef string
	}{
		ToolName: toolConfig.Name,
		ImageRef: imageRef,
	})
	if err != nil {
		return CacheKey{}, fmt.Errorf("computing scan cache hash: %w", err)
	}

	return CacheKey{
		Operation:   "scan-" + toolConfig.Name,
		ImageDigest: imageRef,
		ConfigHash:  configHash,
	}, nil
}

func (e *SecurityExecutor) scanCacheTTL() time.Duration {
	if e.Config == nil || e.Config.Scan == nil || e.Config.Scan.Cache == nil || e.Config.Scan.Cache.TTL == "" {
		return TTL_SCAN_GRYPE
	}

	ttl, err := time.ParseDuration(e.Config.Scan.Cache.TTL)
	if err != nil || ttl <= 0 {
		return TTL_SCAN_GRYPE
	}

	return ttl
}

// ExecuteSigning performs signing operations on the image
func (e *SecurityExecutor) ExecuteSigning(ctx context.Context, imageRef string) (*signing.SignResult, error) {
	if !e.Config.Enabled || e.Config.Signing == nil || !e.Config.Signing.Enabled {
		return nil, nil // Signing disabled
	}
	if err := ValidateImageRef(imageRef); err != nil {
		return nil, fmt.Errorf("invalid image ref: %w", err)
	}

	// Propagate OIDC token to signing config so SBOM/provenance attestation
	// attachers can pass it to cosign via SIGSTORE_ID_TOKEN environment variable.
	// Safe: SecurityExecutor is created per-deploy invocation, not shared concurrently.
	if e.Context.OIDCToken != "" && e.Config.Signing.OIDCToken == "" {
		e.Config.Signing.OIDCToken = e.Context.OIDCToken
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
	if err := ValidateImageRef(imageRef); err != nil {
		return nil, fmt.Errorf("invalid image ref: %w", err)
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

	cache, cacheErr := e.newSBOMCache()
	if cacheErr != nil {
		if e.Config.SBOM.Required {
			return nil, fmt.Errorf("creating SBOM cache: %w", cacheErr)
		}
		fmt.Printf("Warning: failed to initialize SBOM cache, continuing without cache: %v\n", cacheErr)
	}
	if cache != nil {
		cachedSBOM, found, err := e.loadSBOMFromCache(cache, imageRef)
		if err != nil {
			fmt.Printf("Warning: failed to read cached SBOM: %v\n", err)
		} else if found {
			fmt.Printf("Using cached SBOM for %s...\n", imageRef)
			outputPath := ""
			if e.Config.SBOM.Output != nil && e.Config.SBOM.Output.Local != "" {
				outputPath = e.Config.SBOM.Output.Local
				if err := e.saveSBOMLocal(cachedSBOM); err != nil {
					return nil, fmt.Errorf("saving cached SBOM locally: %w", err)
				}
			}
			if e.Config.SBOM.ShouldAttach() {
				if e.Config.Signing != nil && e.Config.Signing.Enabled {
					if err := e.attachSBOM(ctx, cachedSBOM, imageRef); err != nil {
						return nil, fmt.Errorf("attaching cached SBOM: %w", err)
					}
				} else {
					err := fmt.Errorf("sbom attachment requires signing.enabled")
					if e.Config.SBOM.Required {
						return nil, err
					}
					fmt.Printf("Warning: %v\n", err)
				}
			}
			if e.Summary != nil {
				e.Summary.RecordSBOM(cachedSBOM, nil, 0, outputPath)
			}
			return cachedSBOM, nil
		}
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

	if cache != nil {
		if err := e.saveSBOMToCache(cache, imageRef, generatedSBOM); err != nil {
			fmt.Printf("Warning: failed to cache SBOM: %v\n", err)
		}
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
	if e.Config.SBOM.ShouldAttach() {
		if e.Config.Signing != nil && e.Config.Signing.Enabled {
			if err := e.attachSBOM(ctx, generatedSBOM, imageRef); err != nil {
				if e.Config.SBOM.Required {
					return nil, fmt.Errorf("attaching SBOM: %w", err)
				}
				fmt.Printf("Warning: failed to attach SBOM, continuing: %v\n", err)
			}
		} else {
			err := fmt.Errorf("sbom attachment requires signing.enabled")
			if e.Config.SBOM.Required {
				return nil, err
			}
			fmt.Printf("Warning: %v\n", err)
		}
	}

	return generatedSBOM, nil
}

// ExecuteProvenance generates and optionally attaches provenance for the image.
func (e *SecurityExecutor) ExecuteProvenance(ctx context.Context, imageRef string) (*provenance.Statement, error) {
	if !e.Config.Enabled || e.Config.Provenance == nil || !e.Config.Provenance.Enabled {
		return nil, nil
	}
	if err := ValidateImageRef(imageRef); err != nil {
		return nil, fmt.Errorf("invalid image ref: %w", err)
	}

	if err := e.Config.Provenance.Validate(); err != nil {
		if e.Config.Provenance.Required {
			return nil, fmt.Errorf("provenance validation failed: %w", err)
		}
		fmt.Printf("Warning: provenance validation failed, skipping: %v\n", err)
		if e.Summary != nil {
			e.Summary.RecordProvenance(e.Config.Provenance.Format, err, 0, false)
		}
		return nil, nil
	}

	format, err := provenance.ParseFormat(e.Config.Provenance.Format)
	if err != nil {
		if e.Config.Provenance.Required {
			return nil, fmt.Errorf("invalid provenance format: %w", err)
		}
		fmt.Printf("Warning: invalid provenance format, skipping: %v\n", err)
		if e.Summary != nil {
			e.Summary.RecordProvenance(e.Config.Provenance.Format, err, 0, false)
		}
		return nil, nil
	}

	includeEnv := e.Config.Provenance.Metadata != nil && e.Config.Provenance.Metadata.IncludeEnv
	includeMaterials := e.Config.Provenance.Metadata == nil || e.Config.Provenance.Metadata.IncludeMaterials

	startTime := time.Now()
	statement, err := provenance.Generate(ctx, imageRef, format, provenance.GenerateOptions{
		BuilderID:         builderID(e.Config.Provenance),
		SourceRoot:        ".",
		IncludeGit:        e.Config.Provenance.IncludeGit,
		IncludeDockerfile: e.Config.Provenance.IncludeDocker,
		IncludeEnv:        includeEnv,
		IncludeMaterials:  includeMaterials,
	})
	duration := time.Since(startTime)
	if err != nil {
		if e.Config.Provenance.Required {
			return nil, fmt.Errorf("generating provenance: %w", err)
		}
		fmt.Printf("Warning: provenance generation failed, continuing: %v\n", err)
		if e.Summary != nil {
			e.Summary.RecordProvenance(string(format), err, duration, false)
		}
		return nil, nil
	}

	if e.Config.Provenance.Output != nil && e.Config.Provenance.Output.Local != "" {
		if err := statement.Save(e.Config.Provenance.Output.Local); err != nil {
			if e.Config.Provenance.Required {
				return nil, fmt.Errorf("saving provenance locally: %w", err)
			}
			fmt.Printf("Warning: failed to save provenance locally: %v\n", err)
		}
	}

	attached := false
	if provenanceShouldAttach(e.Config.Provenance) {
		if e.Config.Signing == nil || !e.Config.Signing.Enabled {
			err := fmt.Errorf("provenance registry attachment requires signing.enabled")
			if e.Config.Provenance.Required {
				return nil, err
			}
			fmt.Printf("Warning: %v\n", err)
			if e.Summary != nil {
				e.Summary.RecordProvenance(string(format), nil, duration, false)
			}
			return statement, nil
		}
		if err := provenance.NewAttacher(e.Config.Signing).Attach(ctx, statement, imageRef); err != nil {
			if e.Config.Provenance.Required {
				return nil, fmt.Errorf("attaching provenance: %w", err)
			}
			fmt.Printf("Warning: failed to attach provenance, continuing: %v\n", err)
		} else {
			attached = true
		}
	}

	if e.Summary != nil {
		e.Summary.RecordProvenance(string(format), nil, duration, attached)
	}

	return statement, nil
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

func (e *SecurityExecutor) newSBOMCache() (*Cache, error) {
	if e.Config == nil || e.Config.SBOM == nil || e.Config.SBOM.Cache == nil || !e.Config.SBOM.Cache.Enabled {
		return nil, nil
	}

	return NewCache(e.Config.SBOM.Cache.Dir)
}

func (e *SecurityExecutor) loadSBOMFromCache(cache *Cache, imageRef string) (*sbom.SBOM, bool, error) {
	cacheKey, err := e.sbomCacheKey(imageRef)
	if err != nil {
		return nil, false, err
	}

	data, found, err := cache.Get(cacheKey)
	if err != nil || !found {
		return nil, found, err
	}

	var cachedSBOM sbom.SBOM
	if err := json.Unmarshal(data, &cachedSBOM); err != nil {
		return nil, false, fmt.Errorf("unmarshaling cached SBOM: %w", err)
	}

	return &cachedSBOM, true, nil
}

func (e *SecurityExecutor) saveSBOMToCache(cache *Cache, imageRef string, generatedSBOM *sbom.SBOM) error {
	cacheKey, err := e.sbomCacheKey(imageRef)
	if err != nil {
		return err
	}

	data, err := json.Marshal(generatedSBOM)
	if err != nil {
		return fmt.Errorf("marshaling SBOM: %w", err)
	}

	return cache.SetWithTTL(cacheKey, data, e.sbomCacheTTL())
}

func (e *SecurityExecutor) sbomCacheKey(imageRef string) (CacheKey, error) {
	configHash, err := ComputeConfigHash(struct {
		Format    string
		Generator string
		ImageRef  string
	}{
		Format:    e.Config.SBOM.Format,
		Generator: e.Config.SBOM.Generator,
		ImageRef:  imageRef,
	})
	if err != nil {
		return CacheKey{}, fmt.Errorf("computing SBOM cache hash: %w", err)
	}

	return CacheKey{
		Operation:   "sbom",
		ImageDigest: imageRef,
		ConfigHash:  configHash,
	}, nil
}

func (e *SecurityExecutor) sbomCacheTTL() time.Duration {
	if e.Config == nil || e.Config.SBOM == nil || e.Config.SBOM.Cache == nil || e.Config.SBOM.Cache.TTL == "" {
		return TTL_SBOM
	}

	ttl, err := time.ParseDuration(e.Config.SBOM.Cache.TTL)
	if err != nil || ttl <= 0 {
		return TTL_SBOM
	}

	return ttl
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

func builderID(config *ProvenanceConfig) string {
	if config == nil || config.Builder == nil {
		return ""
	}
	return config.Builder.ID
}

func provenanceShouldAttach(config *ProvenanceConfig) bool {
	return config != nil && config.Output != nil && config.Output.Registry
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
		importResp, err := e.uploadToDefectDojo(ctx, result, imageRef)
		duration := time.Since(startTime)

		if e.Summary != nil {
			url := ""
			if err == nil && importResp != nil && importResp.Engagement > 0 {
				url = fmt.Sprintf("%s/engagement/%d", e.Config.Reporting.DefectDojo.URL, importResp.Engagement)
			}
			e.Summary.RecordUpload("defectdojo", err, url, duration)
		}

		if err != nil {
			fmt.Printf("Warning: failed to upload to DefectDojo: %v\n", err)
		}
	}

	if e.Config.Reporting.PRComment != nil && e.Config.Reporting.PRComment.Enabled && result != nil {
		if err := e.writePRComment(result, imageRef); err != nil {
			return err
		}
	}

	return nil
}

// uploadToDefectDojo uploads scan results to DefectDojo
func (e *SecurityExecutor) uploadToDefectDojo(ctx context.Context, result *scan.ScanResult, imageRef string) (*reporting.ImportScanResponse, error) {
	config := e.Config.Reporting.DefectDojo

	// Create DefectDojo client
	client := reporting.NewDefectDojoClient(config.URL, config.APIKey)

	// Create uploader config. Append scanner tool names to the test type
	// so DefectDojo shows "Container Image Scan (grype, trivy)" instead of
	// a generic title.
	testType := config.TestType
	if testType == "" {
		testType = "Container Image Scan"
	}
	if result != nil && result.Tool != "" {
		toolName := string(result.Tool)
		if result.Metadata != nil {
			if mergedTools, ok := result.Metadata["mergedTools"]; ok {
				switch tools := mergedTools.(type) {
				case []scan.ScanTool:
					names := make([]string, 0, len(tools))
					for _, t := range tools {
						names = append(names, string(t))
					}
					toolName = strings.Join(names, ", ")
				}
			}
		}
		testType = fmt.Sprintf("%s (%s)", testType, toolName)
	}

	uploaderConfig := &reporting.DefectDojoUploaderConfig{
		EngagementID:   config.EngagementID,
		EngagementName: config.EngagementName,
		ProductID:      config.ProductID,
		ProductName:    config.ProductName,
		TestType:       testType,
		Tags:           config.Tags,
		Environment:    config.Environment,
		AutoCreate:     config.AutoCreate,
	}

	// Upload
	fmt.Printf("Uploading scan results to DefectDojo at %s...\n", config.URL)
	importResp, err := client.UploadScanResult(ctx, result, imageRef, uploaderConfig)
	if err != nil {
		return nil, err
	}

	details := make([]string, 0, 2)
	if importResp.Test > 0 {
		details = append(details, fmt.Sprintf("test ID: %d", importResp.Test))
	}
	if importResp.NumberOfFindings > 0 {
		details = append(details, fmt.Sprintf("%d findings", importResp.NumberOfFindings))
	}
	if len(details) == 0 {
		fmt.Printf("✓ Successfully uploaded to DefectDojo\n")
	} else {
		fmt.Printf("✓ Successfully uploaded to DefectDojo (%s)\n", strings.Join(details, ", "))
	}
	return importResp, nil
}

func (e *SecurityExecutor) writePRComment(result *scan.ScanResult, imageRef string) error {
	outputPath := e.Config.Reporting.PRComment.Output
	if outputPath == "" {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return fmt.Errorf("creating PR comment output directory: %w", err)
	}

	content := reporting.BuildScanResultsComment(imageRef, result, e.summaryUploads())
	if err := os.WriteFile(outputPath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("writing PR comment output: %w", err)
	}

	fmt.Printf("PR comment saved to: %s\n", outputPath)
	return nil
}

func (e *SecurityExecutor) summaryUploads() []*reporting.UploadSummary {
	if e.Summary == nil {
		return nil
	}
	return e.Summary.UploadResults
}
