package testutil

import (
	"testing"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/secrets"
)

func ReadIntegrationTestConfig(t *testing.T, testSecretFiles ...string) (*api.ConfigFile, secrets.Cryptor) {
	c, err := secrets.NewCryptor("", secrets.WithDetectGitDir(), secrets.WithProfile("test"), secrets.WithKeysFromCurrentProfile())
	Expect(err).To(BeNil())

	Expect(c.ReadSecretFiles()).To(BeNil())

	cfg, err := api.ReadConfigFile(c.Workdir(), "test")
	Expect(err).To(BeNil())

	// add service account to secrets
	for _, testSecretFile := range testSecretFiles {
		err = c.AddFile(testSecretFile)
		Expect(err).To(BeNil())
	}

	return cfg, c
}
