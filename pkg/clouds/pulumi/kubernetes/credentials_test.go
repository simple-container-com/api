// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package kubernetes

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/internals"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/logger"
	"github.com/simple-container-com/api/pkg/clouds/k8s"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

const tokenK8sProvider = "pulumi:providers:kubernetes"

// A kubeconfig with the shape that actually matters: the client key is a
// credential, and an unmarked one is printed verbatim in the update summary.
const testKubeconfig = `apiVersion: v1
clusters:
- cluster: {server: https://kube.example.com}
  name: c
contexts:
- context: {cluster: c, user: u}
  name: c
current-context: c
kind: Config
users:
- name: u
  user:
    client-key-data: dGVzdC1jbGllbnQta2V5
    token: test-bearer-token
`

// inputsWithProp returns the inputs of the first resource of the given type that
// actually carries prop. A plain "first provider of this type" lookup passes
// vacuously on any path that creates a keyless provider first.
func inputsWithProp(m *simpleContainerMocks, typeToken, prop string) resource.PropertyMap {
	for _, in := range m.InputsFor(typeToken) {
		if in.HasValue(resource.PropertyKey(prop)) {
			return in
		}
	}
	return nil
}

// The one property that must never be printable. pulumi-kubernetes declares no
// AdditionalSecretOutputs at all, so nothing marks `kubeconfig` unless we do —
// and an unmarked provider credential is how a Yandex service-account private
// key reached a world-readable CI log on 2026-09-30.
func TestProvider_KubeconfigIsSecret(t *testing.T) {
	RegisterTestingT(t)

	mocks := NewSimpleContainerMocks()
	err := sdk.RunErr(func(ctx *sdk.Context) error {
		_, err := Provider(ctx, api.Stack{Name: "test-stack"}, api.ResourceInput{
			Descriptor: &api.ResourceDescriptor{
				Name: "kube",
				Type: k8s.AuthTypeKubeconfig,
				Config: api.Config{Config: &k8s.KubernetesConfig{
					Kubeconfig: testKubeconfig,
				}},
			},
			StackParams: &api.StackParams{Environment: "test"},
		}, pApi.ProvisionParams{Log: logger.New()})
		return err
	}, sdk.WithMocks("project", "stack", mocks))
	Expect(err).To(BeNil())

	inputs := inputsWithProp(mocks, tokenK8sProvider, "kubeconfig")
	Expect(inputs).NotTo(BeNil(), "no provider was created with a kubeconfig")
	kubeconfig := inputs["kubeconfig"]
	Expect(kubeconfig.IsSecret()).To(BeTrue(),
		"kubeconfig reached the engine unmarked — it will be printed verbatim in the "+
			"preview and update summaries and archived in CI logs")
	// ...and arrive intact. Masking is the engine's job, so a secret
	// PropertyValue still carries the plaintext document as its element — that
	// is not the leak. The leak is it reaching the engine unmarked, above.
	Expect(kubeconfig.SecretValue().Element.StringValue()).To(ContainSubstring("client-key-data"))
}

// Caddy builds its own provider from its own kubeconfig, so it leaks
// independently of Provider() and needs its own assertion.
func TestDeployCaddyService_KubeconfigIsSecret(t *testing.T) {
	RegisterTestingT(t)

	mocks := NewSimpleContainerMocks()
	err := sdk.RunErr(func(ctx *sdk.Context) error {
		input := api.ResourceInput{
			Descriptor:  &api.ResourceDescriptor{Name: "cluster", Type: k8s.ResourceTypeCaddy},
			StackParams: &api.StackParams{StackName: "infra", Environment: "production"},
		}
		params := pApi.ProvisionParams{
			Log:            logger.New(),
			ComputeContext: pApi.NewComputeContextCollector(context.Background(), logger.New(), "infra", "production"),
		}
		_, err := DeployCaddyService(ctx, CaddyDeployment{CaddyConfig: &k8s.CaddyConfig{}, ClusterName: "cluster"},
			input, params, sdk.String(testKubeconfig).ToStringOutput())
		return err
	}, sdk.WithMocks("test", "test", mocks))
	Expect(err).To(BeNil())

	inputs := inputsWithProp(mocks, tokenK8sProvider, "kubeconfig")
	Expect(inputs).NotTo(BeNil(), "caddy did not create a provider with a kubeconfig")
	Expect(inputs["kubeconfig"].IsSecret()).To(BeTrue())
}

// The helpers' own contract, asserted directly so a refactor of either cannot
// pass the call-site tests by accident.
func TestSecretKubeconfigMarksSecret(t *testing.T) {
	RegisterTestingT(t)

	err := sdk.RunErr(func(ctx *sdk.Context) error {
		fromString, err := internals.UnsafeAwaitOutput(ctx.Context(), SecretKubeconfig(testKubeconfig))
		if err != nil {
			return err
		}
		Expect(fromString.Secret).To(BeTrue())
		Expect(fromString.Value).To(Equal(testKubeconfig))

		fromOutput, err := internals.UnsafeAwaitOutput(ctx.Context(),
			SecretKubeconfigOutput(sdk.String(testKubeconfig).ToStringOutput()))
		if err != nil {
			return err
		}
		Expect(fromOutput.Secret).To(BeTrue())
		Expect(fromOutput.Value).To(Equal(testKubeconfig))
		return nil
	}, sdk.WithMocks("project", "stack", NewSimpleContainerMocks()))
	Expect(err).To(BeNil())
}
