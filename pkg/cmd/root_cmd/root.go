package root_cmd

import (
	"context"
	"path"

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
}

type RootCmd struct {
	Params

	GitRepo     git.Repo
	Logger      logger.Logger
	Provisioner provisioner.Provisioner
}

func (c *RootCmd) Init(skipConfigRead bool) error {
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
		Profile:             c.Params.Profile,
		GenerateKeyPair:     c.Params.GenerateKeyPair,
	}); err != nil {
		return err
	}

	if skipConfigRead {
		return nil
	}

	if err := c.Provisioner.Cryptor().ReadProfileConfig(); err != nil {
		return errors.Wrapf(err, "failed to read profile config, did you run `init`?")
	}
	if err := c.Provisioner.Cryptor().ReadSecretFiles(); err != nil {
		return errors.Wrapf(err, "failed to read secrets file, did you run `init`?")
	}

	return nil
}
