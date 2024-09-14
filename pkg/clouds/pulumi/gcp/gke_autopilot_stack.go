package gcp

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"github.com/samber/lo"

	"github.com/pulumi/pulumi-command/sdk/go/command/local"
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

	clusterResource := gkeAutopilotInput.GkeAutopilotTemplate.GkeClusterResource
	registryResource := gkeAutopilotInput.GkeAutopilotTemplate.ArtifactRegistryResource
	clusterName := toClusterName(input, clusterResource)
	registryName := toArtifactRegistryName(input, registryResource)
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
	kubeConfig, err := pApi.GetStringValueFromStack(ctx, fullParentReference, toKubeconfigExport(clusterName), true)
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

	params.Log.Info(ctx.Context(), "Getting registry url for %q from parent stack %q", registryResource, fullParentReference)
	registryURL, err := pApi.GetStringValueFromStack(ctx, fullParentReference, toRegistryUrlExport(registryName), false)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get registry url from parent stack's %q resources for resource %q", fullParentReference, registryResource)
	}
	if registryURL == "" {
		return nil, errors.Errorf("parent stack's registry url is empty for stack %q", stackName)
	}

	_, err = buildAndPushImages(ctx, stack, input, params, gkeAutopilotInput, registryURL)
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

func buildAndPushImages(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params pApi.ProvisionParams, stackInput *gcloud.GkeAutopilotInput, registryURL string) ([]*GKEImage, error) {
	authConfig, ok := input.Descriptor.Config.Config.(api.AuthConfig)
	if !ok {
		return nil, errors.Errorf("failed to convert resource input to api.AuthConfig for %q", input.Descriptor.Type)
	}

	var opts []sdk.ResourceOption

	if _, err := exec.LookPath("gcloud"); err == nil {
		env := lo.SliceToMap(os.Environ(), func(env string) (string, string) {
			parts := strings.SplitN(env, "=", 2)
			return parts[0], parts[1]
		})
		env["GOOGLE_CREDENTIALS"] = authConfig.CredentialsValue()
		env["GOOGLE_APPLICATION_CREDENTIALS"] = authConfig.CredentialsValue()
		parsedRegistryURL, err := url.Parse(fmt.Sprintf("https://%s", registryURL))
		if err != nil {
			return nil, errors.Wrapf(err, "failed to parse registry url %q", registryURL)
		}
		configureDockerCmd, err := local.NewCommand(ctx, fmt.Sprintf("%s-%s", stack.Name, input.StackParams.Environment), &local.CommandArgs{
			Update:      sdk.String(fmt.Sprintf("gcloud auth configure-docker %s --quiet", parsedRegistryURL.Host)),
			Create:      sdk.String(fmt.Sprintf("gcloud auth configure-docker %s --quiet", parsedRegistryURL.Host)),
			Triggers:    sdk.ArrayInput(sdk.Array{sdk.String(lo.RandomString(5, lo.AllCharset))}),
			Environment: sdk.ToStringMap(env),
		})
		if err != nil {
			return nil, errors.Wrapf(err, "failed to authenticate against docker registry")
		}
		opts = append(opts, sdk.DependsOn([]sdk.Resource{configureDockerCmd}))
	} else {
		return nil, errors.Errorf("command `gcloud` was not found, cannot authenticate against artifact registry %s", registryURL)
	}

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

		image, err := pDocker.BuildAndPushImage(ctx, stack, params, *input.StackParams, pDocker.Image{
			Name:                   fmt.Sprintf("%s--%s", stack.Name, container.Name),
			Dockerfile:             dockerfile,
			Context:                container.Image.Context,
			Args:                   lo.FromPtr(container.Image.Build).Args,
			RepositoryUrlWithImage: false,
			Version:                lo.If(input.StackParams.Version != "", input.StackParams.Version).Else("latest"),
			RepositoryUrl:          sdk.String(registryURL).ToStringOutput(),
			ProviderOptions:        opts,
			Registry: docker.RegistryArgs{
				Server: sdk.String(registryURL),
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
