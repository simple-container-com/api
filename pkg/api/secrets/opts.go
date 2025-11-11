package secrets

import (
	"io"
	"io/fs"
	"os"
	"strings"

	"github.com/pkg/errors"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/git"
	"github.com/simple-container-com/api/pkg/util"
)

type Option struct {
	f          func(*cryptor) error
	beforeInit bool
}

func WithConsoleWriter(writer util.ConsoleWriter) Option {
	return Option{
		beforeInit: true,
		f: func(c *cryptor) error {
			c.consoleWriter = writer
			return nil
		},
	}
}

func WithConfirmationReader(reader util.ConsoleReader) Option {
	return Option{
		beforeInit: true,
		f: func(c *cryptor) error {
			c.confirmationReader = reader
			return nil
		},
	}
}

func WithConsoleReader(reader util.ConsoleReader) Option {
	return Option{
		beforeInit: true,
		f: func(c *cryptor) error {
			c.consoleReader = reader
			return nil
		},
	}
}

func WithGitRepo(gitRepo git.Repo) Option {
	return Option{
		beforeInit: true,
		f: func(c *cryptor) error {
			c.gitRepo = gitRepo
			return nil
		},
	}
}

func WithWorkDir(wd string) Option {
	return Option{
		beforeInit: true,
		f: func(c *cryptor) error {
			c.workDir = wd
			return nil
		},
	}
}

func WithDetectGitDir() Option {
	return Option{
		f: func(c *cryptor) error {
			var err error
			c.gitRepo, err = git.New(git.WithDetectRootDir())
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
		beforeInit: true,
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

func WithGeneratedEd25519Keys(projectName, profile string) Option {
	return Option{
		f: func(c *cryptor) error {
			c.profile = profile
			c.projectName = projectName
			return c.GenerateEd25519KeyPairWithProfile(c.projectName, c.profile)
		},
	}
}

func WithKeysFromScConfig(profile string) Option {
	return Option{
		f: func(c *cryptor) error {
			if c.workDir == "" {
				return errors.Errorf("workdir is not configured")
			}
			if profile == "" {
				return errors.Errorf("profile is not configured")
			}
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
				opt := WithPrivateKeyPath(cfg.PrivateKeyPath)
				if err := opt.f(c); err != nil {
					return err
				}
			}
			if cfg.PublicKeyPath != "" {
				opt := WithPublicKeyPath(cfg.PublicKeyPath)
				if err := opt.f(c); err != nil {
					return err
				}
			}
			if cfg.PublicKey != "" {
				c.currentPublicKey = TrimPubKey(cfg.PublicKey)
			}
			if cfg.PrivateKey != "" {
				c.currentPrivateKey = TrimPrivKey(cfg.PrivateKey)
			}
			if cfg.PrivateKeyPassword != "" {
				c.privateKeyPassphrase = cfg.PrivateKeyPassword
			}
			return nil
		},
	}
}

func WithPublicKey(key string) Option {
	return Option{
		f: func(c *cryptor) error {
			c.currentPublicKey = TrimPubKey(key)
			return nil
		},
	}
}

func WithPrivateKey(key string) Option {
	return Option{
		f: func(c *cryptor) error {
			c.currentPrivateKey = key
			return nil
		},
	}
}

func WithPublicKeyPath(filePath string) Option {
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
				c.currentPublicKey = TrimPubKey(string(data))
			}
			return nil
		},
	}
}

func WithPrivateKeyPath(filePath string) Option {
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
