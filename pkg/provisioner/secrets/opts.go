package secrets

import (
	"api/pkg/api"
	"io"
	"io/fs"
	"os"
	"strings"

	"github.com/pkg/errors"
)

type Option struct {
	f          func(*cryptor) error
	beforeInit bool
}

func WithGitDir(dir string) Option {
	return Option{
		beforeInit: true,
		f: func(c *cryptor) error {
			c.gitDir = dir
			return nil
		},
	}
}

func WithKeysFromScConfig(profile string) Option {
	return Option{
		f: func(c *cryptor) error {
			cfg, err := api.ReadConfigFile(c.workDir, profile)
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
			//if !path.IsAbs(filePath) {
			//	filePath = path.Join(c.workDir, filePath)
			//}

			file, err := c.wdFs.OpenFile(filePath, os.O_RDONLY, fs.ModePerm)
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
			//if !path.IsAbs(filePath) {
			//	filePath = path.Join(c.workDir, filePath)
			//}
			file, err := c.wdFs.OpenFile(filePath, os.O_RDONLY, fs.ModePerm)
			if err != nil {
				return errors.Wrapf(err, "failed to open public key file: %q", filePath)
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
