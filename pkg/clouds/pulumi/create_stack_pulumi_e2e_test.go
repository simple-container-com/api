package pulumi

import (
	"testing"

	"github.com/simple-container-com/api/pkg/clouds/cloudflare"
	"github.com/simple-container-com/api/pkg/clouds/pulumi/testutil"

	"github.com/simple-container-com/api/pkg/clouds/gcloud"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/api"
)

const (
	e2ePulumiStaticParentStackName = "e2e-pulumi--static-parent--stack"
	e2ePulumiStaticStackName       = "e2e-pulumi--static--stack"
)

func Test_CreatePulumiParentStack(t *testing.T) {
	RegisterTestingT(t)

	cfg := testutil.PrepareE2Etest()

	pulumiCreds := testutil.ReadTestSecretConfig(cfg.Cryptor, "pkg/clouds/pulumi/testdata/secrets/pulumi-e2e-config.yaml", &TokenAuthDescriptor{})

	stack := api.Stack{
		Name: e2ePulumiStaticParentStackName,
		Server: e2eServerDescriptorForPulumi(e2eConfig{
			pulumiCreds: *pulumiCreds,
			templates: map[string]api.StackDescriptor{
				"static-website": {
					Type: gcloud.TemplateTypeStaticWebsite,
					Config: api.Config{Config: &gcloud.TemplateConfig{
						Credentials: *cfg.GcpCredentials,
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
		}),
		Client: api.ClientDescriptor{
			Stacks: map[string]api.StackClientDescriptor{
				"test": {
					Type:        api.ClientTypeStatic,
					ParentStack: e2eCreateStaticStackName,
					Environment: "test",
					Config: api.Config{
						Config: &api.StackConfigStatic{
							Domain:    "e2e--pulumi--static-website.simple-container.com",
							BundleDir: "testdata/static",
						},
					},
				},
			},
		},
	}

	runProvisionAndDeployTest(stack, cfg, e2ePulumiStaticStackName)

}
