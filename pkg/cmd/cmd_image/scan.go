package cmd_image

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/simple-container-com/api/pkg/security/scan"
	"github.com/spf13/cobra"
)

// NewScanCmd creates the image scan command
func NewScanCmd() *cobra.Command {
	var (
		image    string
		tool     string
		failOn   string
		output   string
		cacheDir string
	)

	cmd := &cobra.Command{
		Use:   "scan",
		Short: "Scan container image for vulnerabilities",
		Long:  `Scan a container image for vulnerabilities using Grype or Trivy`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if image == "" {
				return fmt.Errorf("--image flag is required")
			}

			ctx := context.Background()

			// Create scan config
			config := &scan.Config{
				Enabled: true,
				FailOn:  scan.Severity(failOn),
				Output: &scan.OutputConfig{
					Local: output,
				},
			}

			// Determine which tools to use
			if tool == "all" {
				config.Tools = []scan.ScanTool{scan.ScanToolGrype, scan.ScanToolTrivy}
			} else {
				config.Tools = []scan.ScanTool{scan.ScanTool(tool)}
			}

			// Validate config
			if err := config.Validate(); err != nil {
				return fmt.Errorf("invalid configuration: %w", err)
			}

			fmt.Printf("Scanning image: %s\n", image)
			fmt.Printf("Using tool(s): %v\n", config.Tools)
			if failOn != "" {
				fmt.Printf("Policy: Fail on %s or higher\n", failOn)
			}
			fmt.Println()

			var results []*scan.ScanResult

			// Run scanners
			for _, toolName := range config.Tools {
				scanner, err := scan.NewScanner(toolName)
				if err != nil {
					return fmt.Errorf("failed to create scanner: %w", err)
				}

				// Check if scanner is installed
				if err := scanner.CheckInstalled(ctx); err != nil {
					fmt.Printf("⚠️  %s not installed, skipping: %v\n", toolName, err)
					continue
				}

				fmt.Printf("Running %s scan...\n", toolName)

				result, err := scanner.Scan(ctx, image)
				if err != nil {
					return fmt.Errorf("%s scan failed: %w", toolName, err)
				}

				results = append(results, result)

				fmt.Printf("✓ %s scan complete\n", toolName)
				fmt.Printf("  %s\n\n", result.Summary.String())
			}

			if len(results) == 0 {
				return fmt.Errorf("no scanners were able to run")
			}

			// Merge results if multiple scanners were used
			var finalResult *scan.ScanResult
			if len(results) > 1 {
				finalResult = scan.MergeResults(results...)
				fmt.Println("Merged results from multiple scanners (deduplicated by CVE ID, highest severity kept)")
				fmt.Printf("%s\n\n", finalResult.Summary.String())
			} else {
				finalResult = results[0]
			}

			// Enforce policy
			if config.FailOn != "" {
				enforcer := scan.NewPolicyEnforcer(config)
				if err := enforcer.Enforce(finalResult); err != nil {
					fmt.Printf("❌ Policy violation: %v\n", err)
					return err
				}
				fmt.Printf("✓ Policy check passed (failOn: %s)\n", failOn)
			}

			// Save output if specified
			if output != "" {
				data, err := json.MarshalIndent(finalResult, "", "  ")
				if err != nil {
					return fmt.Errorf("failed to marshal results: %w", err)
				}

				if err := os.WriteFile(output, data, 0o644); err != nil {
					return fmt.Errorf("failed to write output: %w", err)
				}

				fmt.Printf("✓ Results saved to: %s\n", output)
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&image, "image", "", "Container image to scan (required)")
	cmd.Flags().StringVar(&tool, "tool", "grype", "Scanning tool to use: grype, trivy, or all")
	cmd.Flags().StringVar(&failOn, "fail-on", "critical", "Fail on vulnerabilities at or above this severity: critical, high, medium, low")
	cmd.Flags().StringVar(&output, "output", "", "Output file for scan results (JSON format)")
	cmd.Flags().StringVar(&cacheDir, "cache-dir", "", "Cache directory for scan results")

	return cmd
}
