package cmd_image

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/simple-container-com/api/pkg/security"
	"github.com/simple-container-com/api/pkg/security/reporting"
	"github.com/simple-container-com/api/pkg/security/scan"
)

type scanOptions struct {
	image             string
	tool              string
	failOn            string
	warnOn            string
	softFail          bool
	output            string
	sarifOutput       string
	cacheDir          string
	required          bool
	inputs            []string
	commentOutput     string
	uploadDefectDojo  bool
	defectDojoURL     string
	defectDojoAPIKey  string
	defectDojoEngID   int
	defectDojoEngName string
	defectDojoProdID  int
	defectDojoProd    string
	defectDojoTest    string
	defectDojoEnv     string
	defectDojoTags    []string
	defectDojoCreate  bool
}

// NewScanCmd creates the image scan command.
func NewScanCmd() *cobra.Command {
	opts := &scanOptions{}

	cmd := &cobra.Command{
		Use:   "scan",
		Short: "Scan container image for vulnerabilities",
		Long:  `Scan a container image for vulnerabilities using Grype, Trivy, or both via the shared security executor.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runScan(cmd.Context(), opts)
		},
	}

	cmd.Flags().StringVar(&opts.image, "image", "", "Container image to scan (required)")
	cmd.Flags().StringVar(&opts.tool, "tool", "grype", "Scanning tool to use: grype, trivy, or all")
	cmd.Flags().StringVar(&opts.failOn, "fail-on", "", "Fail on vulnerabilities at or above this severity: critical, high, medium, low")
	cmd.Flags().StringVar(&opts.warnOn, "warn-on", "high", "Warn on vulnerabilities at or above this severity: critical, high, medium, low")
	cmd.Flags().BoolVar(&opts.softFail, "soft-fail", false, "Treat failOn policy violations as warnings (exit 0). Findings are still reported and uploaded.")
	cmd.Flags().StringVar(&opts.output, "output", "", "Output file for merged scan results (JSON format)")
	cmd.Flags().StringVar(&opts.sarifOutput, "sarif-output", "", "Optional path to write merged scan results in SARIF format")
	cmd.Flags().StringVar(&opts.cacheDir, "cache-dir", "", "Cache directory for scan results")
	cmd.Flags().BoolVar(&opts.required, "required", false, "Fail if a configured scanner cannot run")
	cmd.Flags().StringSliceVar(&opts.inputs, "input", nil, "Existing scan result JSON file to merge instead of running a scanner")
	cmd.Flags().StringVar(&opts.commentOutput, "comment-output", "", "Optional path to write a markdown PR comment summary")
	_ = cmd.Flags().MarkHidden("input")

	cmd.Flags().BoolVar(&opts.uploadDefectDojo, "upload-defectdojo", false, "Upload results to DefectDojo")
	cmd.Flags().StringVar(&opts.defectDojoURL, "defectdojo-url", "", "DefectDojo instance URL")
	cmd.Flags().StringVar(&opts.defectDojoAPIKey, "defectdojo-api-key", "", "DefectDojo API key (or use DEFECTDOJO_API_KEY env var)")
	cmd.Flags().IntVar(&opts.defectDojoEngID, "defectdojo-engagement-id", 0, "Existing DefectDojo engagement ID")
	cmd.Flags().StringVar(&opts.defectDojoEngName, "defectdojo-engagement-name", "", "DefectDojo engagement name for auto-create")
	cmd.Flags().IntVar(&opts.defectDojoProdID, "defectdojo-product-id", 0, "Existing DefectDojo product ID")
	cmd.Flags().StringVar(&opts.defectDojoProd, "defectdojo-product-name", "", "DefectDojo product name for auto-create")
	cmd.Flags().StringVar(&opts.defectDojoTest, "defectdojo-test-type", "Container Scan", "DefectDojo test title prefix")
	cmd.Flags().StringVar(&opts.defectDojoEnv, "defectdojo-environment", "", "DefectDojo environment label")
	cmd.Flags().StringSliceVar(&opts.defectDojoTags, "defectdojo-tag", nil, "DefectDojo tag to attach to the imported scan result")
	cmd.Flags().BoolVar(&opts.defectDojoCreate, "defectdojo-auto-create", false, "Auto-create the product/engagement in DefectDojo when needed")

	_ = cmd.MarkFlagRequired("image")

	return cmd
}

func runScan(ctx context.Context, opts *scanOptions) error {
	if err := validateImage(opts.image); err != nil {
		return err
	}

	config, err := buildScanSecurityConfig(opts)
	if err != nil {
		return err
	}

	executor, err := security.NewSecurityExecutorWithSummary(ctx, config, opts.image)
	if err != nil {
		return fmt.Errorf("creating security executor: %w", err)
	}

	var result *scan.ScanResult
	if len(opts.inputs) > 0 {
		result, err = loadMergedScanResult(opts.inputs)
		if result == nil {
			if err != nil {
				return err
			}
			return fmt.Errorf("no scan results available")
		}
		if executor.Summary != nil && len(opts.inputs) > 1 {
			executor.Summary.RecordMergedScan(result)
		}
		err = enforceScanPolicy(config, result)
	} else {
		result, err = executor.ExecuteScanning(ctx, opts.image)
	}
	if result == nil {
		if err != nil {
			return err
		}
		return fmt.Errorf("no scan results available")
	}

	if err := executor.UploadReports(ctx, result, opts.image); err != nil {
		return err
	}

	if opts.sarifOutput != "" {
		sarifData, err := reporting.NewSARIFFromScanResult(result, opts.image)
		if err != nil {
			return err
		}
		if err := writeOutput(opts.sarifOutput, sarifData); err != nil {
			return err
		}
		fmt.Printf("SARIF results saved to: %s\n", opts.sarifOutput)
	}

	if executor.Summary != nil {
		executor.Summary.Display()
	}

	// Soft-fail: convert policy violations to warnings so the deployment continues.
	// Tool errors (scanner not installed, I/O failures) are still hard failures.
	if err != nil && opts.softFail {
		var pve *scan.PolicyViolationError
		if errors.As(err, &pve) {
			fmt.Printf("WARNING (soft-fail): %s — deployment will continue\n", pve.Message)
			return nil
		}
	}

	return err
}

func loadMergedScanResult(paths []string) (*scan.ScanResult, error) {
	results := make([]*scan.ScanResult, 0, len(paths))
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading scan result %s: %w", path, err)
		}
		var result scan.ScanResult
		if err := json.Unmarshal(data, &result); err != nil {
			return nil, fmt.Errorf("parsing scan result %s: %w", path, err)
		}
		// Validate content integrity — catches truncation, corruption, or
		// in-place tampering of intermediate result files on shared runners.
		if result.Digest != "" {
			if err := result.ValidateDigest(); err != nil {
				return nil, fmt.Errorf("scan result integrity check failed for %s: %w", path, err)
			}
		}
		results = append(results, &result)
	}

	switch len(results) {
	case 0:
		return nil, nil
	case 1:
		return results[0], nil
	default:
		return scan.MergeResults(results...), nil
	}
}

func enforceScanPolicy(config *security.SecurityConfig, result *scan.ScanResult) error {
	if config == nil || config.Scan == nil || result == nil {
		return nil
	}

	enforcer := scan.NewPolicyEnforcer(&scan.Config{
		Enabled: true,
		Tools:   []scan.ScanTool{result.Tool},
		FailOn:  scan.Severity(config.Scan.FailOn),
		WarnOn:  scan.Severity(config.Scan.WarnOn),
	})

	return enforcer.Enforce(result)
}

func buildScanSecurityConfig(opts *scanOptions) (*security.SecurityConfig, error) {
	toolNames, err := toolNames(opts.tool)
	if err != nil {
		return nil, err
	}

	scanTools := make([]security.ScanToolConfig, 0, len(toolNames))
	for _, name := range toolNames {
		scanTools = append(scanTools, security.ScanToolConfig{
			Name:     name,
			Enabled:  boolPtr(true),
			Required: opts.required,
			FailOn:   security.Severity(opts.failOn),
			WarnOn:   security.Severity(opts.warnOn),
		})
	}

	cfg := &security.SecurityConfig{
		Enabled: true,
		Scan: &security.ScanConfig{
			Enabled:  true,
			Tools:    scanTools,
			FailOn:   security.Severity(opts.failOn),
			WarnOn:   security.Severity(opts.warnOn),
			Required: opts.required,
		},
	}

	if opts.output != "" {
		cfg.Scan.Output = &security.OutputConfig{Local: opts.output}
	}
	if opts.cacheDir != "" {
		cfg.Scan.Cache = &security.CacheConfig{
			Enabled: true,
			Dir:     opts.cacheDir,
			TTL:     "6h",
		}
	}

	if opts.uploadDefectDojo {
		url := opts.defectDojoURL
		if url == "" {
			url = os.Getenv("DEFECTDOJO_URL")
		}
		apiKey := opts.defectDojoAPIKey
		if apiKey == "" {
			apiKey = os.Getenv("DEFECTDOJO_API_KEY")
		}
		if url == "" || apiKey == "" {
			return nil, fmt.Errorf("--defectdojo-url and --defectdojo-api-key are required when --upload-defectdojo is enabled")
		}

		cfg.Reporting = &security.ReportingConfig{
			DefectDojo: &security.DefectDojoConfig{
				Enabled:        true,
				URL:            url,
				APIKey:         apiKey,
				EngagementID:   opts.defectDojoEngID,
				EngagementName: opts.defectDojoEngName,
				ProductID:      opts.defectDojoProdID,
				ProductName:    opts.defectDojoProd,
				TestType:       opts.defectDojoTest,
				Tags:           opts.defectDojoTags,
				Environment:    opts.defectDojoEnv,
				AutoCreate:     opts.defectDojoCreate,
			},
		}
	}

	if opts.commentOutput != "" {
		if cfg.Reporting == nil {
			cfg.Reporting = &security.ReportingConfig{}
		}
		cfg.Reporting.PRComment = &security.PRCommentConfig{
			Enabled: true,
			Output:  opts.commentOutput,
		}
	}

	return cfg, cfg.Validate()
}

func toolNames(tool string) ([]string, error) {
	switch tool {
	case "grype":
		return []string{"grype"}, nil
	case "trivy":
		return []string{"trivy"}, nil
	case "all":
		return []string{"grype", "trivy"}, nil
	default:
		return nil, fmt.Errorf("invalid tool %q: expected grype, trivy, or all", tool)
	}
}

func writeOutput(path string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return fmt.Errorf("writing output: %w", err)
	}
	return nil
}

func boolPtr(value bool) *bool {
	return &value
}
