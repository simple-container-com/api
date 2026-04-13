package cmd_sbom

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/simple-container-com/api/pkg/security"
	"github.com/simple-container-com/api/pkg/security/sbom"
)

// generateOptions holds options for the generate command
type generateOptions struct {
	image    string
	format   string
	output   string
	cacheDir string
}

// NewGenerateCommand creates the generate command
func NewGenerateCommand() *cobra.Command {
	opts := &generateOptions{}

	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate SBOM for a container image",
		Long:  `Generate a Software Bill of Materials (SBOM) for a container image using Syft`,
		Example: `  # Generate CycloneDX JSON SBOM
  sc sbom generate --image myapp:v1.0 --format cyclonedx-json --output sbom.json

  # Generate SPDX JSON SBOM
  sc sbom generate --image myapp:v1.0 --format spdx-json --output sbom.spdx.json

  # Generate Syft native format
  sc sbom generate --image myapp:v1.0 --format syft-json --output sbom.syft.json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGenerate(cmd.Context(), opts)
		},
	}

	cmd.Flags().StringVar(&opts.image, "image", "", "Container image reference (required)")
	cmd.Flags().StringVar(&opts.format, "format", "cyclonedx-json", "SBOM format (cyclonedx-json, cyclonedx-xml, spdx-json, spdx-tag-value, syft-json)")
	cmd.Flags().StringVar(&opts.output, "output", "", "Output file path (required)")
	cmd.Flags().StringVar(&opts.cacheDir, "cache-dir", "", "Optional cache directory for generated SBOMs")

	_ = cmd.MarkFlagRequired("image")
	_ = cmd.MarkFlagRequired("output")

	return cmd
}

func runGenerate(ctx context.Context, opts *generateOptions) error {
	if err := ensureTool(ctx, "syft"); err != nil {
		return err
	}
	// Validate format
	format, err := sbom.ParseFormat(opts.format)
	if err != nil {
		return fmt.Errorf("invalid format: %w", err)
	}

	cfg := &security.SecurityConfig{
		Enabled: true,
		SBOM: &security.SBOMConfig{
			Enabled:   true,
			Format:    string(format),
			Generator: "syft",
			Output: &security.OutputConfig{
				Local: opts.output,
			},
		},
	}
	if opts.cacheDir != "" {
		cfg.SBOM.Cache = &security.CacheConfig{
			Enabled: true,
			Dir:     opts.cacheDir,
			TTL:     "24h",
		}
	}

	executor, err := security.NewSecurityExecutor(ctx, cfg)
	if err != nil {
		return fmt.Errorf("creating security executor: %w", err)
	}

	generatedSBOM, err := executor.ExecuteSBOM(ctx, opts.image)
	if err != nil {
		return fmt.Errorf("failed to generate SBOM: %w", err)
	}
	if generatedSBOM == nil {
		return fmt.Errorf("no SBOM generated")
	}

	// Print summary
	fmt.Printf("✓ SBOM generated successfully\n")
	fmt.Printf("  Format: %s\n", generatedSBOM.Format)
	fmt.Printf("  Size: %d bytes\n", generatedSBOM.Size())
	fmt.Printf("  Digest: %s\n", generatedSBOM.Digest)
	if generatedSBOM.Metadata != nil {
		fmt.Printf("  Tool: %s %s\n", generatedSBOM.Metadata.ToolName, generatedSBOM.Metadata.ToolVersion)
		if generatedSBOM.Metadata.PackageCount > 0 {
			fmt.Printf("  Packages: %d\n", generatedSBOM.Metadata.PackageCount)
		}
	}
	fmt.Printf("  Output: %s\n", opts.output)

	return nil
}
