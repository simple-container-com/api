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
	testSAFile            = "pkg/clouds/pulumi/testdata/sc-test-project-sa.json"
	e2eTestProject        = "sc-test-project-408205"
	e2eCreateStackName    = "e2e-create--stack"
	e2eDeployStackName    = "e2e-deploy--stack"
	e2eKmsTestKeyName     = "e2e-create--kms-key"
	e2eKmsTestKeyringName = "e2e-create--kms-keyring"
	e2eBucketName         = "sc-pulumi-test"
)

func Test_CreateStack(t *testing.T) {
	RegisterTestingT(t)

	ctx := context.Background()

	cfg, cryptor := secretTestutil.ReadIntegrationTestConfig(t, testSAFile)
	gcpSa, err := cryptor.GetAndDecryptFileContent(testSAFile)
	Expect(err).To(BeNil())

	stack := api.Stack{
		Name: e2eCreateStackName,
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
								Credentials: gcloud.Credentials{
									Credentials: api.Credentials{
										Credentials: string(gcpSa),
									},
									ServiceAccountConfig: gcloud.ServiceAccountConfig{
										ProjectId: e2eTestProject,
									},
								},
							}},
						},
						SecretsProvider: SecretsProviderConfig{
							Type: SecretsProviderTypeGcpKms,
							Config: api.Config{Config: &gcloud.SecretsProviderConfig{
								KeyName:     e2eKmsTestKeyName,
								KeyLocation: "global",
								KeyRingName: e2eKmsTestKeyringName,
								Provision:   true,
								Credentials: gcloud.Credentials{
									Credentials: api.Credentials{
										Credentials: string(gcpSa),
									},
									ServiceAccountConfig: gcloud.ServiceAccountConfig{
										ProjectId: e2eTestProject,
									},
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
						Credentials: gcloud.Credentials{
							Credentials: api.Credentials{
								Credentials: string(gcpSa),
							},
							ServiceAccountConfig: gcloud.ServiceAccountConfig{
								ProjectId: e2eTestProject,
							},
						},
					}},
				},
			},
			Resources: api.PerStackResourcesDescriptor{
				Resources: map[string]api.PerEnvResourcesDescriptor{
					"test": {
						Template: "stack-per-app",
						Resources: map[string]api.ResourceDescriptor{
							"test-bucket": {
								Type: gcloud.ResourceTypeBucket,
								Config: api.Config{
									Config: &gcloud.GcpBucket{
										Credentials: gcloud.Credentials{
											Credentials: api.Credentials{
												Credentials: string(gcpSa),
											},
											ServiceAccountConfig: gcloud.ServiceAccountConfig{
												ProjectId: e2eTestProject,
											},
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
				"test": {
					Stack:       e2eCreateStackName,
					Environment: "test",
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

	createProv, err := InitPulumiProvisioner(stack.Server.Provisioner.Config)
	Expect(err).To(BeNil())

	createProv.SetPublicKey(cryptor.PublicKey())

	err = createProv.ProvisionStack(ctx, cfg, stack)
	Expect(err).To(BeNil())

	deployProv, err := InitPulumiProvisioner(stack.Server.Provisioner.Config)
	Expect(err).To(BeNil())

	deployProv.SetPublicKey(cryptor.PublicKey())

	err = deployProv.DeployStack(ctx, cfg, stack, api.DeployParams{
		Stack:       e2eDeployStackName,
		Environment: "test",
	})
	Expect(err).To(BeNil())
}
