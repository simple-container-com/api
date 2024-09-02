//go:build e2e

package pulumi

import (
	"fmt"
	"testing"

	"github.com/simple-container-com/api/pkg/clouds/cloudflare"
	"github.com/simple-container-com/api/pkg/clouds/pulumi/testutil"

	"github.com/simple-container-com/api/pkg/clouds/gcloud"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/api"
)

const (
	e2ePulumiStaticParentStackName = "e2e-pulumi--static-parent--stack"
	e2ePulumiStaticChildStackName  = "e2e-pulumi--static-child--stack"
)

func Test_CreatePulumiParentStack(t *testing.T) {
	RegisterTestingT(t)

	cfg := testutil.PrepareE2Etest()

	pulumiCreds := testutil.ReadTestSecretConfig(cfg.Cryptor, "pkg/clouds/pulumi/testdata/secrets/pulumi-e2e-config.yaml", &TokenAuthDescriptor{})

	parentStackName := tmpResName(e2ePulumiStaticParentStackName)
	childStackName := tmpResName(e2ePulumiStaticChildStackName)

	stack := api.Stack{
		Name: parentStackName,
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
					ParentStack: e2eStaticParentStackName,
					Config: api.Config{
						Config: &api.StackConfigStatic{
							Site: api.StaticSiteConfig{
								Domain: fmt.Sprintf("e2e--pulumi--%s.simple-container.com", tmpResName("static-website")),
							},
							BundleDir: "static",
						},
					},
				},
			},
		},
	}

	runProvisionAndDeployTest(stack, cfg, childStackName)
	runDestroyTest(stack, cfg, childStackName)
}
