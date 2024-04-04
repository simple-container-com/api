package pulumi

import (
	secretTestutil "github.com/simple-container-com/api/pkg/clouds/pulumi/testutil"
	"testing"

	"github.com/simple-container-com/api/pkg/clouds/cloudflare"

	"github.com/simple-container-com/api/pkg/clouds/gcloud"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/api"
)

const (
	e2eCreateStackName    = "e2e-create--stack"
	e2eDeployStackName    = "e2e-deploy--stack"
	e2eKmsTestKeyName     = "e2e-create--kms-key"
	e2eKmsTestKeyringName = "e2e-create--kms-keyring"
)

func Test_CreateComposeStackGCP(t *testing.T) {
	RegisterTestingT(t)

	cfg := secretTestutil.PrepareE2EtestForGCP()
	stack := api.Stack{
		Name: e2eCreateStackName,
		Server: e2eServerDescriptorForGCP(e2eGCPConfig{
			credentials: *cfg.Credentials,
			e2eCommon: e2eCommon{
				kmsKeyName:     e2eKmsTestKeyName,
				kmsKeyringName: e2eKmsTestKeyringName,
				templates: map[string]api.StackDescriptor{
					"stack-per-app": {
						Type: gcloud.TemplateTypeGcpCloudrun,
						Config: api.Config{Config: &gcloud.TemplateConfig{
							Credentials: *cfg.Credentials,
						}},
					},
				},
				registrar: api.RegistrarDescriptor{
					Type: cloudflare.RegistrarType,
					Config: api.Config{
						Config: cfg.CloudflareConfig,
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
										Credentials: *cfg.Credentials,
										Name:        "e2e-create--test-bucket",
									},
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

	runProvisionAndDeployTest(stack, cfg.E2ETestBasics, e2eDeployStackName)
}
