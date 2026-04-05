package cmd_provenance

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

type generateOptions struct {
	statementOptions
}

// NewGenerateCommand creates the provenance generate command.
func NewGenerateCommand() *cobra.Command {
	opts := &generateOptions{}

	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate provenance statement",
		Long:  `Generate a provenance statement for the supplied image without attaching it to the registry.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGenerate(cmd.Context(), opts)
		},
	}

	addStatementFlags(cmd, &opts.statementOptions)

	_ = cmd.MarkFlagRequired("image")

	return cmd
}

func runGenerate(ctx context.Context, opts *generateOptions) error {
	statement, err := generateStatement(ctx, opts.statementOptions)
	if err != nil {
		return fmt.Errorf("generating provenance: %w", err)
	}

	if opts.output != "" {
		if err := statement.Save(opts.output); err != nil {
			return err
		}
		fmt.Printf("✓ Provenance generated successfully\n")
		fmt.Printf("  Image: %s\n", opts.image)
		fmt.Printf("  Format: %s\n", statement.Format)
		fmt.Printf("  Digest: %s\n", statement.Digest)
		fmt.Printf("  Output: %s\n", opts.output)
		return nil
	}

	if _, err := os.Stdout.Write(append(statement.Content, '\n')); err != nil {
		return fmt.Errorf("writing provenance to stdout: %w", err)
	}
	return nil
}
