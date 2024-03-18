package testutil

import (
	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/secrets"
)

const (
	testSAFile = "pkg/clouds/pulumi/testdata/sc-test-project-sa.json"
)

func ReadIntegrationTestConfig() (*api.ConfigFile, secrets.Cryptor) {
	c, err := secrets.NewCryptor("", secrets.WithDetectGitDir(), secrets.WithProfile("test"), secrets.WithKeysFromCurrentProfile())
	Expect(err).To(BeNil())

	Expect(c.ReadSecretFiles()).To(BeNil())

	cfg, err := api.ReadConfigFile(c.Workdir(), "test")
	Expect(err).To(BeNil())

	return cfg, c
}

func PrepareE2EtestForGCP() (*api.ConfigFile, secrets.Cryptor, string) {
	cfg, cryptor := ReadIntegrationTestConfig()
	gcpSa, err := cryptor.GetAndDecryptFileContent(testSAFile)
	Expect(err).To(BeNil())
	return cfg, cryptor, string(gcpSa)
}
