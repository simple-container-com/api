// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package secrets

import (
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
	Expect(os.WriteFile(secretsPath, []byte("version: 99\nregistry:\n  files: []\nsecrets: {}\n"), 0o600)).To(Succeed())

	err := c.ReadSecretFiles()
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("version 99"))
	Expect(err.Error()).To(ContainSubstring("refusing to read"))
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
