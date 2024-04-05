package pulumi

import (
	"github.com/simple-container-com/api/pkg/clouds/aws"
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

	cfg := testutil.PrepareE2Etest()

	stack := api.Stack{
		Name: e2eCreateStaticStackName,
		Server: e2eServerDescriptorForGcp(e2eConfig{
			gcpCreds:       *cfg.GcpCredentials,
			kmsKeyName:     e2eStaticKmsTestKeyName,
			kmsKeyringName: e2eStaticKmsTestKeyringName,
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
							Domain:    "e2e-gcp-static-website.simple-container.com",
							BundleDir: "testdata/static",
						},
					},
				},
			},
		},
	}

	runProvisionAndDeployTest(stack, cfg, e2eDeployStaticStackName)
}

func Test_CreateStaticStackAWS(t *testing.T) {
	RegisterTestingT(t)

	cfg := testutil.PrepareE2Etest()

	stack := api.Stack{
		Name: e2eCreateStaticStackName,
		Server: e2eServerDescriptorForAws(e2eConfig{
			gcpCreds:       *cfg.GcpCredentials,
			awsCreds:       *cfg.AwsCredentials,
			kmsKeyName:     e2eStaticKmsTestKeyName,
			kmsKeyringName: e2eStaticKmsTestKeyringName,
			templates: map[string]api.StackDescriptor{
				"static-website": {
					Type: aws.TemplateTypeStaticWebsite,
					Config: api.Config{Config: &aws.TemplateConfig{
						AccountConfig: *cfg.AwsCredentials,
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
							BundleDir:          "testdata/static",
							Domain:             "e2e-aws-static-website.simple-container.com",
							IndexDocument:      "index.html",
							ErrorDocument:      "index.html",
							ProvisionWwwDomain: false,
						},
					},
				},
			},
		},
	}

	runProvisionAndDeployTest(stack, cfg, e2eDeployStaticStackName)
}
