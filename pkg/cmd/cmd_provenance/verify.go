package cmd_provenance

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/simple-container-com/api/pkg/security/provenance"
	"github.com/simple-container-com/api/pkg/security/signing"
)

type verifyOptions struct {
	image             string
	format            string
	output            string
	key               string
	keyless           bool
	certIdent         string
	certIssuer        string
	expectedDigest    string
	expectedBuilderID string
	expectedSourceURI string
	expectedCommit    string
}

// NewVerifyCommand creates the provenance verify command.
func NewVerifyCommand() *cobra.Command {
	opts := &verifyOptions{}

	cmd := &cobra.Command{
		Use:   "verify",
		Short: "Verify provenance attestation",
		Long:  `Verify a provenance attestation attached to the supplied image and optionally write the predicate locally.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runVerify(cmd.Context(), opts)
		},
	}

	cmd.Flags().StringVar(&opts.image, "image", "", "Container image reference (required)")
	cmd.Flags().StringVar(&opts.format, "format", string(provenance.FormatSLSAV10), "Expected provenance format")
	cmd.Flags().StringVar(&opts.output, "output", "", "Optional path to save the verified provenance statement")
	cmd.Flags().StringVar(&opts.key, "key", "", "Path to cosign public key for key-based verification")
	cmd.Flags().BoolVar(&opts.keyless, "keyless", false, "Use keyless verification")
	cmd.Flags().StringVar(&opts.certIdent, "cert-identity", "", "Certificate identity regexp for keyless verification")
	cmd.Flags().StringVar(&opts.certIssuer, "cert-issuer", "", "Certificate OIDC issuer for keyless verification")
	cmd.Flags().StringVar(&opts.expectedDigest, "expected-digest", "", "Expected image digest (defaults to the digest in --image when present)")
	cmd.Flags().StringVar(&opts.expectedBuilderID, "expected-builder-id", "", "Expected builder ID in the provenance statement")
	cmd.Flags().StringVar(&opts.expectedSourceURI, "expected-source-uri", "", "Expected source repository URI in provenance materials")
	cmd.Flags().StringVar(&opts.expectedCommit, "expected-commit", "", "Expected source commit in provenance materials")

	_ = cmd.MarkFlagRequired("image")

	return cmd
}

func runVerify(ctx context.Context, opts *verifyOptions) error {
	if err := validateImage(opts.image); err != nil {
		return err
	}
	format, err := provenance.ParseFormat(opts.format)
	if err != nil {
		return err
	}

	if opts.key != "" && opts.keyless {
		return fmt.Errorf("cannot specify both --keyless and --key")
	}
	if !opts.keyless && opts.key == "" {
		return fmt.Errorf("either --keyless or --key is required for provenance verification")
	}

	if opts.keyless && (opts.certIdent == "" || opts.certIssuer == "") {
		return fmt.Errorf("--cert-identity and --cert-issuer are required for keyless verification")
	}
	attacher := provenance.NewAttacher(&signing.Config{
		Enabled:        true,
		Keyless:        opts.keyless,
		PublicKey:      opts.key,
		IdentityRegexp: opts.certIdent,
		OIDCIssuer:     opts.certIssuer,
	})

	statement, err := attacher.Verify(ctx, opts.image, format)
	if err != nil {
		return fmt.Errorf("verifying provenance: %w", err)
	}

	expectedDigest := opts.expectedDigest
	if expectedDigest == "" {
		expectedDigest = provenance.ExtractDigestFromImageRef(opts.image)
	}
	if err := statement.Validate(provenance.ValidateOptions{
		ExpectedFormat:    format,
		ExpectedDigest:    expectedDigest,
		ExpectedBuilderID: opts.expectedBuilderID,
		ExpectedSourceURI: opts.expectedSourceURI,
		ExpectedCommit:    opts.expectedCommit,
	}); err != nil {
		return fmt.Errorf("validating provenance policy: %w", err)
	}

	if opts.output != "" {
		if err := statement.Save(opts.output); err != nil {
			return err
		}
		fmt.Printf("✓ Provenance verification succeeded for %s\n", opts.image)
		return nil
	}

	if _, err := os.Stdout.Write(append(statement.Content, '\n')); err != nil {
		return fmt.Errorf("writing provenance to stdout: %w", err)
	}
	return nil
}
