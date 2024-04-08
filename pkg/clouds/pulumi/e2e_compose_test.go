//go:build e2e

package pulumi

import (
	"fmt"
	"testing"

	"github.com/simple-container-com/api/pkg/clouds/aws"
	secretTestutil "github.com/simple-container-com/api/pkg/clouds/pulumi/testutil"

	"github.com/simple-container-com/api/pkg/clouds/cloudflare"

	"github.com/simple-container-com/api/pkg/clouds/gcloud"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/api"
)

const (
	e2eCreateStackName    = "e2e-parent--stack"
	e2eDeployStackName    = "e2e-child--stack"
	e2eKmsTestKeyName     = "e2e--kms-key"
	e2eKmsTestKeyringName = "e2e--kms-keyring"
)

func Test_CreateComposeStackGCP(t *testing.T) {
	RegisterTestingT(t)

	cfg := secretTestutil.PrepareE2Etest()
	parentStackName := tmpResName(e2eCreateStackName)
	childStackName := tmpResName(e2eDeployStackName)
	stack := api.Stack{
		Name: parentStackName,
		Server: e2eServerDescriptorForGcp(e2eConfig{
			gcpCreds:       *cfg.GcpCredentials,
			kmsKeyName:     tmpResName(e2eKmsTestKeyName),
			kmsKeyringName: tmpResName(e2eKmsTestKeyringName),
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
									Name:        tmpResName("e2e-create--test-bucket"),
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
					ParentStack: parentStackName,
					Environment: "test",
					Config: api.Config{
						Config: &api.StackConfigCompose{
							Domain:            fmt.Sprintf("e2e--gcp--%s.simple-container.com", tmpResName("cloudrun")),
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

	runProvisionAndDeployTest(stack, cfg, childStackName)
	runDestroyTest(stack, cfg, childStackName)
}

func Test_CreateComposeStackAWS(t *testing.T) {
	RegisterTestingT(t)

	parentStackName := tmpResName(e2eCreateStackName)
	childStackName := tmpResName(e2eDeployStackName)

	cfg := secretTestutil.PrepareE2Etest()
	stack := api.Stack{
		Name: parentStackName,
		Server: e2eServerDescriptorForAws(e2eConfig{
			awsCreds:       *cfg.AwsCredentials,
			kmsKeyName:     tmpResName(e2eKmsTestKeyName),
			kmsKeyringName: tmpResName(e2eKmsTestKeyringName),
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
									Name:          tmpResName("e2e--create--test-bucket"),
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
					ParentStack: parentStackName,
					Environment: "test",
					Config: api.Config{
						Config: &api.StackConfigCompose{
							Domain:            fmt.Sprintf("e2e--aws--%s.simple-container.com", tmpResName("ecs-fargate")),
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

	runProvisionAndDeployTest(stack, cfg, childStackName)
	runDestroyTest(stack, cfg, childStackName)
}
