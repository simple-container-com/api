// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package fs

import (
	"path/filepath"
	"regexp"
)

const (
	StateStorageTypeFileSystem    = "fs"
	SecretsProviderTypePassphrase = "passphrase"
)

// FileSystemStateStorage describes file system state storage
type FileSystemStateStorage struct {
	Path string `json:"path" yaml:"path"`
}

// hasScheme matches a URL scheme at the START of the string, which `strings.Contains`
// (what this used before) does not: a directory may legally contain `://` anywhere
// but the front, and such a path would have been handed to Pulumi unprefixed.
var hasScheme = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.\-]*://`)

// StorageUrl is what Pulumi's DIY backend is pointed at, and it insists on a scheme:
// a bare path is rejected with `diy URL <path> has an illegal prefix; expected one
// of: azblob, file, gs, postgres, s3`. Every example of this state storage — and the
// obvious thing to write — is a plain filesystem path, so the scheme is added here
// rather than demanded of whoever writes the config. Live-caught on the first smoke
// of the byo path, 2026-10-18, where it stopped `sc provision` before it started.
//
// A path that already carries a scheme is passed through untouched: a config that
// says `file://` keeps working, and so does one that (oddly but legally) points this
// state storage at `s3://`.
//
// A RELATIVE path is made absolute first. The backend resolves it against the process
// working directory, so `path: state` or `./state` names a different directory — and
// therefore a different stack, with a different checkpoint — depending on where `sc`
// was invoked from. Absolute here means the config says the same thing from a
// subdirectory as it does from the repo root. filepath.Abs also cleans, so `..` and
// doubled separators are resolved on the way through.
func (d *FileSystemStateStorage) StorageUrl() string {
	// An unset path stays empty rather than becoming `file://$PWD`: filepath.Abs("")
	// resolves to the working directory, which would quietly put the checkpoint
	// wherever `sc` happened to be run from instead of reporting the missing field.
	if d.Path == "" || hasScheme.MatchString(d.Path) {
		return d.Path
	}
	path := d.Path
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	return "file://" + path
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
