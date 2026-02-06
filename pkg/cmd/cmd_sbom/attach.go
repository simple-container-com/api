package cmd_sbom

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/simple-container-com/api/pkg/security/sbom"
	"github.com/simple-container-com/api/pkg/security/signing"
)

// attachOptions holds options for the attach command
type attachOptions struct {
	image      string
	sbomFile   string
	format     string
	keyless    bool
	key        string
	certIdent  string
	certIssuer string
}

// NewAttachCommand creates the attach command
func NewAttachCommand() *cobra.Command {
	opts := &attachOptions{}

	cmd := &cobra.Command{
		Use:   "attach",
		Short: "Attach SBOM as signed attestation to image",
		Long:  `Attach a Software Bill of Materials (SBOM) as a signed in-toto attestation to a container image`,
		Example: `  # Attach SBOM with keyless signing
  sc sbom attach --image myapp:v1.0 --sbom sbom.json --keyless

  # Attach SBOM with key-based signing
  sc sbom attach --image myapp:v1.0 --sbom sbom.json --key cosign.key

  # Attach SBOM with specific format
  sc sbom attach --image myapp:v1.0 --sbom sbom.spdx.json --format spdx-json --keyless`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAttach(cmd.Context(), opts)
		},
	}

	cmd.Flags().StringVar(&opts.image, "image", "", "Container image reference (required)")
	cmd.Flags().StringVar(&opts.sbomFile, "sbom", "", "SBOM file path (required)")
	cmd.Flags().StringVar(&opts.format, "format", "cyclonedx-json", "SBOM format")
	cmd.Flags().BoolVar(&opts.keyless, "keyless", false, "Use keyless signing with OIDC")
	cmd.Flags().StringVar(&opts.key, "key", "", "Path to private key for signing")
	cmd.Flags().StringVar(&opts.certIdent, "cert-identity", "", "Certificate identity for keyless verification")
	cmd.Flags().StringVar(&opts.certIssuer, "cert-issuer", "", "Certificate OIDC issuer for keyless verification")

	_ = cmd.MarkFlagRequired("image")
	_ = cmd.MarkFlagRequired("sbom")

	return cmd
}

func runAttach(ctx context.Context, opts *attachOptions) error {
	// Validate format
	format, err := sbom.ParseFormat(opts.format)
	if err != nil {
		return fmt.Errorf("invalid format: %w", err)
	}

	// Read SBOM file
	content, err := os.ReadFile(opts.sbomFile)
	if err != nil {
		return fmt.Errorf("failed to read SBOM file: %w", err)
	}

	// Create SBOM struct
	sbomObj := sbom.NewSBOM(format, content, opts.image, &sbom.Metadata{
		ToolName:    "syft",
		ToolVersion: "unknown",
	})

	// Create signing config
	signingConfig := &signing.Config{
		Enabled:        opts.keyless || opts.key != "",
		Keyless:        opts.keyless,
		PrivateKey:     opts.key,
		IdentityRegexp: opts.certIdent,
		OIDCIssuer:     opts.certIssuer,
	}

	// Create attacher
	attacher := sbom.NewAttacher(signingConfig)

	// Attach SBOM
	fmt.Printf("Attaching %s SBOM to %s...\n", format, opts.image)
	if err := attacher.Attach(ctx, sbomObj, opts.image); err != nil {
		return fmt.Errorf("failed to attach SBOM: %w", err)
	}

	fmt.Printf("✓ SBOM attached successfully\n")
	fmt.Printf("  Image: %s\n", opts.image)
	fmt.Printf("  Format: %s\n", format)
	fmt.Printf("  Predicate Type: %s\n", format.PredicateType())
	fmt.Printf("  Attestation Type: %s\n", format.AttestationType())

	return nil
}
