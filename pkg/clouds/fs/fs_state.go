package fs

const (
	StateStorageTypeFileSystem    = "fs"
	SecretsProviderTypePassphrase = "passphrase"
)

// FileSystemStateStorage describes file system state storage
type FileSystemStateStorage struct {
	Path string `json:"path" yaml:"path"`
}

func (d *FileSystemStateStorage) StorageUrl() string {
	return d.Path
}

func (d *FileSystemStateStorage) IsProvisionEnabled() bool {
	return false
}

func (d *FileSystemStateStorage) CredentialsValue() string {
	return "n/a"
}

func (d *FileSystemStateStorage) ProviderType() string {
	return StateStorageTypeFileSystem
}

func (d *FileSystemStateStorage) ProjectIdValue() string {
	return "n/a"
}

// PassphraseSecretsProvider describes pass phrase secrets provider
type PassphraseSecretsProvider struct {
	PassPhrase string `json:"passPhrase" yaml:"passPhrase"`
}

func (d *PassphraseSecretsProvider) KeyUrl() string {
	return "passphrase"
}

func (d *PassphraseSecretsProvider) ProjectIdValue() string {
	return "n/a"
}

func (d *PassphraseSecretsProvider) IsProvisionEnabled() bool {
	return false
}

func (d *PassphraseSecretsProvider) CredentialsValue() string {
	return d.PassPhrase
}

func (d *PassphraseSecretsProvider) ProviderType() string {
	return SecretsProviderTypePassphrase
}
