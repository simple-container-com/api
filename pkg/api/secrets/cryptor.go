// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package secrets

import (
	"errors"
	"sync"

	"github.com/samber/lo"

	"github.com/simple-container-com/api/pkg/api/git"
	"github.com/simple-container-com/api/pkg/util"
)

const EncryptedSecretFilesDataFileName = "secrets.yaml"

// CurrentSecretsFileVersion is the highest secrets.yaml schema version this build
// understands. A store with no `version` field is treated as version 0 (the
// original, current format). When a future format bumps this, OLDER binaries
// (which carry a lower CurrentSecretsFileVersion) refuse to read it — see the
// guard in unmarshalSecretsFile — instead of silently dropping the new fields on
// the next write. This reader must therefore ship and roll out fleet-wide BEFORE
// any higher-versioned store is ever written.
const CurrentSecretsFileVersion = 0

// ErrSecretsStoreVersionUnsupported is returned when the on-disk store declares a
// schema version newer than CurrentSecretsFileVersion. It MUST stay fatal on every
// read path — including ones that otherwise tolerate a missing/uninitialized store
// (root_cmd's IgnoreConfigDirError) — because reading a too-new store as empty and
// then writing would clobber it. Detect it with errors.Is.
var ErrSecretsStoreVersionUnsupported = errors.New("unsupported secrets store version")

type Cryptor interface {
	GenerateKeyPairWithProfile(projectName, profile string) error
	GenerateEd25519KeyPairWithProfile(projectName, profile string) error
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
	// Version is the secrets.yaml schema version. Absent/0 = the original format.
	// A reader refuses any version above CurrentSecretsFileVersion (fail-closed).
	Version  int                         `json:"version,omitempty" yaml:"version,omitempty"`
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
