// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package fs

import (
	"testing"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/api"
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

// init() registers the two read funcs under their type keys. A typo in either
// key, or a reader wired to the wrong struct, only shows up as "unknown
// provisioner field config type" at descriptor-read time — so assert the
// registration through the same lookup the reader uses.
func TestInit_RegistersReadersUnderTheirTypeKeys(t *testing.T) {
	RegisterTestingT(t)

	t.Run("fs state storage", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := &api.Config{Config: map[string]any{"path": "/var/state/sc"}}
		out, err := api.ReadProvisionerFieldConfig(StateStorageTypeFileSystem, cfg)
		Expect(err).ToNot(HaveOccurred())
		s, ok := out.Config.(*FileSystemStateStorage)
		Expect(ok).To(BeTrue(), "the fs key must resolve to FileSystemStateStorage, not the passphrase provider")
		Expect(s.StorageUrl()).To(Equal("/var/state/sc"))
	})

	t.Run("passphrase secrets provider", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := &api.Config{Config: map[string]any{"passPhrase": "correct horse battery staple"}}
		out, err := api.ReadProvisionerFieldConfig(SecretsProviderTypePassphrase, cfg)
		Expect(err).ToNot(HaveOccurred())
		p, ok := out.Config.(*PassphraseSecretsProvider)
		Expect(ok).To(BeTrue())
		Expect(p.PassPhrase).To(Equal("correct horse battery staple"))
		Expect(p.KeyUrl()).To(Equal("passphrase"))
	})

	t.Run("unregistered type", func(t *testing.T) {
		RegisterTestingT(t)
		_, err := api.ReadProvisionerFieldConfig("fs-typo", &api.Config{})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("unknown provisioner field config type"))
	})
}
