package secrets

import (
	"api/pkg/api"
	"crypto/rsa"
	"os"
	"path"
	"strings"
	"sync"

	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/osfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/cache"
	"github.com/go-git/go-git/v5/storage/filesystem"
	"github.com/go-git/go-git/v5/storage/filesystem/dotgit"
	"github.com/pkg/errors"
	"github.com/samber/lo"
	"golang.org/x/crypto/ssh"

	"api/pkg/provisioner/secrets/ciphers"
)

const EncryptedSecretFilesDataFileName = "secrets.yaml"

type Cryptor interface {
	AddFile(path string) error
	RemoveFile(path string) error
	DecryptAll() error
	EncryptAll() error
	GetSecretFiles() EncryptedSecretFiles
}

type cryptor struct {
	workDir string
	gitDir  string
	gitFs   *git.Repository

	profile string

	_lock             sync.RWMutex // для защиты secrets & registry
	currentPrivateKey string
	currentPublicKey  string
	registry          SecretsRegistry
	secrets           EncryptedSecretFiles
}

func (c *cryptor) GetSecretFiles() EncryptedSecretFiles {
	defer c.withReadLock()()
	res := c.secrets
	return res
}

func (c *cryptor) AddFile(filePath string) error {
	defer c.withWriteLock()()

	if err := c.initData(); err != nil {
		return err
	}
	if lo.IndexOf(c.registry.Files, filePath) < 0 {
		c.registry.Files = append(c.registry.Files, filePath)
	}
	if err := c.EncryptAll(); err != nil {
		return errors.Wrapf(err, "failed to re-encrypt all secrets")
	}
	err := c.marshalSecretsFile()
	if err != nil {
		return err
	}
	return nil
}

func (c *cryptor) RemoveFile(filePath string) error {
	defer c.withWriteLock()()
	if err := c.initData(); err != nil {
		return err
	}
	c.registry.Files = lo.Filter(c.registry.Files, func(s string, _ int) bool {
		return s != filePath
	})
	if err := c.EncryptAll(); err != nil {
		return errors.Wrapf(err, "failed to re-encrypt all secrets")
	}
	err := c.marshalSecretsFile()
	if err != nil {
		return err
	}
	return nil
}

func (c *cryptor) marshalSecretsFile() error {
	secretsFilePath := path.Join(c.workDir, api.ScConfigDirectory, EncryptedSecretFilesDataFileName)

	bytes, err := api.MarshalDescriptor(&c.secrets)
	if err != nil {
		return errors.Wrapf(err, "failed to marshal secrets")
	}
	if err := os.WriteFile(secretsFilePath, bytes, 0644); err != nil {
		return errors.Wrapf(err, "failed to write secrets file %q", secretsFilePath)
	}
	return nil
}

func (c *cryptor) EncryptAll() error {
	for publicKey := range c.secrets.Secrets {
		filteredSecrets := c.secrets.Secrets[publicKey]
		filteredSecrets.Files = lo.Filter(filteredSecrets.Files, func(file EncryptedSecretFile, _ int) bool {
			return lo.Contains(c.registry.Files, file.Path)
		})
		c.secrets.Secrets[publicKey] = filteredSecrets
	}
	for _, relFilePath := range c.registry.Files {
		for publicKey := range c.secrets.Secrets {
			secrets, err := c.encryptSecretsFileWith(publicKey, relFilePath)
			if err != nil {
				return err
			}
			c.secrets.Secrets[publicKey] = secrets
		}
		secrets, err := c.encryptSecretsFileWith(c.currentPublicKey, relFilePath)
		if err != nil {
			return err
		}
		c.secrets.Secrets[c.currentPublicKey] = secrets
	}
	return nil
}

func (c *cryptor) encryptSecretsFileWith(publicKey string, relFilePath string) (EncryptedSecrets, error) {
	secrets := EncryptedSecrets{}
	secrets.PublicKey = SshKey{Data: []byte(publicKey)}
	encryptedData, err := c.encryptSecretFile(publicKey, relFilePath)
	if err != nil {
		return EncryptedSecrets{}, err
	}
	secrets.Files = append(secrets.Files, EncryptedSecretFile{
		Path:          relFilePath,
		EncryptedData: encryptedData,
	})
	return secrets, nil
}

func (c *cryptor) encryptSecretFile(keyData string, relFilePath string) ([]byte, error) {
	fullPath := path.Join(c.workDir, relFilePath)

	secretData, err := os.ReadFile(fullPath)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read secret file: %q", fullPath)
	}

	parsed, _, _, _, err := ssh.ParseAuthorizedKey([]byte(keyData))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse public key: %q", keyData)
	}
	// To get back to an *rsa.PublicKey, we need to first upgrade to the
	// ssh.CryptoPublicKey interface
	parsedCryptoKey := parsed.(ssh.CryptoPublicKey)

	// Then, we can call CryptoPublicKey() to get the actual crypto.PublicKey
	pubCrypto := parsedCryptoKey.CryptoPublicKey()

	// Finally, we can convert back to an *rsa.PublicKey

	var encryptedData []byte
	if pub, ok := pubCrypto.(*rsa.PublicKey); ok {
		encryptedData, err = ciphers.EncryptWithPublicRsaKey(secretData, pub)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to encrypt secret file: %q with publicKey %q", fullPath, keyData)
		}
	} else {
		return nil, errors.Errorf("unsupported public key type: %T", pubCrypto)
	}

	return encryptedData, nil
}

func (c *cryptor) initData() error {
	if c.secrets.Secrets == nil {
		c.secrets.Secrets = make(map[string]EncryptedSecrets, 0)
	}
	if c.currentPublicKey == "" {
		return errors.New("public key is not configured")
	}
	if c.currentPrivateKey == "" {
		return errors.New("private key is not configured")
	}
	return nil
}

func (c *cryptor) withReadLock() func() {
	c._lock.RLock()
	return func() {
		c._lock.RUnlock()
	}
}

func (c *cryptor) withWriteLock() func() {
	c._lock.Lock()
	return func() {
		c._lock.Unlock()
	}
}

func (c *cryptor) DecryptAll() error {
	return nil
}

type Option func(c *cryptor) error

func WithGitDir(dir string) Option {
	return func(c *cryptor) error {
		c.gitDir = dir
		return nil
	}
}

func WithKeysFromScConfig(profile string) Option {
	return func(c *cryptor) error {
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
			if err := opt(c); err != nil {
				return err
			}
		}
		if cfg.PublicKeyPath != "" {
			opt := WithPublicKey(cfg.PublicKeyPath)
			if err := opt(c); err != nil {
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
	}
}

func WithPublicKey(filePath string) Option {
	return func(c *cryptor) error {
		if !path.IsAbs(filePath) {
			filePath = path.Join(c.workDir, filePath)
		}
		if data, err := os.ReadFile(filePath); err != nil {
			return err
		} else {
			c.currentPublicKey = strings.TrimSpace(string(data))
		}
		return nil
	}
}

func WithPrivateKey(filePath string) Option {
	return func(c *cryptor) error {
		if !path.IsAbs(filePath) {
			filePath = path.Join(c.workDir, filePath)
		}
		if data, err := os.ReadFile(filePath); err != nil {
			return err
		} else {
			c.currentPrivateKey = strings.TrimSpace(string(data))
		}
		return nil
	}
}

func NewCryptor(workDir string, opts ...Option) (Cryptor, error) {
	c := &cryptor{
		workDir: workDir,
	}
	for _, opt := range opts {
		if err := opt(c); err != nil {
			return nil, err
		}
	}
	var err error
	c, err = c.openGitRepo()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to open git repository (must be in a git root: %q)", workDir)
	}
	return c, nil
}

func (c *cryptor) openGitRepo() (*cryptor, error) {
	var fs *dotgit.RepositoryFilesystem
	var bfs billy.Filesystem
	var wt = osfs.New(c.workDir)
	if c.gitDir != "" {
		bfs = osfs.New(path.Join(c.workDir, c.gitDir))
	} else {
		bfs = osfs.New(path.Join(c.workDir, git.GitDirName))
	}
	fs = dotgit.NewRepositoryFilesystem(bfs, nil)
	s := filesystem.NewStorage(fs, cache.NewObjectLRUDefault())
	var err error
	c.gitFs, err = git.Open(s, wt)
	return c, err
}

type EncryptedSecretFiles struct {
	Secrets map[string]EncryptedSecrets `json:"secrets" yaml:"secrets"`
}

type EncryptedSecrets struct {
	Files     []EncryptedSecretFile `json:"secrets" yaml:"secrets"`
	PublicKey SshKey                `json:"publicKeys" yaml:"publicKeys"`

	// not to be serialized
	PrivateKey SshKey `json:"-" yaml:"-"`
}

type SshKey struct {
	Data []byte `json:"data" yaml:"data"`
}

type SecretsRegistry struct {
	Files []string `json:"files" yaml:"files"`
}

type EncryptedSecretFile struct {
	Path          string `json:"path" yaml:"path"`
	EncryptedData []byte `json:"encryptedData" yaml:"encryptedData"`
}
