package cmd_provenance

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/simple-container-com/api/pkg/security/provenance"
)

type statementOptions struct {
	image            string
	format           string
	output           string
	builderID        string
	sourceRoot       string
	contextPath      string
	dockerfilePath   string
	includeGit       bool
	includeDocker    bool
	includeEnv       bool
	includeMaterials bool
}

func addStatementFlags(cmd *cobra.Command, opts *statementOptions) {
	cmd.Flags().StringVar(&opts.image, "image", "", "Container image reference (required)")
	cmd.Flags().StringVar(&opts.format, "format", string(provenance.FormatSLSAV10), "Provenance format")
	cmd.Flags().StringVar(&opts.output, "output", "", "Optional path to save the generated provenance statement")
	cmd.Flags().StringVar(&opts.builderID, "builder-id", "", "Builder ID to embed in the provenance statement")
	cmd.Flags().StringVar(&opts.sourceRoot, "source-root", ".", "Source repository root used for git metadata detection")
	cmd.Flags().StringVar(&opts.contextPath, "context", "", "Build context path to include in the predicate")
	cmd.Flags().StringVar(&opts.dockerfilePath, "dockerfile", "", "Dockerfile path to include as build material")
	cmd.Flags().BoolVar(&opts.includeGit, "include-git", true, "Include git metadata when available")
	cmd.Flags().BoolVar(&opts.includeDocker, "include-dockerfile", true, "Include Dockerfile metadata when a path is supplied")
	cmd.Flags().BoolVar(&opts.includeEnv, "include-env", false, "Include selected CI environment metadata")
	cmd.Flags().BoolVar(&opts.includeMaterials, "include-materials", true, "Include resolved build materials in the predicate")
}

func generateStatement(ctx context.Context, opts statementOptions) (*provenance.Statement, error) {
	format, err := provenance.ParseFormat(opts.format)
	if err != nil {
		return nil, err
	}

	return provenance.Generate(ctx, opts.image, format, provenance.GenerateOptions{
		BuilderID:         opts.builderID,
		SourceRoot:        opts.sourceRoot,
		ContextPath:       opts.contextPath,
		DockerfilePath:    opts.dockerfilePath,
		IncludeGit:        opts.includeGit,
		IncludeDockerfile: opts.includeDocker,
		IncludeEnv:        opts.includeEnv,
		IncludeMaterials:  opts.includeMaterials,
	})
}
