package pulumi

import (
	"github.com/simple-container-com/api/pkg/clouds/cloudflare"
	"github.com/simple-container-com/api/pkg/clouds/pulumi/testutil"
	"testing"

	"github.com/simple-container-com/api/pkg/clouds/gcloud"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/api"
)

const (
	e2eCreateStaticStackName    = "e2e-static-parent--stack"
	e2eDeployStaticStackName    = "e2e-static--stack"
	e2eStaticKmsTestKeyName     = "e2e-static--kms-key"
	e2eStaticKmsTestKeyringName = "e2e-static--kms-keyring"
)

func Test_CreateStaticStackGCP(t *testing.T) {
	RegisterTestingT(t)

	cfg := testutil.PrepareE2EtestForGCP()

	stack := api.Stack{
		Name: e2eCreateStaticStackName,
		Server: e2eServerDescriptorForGCP(e2eGCPConfig{
			credentials: *cfg.Credentials,
			e2eCommon: e2eCommon{
				kmsKeyName:     e2eStaticKmsTestKeyName,
				kmsKeyringName: e2eStaticKmsTestKeyringName,
				templates: map[string]api.StackDescriptor{
					"static-website": {
						Type: gcloud.TemplateTypeStaticWebsite,
						Config: api.Config{Config: &gcloud.TemplateConfig{
							Credentials: *cfg.Credentials,
						}},
					},
				},
				resources: map[string]api.PerEnvResourcesDescriptor{
					"test": {
						Template: "static-website",
					},
				},
				registrar: api.RegistrarDescriptor{
					Type: cloudflare.RegistrarType,
					Config: api.Config{
						Config: cfg.CloudflareConfig,
					},
				},
			},
		}),
		Client: api.ClientDescriptor{
			Stacks: map[string]api.StackClientDescriptor{
				"test": {
					Type:        api.ClientTypeStatic,
					ParentStack: e2eCreateStaticStackName,
					Environment: "test",
					Config: api.Config{
						Config: &api.StackConfigStatic{
							Domain:    "sc-e2e-static-gcp.simple-container.com",
							BundleDir: "testdata/static",
						},
					},
				},
			},
		},
	}

	runProvisionAndDeployTest(stack, cfg.E2ETestBasics, e2eDeployStaticStackName)
}
