package cmd_secrets

import (
	"context"
	"path"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/git"
	"github.com/simple-container-com/api/pkg/api/logger"
	"github.com/simple-container-com/api/pkg/cmd/root_cmd"
	"github.com/simple-container-com/api/pkg/provisioner"
)

type secretsCmd struct {
	gitRepo     git.Repo
	logger      logger.Logger
	provisioner provisioner.Provisioner
	rootParams  root_cmd.Params
}

func NewSecretsCmd(rootParams root_cmd.Params) *cobra.Command {
	sCmd := &secretsCmd{
		rootParams: rootParams,
	}
	cmd := &cobra.Command{
		Use:   "secrets",
		Short: "Control repository-stored secrets",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return sCmd.init()
		},
	}

	cmd.AddCommand(
		NewHideCmd(sCmd),
		NewRevealCmd(sCmd),
		NewAllowCmd(sCmd),
		NewAddCmd(sCmd),
		NewInitCmd(sCmd),
	)
	return cmd
}

func (c *secretsCmd) init() error {
	ctx := context.Background()

	c.logger = logger.New()
	gitRepo, err := git.New(
		git.WithDetectRootDir(),
	)
	if err != nil {
		return err
	}
	c.provisioner, err = provisioner.New(
		provisioner.WithGitRepo(gitRepo),
		provisioner.WithLogger(c.logger),
	)
	if err != nil {
		return err
	}

	if err := c.provisioner.Init(ctx, api.InitParams{
		ProjectName:         path.Base(gitRepo.Workdir()),
		RootDir:             gitRepo.Workdir(),
		SkipInitialCommit:   true,
		SkipProfileCreation: true,
	}); err != nil {
		return err
	}

	if err := c.provisioner.Cryptor().ReadProfileConfig(); err != nil {
		return errors.Wrapf(err, "failed to read profile config, did you run `secrets init`?")
	}

	return nil
}
