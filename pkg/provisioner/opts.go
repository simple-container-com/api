package provisioner

import (
	"context"

	"api/pkg/provisioner/git"
	"api/pkg/provisioner/logger"
	"api/pkg/provisioner/secrets"
)

type Option func(p *provisioner) error

func WithProfile(profile string) Option {
	return func(p *provisioner) error {
		p.profile = profile
		return nil
	}
}

func WithGitRepo(gitRepo git.Repo) Option {
	return func(p *provisioner) error {
		p.gitRepo = gitRepo
		return nil
	}
}

func WithLogger(log logger.Logger) Option {
	return func(p *provisioner) error {
		p.log = log
		return nil
	}
}

func WithContext(ctx context.Context) Option {
	return func(p *provisioner) error {
		p.context = ctx
		return nil
	}
}

func WithCryptor(cryptor secrets.Cryptor) Option {
	return func(p *provisioner) error {
		p.cryptor = cryptor
		return nil
	}
}
