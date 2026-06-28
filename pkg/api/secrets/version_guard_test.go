// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package secrets

import (
	"errors"
	"os"
	"path"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/api"
)

// TestUnmarshalSecretsFile_RejectsNewerVersion is the fail-closed guard: a store
// written by a future schema version must be refused, not partially read (which
// would silently drop the fields this build can't model and corrupt the store).
func TestUnmarshalSecretsFile_RejectsNewerVersion(t *testing.T) {
	RegisterTestingT(t)
	c, wd, cleanup := newTestCryptor(t)
	defer cleanup()

	secretsPath := path.Join(wd, api.ScConfigDirectory, EncryptedSecretFilesDataFileName)
	Expect(os.WriteFile(secretsPath, []byte("schemaVersion: 99\nregistry:\n  files: []\nsecrets: {}\n"), 0o600)).To(Succeed())

	err := c.ReadSecretFiles()
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("version 99"))
	Expect(err.Error()).To(ContainSubstring("refusing to read"))
	// Must be detectable as the sentinel: root_cmd relies on errors.Is to keep
	// this fatal even on the IgnoreConfigDirError CLI path (else a too-new store
	// reads as empty and the next write clobbers it).
	Expect(errors.Is(err, ErrSecretsStoreVersionUnsupported)).To(BeTrue())
}

// TestUnmarshalSecretsFile_AcceptsCurrentVersion confirms back-compat: a store
// with no version field (version 0, the original format) reads normally.
func TestUnmarshalSecretsFile_AcceptsCurrentVersion(t *testing.T) {
	RegisterTestingT(t)
	c, wd, cleanup := newTestCryptor(t)
	defer cleanup()

	secretsPath := path.Join(wd, api.ScConfigDirectory, EncryptedSecretFilesDataFileName)
	Expect(os.WriteFile(secretsPath, []byte("registry:\n  files: []\nsecrets: {}\n"), 0o600)).To(Succeed())

	Expect(c.ReadSecretFiles()).To(Succeed())
}

// TestIsUnsupportedStoreVersion guards the contract every tolerant read-path
// caller relies on (root_cmd.Init + the GitHub Actions reveal fallbacks): the
// too-new-store error must be detectable via the helper so those callers keep it
// fatal instead of swallowing it as "no secrets".
func TestIsUnsupportedStoreVersion(t *testing.T) {
	RegisterTestingT(t)
	c, wd, cleanup := newTestCryptor(t)
	defer cleanup()

	secretsPath := path.Join(wd, api.ScConfigDirectory, EncryptedSecretFilesDataFileName)
	Expect(os.WriteFile(secretsPath, []byte("schemaVersion: 99\nregistry:\n  files: []\nsecrets: {}\n"), 0o600)).To(Succeed())

	Expect(IsUnsupportedStoreVersion(c.ReadSecretFiles())).To(BeTrue())
	Expect(IsUnsupportedStoreVersion(nil)).To(BeFalse())
	Expect(IsUnsupportedStoreVersion(errors.New("some other read error"))).To(BeFalse())
}
