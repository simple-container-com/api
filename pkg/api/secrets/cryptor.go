package secrets

import (
	"sync"

	"api/pkg/api/git"

	"github.com/samber/lo"
)

const EncryptedSecretFilesDataFileName = "secrets.yaml"

type Cryptor interface {
	GenerateKeyPairWithProfile(projectName, profile string) error
	AddFile(path string) error
	RemoveFile(path string) error
	DecryptAll() error
	EncryptChanged() error
	ReadSecretFiles() error
	GetSecretFiles() EncryptedSecretFiles
	GetAndDecryptFileContent(relPath string) ([]byte, error)
	PublicKey() string
	PrivateKey() string
	Workdir() string
}

type cryptor struct {
	workDir     string
	projectName string
	profile     string

	gitRepo git.Repo

	_lock             sync.RWMutex // для защиты secrets & registry
	currentPrivateKey string
	currentPublicKey  string
	registry          Registry
	secrets           EncryptedSecretFiles
}

func (c *cryptor) Workdir() string {
	return c.workDir
}

func (c *cryptor) PublicKey() string {
	return c.currentPublicKey
}

func (c *cryptor) PrivateKey() string {
	return c.currentPrivateKey
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

func (es *EncryptedSecrets) AddFileIfNotExist(f EncryptedSecretFile) {
	if !lo.ContainsBy(es.Files, func(item EncryptedSecretFile) bool {
		return item.Path == f.Path
	}) {
		es.Files = append(es.Files, f)
	}
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
