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
			Templates: map[string]api.StackDescriptor{
				"stack-per-app": {
					Type: gcloud.TemplateTypeGcpCloudrun,
					Config: api.Config{Config: &gcloud.TemplateConfig{
						Credentials: api.Credentials{
							Credentials: string(gcpSa),
						},
						ServiceAccountConfig: gcloud.ServiceAccountConfig{
							ProjectId: e2eTestProject,
						},
					}},
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
										Credentials: api.Credentials{
											Credentials: string(gcpSa),
										},
										ServiceAccountConfig: gcloud.ServiceAccountConfig{
											ProjectId: e2eTestProject,
										},
										Name: "e2e-create--test-bucket",
									},
								},
							},
						},
					},
				},
			},
		},
		Client: api.ClientDescriptor{
			Stacks: map[string]api.StackClientDescriptor{
				"staging": {
					Stack:       e2eStackName,
					Environment: "staging",
					Domain:      "refapp.sc-app.me",
					Config: api.StackConfig{
						DockerComposeFile: "testdata/docker-compose.yaml",
						Uses: []string{
							"test-bucket",
						},
						Runs: []string{
							"backend",
						},
					},
				},
			},
		},
	}

	p, err := InitPulumiProvisioner(stack.Server.Provisioner.Config)
	Expect(err).To(BeNil())

	p.SetPublicKey(cryptor.PublicKey())

	err = p.ProvisionStack(ctx, cfg, stack)
	Expect(err).To(BeNil())

	err = p.DeployStack(ctx, cfg, stack, api.DeployParams{
		Stack:       e2eStackName,
		Environment: "staging",
	})
	Expect(err).To(BeNil())
}
