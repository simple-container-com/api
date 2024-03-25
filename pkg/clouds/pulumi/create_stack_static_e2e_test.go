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
	e2eCreateStaticStackName    = "e2e-static-parent--stack"
	e2eDeployStaticStackName    = "e2e-static--stack"
	e2eStaticKmsTestKeyName     = "e2e-static--kms-key"
	e2eStaticKmsTestKeyringName = "e2e-static--kms-keyring"
)

func Test_CreateStaticStack(t *testing.T) {
	RegisterTestingT(t)

	ctx := context.Background()

	cfg, cryptor, gcpSa := secretTestutil.PrepareE2EtestForGCP()

	stack := api.Stack{
		Name: e2eCreateStaticStackName,
		Server: testServerDescriptorForGCP(e2eTestConfigGCP{
			gcpSa:          gcpSa,
			kmsKeyName:     e2eStaticKmsTestKeyName,
			kmsKeyringName: e2eStaticKmsTestKeyringName,
			templates: map[string]api.StackDescriptor{
				"static-website": {
					Type: gcloud.TemplateTypeStaticWebsite,
					Config: api.Config{Config: &gcloud.TemplateConfig{
						Credentials: gcloud.Credentials{
							Credentials: api.Credentials{
								Credentials: gcpSa,
							},
							ServiceAccountConfig: gcloud.ServiceAccountConfig{
								ProjectId: e2eTestProject,
							},
						},
					}},
				},
			},
			resources: map[string]api.PerEnvResourcesDescriptor{
				"test": {
					Template: "static-website",
				},
			},
		}),
		Client: api.ClientDescriptor{
			Stacks: map[string]api.StackClientDescriptor{
				"test": {
					Type:        api.ClientTypeStatic,
					ParentStack: e2eCreateStaticStackName,
					Environment: "test",
					Domain:      "refapp.sc-app.me",
					Config: api.Config{
						Config: &api.StackConfigStatic{
							BundleDir: "testdata/static",
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
		StackName:   e2eDeployStaticStackName,
		ParentStack: e2eCreateStaticStackName,
		Environment: "test",
	})
	Expect(err).To(BeNil())
}
