// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package fs

import (
	"os"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/api"
)

func TestFileSystemStateStorage(t *testing.T) {
	RegisterTestingT(t)

	s := &FileSystemStateStorage{Path: "/var/state/sc"}

	// Pulumi's DIY backend rejects a bare path (`has an illegal prefix; expected one
	// of: azblob, file, gs, postgres, s3`), and a bare path is what every example of
	// this state storage writes — so the scheme is added rather than demanded.
	Expect(s.StorageUrl()).To(Equal("file:///var/state/sc"))
	Expect(s.IsProvisionEnabled()).To(BeFalse())
	Expect(s.CredentialsValue()).To(Equal("n/a"))
	Expect(s.ProjectIdValue()).To(Equal("n/a"))
	Expect(s.ProviderType()).To(Equal(StateStorageTypeFileSystem))
	Expect(s.ProviderType()).To(Equal("fs"))
}

func TestFileSystemStateStorage_EmptyPath(t *testing.T) {
	RegisterTestingT(t)

	s := &FileSystemStateStorage{}
	// Empty stays empty, and is NOT turned into `file://$PWD`: the missing field has
	// to be reportable, and filepath.Abs("") would silently resolve to the working
	// directory the CLI was invoked from.
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
		Expect(s.Path).To(Equal("/var/state/sc"))
		Expect(s.StorageUrl()).To(Equal("file:///var/state/sc"))
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

// StorageUrl is the only thing between the operator's `path:` and Pulumi's DIY
// backend, which accepts exactly the schemes it lists in its own error message.
//
// This table replaces TestFileSystemStateStorage_SchemeIsNotDoubled: its two
// assertions are the first two rows here, and the rows below them are the cases a
// `strings.Contains(path, "://")` check gets wrong.
func TestFileSystemStateStorage_StorageUrl(t *testing.T) {
	RegisterTestingT(t)

	cwd, err := os.Getwd()
	Expect(err).ToNot(HaveOccurred())

	for _, tc := range []struct {
		name string
		path string
		want string
	}{
		{"bare absolute path gets the file scheme", "/var/state/sc", "file:///var/state/sc"},
		{"an explicit file url is passed through", "file:///var/state/sc", "file:///var/state/sc"},
		// Someone pointing `type: fs` at a bucket is doing something odd, but
		// rewriting their URL would be worse than letting the backend answer.
		{"a non-file scheme is passed through", "s3://bucket/prefix", "s3://bucket/prefix"},
		{"a scheme with a dot or dash is still a scheme", "azblob+v2://c/p", "azblob+v2://c/p"},
		// `://` inside a directory name is not a scheme. That is the reason the check
		// is anchored rather than a substring search — unanchored, this path would be
		// handed to the backend with no scheme at all and rejected. The doubled slash
		// comes back single because filepath.Abs cleans the path, which is the same
		// normalisation the row below pins.
		{"a path merely containing :// is still a path", "/var/state/a://b", "file:///var/state/a:/b"},
		{"an absolute path is cleaned", "/var/state/../state/sc", "file:///var/state/sc"},
		{"empty stays empty", "", ""},
		// Relative is made absolute: the backend resolves it against the process
		// working directory, so `./state` would mean a different directory — and a
		// different stack — depending on where `sc` was invoked from.
		{"relative is made absolute", "state", "file://" + cwd + "/state"},
		{"dot-relative is made absolute", "./state", "file://" + cwd + "/state"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			s := &FileSystemStateStorage{Path: tc.path}
			Expect(s.StorageUrl()).To(Equal(tc.want))
		})
	}
}
