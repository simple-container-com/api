package pulumi

import (
	"testing"

	"github.com/simple-container-com/api/pkg/clouds/aws"
	secretTestutil "github.com/simple-container-com/api/pkg/clouds/pulumi/testutil"

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

	cfg := secretTestutil.PrepareE2Etest()
	stack := api.Stack{
		Name: e2eCreateStackName,
		Server: e2eServerDescriptorForGcp(e2eConfig{
			gcpCreds:       *cfg.GcpCredentials,
			kmsKeyName:     e2eKmsTestKeyName,
			kmsKeyringName: e2eKmsTestKeyringName,
			templates: map[string]api.StackDescriptor{
				"stack-per-app": {
					Type: gcloud.TemplateTypeGcpCloudrun,
					Config: api.Config{Config: &gcloud.TemplateConfig{
						Credentials: *cfg.GcpCredentials,
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
									Credentials: *cfg.GcpCredentials,
									Name:        "e2e-create--test-bucket",
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
					Type:        api.ClientTypeCloudCompose,
					ParentStack: e2eCreateStackName,
					Environment: "test",
					Config: api.Config{
						Config: &api.StackConfigCompose{
							Domain:            "refapp.sc-app.me",
							DockerComposeFile: "docker-compose.yaml",
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

	runProvisionAndDeployTest(stack, cfg, e2eDeployStackName)
}

func Test_CreateComposeStackAWS(t *testing.T) {
	RegisterTestingT(t)

	cfg := secretTestutil.PrepareE2Etest()
	stack := api.Stack{
		Name: e2eCreateStackName,
		Server: e2eServerDescriptorForAws(e2eConfig{
			awsCreds:       *cfg.AwsCredentials,
			kmsKeyName:     e2eKmsTestKeyName,
			kmsKeyringName: e2eKmsTestKeyringName,
			templates: map[string]api.StackDescriptor{
				"stack-per-app": {
					Type: aws.TemplateTypeEcsFargate,
					Config: api.Config{Config: &aws.TemplateConfig{
						AccountConfig: *cfg.AwsCredentials,
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
							Type: aws.ResourceTypeS3Bucket,
							Config: api.Config{
								Config: &aws.S3Bucket{
									AccountConfig: *cfg.AwsCredentials,
									Name:          "e2e--create--test-bucket",
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
					Type:        api.ClientTypeCloudCompose,
					ParentStack: e2eCreateStackName,
					Environment: "test",
					Config: api.Config{
						Config: &api.StackConfigCompose{
							Domain:            "e2e--aws-ecs-fargate.simple-container.com",
							DockerComposeFile: "docker-compose.yaml",
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

	//runDestroyParentTest(stack, cfg, e2eCreateStackName)
	//runDestroyChildTest(stack, cfg, e2eDeployStackName)
	runProvisionAndDeployTest(stack, cfg, e2eDeployStackName)
}
