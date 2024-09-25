package root_cmd

import (
	"context"
	"os"
	"path"

	"go.uber.org/atomic"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/git"
	"github.com/simple-container-com/api/pkg/api/logger"
	"github.com/simple-container-com/api/pkg/provisioner"
)

type Params struct {
	Verbose bool
	Silent  bool
	Profile string

	PreviewTimeout *string
	DeployTimeout  *string

	api.InitParams
	IsCanceled *atomic.Bool
	CancelFunc func()
}

type RootCmd struct {
	*Params

	GitRepo     git.Repo
	Logger      logger.Logger
	Provisioner provisioner.Provisioner
}

type InitOpts struct {
	SkipScDirCreation    bool
	IgnoreConfigDirError bool
	ReturnOnGitError     bool
}

var IgnoreAllErrors = InitOpts{
	SkipScDirCreation:    true,
	IgnoreConfigDirError: true,
	ReturnOnGitError:     true,
}

func (c *RootCmd) Init(opts InitOpts) error {
	ctx := context.Background()

	c.Logger = logger.New()
	gitRepo, err := git.New(
		git.WithDetectRootDir(),
	)
	if err != nil && !opts.ReturnOnGitError {
		return err
	} else if err != nil && opts.ReturnOnGitError {
		return nil
	}

	c.Provisioner, err = provisioner.New(
		provisioner.WithGitRepo(gitRepo),
		provisioner.WithLogger(c.Logger),
	)
	if err != nil {
		return err
	}

	if err := c.Provisioner.Init(ctx, api.InitParams{
		ProjectName:         path.Base(gitRepo.Workdir()),
		RootDir:             gitRepo.Workdir(),
		SkipInitialCommit:   true,
		SkipProfileCreation: true,
		SkipScDirCreation:   opts.SkipScDirCreation,
		IgnoreWorkdirErrors: opts.SkipScDirCreation,
		Profile:             c.Params.Profile,
		GenerateKeyPair:     c.Params.GenerateKeyPair,
	}); err != nil {
		return err
	}

	if err := c.Provisioner.Cryptor().ReadProfileConfig(); err != nil && !opts.IgnoreConfigDirError {
		return errors.Wrapf(err, "failed to read profile config, did you run `init`?")
	}
	if err := c.Provisioner.Cryptor().ReadSecretFiles(); err != nil && !opts.IgnoreConfigDirError {
		return errors.Wrapf(err, "failed to read secrets file, did you run `init`?")
	}

	return nil
}

func RegisterDeployFlags(cmd *cobra.Command, p *api.DeployParams) {
	RegisterStackFlags(cmd, &p.StackParams, false)
	_ = cmd.MarkFlagRequired("env")
	cmd.Flags().StringVarP(&p.Version, "deploy-version", "V", os.Getenv("VERSION"), "Deploy version (default: `latest`)")

	cmd.Flags().StringVarP(&p.Timeouts.PreviewTimeout, "preview-timeout", "M", p.Timeouts.PreviewTimeout, "Timeout on preview operations (in Go's duration format, e.g. `20m`)")
	cmd.Flags().StringVarP(&p.Timeouts.ExecutionTimeout, "execution-timeout", "O", p.Timeouts.ExecutionTimeout, "Timeout on whole command execution (in Go's duration format, e.g. `20m`)")
	cmd.Flags().StringVarP(&p.Timeouts.DeployTimeout, "timeout", "T", p.Timeouts.DeployTimeout, "Timeout on deploy/provision operations (in Go's duration format, e.g. `20m`)")
}

func RegisterStackFlags(cmd *cobra.Command, p *api.StackParams, persistent bool) {
	flags := cmd.Flags()
	if persistent {
		flags = cmd.PersistentFlags()
	}
	flags.StringVarP(&p.Profile, "profile", "p", p.Profile, "Use profile (default: `default`)")
	flags.StringVarP(&p.StackName, "stack", "s", p.StackName, "Stack name to deploy (required)")
	_ = cmd.MarkFlagRequired("stack")
	flags.StringVarP(&p.Environment, "env", "e", p.Environment, "Environment to deploy")
	flags.StringVarP(&p.StacksDir, "dir", "d", p.StacksDir, "Root directory for stack configurations (default: .sc/stacks)")
	cmd.Flags().BoolVarP(&p.SkipRefresh, "skip-refresh", "R", p.SkipRefresh, "Skip refresh before deploy")
	cmd.Flags().BoolVarP(&p.SkipPreview, "skip-preview", "S", p.SkipPreview, "Skip preview before deploy")
}
