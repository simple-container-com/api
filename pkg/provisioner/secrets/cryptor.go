package secrets

import (
	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/osfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/cache"
	"github.com/go-git/go-git/v5/storage/filesystem"
	"github.com/go-git/go-git/v5/storage/filesystem/dotgit"
	"github.com/pkg/errors"
	"path"
)

const EncryptedSecretFilesDataFileName = "secrets.json"

type Cryptor interface {
	AddFile(path string) error
	RemoveFile(path string) error
	DecryptAll() error
}

type cryptor struct {
	workDir     string
	gitDir      string
	gitFs       *git.Repository
	secretFiles EncryptedSecretFiles
}

func (c *cryptor) AddFile(path string) error {
	return nil
}

func (c *cryptor) RemoveFile(path string) error {
	return nil
}

func (c *cryptor) DecryptAll() error {
	return nil
}

type Option func(c *cryptor)

func WithGitDir(dir string) Option {
	return func(c *cryptor) {
		c.gitDir = dir
	}
}

func NewCryptor(workDir string, opts ...Option) (Cryptor, error) {
	c := &cryptor{
		workDir: workDir,
	}
	for _, opt := range opts {
		opt(c)
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
	Secrets []EncryptedSecrets `json:"secrets" yaml:"secrets"`
}

type EncryptedSecrets struct {
	PublicKey             PublicKey             `json:"publicKeys" yaml:"publicKeys"`
	Files                 []EncryptedSecretFile `json:"secrets" yaml:"secrets"`
	DefaultPrivateKeyPath string                `json:"defaultPrivateKeyPath" yaml:"defaultPrivateKeyPath"`
	PrivateKeyData        []byte                `json:"-" yaml:"-"`
}

type PublicKey struct {
	Data []byte `json:"data" yaml:"data"`
}

type EncryptedSecretFile struct {
	Path          string `json:"path" yaml:"path"`
	EncryptedData []byte `json:"encryptedData" yaml:"encryptedData"`
}
