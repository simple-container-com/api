package pulumi

import (
	"context"
	secretTestutil "github.com/simple-container-com/api/pkg/clouds/pulumi/testutil"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/gcloud"
)

const (
	e2eBucketName = "sc-pulumi-test"
)

type e2eCommon struct {
	kmsKeyName     string
	kmsKeyringName string
	templates      map[string]api.StackDescriptor
	resources      map[string]api.PerEnvResourcesDescriptor
	registrar      api.RegistrarDescriptor
}

type e2eGCPConfig struct {
	e2eCommon
	credentials gcloud.Credentials
}

func e2eServerDescriptorForGCP(config e2eGCPConfig) api.ServerDescriptor {
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
							Credentials: config.credentials,
						}},
					},
					SecretsProvider: SecretsProviderConfig{
						Type: SecretsProviderTypeGcpKms,
						Config: api.Config{Config: &gcloud.SecretsProviderConfig{
							KeyName:     config.kmsKeyName,
							KeyLocation: "global",
							KeyRingName: config.kmsKeyringName,
							Provision:   true,
							Credentials: config.credentials,
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

func runProvisionAndDeployTest(stack api.Stack, cfg secretTestutil.E2ETestBasics, deployStackName string) {
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
		Environment: "test",
	})
	Expect(err).To(BeNil())
}
