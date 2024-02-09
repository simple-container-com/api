package pulumi

import (
	"api/pkg/clouds/gcloud"
	"context"
	"testing"

	. "github.com/onsi/gomega"

	"api/pkg/api"

	secretTestutil "api/pkg/api/secrets/testutil"
)

const (
	testSAFile        = "pkg/clouds/pulumi/testdata/sc-test-project-sa.json"
	e2eTestProject    = "sc-test-project-408205"
	e2eStackName      = "e2e-create--stack"
	e2eKmsTestKeyName = "e2e-create--key"
	e2eBucketName     = "sc-pulumi-test"
)

func Test_CreateStack(t *testing.T) {
	RegisterTestingT(t)

	p, err := InitPulumiProvisioner()

	ctx := context.Background()

	Expect(err).To(BeNil())

	cfg, cryptor := secretTestutil.ReadIntegrationTestConfig(t, testSAFile)
	gcpSa, err := cryptor.GetAndDecryptFileContent(testSAFile)
	Expect(err).To(BeNil())

	stack := api.Stack{
		Name: e2eStackName,
		Server: api.ServerDescriptor{
			Provisioner: api.ProvisionerDescriptor{
				Type: ProvisionerTypePulumi,
				Config: api.Config{
					Config: &ProvisionerConfig{
						Organization: "organization",
						StateStorage: StateStorageConfig{
							Type:        StateStorageTypeGcpBucket,
							BucketName:  e2eBucketName,
							ProjectId:   e2eTestProject,
							Credentials: string(gcpSa),
							Provision:   true,
						},
						SecretsProvider: SecretsProviderConfig{
							Type:        SecretsProviderTypeGcpKms,
							Credentials: string(gcpSa),
							ProjectId:   e2eTestProject,
							KeyName:     e2eKmsTestKeyName,
							KeyLocation: "global",
							Provision:   true,
						},
					},
				},
			},
			Resources: api.PerStackResourcesDescriptor{
				Resources: map[string]api.PerEnvResourcesDescriptor{
					"test": {
						Resources: map[string]api.ResourceDescriptor{
							"test-bucket": {
								Type: gcloud.ResourceTypeBucket,
								Config: api.Config{
									Config: &gcloud.GcpBucket{
										Credentials: string(gcpSa),
										ProjectId:   e2eTestProject,
										Name:        "e2e-create--test-bucket",
									},
								},
							},
						},
					},
				},
			},
		},
	}

	err = p.ProvisionStack(ctx, cfg, cryptor.PublicKey(), stack)

	Expect(err).To(BeNil())
}
