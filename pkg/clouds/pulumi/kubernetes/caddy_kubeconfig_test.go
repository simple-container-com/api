// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package kubernetes

import (
	"context"
	"sync"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/logger"
	"github.com/simple-container-com/api/pkg/clouds/k8s"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

// kubeconfigMocks records what the Caddy edge's own kubernetes provider was
// configured with. The provider is the only place the value is observable: it is
// not a stack export, and every object below it merely inherits it.
type kubeconfigMocks struct {
	*simpleContainerMocks
	mu         sync.Mutex
	kubeconfig resource.PropertyValue
	seen       bool
}

func (m *kubeconfigMocks) NewResource(args sdk.MockResourceArgs) (string, resource.PropertyMap, error) {
	if args.TypeToken == "pulumi:providers:kubernetes" {
		m.mu.Lock()
		m.kubeconfig, m.seen = args.Inputs["kubeconfig"], true
		m.mu.Unlock()
	}
	return m.simpleContainerMocks.NewResource(args)
}

// fakeCluster stands in for k3s's K3sOutput: the one thing that matters here is
// that it satisfies pApi.SiblingOutputs and publishes a kubeconfig.
type fakeCluster struct {
	kubeconfig string
}

func (f *fakeCluster) OutputValue(key string) (sdk.StringOutput, bool) {
	if key == k8s.KubeconfigRefKey {
		return sdk.ToSecret(sdk.String(f.kubeconfig)).(sdk.StringOutput), true
	}
	return sdk.StringOutput{}, false
}

func (f *fakeCluster) PublishedOutputKeys() []string { return []string{k8s.KubeconfigRefKey} }

// provisionCaddyWithKubeconfig provisions a sole kubernetes-caddy whose config
// carries the given kubeconfig field, against the given sibling outputs, and
// returns what reached the provider.
func provisionCaddyWithKubeconfig(kubeconfig string, outs pApi.ResourcesOutputs) (resource.PropertyValue, bool, error) {
	mocks := &kubeconfigMocks{simpleContainerMocks: NewSimpleContainerMocks()}
	err := sdk.RunErr(func(ctx *sdk.Context) error {
		desc := api.ResourceDescriptor{
			Name: "ingress",
			Type: k8s.ResourceTypeCaddy,
			Config: api.Config{Config: &k8s.CaddyResource{
				KubernetesConfig: &k8s.KubernetesConfig{Kubeconfig: kubeconfig},
				CaddyConfig:      &k8s.CaddyConfig{DeploymentName: strPtr("caddy")},
			}},
		}
		stack := api.Stack{Name: "root", Server: api.ServerDescriptor{Resources: api.PerStackResourcesDescriptor{
			Resources: map[string]api.PerEnvResourcesDescriptor{"staging": {Resources: map[string]api.ResourceDescriptor{
				"ingress": desc,
			}}},
		}}}
		_, err := CaddyResource(ctx, stack, api.ResourceInput{
			Descriptor:  &desc,
			StackParams: &api.StackParams{StackName: "root", Environment: "staging"},
		}, pApi.ProvisionParams{
			Log:             logger.New(),
			ComputeContext:  pApi.NewComputeContextCollector(context.Background(), logger.New(), "root", "staging"),
			ResourceOutputs: outs,
		})
		return err
	}, sdk.WithMocks("test", "test", mocks))
	return mocks.kubeconfig, mocks.seen, err
}

// THE regression test. api#578 taught `${resource:<cluster>.kubeconfig}` to the
// generic provider path (kubernetes.Provider) and to the validator
// (k8s.CaddyReadConfig refuses a malformed reference), but a Caddy edge builds its
// OWN provider in DeployCaddyService — so a WELL-FORMED reference was handed to
// pulumi-kubernetes as the literal string. It parses as a YAML string scalar and
// the provider answers "couldn't get version/kind; json parse error: cannot
// unmarshal string into Go value of type struct { APIVersion; Kind }" on the first
// object it tries to create, naming neither the reference nor the Caddy.
// Live-caught on the 3-VPS k3s smoke, 2026-11-09.
func TestCaddyResource_ResolvesAKubeconfigReference(t *testing.T) {
	RegisterTestingT(t)

	const kubeconfig = "apiVersion: v1\nkind: Config\nclusters: []\n"
	got, seen, err := provisionCaddyWithKubeconfig("${resource:my-k3s.kubeconfig}", pApi.ResourcesOutputs{
		"my-k3s": {Ref: &fakeCluster{kubeconfig: kubeconfig}},
	})
	Expect(err).ToNot(HaveOccurred())
	Expect(seen).To(BeTrue(), "the Caddy edge built no kubernetes provider")
	Expect(valueOf(got)).To(Equal(kubeconfig))
	Expect(valueOf(got)).ToNot(ContainSubstring("${resource:"), "the reference reached pulumi-kubernetes unresolved")
}

// A literal kubeconfig — every stack in the fleet today — is unchanged by the
// resolution: this is the assertion that says the fix is backwards compatible.
func TestCaddyResource_LiteralKubeconfigIsUnchanged(t *testing.T) {
	RegisterTestingT(t)

	const kubeconfig = "apiVersion: v1\nkind: Config\n"
	got, seen, err := provisionCaddyWithKubeconfig(kubeconfig, nil)
	Expect(err).ToNot(HaveOccurred())
	Expect(seen).To(BeTrue())
	Expect(valueOf(got)).To(Equal(kubeconfig))
}

// A reference naming a resource that was not provisioned before this one is a
// refusal, never a fallback to the literal — the whole point of resolving it here
// is that the failure says which resource was missed.
func TestCaddyResource_RefusesAReferenceToAnAbsentSibling(t *testing.T) {
	RegisterTestingT(t)

	_, _, err := provisionCaddyWithKubeconfig("${resource:my-k3s.kubeconfig}", pApi.ResourcesOutputs{
		"smoke-vm": {Ref: &fakeCluster{}},
	})
	Expect(err).To(MatchError(ContainSubstring(`invalid kubeconfig for caddy resource "ingress"`)))
	Expect(err).To(MatchError(ContainSubstring(`names resource "my-k3s"`)))
	// And it names what WAS available, which is how an operator spots a resource
	// declared under a different environment's block.
	Expect(err).To(MatchError(ContainSubstring(`"smoke-vm"`)))
}

// A reference to a key the cluster does not publish is refused the same way.
func TestCaddyResource_RefusesAnUnpublishedKey(t *testing.T) {
	RegisterTestingT(t)

	_, _, err := provisionCaddyWithKubeconfig("${resource:my-k3s.ca-cert}", pApi.ResourcesOutputs{
		"my-k3s": {Ref: &fakeCluster{kubeconfig: "apiVersion: v1\n"}},
	})
	Expect(err).To(MatchError(ContainSubstring(`publishes no output "ca-cert"`)))
}

// valueOf unwraps the secret the kubeconfig is always marked with before reading
// it. Marked is the assertion of its own test (SecretKubeconfigOutput); here it
// would only hide the value being compared.
func valueOf(v resource.PropertyValue) string {
	for {
		switch {
		case v.IsSecret():
			v = v.SecretValue().Element
		case v.IsOutput():
			v = v.OutputValue().Element
		case v.IsComputed():
			return "<computed>"
		default:
			return v.StringValue()
		}
	}
}
