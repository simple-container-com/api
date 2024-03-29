package testutil

import (
	. "github.com/onsi/gomega"
	"github.com/simple-container-com/api/pkg/clouds/cloudflare"
	"gopkg.in/yaml.v3"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/secrets"
)

const (
	testSAFile       = "pkg/clouds/pulumi/testdata/sc-test-project-sa.json"
	testCfConfigFile = "pkg/clouds/pulumi/testdata/cloudflare-e2e-config.yaml"
)

type E2ETestConfigGCP struct {
	ServiceAccount   string
	CloudflareConfig *cloudflare.RegistrarConfig
}

func ReadIntegrationTestConfig() (*api.ConfigFile, secrets.Cryptor) {
	c, err := secrets.NewCryptor("", secrets.WithDetectGitDir(), secrets.WithProfile("test"), secrets.WithKeysFromCurrentProfile())
	Expect(err).To(BeNil())

	Expect(c.ReadSecretFiles()).To(BeNil())

	cfg, err := api.ReadConfigFile(c.Workdir(), "test")
	Expect(err).To(BeNil())

	return cfg, c
}

func PrepareE2EtestForGCP() (*api.ConfigFile, secrets.Cryptor, E2ETestConfigGCP) {
	cfg, cryptor := ReadIntegrationTestConfig()
	gcpSa, err := cryptor.GetAndDecryptFileContent(testSAFile)
	Expect(err).To(BeNil())
	cfConfigBytes, err := cryptor.GetAndDecryptFileContent(testCfConfigFile)
	Expect(err).To(BeNil())
	cfConfig := cloudflare.RegistrarConfig{}
	err = yaml.Unmarshal(cfConfigBytes, &cfConfig)
	Expect(err).To(BeNil())
	return cfg, cryptor, E2ETestConfigGCP{
		ServiceAccount:   string(gcpSa),
		CloudflareConfig: &cfConfig,
	}
}
