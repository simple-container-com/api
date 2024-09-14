package gcp

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/samber/lo"

	"github.com/pulumi/pulumi-docker/sdk/v4/go/docker"
	k8s "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/gcloud"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	pDocker "github.com/simple-container-com/api/pkg/clouds/pulumi/docker"
	"github.com/simple-container-com/api/pkg/util"
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
	registryResource := gkeAutopilotInput.TemplateConfig.ArtifactRegistryResource
	clusterName := toClusterName(input, clusterResource)
	environment := input.StackParams.Environment
	stackName := input.StackParams.StackName
	fullParentReference := params.ParentStack.FullReference

	if clusterResource == "" {
		return nil, errors.Errorf("`clusterResource` must be specified for gke autopilot config for %q/%q in %q", stackName, input.Descriptor.Name, environment)
	}

	if registryResource == "" {
		return nil, errors.Errorf("`artifactRegistryResource` must be specified for gke autopilot config for %q/%q in %q", stackName, input.Descriptor.Name, environment)
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

	params.Log.Info(ctx.Context(), "Getting registry url for %q from parent stack %q", clusterName)
	registryURL, err := pApi.GetSecretStringValueFromStack(ctx, fullParentReference, toRegistryUrlExport(input, registryResource))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get kubeconfig from parent stack's resources %q for stack %q", fullParentReference, registryResource)
	}

	_, err = buildAndPushImages(ctx, stack, input, params, gkeAutopilotInput, sdk.String(registryURL).ToStringOutput())
	if err != nil {
		return nil, errors.Wrapf(err, "failed to build and push docker images for stack %q in %q", stackName, input.StackParams.Environment)
	}

	return &api.ResourceOutput{Ref: out}, nil
}

type GKEImage struct {
	Container gcloud.CloudRunContainer
	ImageName sdk.StringOutput
	AddOpts   []sdk.ResourceOption
}

func buildAndPushImages(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params pApi.ProvisionParams, stackInput *gcloud.GkeAutopilotInput, registryURL sdk.StringOutput) ([]*GKEImage, error) {
	authConfig, ok := input.Descriptor.Config.Config.(api.AuthConfig)
	if !ok {
		return nil, errors.Errorf("failed to convert resource input to api.AuthConfig for %q", input.Descriptor.Type)
	}
	// hackily set google creds env variable, so that docker can access it (see github.com/pulumi/pulumi/pkg/v3/authhelpers/gcpauth.go:28)
	if err := os.Setenv("GOOGLE_CREDENTIALS", authConfig.CredentialsValue()); err != nil {
		return nil, errors.Wrapf(err, "failed to set GOOGLE_CREDENTIALS variable")
	}
	defer os.Unsetenv("GOOGLE_CREDENTIALS")

	return util.MapErr(stackInput.Containers, func(container gcloud.CloudRunContainer, _ int) (*GKEImage, error) {
		dockerfile := container.Image.Dockerfile
		if dockerfile == "" && container.Image.Context == "" && container.Name != "" {
			// do not build and return right away
			return &GKEImage{
				Container: container,
				ImageName: sdk.String(container.Name).ToStringOutput(),
			}, nil
		}
		if !filepath.IsAbs(dockerfile) {
			dockerfile = filepath.Join(container.ComposeDir, dockerfile)
		}

		imageName := fmt.Sprintf("%s/%s", stack.Name, container.Name)
		image, err := pDocker.BuildAndPushImage(ctx, stack, params, *input.StackParams, pDocker.Image{
			Name:          imageName,
			Dockerfile:    dockerfile,
			Context:       container.Image.Context,
			Args:          lo.FromPtr(container.Image.Build).Args,
			Version:       lo.If(input.StackParams.Version != "", input.StackParams.Version).Else("latest"),
			RepositoryUrl: registryURL,
			Registry: docker.RegistryArgs{
				Server: registryURL,
			},
		})
		if err != nil {
			return nil, errors.Wrapf(err, "failed to build and push image for container %q in stack %q env %q", container.Name, stack.Name, input.StackParams.Environment)
		}
		return &GKEImage{
			Container: container,
			ImageName: image.Image.ImageName,
			AddOpts:   image.AddOpts,
		}, nil
	})
}
