package secrets

import (
	"api/pkg/api"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/asn1"
	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
	"io"
	"io/fs"
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

	wdFs    billy.Filesystem
	gitFs   billy.Filesystem
	gitRepo *git.Repository

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

	ignorePatterns, err := gitignore.ReadPatterns(c.wdFs, nil)
	if err != nil {
		return errors.Wrapf(err, "failed to read gitignore patterns")
	}

	if err := c.initData(); err != nil {
		return err
	}
	if lo.IndexOf(c.registry.Files, filePath) < 0 {
		c.registry.Files = append(c.registry.Files, filePath)
	}
	if err := c.EncryptAll(); err != nil {
		return errors.Wrapf(err, "failed to re-encrypt all secrets")
	}
	if err = c.marshalSecretsFile(); err != nil {
		return err
	}

	if _, found := lo.Find(ignorePatterns, func(pattern gitignore.Pattern) bool {
		return pattern.Match(strings.Split(filePath, "/"), false) == gitignore.Exclude
	}); !found {
		err = c.addFileToIgnore(filePath)
		if err != nil {
			return errors.Wrapf(err, "failed to add file to .gitignore %q", filePath)
		}
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
	secretsFilePath := path.Join(api.ScConfigDirectory, EncryptedSecretFilesDataFileName)

	bytes, err := api.MarshalDescriptor(&c.secrets)
	if err != nil {
		return errors.Wrapf(err, "failed to marshal secrets")
	}
	var file billy.File
	if _, err := c.wdFs.Stat(secretsFilePath); os.IsNotExist(err) {
		file, err = c.wdFs.Create(secretsFilePath)
	} else {
		file, err = c.wdFs.OpenFile(secretsFilePath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.ModePerm)
	}
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	if _, err := file.Write(bytes); err != nil {
		return errors.Wrapf(err, "failed to write secrets file %q", secretsFilePath)
	}
	return nil
}

func (c *cryptor) DecryptAll() error {
	defer c.withReadLock()()

	if c.currentPublicKey == "" {
		return errors.New("public key is not configured")
	}

	if _, ok := c.secrets.Secrets[c.currentPublicKey]; !ok {
		return errors.New("current public key is not found in secrets: no decryption can be made")
	}

	for _, sFile := range c.secrets.Secrets[c.currentPublicKey].Files {
		if _, err := c.decryptSecretDataToFile(string(sFile.EncryptedData), sFile.Path); err != nil {
			return errors.Wrapf(err, "failed to decrypt secret file %q with configured public key %q", sFile.Path, c.currentPublicKey)
		}
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
	file, err := c.wdFs.Open(relFilePath)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to open secret file: %q", relFilePath)
	}
	secretData, err := io.ReadAll(file)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read secret file: %q", relFilePath)
	}

	parsed, err := ciphers.ParsePublicKey(keyData)

	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse public key: %q", keyData)
	}

	var encryptedData []byte
	encryptedData, err = rsa.EncryptOAEP(sha256.New(), rand.Reader, parsed, secretData, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to encrypt secret file: %q with publicKey %q", relFilePath, keyData)
	}

	return encryptedData, nil
}

func (c *cryptor) decryptSecretDataToFile(encryptedData string, relFilePath string) ([]byte, error) {
	if c.currentPrivateKey == "" {
		return nil, errors.New("private key is not configured")
	}

	var key *rsa.PrivateKey
	var err error
	if rawKey, err := ssh.ParseRawPrivateKey([]byte(c.currentPrivateKey)); err != nil && errors.As(err, &asn1.StructuralError{}) {
		return nil, errors.Wrapf(err, "invalid key format")
	} else if err != nil {
		return nil, errors.Wrapf(err, "failed to parse private key")
	} else if castedKey, ok := rawKey.(*rsa.PrivateKey); !ok {
		return nil, errors.Errorf("unsupported private key type: %T", rawKey)
	} else {
		key = castedKey
	}

	decrypted, err := rsa.DecryptOAEP(sha256.New(), rand.Reader, key, []byte(encryptedData), nil)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to decrypt oaep")
	}

	var file billy.File
	if _, err = c.wdFs.Stat(relFilePath); os.IsNotExist(err) {
		file, err = c.wdFs.Create(relFilePath)
	} else {
		file, err = c.wdFs.OpenFile(relFilePath, os.O_TRUNC|os.O_CREATE|os.O_WRONLY, fs.ModePerm)
	}

	if err != nil {
		return nil, errors.Wrapf(err, "failed to open secret file: %q", relFilePath)
	}
	defer func() { _ = file.Close() }()
	if _, err := io.WriteString(file, string(decrypted)); err != nil {
		return nil, errors.Wrapf(err, "failed to write secret to file %q", relFilePath)
	}

	return decrypted, nil
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

func NewCryptor(workDir string, opts ...Option) (Cryptor, error) {
	c := &cryptor{
		workDir: workDir,
	}

	beforeInitOpts := lo.Filter(opts, func(item Option, index int) bool {
		return item.beforeInit
	})
	afterInitOpts := lo.Filter(opts, func(item Option, index int) bool {
		return !item.beforeInit
	})
	if err := c.applyOpts(beforeInitOpts); err != nil {
		return nil, err
	}

	var err error
	c, err = c.openGitRepo()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to open git repository (must be in a git root: %q)", workDir)
	}
	if err := c.applyOpts(afterInitOpts); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *cryptor) applyOpts(opts []Option) error {
	for _, opt := range opts {
		if err := opt.f(c); err != nil {
			return err
		}
	}
	return nil
}

func (c *cryptor) openGitRepo() (*cryptor, error) {
	var fs *dotgit.RepositoryFilesystem
	var wt = osfs.New(c.workDir)
	if c.gitDir != "" {
		c.gitFs = osfs.New(path.Join(c.workDir, c.gitDir))
	} else {
		c.gitFs = osfs.New(path.Join(c.workDir, git.GitDirName))
	}
	if c.workDir != "" {
		c.wdFs = osfs.New(c.workDir)
	} else {
		c.wdFs = osfs.New("")
	}
	fs = dotgit.NewRepositoryFilesystem(c.gitFs, nil)
	s := filesystem.NewStorage(fs, cache.NewObjectLRUDefault())
	var err error
	c.gitRepo, err = git.Open(s, wt)
	return c, err
}

func (c *cryptor) addFileToIgnore(filePath string) error {
	filename := ".gitignore"
	var file billy.File
	var err error
	if _, err := c.wdFs.Stat(filename); os.IsNotExist(err) {
		file, err = c.wdFs.Create(filename)
	} else if err == nil {
		file, err = c.wdFs.Open(filename)
	}

	if err != nil {
		return errors.Wrapf(err, "failed to open .gitignore file")
	}

	currentContent, err := io.ReadAll(file)
	if err != nil {
		return errors.Wrapf(err, "failed to read .gitignore file")
	}

	_, err = file.Write([]byte(string(currentContent) + "\n" + filePath))
	if err != nil {
		return errors.Wrapf(err, "failed to write .gitignore file")
	}
	return nil
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
