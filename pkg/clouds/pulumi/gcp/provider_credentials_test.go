// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package gcp

import (
	"sync"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/logger"
	"github.com/simple-container-com/api/pkg/clouds/gcloud"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

// serviceAccountKeyJSON is the shape of the value that reached a GitHub Actions
// log: a service-account key, private key and all.
const serviceAccountKeyJSON = `{"type":"service_account","project_id":"acme-staging",` +
	`"private_key_id":"9f1c0de4cafe",` +
	`"private_key":"-----BEGIN PRIVATE KEY-----\nMIIEvQIBADANBgkqhkiG9w0BAQEFAASCBKcwggSjAgEA\n-----END PRIVATE KEY-----\n",` +
	`"client_email":"deploy@acme-staging.iam.gserviceaccount.com"}`

// providerInputRecorder captures the inputs of every registered resource so a
// test can assert on what the engine was actually handed.
type providerInputRecorder struct {
	mu     sync.Mutex
	inputs map[string]resource.PropertyMap
}

func newProviderInputRecorder() *providerInputRecorder {
	return &providerInputRecorder{inputs: map[string]resource.PropertyMap{}}
}

func (m *providerInputRecorder) NewResource(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.inputs[args.TypeToken] = args.Inputs
	return args.Name + "-id", args.Inputs, nil
}

func (m *providerInputRecorder) Call(pulumi.MockCallArgs) (resource.PropertyMap, error) {
	return resource.PropertyMap{}, nil
}

func (m *providerInputRecorder) inputsFor(typeToken string) resource.PropertyMap {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.inputs[typeToken]
}

// A GCP provider is configured with a service-account key. Passed as a plain
// string it lands in the checkpoint verbatim and the engine renders it in the
// preview diff, which is how a dry-run printed a private key into CI logs.
func TestGcpProviderCredentialsAreSecret(t *testing.T) {
	RegisterTestingT(t)

	mocks := newProviderInputRecorder()

	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		out, err := Provider(ctx, api.Stack{Name: "acme"}, api.ResourceInput{
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
		}, pApi.ProvisionParams{Log: logger.New()})
		Expect(err).ToNot(HaveOccurred())
		Expect(out.Ref).ToNot(BeNil())
		return nil
	}, pulumi.WithMocks("acme", "staging", mocks))
	Expect(err).ToNot(HaveOccurred())

	inputs := mocks.inputsFor("pulumi:providers:gcp")
	Expect(inputs).ToNot(BeNil(), "the gcp provider should have been registered")

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
