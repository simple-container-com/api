// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package k3s

import (
	"fmt"

	"github.com/pkg/errors"
	"github.com/samber/lo"

	sdkK8s "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	cfgK3s "github.com/simple-container-com/api/pkg/clouds/k3s"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	pulumiKubernetes "github.com/simple-container-com/api/pkg/clouds/pulumi/kubernetes"
)

// K3sCloudrun deploys a cloud-compose client stack onto a k3s cluster this parent
// stack provisioned.
//
// Its entire job is the kubeconfig: read it from the parent stack's output, build a
// Kubernetes provider from it, and hand both to the shared deploy path. Nothing about
// the workload itself is k3s-specific, so there is no k3s-specific deploy code below
// this function — that is the design claim the roadmap item makes ("everything below
// the kubeconfig seam is reused unchanged"), and this is where it is either true or
// not.
func K3sCloudrun(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	if input.Descriptor.Type != cfgK3s.TemplateTypeK3sCloudrun {
		return nil, errors.Errorf("unsupported template type %q", input.Descriptor.Type)
	}

	in, ok := input.Descriptor.Config.Config.(*cfgK3s.K3sCloudrunInput)
	if !ok {
		return nil, errors.Errorf("failed to convert %s config for %q (given %T)",
			cfgK3s.TemplateTypeK3sCloudrun, input.Descriptor.Name, input.Descriptor.Config.Config)
	}
	if in.K3sClusterResource == "" {
		return nil, errors.Errorf("`k3sClusterResource` is empty for %q: it names the %s resource in the parent stack whose kubeconfig to deploy through",
			input.Descriptor.Name, cfgK3s.ResourceTypeK3sCluster)
	}
	if params.ParentStack == nil {
		return nil, errors.Errorf("cannot deploy %q onto a %s: no parent stack, so there is no kubeconfig output to read",
			input.Descriptor.Name, cfgK3s.ResourceTypeK3sCluster)
	}

	// Same correction GkeAutopilotStack makes (gke_autopilot_stack.go:46-51), and for
	// the same reason: a custom (sub-env) client stack reaches a parent resource whose
	// name carries the PARENT's env suffix, and ToResName reads that suffix off
	// StackParams.ParentEnv. Without this the cluster name is spelled with the child's
	// env and the lookup below misses.
	environment := input.StackParams.Environment
	if params.ParentStack.ParentEnv != "" && params.ParentStack.ParentEnv != environment {
		input.StackParams.ParentEnv = params.ParentStack.ParentEnv
		params.Log.Info(ctx.Context(), "custom stack detected: resolving %s resource names against parent env %q",
			cfgK3s.ResourceTypeK3sCluster, params.ParentStack.ParentEnv)
	}

	clusterName := pulumiKubernetes.ToClusterName(input, in.K3sClusterResource)
	stackName := input.StackParams.StackName
	fullParentReference := params.ParentStack.FullReference
	suffix := lo.If(params.ParentStack.DependsOnResource != nil, "--"+lo.FromPtr(params.ParentStack.DependsOnResource).Name).Else("")

	params.Log.Info(ctx.Context(), "Getting kubeconfig for k3s cluster %q from parent stack %q (%q)",
		clusterName, fullParentReference, suffix)
	// `true` is the secret flag, and it is not optional: this is a cluster-admin
	// kubeconfig with an embedded client key. The same call site is how
	// gke-autopilot reads its own (gke_autopilot_stack.go:76).
	kubeconfig, err := pApi.GetValueFromStack[string](ctx,
		fmt.Sprintf("%s%s-stack-kubeconfig", clusterName, suffix),
		fullParentReference,
		pulumiKubernetes.ToKubeconfigExport(clusterName),
		true)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get kubeconfig for k3s cluster %q from parent stack %q", clusterName, fullParentReference)
	}
	if kubeconfig == "" {
		return nil, errors.Errorf("kubeconfig for k3s cluster %q is empty: looked for output %q in parent stack %q. Either the cluster resource is named differently in the parent stack, or it has not been provisioned yet",
			clusterName, pulumiKubernetes.ToKubeconfigExport(clusterName), fullParentReference)
	}

	kubeProvider, err := sdkK8s.NewProvider(ctx, input.ToResName(stackName), &sdkK8s.ProviderArgs{
		// SecretKubeconfig, not the bare string: pulumi-kubernetes's generated
		// NewProvider declares no AdditionalSecretOutputs, so an unwrapped kubeconfig
		// is stored in plaintext in the checkpoint and printed in the update summary.
		Kubeconfig:            pulumiKubernetes.SecretKubeconfig(kubeconfig),
		EnableServerSideApply: sdk.BoolPtr(true), // required for the DeploymentPatch the Caddy patch uses
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to init kubernetes provider from k3s cluster %q kubeconfig for stack %q in %q",
			clusterName, stackName, environment)
	}

	// Both halves of KubeRunWithProvider's contract. params.Provider is what the
	// deployed resources are created against; kubeRunInput.Kubeconfig is what the
	// Caddy patch at the end of the deploy connects with. ProvisionParams is a value,
	// so this assignment is local to this call and does not leak into the caller.
	params.Provider = kubeProvider
	kubeRunInput := in.KubeRunInput
	kubeRunInput.Kubeconfig = kubeconfig

	params.Log.Info(ctx.Context(), "Deploying stack %q in %q onto k3s cluster %q", stackName, environment, clusterName)
	return pulumiKubernetes.KubeRunWithProvider(ctx, stack, input, params, &kubeRunInput, pulumiKubernetes.KubeRunOpts{
		// Must be said explicitly. KubeRun's default asks the parent for
		// ToIngressIpExport(<parent stack name>), but a k3s cluster exports its
		// ingress address under ToIngressIpExport(<cluster resource>--<env>)
		// (cluster.go, matching what caddy.go and gke-autopilot export). Leaving the
		// default would miss the output and fail every workload that declares a
		// `domain` — and only those, which is why it would have shipped green.
		IngressIPExport: pulumiKubernetes.ToIngressIpExport(clusterName),
	})
}
