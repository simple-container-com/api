package cmd_image

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/simple-container-com/api/pkg/security/signing"
)

type verifyFlags struct {
	image          string
	oidcIssuer     string
	identityRegexp string
	publicKey      string
	timeout        string
}

// NewVerifyCmd creates the verify command
func NewVerifyCmd() *cobra.Command {
	flags := &verifyFlags{}

	cmd := &cobra.Command{
		Use:   "verify",
		Short: "Verify a container image signature",
		Long: `Verify a container image signature using either keyless verification or key-based verification.

Examples:
  # Verify keyless signature
  sc image verify --image docker.io/myorg/myapp:v1.0.0 \
    --oidc-issuer https://token.actions.githubusercontent.com \
    --identity-regexp "^https://github.com/myorg/.*$"

  # Verify key-based signature
  sc image verify --image docker.io/myorg/myapp:v1.0.0 --public-key cosign.pub
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runVerify(cmd.Context(), flags)
		},
	}

	cmd.Flags().StringVar(&flags.image, "image", "", "Image reference to verify (required)")
	cmd.Flags().StringVar(&flags.oidcIssuer, "oidc-issuer", "", "OIDC issuer URL for keyless verification")
	cmd.Flags().StringVar(&flags.identityRegexp, "identity-regexp", "", "Identity regexp for keyless verification")
	cmd.Flags().StringVar(&flags.publicKey, "public-key", "", "Path to public key file for key-based verification")
	cmd.Flags().StringVar(&flags.timeout, "timeout", "2m", "Timeout for verification operation")

	_ = cmd.MarkFlagRequired("image")

	return cmd
}

func runVerify(ctx context.Context, flags *verifyFlags) error {
	if flags.image == "" {
		return fmt.Errorf("image reference is required")
	}

	// Validate verification mode
	keylessMode := flags.oidcIssuer != "" || flags.identityRegexp != ""
	keyBasedMode := flags.publicKey != ""

	if !keylessMode && !keyBasedMode {
		return fmt.Errorf("either (--oidc-issuer and --identity-regexp) or --public-key must be specified")
	}

	if keylessMode && keyBasedMode {
		return fmt.Errorf("cannot specify both keyless and key-based verification parameters")
	}

	if keylessMode && (flags.oidcIssuer == "" || flags.identityRegexp == "") {
		return fmt.Errorf("both --oidc-issuer and --identity-regexp are required for keyless verification")
	}

	config := &signing.Config{
		Timeout: flags.timeout,
	}

	var verifier *signing.Verifier
	var err error
	var verifierType string

	if keylessMode {
		config.Keyless = true
		config.OIDCIssuer = flags.oidcIssuer
		config.IdentityRegexp = flags.identityRegexp
		verifier, err = config.CreateVerifier()
		verifierType = "keyless"
		fmt.Printf("Verifying image %s with keyless verification...\n", flags.image)
	} else {
		config.PublicKey = flags.publicKey
		verifier, err = config.CreateVerifier()
		verifierType = "key-based"
		fmt.Printf("Verifying image %s with key-based verification...\n", flags.image)
	}

	if err != nil {
		return fmt.Errorf("creating verifier: %w", err)
	}

	result, err := verifier.Verify(ctx, flags.image)
	if err != nil {
		fmt.Printf("\n✗ Verification failed: %v\n", err)
		return err
	}

	if !result.Verified {
		fmt.Printf("\n✗ Image signature verification failed\n")
		return fmt.Errorf("signature verification failed")
	}

	// Display results
	fmt.Printf("\n✓ Image signature verified successfully with %s verification\n", verifierType)
	if result.ImageDigest != "" {
		fmt.Printf("  Image Digest: %s\n", result.ImageDigest)
	}
	if result.CertificateInfo != nil && result.CertificateInfo.Issuer != "" {
		fmt.Printf("  Certificate Issuer: %s\n", result.CertificateInfo.Issuer)
		if result.CertificateInfo.Identity != "" {
			fmt.Printf("  Certificate Identity: %s\n", result.CertificateInfo.Identity)
		}
	}
	if result.VerifiedAt != "" {
		fmt.Printf("  Verified At: %s\n", result.VerifiedAt)
	}

	return nil
}
