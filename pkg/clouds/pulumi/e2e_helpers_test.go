package pulumi

import (
	"context"
	"fmt"

	. "github.com/onsi/gomega"
	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/aws"
	"github.com/simple-container-com/api/pkg/clouds/gcloud"
	secretTestutil "github.com/simple-container-com/api/pkg/clouds/pulumi/testutil"
)

const (
	e2eBucketName = "sc-pulumi-test"
)

type e2eConfig struct {
	kmsKeyName  string
	templates   map[string]api.StackDescriptor
	resources   map[string]api.PerEnvResourcesDescriptor
	registrar   api.RegistrarDescriptor
	gcpCreds    gcloud.Credentials
	awsCreds    aws.AccountConfig
	pulumiCreds TokenAuthDescriptor
}

func e2eServerDescriptorForPulumi(config e2eConfig) api.ServerDescriptor {
	return api.ServerDescriptor{
		Provisioner: api.ProvisionerDescriptor{
			Type: ProvisionerTypePulumi,
			Config: api.Config{
				Config: &ProvisionerConfig{
					Organization: "simple-container-com",
					StateStorage: StateStorageConfig{
						Type:   BackendTypePulumiCloud,
						Config: api.Config{Config: &config.pulumiCreds},
					},
					SecretsProvider: SecretsProviderConfig{
						Type:   BackendTypePulumiCloud,
						Config: api.Config{Config: &config.pulumiCreds},
					},
				},
			},
		},
		Templates: config.templates,
		Resources: api.PerStackResourcesDescriptor{
			Resources: config.resources,
			Registrar: config.registrar,
		},
	}
}

func e2eServerDescriptorForGcp(config e2eConfig) api.ServerDescriptor {
	return api.ServerDescriptor{
		Provisioner: api.ProvisionerDescriptor{
			Type: ProvisionerTypePulumi,
			Config: api.Config{
				Config: &ProvisionerConfig{
					Organization: "organization",
					StateStorage: StateStorageConfig{
						Type: StateStorageTypeGcpBucket,
						Config: api.Config{Config: &gcloud.StateStorageConfig{
							Provision:   false,
							BucketName:  e2eBucketName,
							Credentials: config.gcpCreds,
						}},
					},
					SecretsProvider: SecretsProviderConfig{
						Type: SecretsProviderTypeGcpKms,
						Config: api.Config{Config: &gcloud.SecretsProviderConfig{
							KeyName:     config.kmsKeyName,
							KeyLocation: "global",
							Provision:   true,
							Credentials: config.gcpCreds,
						}},
					},
				},
			},
		},
		Templates: config.templates,
		Resources: api.PerStackResourcesDescriptor{
			Resources: config.resources,
			Registrar: config.registrar,
		},
	}
}

func e2eServerDescriptorForAws(config e2eConfig) api.ServerDescriptor {
	return api.ServerDescriptor{
		Provisioner: api.ProvisionerDescriptor{
			Type: ProvisionerTypePulumi,
			Config: api.Config{
				Config: &ProvisionerConfig{
					Organization: "organization",
					StateStorage: StateStorageConfig{
						Type: StateStorageTypeS3Bucket,
						Config: api.Config{Config: &aws.StateStorageConfig{
							Provision:     false,
							BucketName:    e2eBucketName,
							AccountConfig: config.awsCreds,
						}},
					},
					SecretsProvider: SecretsProviderConfig{
						Type: SecretsProviderTypeAwsKms,
						Config: api.Config{Config: &aws.SecretsProviderConfig{
							AccountConfig: config.awsCreds,
							Provision:     true,
							KeyName:       config.kmsKeyName,
						}},
					},
				},
			},
		},
		Templates: config.templates,
		Resources: api.PerStackResourcesDescriptor{
			Resources: config.resources,
			Registrar: config.registrar,
		},
	}
}

func runProvisionAndDeployTest(stack api.Stack, cfg secretTestutil.E2ETestConfig, deployStackName string) {
	ctx := context.Background()

	runProvisionTest(stack, cfg)

	deployProv, err := InitPulumiProvisioner(stack.Server.Provisioner.Config)
	Expect(err).To(BeNil())

	deployProv.SetPublicKey(cfg.Cryptor.PublicKey())

	err = deployProv.DeployStack(ctx, cfg.ConfigFile, stack, api.DeployParams{
		StackParams: api.StackParams{
			StackDir:    cfg.StacksDir,
			StackName:   deployStackName,
			Environment: "test",
		},
	})
	Expect(err).To(BeNil())
}

func runProvisionTest(stack api.Stack, cfg secretTestutil.E2ETestConfig) {
	ctx := context.Background()

	createProv, err := InitPulumiProvisioner(stack.Server.Provisioner.Config)
	Expect(err).To(BeNil())

	createProv.SetPublicKey(cfg.Cryptor.PublicKey())

	err = createProv.ProvisionStack(ctx, cfg.ConfigFile, stack, api.ProvisionParams{})
	Expect(err).To(BeNil())
}

func runDestroyTest(stack api.Stack, cfg secretTestutil.E2ETestConfig, deployStackName string) {
	runDestroyChildTest(stack, cfg, deployStackName)
	runDestroyParentTest(stack, cfg)
}

func runDestroyChildTest(stack api.Stack, cfg secretTestutil.E2ETestConfig, deployStackName string) {
	ctx := context.Background()

	destroyProv, err := InitPulumiProvisioner(stack.Server.Provisioner.Config)
	Expect(err).To(BeNil())

	destroyProv.SetPublicKey(cfg.Cryptor.PublicKey())

	err = destroyProv.DestroyChildStack(ctx, cfg.ConfigFile, stack, api.DestroyParams{
		StackParams: api.StackParams{
			StackName:   deployStackName,
			StacksDir:   cfg.StacksDir,
			Environment: "test",
		},
	})
	Expect(err).To(BeNil())
}

func runDestroyParentTest(stack api.Stack, cfg secretTestutil.E2ETestConfig) {
	ctx := context.Background()

	destroyProv, err := InitPulumiProvisioner(stack.Server.Provisioner.Config)
	Expect(err).To(BeNil())

	destroyProv.SetPublicKey(cfg.Cryptor.PublicKey())

	err = destroyProv.DestroyParentStack(ctx, cfg.ConfigFile, stack, api.DestroyParams{})
	Expect(err).To(BeNil())
}

func tmpResName(name string) string {
	return fmt.Sprintf("%s-%d", name, 1712558595)
	// return fmt.Sprintf("%s-%d", name, time.Now().Unix())
}
