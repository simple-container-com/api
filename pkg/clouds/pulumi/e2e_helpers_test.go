package pulumi

import (
	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/gcloud"
)

const (
	e2eTestProject = "sc-test-project-408205"
	e2eBucketName  = "sc-pulumi-test"
)

type e2eTestConfigGCP struct {
	gcpSa          string
	kmsKeyName     string
	kmsKeyringName string
	templates      map[string]api.StackDescriptor
	resources      map[string]api.PerEnvResourcesDescriptor
}

func testServerDescriptorForGCP(config e2eTestConfigGCP) api.ServerDescriptor {
	return api.ServerDescriptor{
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
									Credentials: config.gcpSa,
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
							KeyName:     config.kmsKeyName,
							KeyLocation: "global",
							KeyRingName: config.kmsKeyringName,
							Provision:   true,
							Credentials: gcloud.Credentials{
								Credentials: api.Credentials{
									Credentials: config.gcpSa,
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
		Templates: config.templates,
		Resources: api.PerStackResourcesDescriptor{
			Resources: config.resources,
		},
	}
}
