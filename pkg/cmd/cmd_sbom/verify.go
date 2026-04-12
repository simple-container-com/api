package cmd_sbom

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/simple-container-com/api/pkg/security/sbom"
	"github.com/simple-container-com/api/pkg/security/signing"
)

// verifyOptions holds options for the verify command
type verifyOptions struct {
	image      string
	format     string
	output     string
	keyless    bool
	key        string
	certIdent  string
	certIssuer string
}

// NewVerifyCommand creates the verify command
func NewVerifyCommand() *cobra.Command {
	opts := &verifyOptions{}

	cmd := &cobra.Command{
		Use:   "verify",
		Short: "Verify SBOM attestation for an image",
		Long:  `Verify and retrieve a Software Bill of Materials (SBOM) attestation from a container image`,
		Example: `  # Verify SBOM with keyless verification
  sc sbom verify --image myapp:v1.0 --format cyclonedx-json --output verified.json --keyless

  # Verify SBOM with key-based verification
  sc sbom verify --image myapp:v1.0 --format cyclonedx-json --output verified.json --key cosign.pub

  # Verify SBOM with certificate identity
  sc sbom verify --image myapp:v1.0 --keyless --cert-identity user@example.com --cert-issuer https://token.actions.githubusercontent.com`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runVerify(cmd.Context(), opts)
		},
	}

	cmd.Flags().StringVar(&opts.image, "image", "", "Container image reference (required)")
	cmd.Flags().StringVar(&opts.format, "format", "cyclonedx-json", "SBOM format to verify")
	cmd.Flags().StringVar(&opts.output, "output", "", "Output file path for verified SBOM (required)")
	cmd.Flags().BoolVar(&opts.keyless, "keyless", false, "Use keyless verification with OIDC")
	cmd.Flags().StringVar(&opts.key, "key", "", "Path to public key for verification")
	cmd.Flags().StringVar(&opts.certIdent, "cert-identity", "", "Certificate identity for keyless verification")
	cmd.Flags().StringVar(&opts.certIssuer, "cert-issuer", "", "Certificate OIDC issuer for keyless verification")

	_ = cmd.MarkFlagRequired("image")
	_ = cmd.MarkFlagRequired("output")

	return cmd
}

func runVerify(ctx context.Context, opts *verifyOptions) error {
	// Validate format
	format, err := sbom.ParseFormat(opts.format)
	if err != nil {
		return fmt.Errorf("invalid format: %w", err)
	}

	// Create signing config for verification
	signingConfig := &signing.Config{
		Enabled:        opts.keyless || opts.key != "",
		Keyless:        opts.keyless,
		PublicKey:      opts.key,
		IdentityRegexp: opts.certIdent,
		OIDCIssuer:     opts.certIssuer,
	}

	// Create attacher (also handles verification)
	attacher := sbom.NewAttacher(signingConfig)

	// Verify SBOM
	fmt.Printf("Verifying %s SBOM for %s...\n", format, opts.image)
	verifiedSBOM, err := attacher.Verify(ctx, opts.image, format)
	if err != nil {
		return fmt.Errorf("failed to verify SBOM: %w", err)
	}

	// Create output directory if needed
	outputDir := filepath.Dir(opts.output)
	if outputDir != "." && outputDir != "" {
		if err := os.MkdirAll(outputDir, 0o755); err != nil {
			return fmt.Errorf("failed to create output directory: %w", err)
		}
	}

	// Write verified SBOM to file
	if err := os.WriteFile(opts.output, verifiedSBOM.Content, 0o644); err != nil {
		return fmt.Errorf("failed to write SBOM to file: %w", err)
	}

	// Print summary
	fmt.Printf("✓ SBOM verified successfully\n")
	fmt.Printf("  Image: %s\n", opts.image)
	fmt.Printf("  Format: %s\n", verifiedSBOM.Format)
	fmt.Printf("  Size: %d bytes\n", verifiedSBOM.Size())
	fmt.Printf("  Digest: %s\n", verifiedSBOM.Digest)
	if verifiedSBOM.Metadata != nil {
		fmt.Printf("  Tool: %s %s\n", verifiedSBOM.Metadata.ToolName, verifiedSBOM.Metadata.ToolVersion)
	}
	fmt.Printf("  Output: %s\n", opts.output)

	return nil
}
