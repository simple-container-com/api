// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package fs

import "strings"

const (
	StateStorageTypeFileSystem    = "fs"
	SecretsProviderTypePassphrase = "passphrase"
)

// FileSystemStateStorage describes file system state storage
type FileSystemStateStorage struct {
	Path string `json:"path" yaml:"path"`
}

// StorageUrl is what Pulumi's DIY backend is pointed at, and it insists on a scheme:
// a bare path is rejected with `diy URL <path> has an illegal prefix; expected one
// of: azblob, file, gs, postgres, s3`. Every example of this state storage — and the
// obvious thing to write — is a plain filesystem path, so the scheme is added here
// rather than demanded of whoever writes the config. Live-caught on the first smoke
// of the byo path, 2026-10-18, where it stopped `sc provision` before it started.
//
// A path that already carries any scheme is passed through untouched: a config that
// says `file://` keeps working, and so does one that (oddly but legally) points this
// state storage at `s3://`.
func (d *FileSystemStateStorage) StorageUrl() string {
	if d.Path == "" || strings.Contains(d.Path, "://") {
		return d.Path
	}
	return "file://" + d.Path
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
