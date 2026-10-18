// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package k3s

import (
	"fmt"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/api"
	cfgK3s "github.com/simple-container-com/api/pkg/clouds/k3s"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	pulumiKubernetes "github.com/simple-container-com/api/pkg/clouds/pulumi/kubernetes"
)

// resourceInput mirrors how a cluster resource and a client stack deploying onto it
// each see the same cluster: K3sCluster spells the name from its OWN descriptor name,
// K3sCloudrun from the `k3sClusterResource` field of a different stack's template. The
// two sides never meet in one process, which is why their agreement is asserted here.
func resourceInput(resName, env string) api.ResourceInput {
	return api.ResourceInput{
		Descriptor: &api.ResourceDescriptor{
			Type: cfgK3s.ResourceTypeK3sCluster,
			Name: resName,
		},
		StackParams: &api.StackParams{Environment: env},
	}
}

// The two sides of the kubeconfig seam, asserted against each other.
//
// cluster.go exports under ToClusterName(input, input.Descriptor.Name); cloudrun.go
// reads under ToClusterName(input, in.K3sClusterResource). Same helper, same env
// suffix — but the two live in different functions provisioned from different stacks,
// so a change to either alone is invisible until a deploy says "output not found".
func TestClusterExportNamesAndCloudrunReadNamesAgree(t *testing.T) {
	RegisterTestingT(t)

	for _, env := range []string{"staging", "prod", ""} {
		t.Run(fmt.Sprintf("env=%q", env), func(t *testing.T) {
			RegisterTestingT(t)

			// The parent provisioning the cluster resource named "ru-cluster".
			exportSide := pulumiKubernetes.ToClusterName(resourceInput("ru-cluster", env), "ru-cluster")
			// The child stack naming that same resource in `k3sClusterResource`.
			readSide := pulumiKubernetes.ToClusterName(resourceInput("my-service", env), "ru-cluster")

			Expect(readSide).To(Equal(exportSide))
			Expect(pulumiKubernetes.ToKubeconfigExport(readSide)).To(Equal(pulumiKubernetes.ToKubeconfigExport(exportSide)))
			Expect(pulumiKubernetes.ToIngressIpExport(readSide)).To(Equal(pulumiKubernetes.ToIngressIpExport(exportSide)))
		})
	}
}

// A sub-env ("custom") client stack resolves parent resource names against the
// PARENT's env, not its own — ToResName reads StackParams.ParentEnv when set. K3sCloudrun
// copies params.ParentStack.ParentEnv onto StackParams for exactly this reason
// (gke_autopilot_stack.go:46-51 does the same). Without it the lookup is spelled with
// the child's env and misses an output that is sitting right there.
func TestClusterNameFollowsTheParentEnvForSubEnvStacks(t *testing.T) {
	RegisterTestingT(t)

	input := resourceInput("my-service", "pr-123")
	Expect(pulumiKubernetes.ToClusterName(input, "ru-cluster")).To(Equal("ru-cluster--pr-123"))

	input.StackParams.ParentEnv = "staging"
	Expect(pulumiKubernetes.ToClusterName(input, "ru-cluster")).To(Equal("ru-cluster--staging"),
		"a sub-env stack must look the cluster up under the parent's env")
}

func TestK3sCloudrunRefusesTheWrongTemplateType(t *testing.T) {
	RegisterTestingT(t)

	input := api.ResourceInput{
		Descriptor: &api.ResourceDescriptor{
			Type: "kubernetes-cloudrun",
			Name: "my-service",
		},
		StackParams: &api.StackParams{Environment: "staging"},
	}
	// The type check is the first thing the function does, before it touches ctx or
	// params — which is why a nil context and an empty params are safe here, and why
	// a refactor that moved the check later would make this test panic rather than
	// quietly pass.
	_, err := K3sCloudrun(nil, api.Stack{Name: "my-service"}, input, pApi.ProvisionParams{})
	Expect(err).NotTo(BeNil())
	Expect(err.Error()).To(ContainSubstring("unsupported template type"))
}
