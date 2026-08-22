// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package kubernetes

import (
	"context"
	"strings"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/require"

	k8sprovider "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/logger"
	"github.com/simple-container-com/api/pkg/clouds/k8s"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

// securityGate stands in for the sign / verify / SBOM / provenance commands that
// BuildAndPushImage hangs off the image push and returns on ImageOut.AddOpts.
type securityGate struct {
	sdk.CustomResourceState
}

func deployArgsWithImage(img *ContainerImage, provider sdk.ProviderResource) Args {
	return Args{
		KubeProvider:   provider,
		Namespace:      "shop",
		DeploymentName: "shop",
		Input: api.ResourceInput{
			StackParams: &api.StackParams{StackName: "shop", Environment: "staging"},
		},
		Deployment: k8s.DeploymentConfig{
			StackConfig:      &api.StackConfigCompose{},
			IngressContainer: &img.Container,
			Containers:       []k8s.CloudRunContainer{img.Container},
		},
		Images: []*ContainerImage{img},
		Params: pApi.ProvisionParams{
			Log: logger.New(),
			ComputeContext: pApi.NewComputeContextCollector(
				context.Background(), logger.New(), "shop", "staging"),
		},
	}
}

func testContainerImage(addOpts ...sdk.ResourceOption) *ContainerImage {
	return &ContainerImage{
		Container: k8s.CloudRunContainer{
			Name:     "shop",
			MainPort: lo.ToPtr(8080),
			Ports:    k8s.ContainerPorts(8080),
		},
		ImageName: sdk.String("registry.example.com/team/shop@sha256:f7ed9277c480591d7ec36fe7da13e112b33d898b7687f9bcbcda5c214a242099").ToStringOutput(),
		AddOpts:   addOpts,
	}
}

// Regression: the workload must not roll out before the image is signed and
// attested.
//
// BuildAndPushImages returns the security fan-in on ContainerImage.AddOpts, but
// DeploySimpleContainer used to drop it, leaving the Deployment with no
// dependency edge to signing. Pulumi could then update the workload first and
// surface the signing failure afterwards, which puts an unsigned, unattested
// image in front of traffic while the run reports failure. The ECS path already
// folded AddOpts into its resource options; this asserts the Kubernetes path
// does the same.
func TestDeploySimpleContainer_RolloutWaitsForImageSecurityGate(t *testing.T) {
	mocks := NewSimpleContainerMocks()

	var gateURN string
	err := sdk.RunErr(func(ctx *sdk.Context) error {
		gate := &securityGate{}
		if err := ctx.RegisterResource("test:index:SecurityGate", "sign-shop", nil, gate); err != nil {
			return err
		}
		gate.URN().ApplyT(func(u sdk.URN) error {
			gateURN = string(u)
			return nil
		})

		provider, err := k8sprovider.NewProvider(ctx, "kube", &k8sprovider.ProviderArgs{})
		if err != nil {
			return err
		}

		img := testContainerImage(sdk.DependsOn([]sdk.Resource{gate}))
		_, err = DeploySimpleContainer(ctx, deployArgsWithImage(img, provider))
		return err
	}, sdk.WithMocks("test", "test", mocks))
	require.NoError(t, err)
	require.NotEmpty(t, gateURN, "the stand-in gate resource must have registered")

	deps := mocks.DependenciesFor("kubernetes:apps/v1:Deployment")
	require.NotEmpty(t, deps, "the Deployment registered with no dependencies at all")
	require.True(t, containsURN(deps, gateURN),
		"Deployment dependencies %v do not include the image security gate %q", deps, gateURN)
}

// The counterpart: an image that contributes no security options (security
// disabled, or a pre-built image reference SC never pushed) must still deploy.
func TestDeploySimpleContainer_NoSecurityOptsStillDeploys(t *testing.T) {
	mocks := NewSimpleContainerMocks()

	err := sdk.RunErr(func(ctx *sdk.Context) error {
		provider, err := k8sprovider.NewProvider(ctx, "kube", &k8sprovider.ProviderArgs{})
		if err != nil {
			return err
		}
		_, err = DeploySimpleContainer(ctx, deployArgsWithImage(testContainerImage(), provider))
		return err
	}, sdk.WithMocks("test", "test", mocks))
	require.NoError(t, err)
	require.Positive(t, mocks.GetResourceCount("kubernetes:apps/v1:Deployment"))
}

func TestImageSecurityOpts(t *testing.T) {
	first := sdk.DependsOn(nil)
	second := sdk.DependsOn(nil)

	require.Empty(t, imageSecurityOpts(nil))
	require.Empty(t, imageSecurityOpts([]*ContainerImage{nil, {}}))
	require.Len(t, imageSecurityOpts([]*ContainerImage{
		{AddOpts: []sdk.ResourceOption{first}},
		nil,
		{AddOpts: []sdk.ResourceOption{second}},
	}), 2, "every image contributes its own gate")
}

func containsURN(urns []string, want string) bool {
	for _, u := range urns {
		if u == want || strings.HasSuffix(u, want) {
			return true
		}
	}
	return false
}
