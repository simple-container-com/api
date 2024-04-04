package testutil

import (
	. "github.com/onsi/gomega"
	"github.com/simple-container-com/api/pkg/clouds/cloudflare"
	"github.com/simple-container-com/api/pkg/clouds/gcloud"
	"gopkg.in/yaml.v3"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/secrets"
)

const (
	testGCPConfigFile = "pkg/clouds/pulumi/testdata/secrets/gcp-e2e-config.yaml"
	testCfConfigFile  = "pkg/clouds/pulumi/testdata/secrets/cloudflare-e2e-config.yaml"
)

type E2ETestBasics struct {
	ConfigFile       *api.ConfigFile
	Cryptor          secrets.Cryptor
	CloudflareConfig *cloudflare.RegistrarConfig
}

type E2ETestConfigGCP struct {
	Credentials *gcloud.Credentials
	E2ETestBasics
}

type E2ETestConfigAWS struct {
	ServiceAccount   string
	CloudflareConfig *cloudflare.RegistrarConfig
	E2ETestBasics
}

func ReadIntegrationTestConfig() (*api.ConfigFile, secrets.Cryptor) {
	c, err := secrets.NewCryptor("", secrets.WithDetectGitDir(), secrets.WithProfile("test"), secrets.WithKeysFromCurrentProfile())
	Expect(err).To(BeNil())

	Expect(c.ReadSecretFiles()).To(BeNil())

	cfg, err := api.ReadConfigFile(c.Workdir(), "test")
	Expect(err).To(BeNil())

	return cfg, c
}

func readTestSecretConfig[T any](cryptor secrets.Cryptor, path string, cfg *T) *T {
	cfgBytes, err := cryptor.GetAndDecryptFileContent(path)
	Expect(err).To(BeNil())
	err = yaml.Unmarshal(cfgBytes, cfg)
	Expect(err).To(BeNil())
	return cfg
}

func PrepareE2EtestForGCP() E2ETestConfigGCP {
	cfg, cryptor := ReadIntegrationTestConfig()
	gcpCreds := readTestSecretConfig(cryptor, testGCPConfigFile, &gcloud.Credentials{})
	cfConfig := readTestSecretConfig(cryptor, testCfConfigFile, &cloudflare.RegistrarConfig{})
	return E2ETestConfigGCP{
		E2ETestBasics: E2ETestBasics{
			ConfigFile:       cfg,
			Cryptor:          cryptor,
			CloudflareConfig: cfConfig,
		},
		Credentials: gcpCreds,
	}
}
