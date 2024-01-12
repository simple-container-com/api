package cmd_secrets

import (
	"context"
	"path"

	"api/pkg/api/git"
	"api/pkg/provisioner"

	"github.com/spf13/cobra"

	"api/pkg/api/logger"
	"api/pkg/cmd/root_cmd"
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

	return c.provisioner.Init(ctx, provisioner.InitParams{
		ProjectName:         path.Base(gitRepo.Workdir()),
		RootDir:             gitRepo.Workdir(),
		SkipInitialCommit:   true,
		SkipProfileCreation: true,
	})
}
