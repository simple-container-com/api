package pulumi

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"

	"api/pkg/api"

	secretTestutil "api/pkg/api/secrets/testutil"
)

const testSAFile = "pkg/clouds/pulumi/testdata/sc-test-project-sa.json"

func Test_CreateStack(t *testing.T) {
	RegisterTestingT(t)

	p, err := InitPulumiProvisioner()

	ctx := context.Background()

	Expect(err).To(BeNil())

	cfg, cryptor := secretTestutil.ReadIntegrationTestConfig(t, testSAFile)
	gcpSa, err := cryptor.GetAndDecryptFileContent(testSAFile)
	Expect(err).To(BeNil())

	stack := api.Stack{
		Name:    "test-stack",
		Secrets: api.SecretsDescriptor{},
		Server: api.ServerDescriptor{
			Provisioner: api.ProvisionerDescriptor{
				Type: ProvisionerTypePulumi,
				Config: api.Config{
					Config: &ProvisionerConfig{
						Organization: "organization",
						StateStorage: StateStorageConfig{
							Type:        StateStorageTypeGcpBucket,
							BucketName:  "sc-pulumi-test",
							ProjectId:   "sc-test-project-408205",
							Credentials: string(gcpSa),
							Provision:   true,
						},
						SecretsProvider: SecretsProviderConfig{
							Type:        SecretsProviderTypeGcpKms,
							Credentials: string(gcpSa),
							ProjectId:   "sc-test-project-408205",
							KeyName:     "test-key",
							KeyLocation: "global",
							Provision:   true,
						},
					},
				},
				Inherit: api.Inherit{},
			},
		},
		Client: api.ClientDescriptor{},
	}

	err = p.ProvisionStack(ctx, cfg, cryptor.PublicKey(), stack)

	Expect(err).To(BeNil())
}
