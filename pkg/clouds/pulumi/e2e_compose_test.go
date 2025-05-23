//go:build e2e

package pulumi

import (
	"fmt"
	"testing"

	"github.com/simple-container-com/api/pkg/clouds/mongodb"

	"github.com/simple-container-com/api/pkg/clouds/aws"
	secretTestutil "github.com/simple-container-com/api/pkg/clouds/pulumi/testutil"

	"github.com/simple-container-com/api/pkg/clouds/cloudflare"

	"github.com/simple-container-com/api/pkg/clouds/gcloud"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/api"
)

const (
	e2eCreateStackName = "e2e-parent--stack"
	e2eDeployStackName = "e2e-child--stack"
	e2eKmsTestKeyName  = "e2e--kms-key"
)

func Test_CreateComposeStackGCPWithBucket(t *testing.T) {
	RegisterTestingT(t)

	cfg := secretTestutil.PrepareE2Etest()
	parentStackName := tmpResName(e2eCreateStackName)
	childStackName := tmpResName(e2eDeployStackName)
	stack := api.Stack{
		Name: parentStackName,
		Server: e2eServerDescriptorForGcp(e2eConfig{
			gcpCreds:   *cfg.GcpCredentials,
			kmsKeyName: tmpResName(e2eKmsTestKeyName),
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
					Config: api.Config{
						Config: &api.StackConfigCompose{
							Domain:            fmt.Sprintf("e2e--gcp--%s-bucket.simple-container.com", tmpResName("cloudrun")),
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

func Test_CreateComposeStackGCPWithMongo(t *testing.T) {
	RegisterTestingT(t)

	parentStackName := tmpResName(e2eCreateStackName)
	childStackName := tmpResName(e2eDeployStackName)

	cfg := secretTestutil.PrepareE2Etest()
	stack := api.Stack{
		Name: parentStackName,
		Server: e2eServerDescriptorForGcp(e2eConfig{
			gcpCreds:   *cfg.GcpCredentials,
			kmsKeyName: tmpResName(e2eKmsTestKeyName),
			templates: map[string]api.StackDescriptor{
				"stack-per-app": {
					Type: gcloud.TemplateTypeGcpCloudrun,
					Config: api.Config{Config: &gcloud.TemplateConfig{
						Credentials: *cfg.GcpCredentials,
					}},
				},
			},
			resources: map[string]api.PerEnvResourcesDescriptor{
				"test": {
					Template: "stack-per-app",
					Resources: map[string]api.ResourceDescriptor{
						"mongodb": {
							Type: mongodb.ResourceTypeMongodbAtlas,
							Config: api.Config{
								Config: cfg.MongoConfig,
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
					Config: api.Config{
						Config: &api.StackConfigCompose{
							Domain:            fmt.Sprintf("e2e--gcp--%s-mongo.simple-container.com", tmpResName("cloudrun")),
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

func Test_CreateComposeStackAWSWithMongo(t *testing.T) {
	RegisterTestingT(t)

	parentStackName := tmpResName(e2eCreateStackName)
	childStackName := tmpResName(e2eDeployStackName)

	cfg := secretTestutil.PrepareE2Etest()
	stack := api.Stack{
		Name: parentStackName,
		Server: e2eServerDescriptorForAws(e2eConfig{
			awsCreds:   *cfg.AwsCredentials,
			kmsKeyName: tmpResName(e2eKmsTestKeyName),
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
						"mongodb": {
							Type: mongodb.ResourceTypeMongodbAtlas,
							Config: api.Config{
								Config: cfg.MongoConfig,
							},
						},
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
					Config: api.Config{
						Config: &api.StackConfigCompose{
							Domain:            fmt.Sprintf("e2e--aws--%s-mongo.simple-container.com", tmpResName("ecs-fargate")),
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

func Test_CreateComposeStackAWSWithBucket(t *testing.T) {
	RegisterTestingT(t)

	parentStackName := tmpResName(e2eCreateStackName)
	childStackName := tmpResName(e2eDeployStackName)

	cfg := secretTestutil.PrepareE2Etest()
	stack := api.Stack{
		Name: parentStackName,
		Server: e2eServerDescriptorForAws(e2eConfig{
			awsCreds:   *cfg.AwsCredentials,
			kmsKeyName: tmpResName(e2eKmsTestKeyName),
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
					Config: api.Config{
						Config: &api.StackConfigCompose{
							Domain:            fmt.Sprintf("e2e--aws--%s-bucket.simple-container.com", tmpResName("ecs-fargate")),
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
