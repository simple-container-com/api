//go:build e2e

package pulumi

import (
	"fmt"
	"testing"

	"github.com/simple-container-com/api/pkg/clouds/aws"
	"github.com/simple-container-com/api/pkg/clouds/cloudflare"
	"github.com/simple-container-com/api/pkg/clouds/pulumi/testutil"

	"github.com/simple-container-com/api/pkg/clouds/gcloud"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/api"
)

const (
	e2eStaticParentStackName    = "e2e-static-parent--stack"
	e2eStaticChildStackName     = "e2e-static-child--stack"
	e2eStaticKmsTestKeyName     = "e2e-static--kms-key"
	e2eStaticKmsTestKeyringName = "e2e-static--kms-keyring"
)

func Test_CreateStaticStackGCP(t *testing.T) {
	RegisterTestingT(t)

	cfg := testutil.PrepareE2Etest()

	parentStackName := tmpResName(e2ePulumiStaticParentStackName)
	childStackName := tmpResName(e2eStaticChildStackName)

	stack := api.Stack{
		Name: parentStackName,
		Server: e2eServerDescriptorForGcp(e2eConfig{
			gcpCreds:       *cfg.GcpCredentials,
			kmsKeyName:     tmpResName(e2eStaticKmsTestKeyName),
			kmsKeyringName: tmpResName(e2eStaticKmsTestKeyringName),
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
					ParentStack: parentStackName,
					Environment: "test",
					Config: api.Config{
						Config: &api.StackConfigStatic{
							Domain:    fmt.Sprintf("e2e--gcp--%s.simple-container.com", tmpResName("static-website")),
							BundleDir: "testdata/static",
						},
					},
				},
			},
		},
	}

	runProvisionAndDeployTest(stack, cfg, childStackName)
	runDestroyTest(stack, cfg, childStackName)
}

func Test_CreateStaticStackAWS(t *testing.T) {
	RegisterTestingT(t)

	cfg := testutil.PrepareE2Etest()

	parentStackName := tmpResName(e2ePulumiStaticParentStackName)
	childStackName := tmpResName(e2ePulumiStaticChildStackName)

	stack := api.Stack{
		Name: parentStackName,
		Server: e2eServerDescriptorForAws(e2eConfig{
			gcpCreds:       *cfg.GcpCredentials,
			awsCreds:       *cfg.AwsCredentials,
			kmsKeyName:     tmpResName(e2eStaticKmsTestKeyName),
			kmsKeyringName: tmpResName(e2eStaticKmsTestKeyringName),
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
					ParentStack: parentStackName,
					Environment: "test",
					Config: api.Config{
						Config: &api.StackConfigStatic{
							BundleDir:          "static",
							Domain:             fmt.Sprintf("e2e--aws--%s.simple-container.com", tmpResName("static-website")),
							IndexDocument:      "index.html",
							ErrorDocument:      "index.html",
							ProvisionWwwDomain: false,
						},
					},
				},
			},
		},
	}

	runProvisionAndDeployTest(stack, cfg, childStackName)
	runDestroyTest(stack, cfg, childStackName)
}
