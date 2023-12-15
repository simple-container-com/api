package secrets

import (
	"io"
	"io/fs"
	"os"
	"strings"

	git2 "api/pkg/api/git"

	"github.com/pkg/errors"

	"api/pkg/api"
)

type Option struct {
	f          func(*cryptor) error
	beforeInit bool
}

func WithGitRepo(gitRepo git2.Repo) Option {
	return Option{
		beforeInit: true,
		f: func(c *cryptor) error {
			c.gitRepo = gitRepo
			return nil
		},
	}
}

func WithDetectGitDir() Option {
	return Option{
		f: func(c *cryptor) error {
			var err error
			c.gitRepo, err = git2.New(git2.WithDetectRootDir())
			if err != nil {
				return err
			}
			c.workDir = c.gitRepo.Workdir()
			return nil
		},
		beforeInit: true,
	}
}

func WithProfile(profile string) Option {
	return Option{
		f: func(c *cryptor) error {
			c.profile = profile
			return nil
		},
	}
}

func WithKeysFromCurrentProfile() Option {
	return Option{
		f: func(c *cryptor) error {
			return WithKeysFromScConfig(c.profile).f(c)
		},
	}
}

func WithGeneratedKeys(projectName, profile string) Option {
	return Option{
		f: func(c *cryptor) error {
			c.profile = profile
			c.projectName = projectName
			return c.GenerateKeyPairWithProfile(c.projectName, c.profile)
		},
	}
}

func WithKeysFromScConfig(profile string) Option {
	return Option{
		f: func(c *cryptor) error {
			c.profile = profile
			cfg, err := api.ReadConfigFile(c.workDir, c.profile)
			if err != nil {
				return err
			}
			if cfg.PublicKeyPath != "" && cfg.PrivateKey != "" {
				return errors.New("both public key path and public key are configured")
			}
			if cfg.PrivateKeyPath != "" && cfg.PrivateKey != "" {
				return errors.New("both private key path and private key are configured")
			}
			if cfg.PrivateKeyPath != "" {
				opt := WithPrivateKey(cfg.PrivateKeyPath)
				if err := opt.f(c); err != nil {
					return err
				}
			}
			if cfg.PublicKeyPath != "" {
				opt := WithPublicKey(cfg.PublicKeyPath)
				if err := opt.f(c); err != nil {
					return err
				}
			}
			if cfg.PublicKey != "" {
				c.currentPublicKey = cfg.PublicKey
			}
			if cfg.PrivateKey != "" {
				c.currentPrivateKey = cfg.PrivateKey
			}
			return nil
		},
	}
}

func WithPublicKey(filePath string) Option {
	return Option{
		f: func(c *cryptor) error {
			file, err := c.gitRepo.OpenFile(filePath, os.O_RDONLY, fs.ModePerm)
			if err != nil {
				return errors.Wrapf(err, "failed to open public key file: %q", filePath)
			}
			defer func() { _ = file.Close() }()

			if data, err := io.ReadAll(file); err != nil {
				return err
			} else {
				c.currentPublicKey = strings.TrimSpace(string(data))
			}
			return nil
		},
	}
}

func WithPrivateKey(filePath string) Option {
	return Option{
		f: func(c *cryptor) error {
			if c.gitRepo == nil {
				return errors.New("git repo is not configured")
			}
			file, err := c.gitRepo.OpenFile(filePath, os.O_RDONLY, fs.ModePerm)
			if err != nil {
				return errors.Wrapf(err, "failed to open private key file: %q", filePath)
			}
			defer func() { _ = file.Close() }()
			if data, err := io.ReadAll(file); err != nil {
				return err
			} else {
				c.currentPrivateKey = strings.TrimSpace(string(data))
			}
			return nil
		},
	}
}
