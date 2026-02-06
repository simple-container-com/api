package cmd_image

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/simple-container-com/api/pkg/security"
	"github.com/simple-container-com/api/pkg/security/signing"
)

type signFlags struct {
	image    string
	keyless  bool
	key      string
	password string
	timeout  string
}

// NewSignCmd creates the sign command
func NewSignCmd() *cobra.Command {
	flags := &signFlags{}

	cmd := &cobra.Command{
		Use:   "sign",
		Short: "Sign a container image",
		Long: `Sign a container image using either keyless OIDC signing or key-based signing.

Examples:
  # Sign with keyless OIDC (default for CI environments)
  sc image sign --image docker.io/myorg/myapp:v1.0.0 --keyless

  # Sign with a private key
  sc image sign --image docker.io/myorg/myapp:v1.0.0 --key cosign.key

  # Sign with a password-protected key
  sc image sign --image docker.io/myorg/myapp:v1.0.0 --key cosign.key --password mysecret
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSign(cmd.Context(), flags)
		},
	}

	cmd.Flags().StringVar(&flags.image, "image", "", "Image reference to sign (required)")
	cmd.Flags().BoolVar(&flags.keyless, "keyless", false, "Use keyless OIDC signing")
	cmd.Flags().StringVar(&flags.key, "key", "", "Path to private key file for key-based signing")
	cmd.Flags().StringVar(&flags.password, "password", os.Getenv("COSIGN_PASSWORD"), "Password for encrypted private key")
	cmd.Flags().StringVar(&flags.timeout, "timeout", "5m", "Timeout for signing operation")

	_ = cmd.MarkFlagRequired("image")

	return cmd
}

func runSign(ctx context.Context, flags *signFlags) error {
	if flags.image == "" {
		return fmt.Errorf("image reference is required")
	}

	// Validate signing mode
	if !flags.keyless && flags.key == "" {
		return fmt.Errorf("either --keyless or --key must be specified")
	}

	if flags.keyless && flags.key != "" {
		return fmt.Errorf("cannot specify both --keyless and --key")
	}

	timeout, err := time.ParseDuration(flags.timeout)
	if err != nil {
		return fmt.Errorf("invalid timeout: %w", err)
	}

	var signer signing.Signer
	var signerType string

	if flags.keyless {
		// Keyless signing - get OIDC token from environment
		execCtx, err := security.NewExecutionContext(ctx)
		if err != nil {
			return fmt.Errorf("creating execution context: %w", err)
		}

		if execCtx.OIDCToken == "" {
			return fmt.Errorf("OIDC token not available. Ensure you're running in a CI environment with OIDC configured, or set SIGSTORE_ID_TOKEN environment variable")
		}

		signer = signing.NewKeylessSigner(execCtx.OIDCToken, timeout)
		signerType = "keyless OIDC"
		fmt.Printf("Signing image %s with keyless OIDC signing...\n", flags.image)
	} else {
		// Key-based signing
		signer = signing.NewKeyBasedSigner(flags.key, flags.password, timeout)
		signerType = "key-based"
		fmt.Printf("Signing image %s with key-based signing...\n", flags.image)
	}

	result, err := signer.Sign(ctx, flags.image)
	if err != nil {
		return fmt.Errorf("signing failed: %w", err)
	}

	// Display results
	fmt.Printf("\n✓ Image signed successfully with %s signing\n", signerType)
	if result.ImageDigest != "" {
		fmt.Printf("  Image Digest: %s\n", result.ImageDigest)
	}
	if result.RekorEntry != "" {
		fmt.Printf("  Rekor Entry: %s\n", result.RekorEntry)
	}
	if result.SignedAt != "" {
		fmt.Printf("  Signed At: %s\n", result.SignedAt)
	}

	return nil
}
