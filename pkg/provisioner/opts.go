package provisioner

import (
	"context"

	"github.com/simple-container-com/api/pkg/api/git"
	"github.com/simple-container-com/api/pkg/api/logger"
	"github.com/simple-container-com/api/pkg/api/secrets"

	"github.com/simple-container-com/api/pkg/api"

	"github.com/simple-container-com/api/pkg/provisioner/placeholders"
)

type Option func(p *provisioner) error

func WithProfile(profile string) Option {
	return func(p *provisioner) error {
		p.profile = profile
		return nil
	}
}

func WithPlaceholders(ph placeholders.Placeholders) Option {
	return func(p *provisioner) error {
		p.phResolver = ph
		return nil
	}
}

func WithGitRepo(gitRepo git.Repo) Option {
	return func(p *provisioner) error {
		p.gitRepo = gitRepo
		return nil
	}
}

func WithOverrideProvisioner(prov api.Provisioner) Option {
	return func(p *provisioner) error {
		p.overrideProvisioner = prov
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
