package testutil

import (
	"testing"

	. "github.com/onsi/gomega"

	"api/pkg/api"
	"api/pkg/api/secrets"
)

func ReadIntegrationTestConfig(t *testing.T, testSecretFiles ...string) (*api.ConfigFile, secrets.Cryptor) {
	c, err := secrets.NewCryptor("", secrets.WithDetectGitDir(), secrets.WithProfile("test"), secrets.WithKeysFromCurrentProfile())
	Expect(err).To(BeNil())

	cfg, err := api.ReadConfigFile(c.Workdir(), "test")
	Expect(err).To(BeNil())

	// add service account to secrets
	for _, testSecretFile := range testSecretFiles {
		err = c.AddFile(testSecretFile)
		Expect(err).To(BeNil())
	}

	return cfg, c
}
