package pulumi

import (
	"context"
	"testing"

	"github.com/simple-container-com/api/pkg/clouds/gcloud"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/api"

	secretTestutil "github.com/simple-container-com/api/pkg/api/secrets/testutil"
)

const (
	testSAFile        = "pkg/clouds/pulumi/testdata/sc-test-project-sa.json"
	e2eTestProject    = "sc-test-project-408205"
	e2eStackName      = "e2e-create--stack"
	e2eKmsTestKeyName = "e2e-create--kms-key"
	e2eBucketName     = "sc-pulumi-test"
)

func Test_CreateStack(t *testing.T) {
	RegisterTestingT(t)

	ctx := context.Background()

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
							Type: StateStorageTypeGcpBucket,
							Config: api.Config{Config: &gcloud.StateStorageConfig{
								Provision:  false,
								BucketName: e2eBucketName,
								Credentials: api.Credentials{
									Credentials: string(gcpSa),
								},
								ServiceAccountConfig: gcloud.ServiceAccountConfig{
									ProjectId: e2eTestProject,
								},
							}},
						},
						SecretsProvider: SecretsProviderConfig{
							Type: SecretsProviderTypeGcpKms,
							Config: api.Config{Config: &gcloud.SecretsProviderConfig{
								KeyName:     e2eKmsTestKeyName,
								KeyLocation: "global",
								Provision:   true,
								Credentials: api.Credentials{
									Credentials: string(gcpSa),
								},
								ServiceAccountConfig: gcloud.ServiceAccountConfig{
									ProjectId: e2eTestProject,
								},
							}},
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

	p, err := InitPulumiProvisioner(stack.Server.Provisioner.Config)
	Expect(err).To(BeNil())

	err = p.ProvisionStack(ctx, cfg, cryptor.PublicKey(), stack)

	Expect(err).To(BeNil())
}
