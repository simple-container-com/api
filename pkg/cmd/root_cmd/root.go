package root_cmd

import (
	"context"
	"path"

	"github.com/spf13/cobra"

	"go.uber.org/atomic"

	"github.com/pkg/errors"
	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/git"
	"github.com/simple-container-com/api/pkg/api/logger"
	"github.com/simple-container-com/api/pkg/provisioner"
)

type Params struct {
	Verbose bool
	Silent  bool
	Profile string

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

func (c *RootCmd) Init(skipScDirCreation bool, ignoreConfigDirError bool) error {
	ctx := context.Background()

	c.Logger = logger.New()
	gitRepo, err := git.New(
		git.WithDetectRootDir(),
	)
	if err != nil {
		return err
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
		SkipScDirCreation:   skipScDirCreation,
		IgnoreWorkdirErrors: skipScDirCreation,
		Profile:             c.Params.Profile,
		GenerateKeyPair:     c.Params.GenerateKeyPair,
	}); err != nil {
		return err
	}

	if err := c.Provisioner.Cryptor().ReadProfileConfig(); err != nil && !ignoreConfigDirError {
		return errors.Wrapf(err, "failed to read profile config, did you run `init`?")
	}
	if err := c.Provisioner.Cryptor().ReadSecretFiles(); err != nil && !ignoreConfigDirError {
		return errors.Wrapf(err, "failed to read secrets file, did you run `init`?")
	}

	return nil
}

func RegisterDeployFlags(cmd *cobra.Command, p *api.DeployParams) {
	cmd.Flags().StringVarP(&p.Profile, "profile", "p", p.Profile, "Use profile (default: `default`)")
	cmd.Flags().StringVarP(&p.StackName, "stack", "s", p.StackName, "Stack name to deploy (required)")
	_ = cmd.MarkFlagRequired("stack")
	cmd.Flags().StringVarP(&p.Environment, "env", "e", p.Environment, "Environment to deploy (required)")
	_ = cmd.MarkFlagRequired("env")
	cmd.Flags().StringVarP(&p.StacksDir, "dir", "d", p.StacksDir, "Root directory for stack configurations (default: .sc/stacks)")
	cmd.Flags().BoolVarP(&p.SkipRefresh, "skip-refresh", "R", p.SkipRefresh, "Skip refresh before deploy")
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
}
