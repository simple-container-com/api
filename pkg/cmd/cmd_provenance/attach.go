package cmd_provenance

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/simple-container-com/api/pkg/security"
	"github.com/simple-container-com/api/pkg/security/provenance"
	"github.com/simple-container-com/api/pkg/security/signing"
)

type attachOptions struct {
	statementOptions
	keyless  bool
	key      string
	password string
}

// NewAttachCommand creates the provenance attach command.
func NewAttachCommand() *cobra.Command {
	opts := &attachOptions{}

	cmd := &cobra.Command{
		Use:   "attach",
		Short: "Generate and attach provenance attestation",
		Long:  `Generate a provenance predicate for the supplied image and attach it as a signed cosign attestation.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAttach(cmd.Context(), opts)
		},
	}

	addStatementFlags(cmd, &opts.statementOptions)
	cmd.Flags().BoolVar(&opts.keyless, "keyless", false, "Use keyless signing with OIDC")
	cmd.Flags().StringVar(&opts.key, "key", "", "Path to cosign private key for key-based signing")
	cmd.Flags().StringVar(&opts.password, "password", os.Getenv("COSIGN_PASSWORD"), "Password for encrypted cosign private key")

	_ = cmd.MarkFlagRequired("image")

	return cmd
}

func runAttach(ctx context.Context, opts *attachOptions) error {
	if err := validateImage(opts.image); err != nil {
		return err
	}
	if err := ensureTool(ctx, "cosign"); err != nil {
		return err
	}
	statement, err := generateStatement(ctx, opts.statementOptions)
	if err != nil {
		return fmt.Errorf("generating provenance: %w", err)
	}

	if opts.output != "" {
		if err := statement.Save(opts.output); err != nil {
			return err
		}
	}

	useKeyless := opts.keyless || opts.key == ""
	if opts.key != "" && opts.keyless {
		return fmt.Errorf("cannot specify both --keyless and --key")
	}

	signingCfg := &signing.Config{
		Enabled:    true,
		Keyless:    useKeyless,
		PrivateKey: opts.key,
		Password:   opts.password,
	}

	// Keyless cosign attest needs an OIDC identity token. Without it cosign 3.x
	// can exit 0 while uploading no attestation, leaving the verify step to
	// fail with "none of the attestations matched the predicate type". Mirror
	// the CLI sign path: pull from SIGSTORE_ID_TOKEN or the GitHub Actions
	// OIDC request endpoint, and fail loudly if neither is available.
	if useKeyless {
		execCtx, err := security.NewExecutionContext(ctx)
		if err != nil {
			return fmt.Errorf("creating execution context: %w", err)
		}
		if execCtx.OIDCToken == "" {
			return fmt.Errorf("OIDC token not available for keyless provenance attestation. " +
				"Ensure you're running in a CI environment with OIDC configured " +
				"(id-token: write permission on GitHub Actions), or set SIGSTORE_ID_TOKEN")
		}
		signingCfg.OIDCToken = execCtx.OIDCToken
	}

	attacher := provenance.NewAttacher(signingCfg)
	if err := attacher.Attach(ctx, statement, opts.image); err != nil {
		return fmt.Errorf("attaching provenance: %w", err)
	}

	fmt.Printf("✓ Provenance attached successfully\n")
	fmt.Printf("  Image: %s\n", opts.image)
	fmt.Printf("  Format: %s\n", statement.Format)
	fmt.Printf("  Digest: %s\n", statement.Digest)
	if opts.output != "" {
		fmt.Printf("  Output: %s\n", opts.output)
	}

	return nil
}
