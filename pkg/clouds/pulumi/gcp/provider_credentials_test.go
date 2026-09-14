// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package gcp

import (
	"testing"

	. "github.com/onsi/gomega"

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/logger"
	"github.com/simple-container-com/api/pkg/clouds/gcloud"
	"github.com/simple-container-com/api/pkg/clouds/k8s"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	"github.com/simple-container-com/api/pkg/clouds/pulumi/testutil"
)

// serviceAccountKeyJSON is the shape of the value that reached a CI log: a
// service-account key, private key and all.
const serviceAccountKeyJSON = `{"type":"service_account","project_id":"acme-staging",` +
	`"private_key_id":"9f1c0de4cafe",` +
	`"private_key":"-----BEGIN PRIVATE KEY-----\nMIIEvQIBADANBgkqhkiG9w0BAQEFAASCBKcwggSjAgEA\n-----END PRIVATE KEY-----\n",` +
	`"client_email":"deploy@acme-staging.iam.gserviceaccount.com"}`

const kubeconfigFromParentStack = `apiVersion: v1
clusters:
- cluster:
    server: https://203.0.113.10
  name: acme
users:
- name: acme
  user:
    client-key-data: LS0tLS1CRUdJTlBSSVZBVEVLRVk=
`

func gcpAuthInput() api.ResourceInput {
	return api.ResourceInput{
		Descriptor: &api.ResourceDescriptor{
			Name: "gcp-auth",
			Config: api.Config{
				Config: &gcloud.Credentials{
					Credentials:          api.Credentials{Credentials: serviceAccountKeyJSON},
					ServiceAccountConfig: gcloud.ServiceAccountConfig{ProjectId: "acme-staging"},
				},
			},
		},
		StackParams: &api.StackParams{Environment: "staging"},
	}
}

// A GCP provider is configured with a service-account key. Passed as a plain
// string it lands in the checkpoint verbatim and the engine renders it in the
// preview diff, which is how a dry-run printed a private key into CI logs.
func TestGcpProviderCredentialsAreSecret(t *testing.T) {
	RegisterTestingT(t)

	mocks := testutil.NewRecordingMocks()
	var out *api.ResourceOutput

	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		var err error
		out, err = Provider(ctx, api.Stack{Name: "acme"}, gcpAuthInput(), pApi.ProvisionParams{Log: logger.New()})
		return err
	}, pulumi.WithMocks("acme", "staging", mocks))
	Expect(err).ToNot(HaveOccurred())
	Expect(out).ToNot(BeNil())
	Expect(out.Ref).ToNot(BeNil())

	inputs, ok := mocks.Inputs("pulumi:providers:gcp", "gcp-auth--staging")
	Expect(ok).To(BeTrue(), "the gcp provider should have been registered")

	creds, ok := inputs["credentials"]
	Expect(ok).To(BeTrue(), "provider should be configured with credentials")
	Expect(creds.IsSecret()).To(BeTrue(),
		"credentials must reach the engine as a secret, or the preview diff prints the private key")
	Expect(creds.SecretValue().Element.StringValue()).To(Equal(serviceAccountKeyJSON),
		"marking the value secret must not change it")

	// The project is not a credential; keeping it visible is what makes a diff
	// readable, and it also proves the test would notice blanket secreting.
	project, ok := inputs["project"]
	Expect(ok).To(BeTrue())
	Expect(project.IsSecret()).To(BeFalse())
	Expect(project.StringValue()).To(Equal("acme-staging"))
}

func TestGcpProviderRejectsNonAuthConfig(t *testing.T) {
	RegisterTestingT(t)

	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		input := gcpAuthInput()
		input.Descriptor.Config.Config = map[string]string{"not": "an auth config"}
		_, err := Provider(ctx, api.Stack{Name: "acme"}, input, pApi.ProvisionParams{Log: logger.New()})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to cast config to api.AuthConfig"))
		return nil
	}, pulumi.WithMocks("acme", "staging", testutil.NewRecordingMocks()))
	Expect(err).ToNot(HaveOccurred())
}

// Adopting a cluster builds a second Kubernetes provider from the generated
// kubeconfig. The parent stack holds that value as a secret; this is the path
// where it used to be handed back as a plain string.
func TestGkeAutopilotAdoptionKubeconfigIsSecret(t *testing.T) {
	RegisterTestingT(t)

	setGlobalServicesAPIClient(newMockServicesAPIClient())
	defer resetGlobalServicesAPIClient()

	mocks := testutil.NewRecordingMocks()
	mocks.CallResults["gcp:container/getCluster:getCluster"] = resource.NewPropertyMapFromMap(map[string]any{
		"name":             "existing-cluster",
		"location":         "us-central1",
		"minMasterVersion": "1.28",
		"enableAutopilot":  true,
		"project":          "acme-staging",
		"endpoint":         "203.0.113.10",
		"masterAuths": []any{map[string]any{
			"clusterCaCertificate": "LS0tLS1CRUdJTi0tLS0t",
		}},
	})

	gkeInput := &gcloud.GkeAutopilotResource{
		Credentials: gcloud.Credentials{
			Credentials:          api.Credentials{Credentials: serviceAccountKeyJSON},
			ServiceAccountConfig: gcloud.ServiceAccountConfig{ProjectId: "acme-staging"},
		},
		Location:      "us-central1",
		GkeMinVersion: "1.28",
		Adopt:         true,
		ClusterName:   "existing-cluster",
		Caddy:         &k8s.CaddyConfig{},
	}

	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		_, err := GkeAutopilot(ctx, api.Stack{Name: "acme"}, api.ResourceInput{
			Descriptor: &api.ResourceDescriptor{
				Name:   "test-cluster",
				Type:   gcloud.ResourceTypeGkeAutopilot,
				Config: api.Config{Config: gkeInput},
			},
			StackParams: &api.StackParams{Environment: "staging"},
		}, pApi.ProvisionParams{Log: logger.New()})
		return err
	}, pulumi.WithMocks("acme", "staging", mocks))
	Expect(err).ToNot(HaveOccurred())

	providers := mocks.InputsOfType("pulumi:providers:kubernetes")
	Expect(providers).ToNot(BeEmpty(), "adoption should have created a kubernetes provider")
	for _, inputs := range providers {
		kubeconfig, ok := inputs["kubeconfig"]
		Expect(ok).To(BeTrue())
		Expect(kubeconfig.IsSecret()).To(BeTrue(),
			"the adopted cluster's kubeconfig must reach the engine as a secret")
	}
}

// A client stack reads the cluster's kubeconfig back from the parent stack,
// where it is held as a secret, and configures its own Kubernetes provider
// with it. GetValueFromStack hands it over as a plain Go string, so this is
// the call site where the parent's marking used to be dropped.
//
// The deployment that follows needs a live registry credential, which this
// test does not have; the assertion is on what the engine was handed before
// that point, since the provider is configured first.
func TestGkeAutopilotStackKubeconfigIsSecret(t *testing.T) {
	RegisterTestingT(t)

	mocks := testutil.NewRecordingMocks()
	mocks.StackOutputs = map[string]string{
		toKubeconfigExport("acme-cluster--staging"): kubeconfigFromParentStack,
	}

	_ = pulumi.RunErr(func(ctx *pulumi.Context) error {
		_, err := GkeAutopilotStack(ctx, api.Stack{Name: "acme"}, api.ResourceInput{
			Descriptor: &api.ResourceDescriptor{
				Name: "acme",
				Type: gcloud.TemplateTypeGkeAutopilot,
				Config: api.Config{Config: &gcloud.GkeAutopilotInput{
					GkeAutopilotTemplate: gcloud.GkeAutopilotTemplate{
						Credentials: gcloud.Credentials{
							Credentials:          api.Credentials{Credentials: serviceAccountKeyJSON},
							ServiceAccountConfig: gcloud.ServiceAccountConfig{ProjectId: "acme-staging"},
						},
						GkeClusterResource:       "acme-cluster",
						ArtifactRegistryResource: "acme-registry",
					},
				}},
			},
			StackParams: &api.StackParams{Environment: "staging", StackName: "acme"},
		}, pApi.ProvisionParams{
			Log:         logger.New(),
			ParentStack: &pApi.ParentInfo{FullReference: "acme/infra/staging"},
		})
		return err
	}, pulumi.WithMocks("acme", "staging", mocks))

	providers := mocks.InputsOfType("pulumi:providers:kubernetes")
	Expect(providers).ToNot(BeEmpty(), "the client stack should have created a kubernetes provider")
	for _, inputs := range providers {
		kubeconfig, ok := inputs["kubeconfig"]
		Expect(ok).To(BeTrue())
		Expect(kubeconfig.IsSecret()).To(BeTrue(),
			"a kubeconfig read back from the parent stack must not be handed to the provider in clear")
		Expect(kubeconfig.SecretValue().Element.StringValue()).To(Equal(kubeconfigFromParentStack))
	}
}
