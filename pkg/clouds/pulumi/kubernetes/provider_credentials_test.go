// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package kubernetes

import (
	"sync"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/logger"
	"github.com/simple-container-com/api/pkg/clouds/k8s"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

// kubeconfigWithCredentials carries a client key, so it is worth exactly as
// much as the cluster it points at.
const kubeconfigWithCredentials = `apiVersion: v1
clusters:
- cluster:
    server: https://203.0.113.10
  name: acme
users:
- name: acme
  user:
    client-key-data: LS0tLS1CRUdJTlBSSVZBVEVLRVk=
`

type kubeProviderInputRecorder struct {
	mu     sync.Mutex
	inputs map[string]resource.PropertyMap
}

func newKubeProviderInputRecorder() *kubeProviderInputRecorder {
	return &kubeProviderInputRecorder{inputs: map[string]resource.PropertyMap{}}
}

func (m *kubeProviderInputRecorder) NewResource(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.inputs[args.TypeToken] = args.Inputs
	return args.Name + "-id", args.Inputs, nil
}

func (m *kubeProviderInputRecorder) Call(pulumi.MockCallArgs) (resource.PropertyMap, error) {
	return resource.PropertyMap{}, nil
}

// The kubeconfig a parent stack hands out is held as a secret there; reading it
// back into a plain string and passing it to the provider drops that marking,
// and the diff prints the cluster credentials.
func TestKubernetesProviderKubeconfigIsSecret(t *testing.T) {
	RegisterTestingT(t)

	mocks := newKubeProviderInputRecorder()

	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		out, err := Provider(ctx, api.Stack{Name: "acme"}, api.ResourceInput{
			Descriptor: &api.ResourceDescriptor{
				Name: "kubernetes-auth",
				Config: api.Config{
					Config: &k8s.KubernetesConfig{Kubeconfig: kubeconfigWithCredentials},
				},
			},
			StackParams: &api.StackParams{Environment: "staging"},
		}, pApi.ProvisionParams{Log: logger.New()})
		Expect(err).ToNot(HaveOccurred())
		Expect(out.Ref).ToNot(BeNil())
		return nil
	}, pulumi.WithMocks("acme", "staging", mocks))
	Expect(err).ToNot(HaveOccurred())

	inputs := mocks.inputs["pulumi:providers:kubernetes"]
	Expect(inputs).ToNot(BeNil(), "the kubernetes provider should have been registered")

	kubeconfig, ok := inputs["kubeconfig"]
	Expect(ok).To(BeTrue(), "provider should be configured with a kubeconfig")
	Expect(kubeconfig.IsSecret()).To(BeTrue(),
		"kubeconfig must reach the engine as a secret, or the preview diff prints the client key")
	Expect(kubeconfig.SecretValue().Element.StringValue()).To(Equal(kubeconfigWithCredentials))

	// Server-side apply is a behaviour flag, not a credential: it stays legible.
	ssa, ok := inputs["enableServerSideApply"]
	Expect(ok).To(BeTrue())
	Expect(ssa.IsSecret()).To(BeFalse())
	Expect(ssa.BoolValue()).To(BeTrue())
}
