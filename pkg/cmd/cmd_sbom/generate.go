package cmd_sbom

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/simple-container-com/api/pkg/security/sbom"
)

// generateOptions holds options for the generate command
type generateOptions struct {
	image  string
	format string
	output string
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

	_ = cmd.MarkFlagRequired("image")
	_ = cmd.MarkFlagRequired("output")

	return cmd
}

func runGenerate(ctx context.Context, opts *generateOptions) error {
	// Validate format
	format, err := sbom.ParseFormat(opts.format)
	if err != nil {
		return fmt.Errorf("invalid format: %w", err)
	}

	// Check if syft is installed
	if err := sbom.CheckInstalled(ctx); err != nil {
		return err
	}

	// Create generator
	generator := sbom.NewSyftGenerator()

	// Generate SBOM
	fmt.Printf("Generating %s SBOM for %s...\n", format, opts.image)
	generatedSBOM, err := generator.Generate(ctx, opts.image, format)
	if err != nil {
		return fmt.Errorf("failed to generate SBOM: %w", err)
	}

	// Create output directory if needed
	outputDir := filepath.Dir(opts.output)
	if outputDir != "." && outputDir != "" {
		if err := os.MkdirAll(outputDir, 0o755); err != nil {
			return fmt.Errorf("failed to create output directory: %w", err)
		}
	}

	// Write SBOM to file
	if err := os.WriteFile(opts.output, generatedSBOM.Content, 0o644); err != nil {
		return fmt.Errorf("failed to write SBOM to file: %w", err)
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
