package cmd_provenance

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

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

	attacher := provenance.NewAttacher(&signing.Config{
		Enabled:    true,
		Keyless:    useKeyless,
		PrivateKey: opts.key,
		Password:   opts.password,
	})
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
