package pulumi

import (
	"context"
	"testing"

	"github.com/simple-container-com/api/pkg/clouds/cloudflare"

	"github.com/simple-container-com/api/pkg/clouds/gcloud"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/api"

	secretTestutil "github.com/simple-container-com/api/pkg/api/secrets/testutil"
)

const (
	e2eCreateStackName    = "e2e-create--stack"
	e2eDeployStackName    = "e2e-deploy--stack"
	e2eKmsTestKeyName     = "e2e-create--kms-key"
	e2eKmsTestKeyringName = "e2e-create--kms-keyring"
)

func Test_CreateComposeStack(t *testing.T) {
	RegisterTestingT(t)

	ctx := context.Background()

	cfg, cryptor, gcpCfg := secretTestutil.PrepareE2EtestForGCP()

	stack := api.Stack{
		Name: e2eCreateStackName,
		Server: testServerDescriptorForGCP(e2eTestConfigGCP{
			gcpSa:          gcpCfg.ServiceAccount,
			kmsKeyName:     e2eKmsTestKeyName,
			kmsKeyringName: e2eKmsTestKeyringName,
			templates: map[string]api.StackDescriptor{
				"stack-per-app": {
					Type: gcloud.TemplateTypeGcpCloudrun,
					Config: api.Config{Config: &gcloud.TemplateConfig{
						Credentials: gcloud.Credentials{
							Credentials: api.Credentials{
								Credentials: gcpCfg.ServiceAccount,
							},
							ServiceAccountConfig: gcloud.ServiceAccountConfig{
								ProjectId: e2eTestProject,
							},
						},
					}},
				},
			},
			registrar: api.RegistrarDescriptor{
				Type: cloudflare.RegistrarType,
				Config: api.Config{
					Config: gcpCfg.CloudflareConfig,
				},
			},
			resources: map[string]api.PerEnvResourcesDescriptor{
				"test": {
					Template: "stack-per-app",
					Resources: map[string]api.ResourceDescriptor{
						"test-bucket": {
							Type: gcloud.ResourceTypeBucket,
							Config: api.Config{
								Config: &gcloud.GcpBucket{
									Credentials: gcloud.Credentials{
										Credentials: api.Credentials{
											Credentials: gcpCfg.ServiceAccount,
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
		}),
		Client: api.ClientDescriptor{
			Stacks: map[string]api.StackClientDescriptor{
				"test": {
					Type:        api.ClientTypeCompose,
					ParentStack: e2eCreateStackName,
					Environment: "test",
					Config: api.Config{
						Config: &api.StackConfigCompose{
							Domain:            "refapp.sc-app.me",
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
		StackName:   e2eDeployStackName,
		ParentStack: e2eCreateStackName,
		Environment: "test",
	})
	Expect(err).To(BeNil())
}
