package secrets

import (
	"sync"

	"github.com/samber/lo"

	"github.com/simple-container-com/api/pkg/api/git"
	"github.com/simple-container-com/api/pkg/util"
)

const EncryptedSecretFilesDataFileName = "secrets.yaml"

type Cryptor interface {
	GenerateKeyPairWithProfile(projectName, profile string) error
	ReadProfileConfig() error
	AddFile(path string) error
	RemoveFile(path string) error
	DecryptAll(forceChanged bool) error
	EncryptChanged(force bool, forceChanged bool) error
	ReadSecretFiles() error
	MarshalSecretsFile() error
	GetSecretFiles() EncryptedSecretFiles
	GetAndDecryptFileContent(relPath string) ([]byte, error)
	PublicKey() string
	PrivateKey() string
	Workdir() string
	// AddPublicKey allow another public key to encrypt secrets
	AddPublicKey(pubKey string) error
	// RemovePublicKey remove public key from encrypting secrets
	RemovePublicKey(pubKey string) error
	// GetKnownPublicKeys return all public keys
	GetKnownPublicKeys() []string

	Options() []Option
	GitRepo() git.Repo
}

type cryptor struct {
	workDir     string
	projectName string
	profile     string

	options []Option

	gitRepo git.Repo

	_lock                sync.RWMutex // для защиты secrets & registry
	currentPrivateKey    string
	currentPublicKey     string
	privateKeyPassphrase string
	secrets              EncryptedSecretFiles
	consoleWriter        util.ConsoleWriter
	consoleReader        util.ConsoleReader
	confirmationReader   util.ConsoleReader
}

func (c *cryptor) Workdir() string {
	return c.workDir
}

func (c *cryptor) GitRepo() git.Repo {
	return c.gitRepo
}

func (c *cryptor) PublicKey() string {
	return c.currentPublicKey
}

func (c *cryptor) PrivateKey() string {
	return c.currentPrivateKey
}

type EncryptedSecretFiles struct {
	Registry Registry                    `json:"registry" yaml:"registry"`
	Secrets  map[string]EncryptedSecrets `json:"secrets" yaml:"secrets"`
}

type EncryptedSecrets struct {
	Files     []EncryptedSecretFile `json:"secrets" yaml:"secrets"`
	PublicKey SshKey                `json:"publicKeys" yaml:"publicKeys"`

	// not to be serialized
	PrivateKey SshKey `json:"-" yaml:"-"`
}

func (es *EncryptedSecrets) AddFileIfNotExist(f EncryptedSecretFile) {
	if !lo.ContainsBy(es.Files, func(item EncryptedSecretFile) bool {
		return item.Path == f.Path
	}) {
		es.Files = append(es.Files, f)
	}
}

func (es *EncryptedSecrets) RemoveFile(f EncryptedSecretFile) {
	es.Files = lo.Filter(es.Files, func(item EncryptedSecretFile, _ int) bool {
		return item.Path != f.Path
	})
}

func (es *EncryptedSecrets) GetEncryptedContent(path string) []string {
	if file, found := lo.Find(es.Files, func(item EncryptedSecretFile) bool {
		return item.Path == path
	}); !found {
		return []string{}
	} else {
		return file.EncryptedData
	}
}

type SshKey struct {
	Data []byte `json:"data" yaml:"data"`
}

type Registry struct {
	Files []string `json:"files" yaml:"files"`
}

type EncryptedSecretFile struct {
	Path          string   `json:"path" yaml:"path"`
	EncryptedData []string `json:"encryptedData" yaml:"encryptedData"`
}
