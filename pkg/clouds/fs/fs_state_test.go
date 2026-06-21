// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package fs

import (
	"testing"

	. "github.com/onsi/gomega"
)

func TestFileSystemStateStorage(t *testing.T) {
	RegisterTestingT(t)

	s := &FileSystemStateStorage{Path: "/var/state/sc"}

	Expect(s.StorageUrl()).To(Equal("/var/state/sc"))
	Expect(s.IsProvisionEnabled()).To(BeFalse())
	Expect(s.CredentialsValue()).To(Equal("n/a"))
	Expect(s.ProjectIdValue()).To(Equal("n/a"))
	Expect(s.ProviderType()).To(Equal(StateStorageTypeFileSystem))
	Expect(s.ProviderType()).To(Equal("fs"))
}

func TestFileSystemStateStorage_EmptyPath(t *testing.T) {
	RegisterTestingT(t)

	s := &FileSystemStateStorage{}
	Expect(s.StorageUrl()).To(Equal(""))
	// All other getters return constants regardless of state.
	Expect(s.ProviderType()).To(Equal(StateStorageTypeFileSystem))
	Expect(s.IsProvisionEnabled()).To(BeFalse())
}

func TestPassphraseSecretsProvider(t *testing.T) {
	RegisterTestingT(t)

	p := &PassphraseSecretsProvider{PassPhrase: "correct horse battery staple"}

	Expect(p.KeyUrl()).To(Equal("passphrase"))
	Expect(p.ProjectIdValue()).To(Equal("n/a"))
	Expect(p.IsProvisionEnabled()).To(BeFalse())
	Expect(p.CredentialsValue()).To(Equal("correct horse battery staple"))
	Expect(p.ProviderType()).To(Equal(SecretsProviderTypePassphrase))
	Expect(p.ProviderType()).To(Equal("passphrase"))
}

func TestPassphraseSecretsProvider_EmptyPassphrase(t *testing.T) {
	RegisterTestingT(t)

	p := &PassphraseSecretsProvider{}
	Expect(p.CredentialsValue()).To(Equal(""))
	// Type identifier is invariant.
	Expect(p.ProviderType()).To(Equal(SecretsProviderTypePassphrase))
	Expect(p.KeyUrl()).To(Equal("passphrase"))
}

func TestProviderTypeConstants(t *testing.T) {
	RegisterTestingT(t)

	// The constants are the contract surface for config parsing —
	// pin them so a rename breaks compilation against the parsed
	// provider-config map.
	Expect(StateStorageTypeFileSystem).To(Equal("fs"))
	Expect(SecretsProviderTypePassphrase).To(Equal("passphrase"))
}
