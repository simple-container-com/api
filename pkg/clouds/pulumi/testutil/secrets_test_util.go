package testutil

import (
	"fmt"
	"path"

	"gopkg.in/yaml.v3"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/secrets"
	awsApi "github.com/simple-container-com/api/pkg/clouds/aws"
	"github.com/simple-container-com/api/pkg/clouds/cloudflare"
	"github.com/simple-container-com/api/pkg/clouds/docker"
	"github.com/simple-container-com/api/pkg/clouds/gcloud"
	"github.com/simple-container-com/api/pkg/clouds/mongodb"
)

const (
	rootDirRelPath       = "pkg/clouds/pulumi/testdata"
	testGCPConfigFile    = "pkg/clouds/pulumi/testdata/secrets/gcp-e2e-config.yaml"
	testAwsConfigFile    = "pkg/clouds/pulumi/testdata/secrets/aws-e2e-config.yaml"
	testCfConfigFile     = "pkg/clouds/pulumi/testdata/secrets/cloudflare-e2e-config.yaml"
	testMongoConfigFile  = "pkg/clouds/pulumi/testdata/secrets/mongodb-e2e-config.yaml"
	testDockerConfigFile = "pkg/clouds/pulumi/testdata/secrets/docker-creds.yaml"
)

type E2ETestConfig struct {
	GcpCredentials   *gcloud.Credentials
	AwsCredentials   *awsApi.AccountConfig
	MongoConfig      *mongodb.AtlasConfig
	ConfigFile       *api.ConfigFile
	Cryptor          secrets.Cryptor
	CloudflareConfig *cloudflare.RegistrarConfig
	StacksDir        string
	DockerCreds      *docker.RegistryCredentials
}

func ReadIntegrationTestConfig() (*api.ConfigFile, secrets.Cryptor) {
	c, err := secrets.NewCryptor("", secrets.WithDetectGitDir(), secrets.WithProfile("test"), secrets.WithKeysFromCurrentProfile())
	Expect(err).To(BeNil())

	Expect(c.ReadSecretFiles()).To(BeNil())

	cfg, err := api.ReadConfigFile(c.Workdir(), "test")
	Expect(err).To(BeNil())

	return cfg, c
}

func ReadTestSecretConfig[T any](cryptor secrets.Cryptor, path string, cfg *T) *T {
	cfgBytes, err := cryptor.GetAndDecryptFileContent(path)
	Expect(err).To(BeNil())
	err = yaml.Unmarshal(cfgBytes, cfg)
	Expect(err).To(BeNil())
	return cfg
}

func PrepareE2Etest() E2ETestConfig {
	cfg, cryptor := ReadIntegrationTestConfig()
	Expect(cryptor.GetSecretFiles().Registry.Files).NotTo(BeEmpty())
	fmt.Println(cryptor.GetSecretFiles().Registry.Files) // for debugging purposes
	gcpCreds := ReadTestSecretConfig(cryptor, testGCPConfigFile, &gcloud.Credentials{})
	awsCreds := ReadTestSecretConfig(cryptor, testAwsConfigFile, &awsApi.AccountConfig{})
	cfCreds := ReadTestSecretConfig(cryptor, testCfConfigFile, &cloudflare.RegistrarConfig{})
	mongoCreds := ReadTestSecretConfig(cryptor, testMongoConfigFile, &mongodb.AtlasConfig{})
	dockerCreds := ReadTestSecretConfig(cryptor, testDockerConfigFile, &docker.RegistryCredentials{})
	return E2ETestConfig{
		ConfigFile:       cfg,
		Cryptor:          cryptor,
		CloudflareConfig: cfCreds,
		AwsCredentials:   awsCreds,
		GcpCredentials:   gcpCreds,
		MongoConfig:      mongoCreds,
		DockerCreds:      dockerCreds,
		StacksDir:        path.Join(cryptor.Workdir(), rootDirRelPath),
	}
}
