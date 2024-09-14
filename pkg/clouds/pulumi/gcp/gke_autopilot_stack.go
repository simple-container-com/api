package gcp

import (
	"github.com/pkg/errors"

	k8s "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/gcloud"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

type GkeAutopilotOutput struct {
	Provider *k8s.Provider
}

func GkeAutopilotStack(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	if input.Descriptor.Type != gcloud.TemplateTypeGkeAutopilot {
		return nil, errors.Errorf("unsupported template type %q", input.Descriptor.Type)
	}

	gkeAutopilotInput, ok := input.Descriptor.Config.Config.(*gcloud.GkeAutopilotInput)
	if !ok {
		return nil, errors.Errorf("failed to convert gke autopilot config for %q", input.Descriptor.Type)
	}

	clusterResource := gkeAutopilotInput.TemplateConfig.GkeClusterResource
	clusterName := toClusterName(input, clusterResource)
	environment := input.StackParams.Environment
	stackName := input.StackParams.StackName
	fullParentReference := params.ParentStack.FullReference

	if clusterResource == "" {
		return nil, errors.Errorf("clusterResource must be specified for gke autopilot config for %q/%q in %q", stackName, input.Descriptor.Name, environment)
	}

	params.Log.Info(ctx.Context(), "Getting kubeconfig for %q from parent stack %q", clusterName)
	kubeConfig, err := pApi.GetSecretStringValueFromStack(ctx, fullParentReference, toKubeconfigExport(clusterName))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get kubeconfig from parent stack's resources")
	}
	out := &GkeAutopilotOutput{}

	kubeProvider, err := k8s.NewProvider(ctx, input.ToResName(stackName), &k8s.ProviderArgs{
		Kubeconfig: sdk.String(kubeConfig),
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to provision kubeconfig provider for %q/%q in %q", stackName, input.Descriptor.Name, environment)
	}

	out.Provider = kubeProvider

	return &api.ResourceOutput{Ref: out}, nil
}
