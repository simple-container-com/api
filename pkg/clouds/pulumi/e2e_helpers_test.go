package pulumi

import (
	"context"
	"github.com/simple-container-com/api/pkg/clouds/aws"
	secretTestutil "github.com/simple-container-com/api/pkg/clouds/pulumi/testutil"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/gcloud"
)

const (
	e2eBucketNameGCP = "sc-pulumi-test"
	e2eBucketNameAWS = "sc-pulumi-test-aws"
)

type e2eConfig struct {
	kmsKeyName     string
	kmsKeyringName string
	templates      map[string]api.StackDescriptor
	resources      map[string]api.PerEnvResourcesDescriptor
	registrar      api.RegistrarDescriptor
	gcpCreds       gcloud.Credentials
	awsCreds       aws.AccountConfig
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
							BucketName:  e2eBucketNameGCP,
							Credentials: config.gcpCreds,
						}},
					},
					SecretsProvider: SecretsProviderConfig{
						Type: SecretsProviderTypeGcpKms,
						Config: api.Config{Config: &gcloud.SecretsProviderConfig{
							KeyName:     config.kmsKeyName,
							KeyLocation: "global",
							KeyRingName: config.kmsKeyringName,
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
							BucketName:    e2eBucketNameAWS,
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

	createProv, err := InitPulumiProvisioner(stack.Server.Provisioner.Config)
	Expect(err).To(BeNil())

	createProv.SetPublicKey(cfg.Cryptor.PublicKey())

	err = createProv.ProvisionStack(ctx, cfg.ConfigFile, stack)
	Expect(err).To(BeNil())

	deployProv, err := InitPulumiProvisioner(stack.Server.Provisioner.Config)
	Expect(err).To(BeNil())

	deployProv.SetPublicKey(cfg.Cryptor.PublicKey())

	err = deployProv.DeployStack(ctx, cfg.ConfigFile, stack, api.DeployParams{
		StackName:   deployStackName,
		ParentStack: stack.Name,
		RootDir:     cfg.RootDir,
		Environment: "test",
	})
	Expect(err).To(BeNil())
}
