// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package kubernetes

import (
	"testing"

	. "github.com/onsi/gomega"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/logger"
	"github.com/simple-container-com/api/pkg/clouds/k8s"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	"github.com/simple-container-com/api/pkg/clouds/pulumi/testutil"
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

func kubernetesAuthInput() api.ResourceInput {
	return api.ResourceInput{
		Descriptor: &api.ResourceDescriptor{
			Name:   "kubernetes-auth",
			Config: api.Config{Config: &k8s.KubernetesConfig{Kubeconfig: kubeconfigWithCredentials}},
		},
		StackParams: &api.StackParams{Environment: "staging"},
	}
}

// The kubeconfig a parent stack hands out is held as a secret there; reading it
// back into a plain string and passing it to the provider drops that marking,
// and the diff prints the cluster credentials.
func TestKubernetesProviderKubeconfigIsSecret(t *testing.T) {
	RegisterTestingT(t)

	mocks := testutil.NewRecordingMocks()
	var out *api.ResourceOutput

	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		var err error
		out, err = Provider(ctx, api.Stack{Name: "acme"}, kubernetesAuthInput(), pApi.ProvisionParams{Log: logger.New()})
		return err
	}, pulumi.WithMocks("acme", "staging", mocks))
	Expect(err).ToNot(HaveOccurred())
	Expect(out).ToNot(BeNil())
	Expect(out.Ref).ToNot(BeNil())

	inputs, ok := mocks.Inputs("pulumi:providers:kubernetes", "kubernetes-auth--staging")
	Expect(ok).To(BeTrue(), "the kubernetes provider should have been registered")

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

func TestKubernetesProviderRejectsNonAuthConfig(t *testing.T) {
	RegisterTestingT(t)

	var provErr error
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		input := kubernetesAuthInput()
		input.Descriptor.Config.Config = map[string]string{"not": "an auth config"}
		_, provErr = Provider(ctx, api.Stack{Name: "acme"}, input, pApi.ProvisionParams{Log: logger.New()})
		return nil
	}, pulumi.WithMocks("acme", "staging", testutil.NewRecordingMocks()))
	Expect(err).ToNot(HaveOccurred())
	Expect(provErr).To(MatchError(ContainSubstring("failed to cast config to api.AuthConfig")))
}
