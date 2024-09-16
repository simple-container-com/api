package gcp

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strings"

	"github.com/pkg/errors"
	"github.com/samber/lo"

	"github.com/pulumi/pulumi-command/sdk/go/command/local"
	sdkK8s "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/gcloud"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	"github.com/simple-container-com/api/pkg/clouds/pulumi/kubernetes"
)

type GkeAutopilotOutput struct {
	Provider        *sdkK8s.Provider
	Images          []*kubernetes.ContainerImage
	SimpleContainer *kubernetes.SimpleContainer
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

	params.Log.Info(ctx.Context(), "Getting kubeconfig for %q from parent stack %q", clusterName, fullParentReference)
	kubeConfig, err := pApi.GetStringValueFromStack(ctx, fmt.Sprintf("%s-stack-kubeconfig", clusterName), fullParentReference, toKubeconfigExport(clusterName), true)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get kubeconfig from parent stack's resources")
	}
	out := &GkeAutopilotOutput{}

	kubeProvider, err := sdkK8s.NewProvider(ctx, input.ToResName(stackName), &sdkK8s.ProviderArgs{
		Kubeconfig: sdk.String(kubeConfig),
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to provision kubeconfig provider for %q/%q in %q", stackName, input.Descriptor.Name, environment)
	}

	out.Provider = kubeProvider

	params.Log.Info(ctx.Context(), "Getting registry url for %q from parent stack %q", registryResource, fullParentReference)
	registryURL, err := pApi.GetStringValueFromStack(ctx, fmt.Sprintf("%s-stack-registryurl", clusterName), fullParentReference, toRegistryUrlExport(registryName), false)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get registry url from parent stack's %q resources for resource %q", fullParentReference, registryResource)
	}
	if registryURL == "" {
		return nil, errors.Errorf("parent stack's registry url is empty for stack %q", stackName)
	}

	params.Log.Info(ctx.Context(), "Authenticating against registry %q for stack %q in %q", registryURL, stackName, environment)
	authOpts, err := authAgainstRegistry(ctx, input, registryURL)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to authenticate against provisioned registry %q for stack %q in %q", registryURL, stackName, environment)
	}

	params.Log.Info(ctx.Context(), "Building and pushing images to registry %q for stack %q in %q", registryURL, stackName, environment)
	images, err := kubernetes.BuildAndPushImages(ctx, kubernetes.BuildArgs{
		RegistryURL: registryURL,
		Stack:       stack,
		Input:       input,
		Params:      params,
		Deployment:  gkeAutopilotInput.Deployment,
		Opts:        authOpts,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to build and push docker images for stack %q in %q", stackName, input.StackParams.Environment)
	}
	out.Images = images

	params.Log.Info(ctx.Context(), "Configure simple container deployment for stack %q in %q", stackName, environment)
	sc, err := kubernetes.DeploySimpleContainer(ctx, kubernetes.Args{
		Input:        input,
		Deployment:   gkeAutopilotInput.Deployment,
		Images:       images,
		Params:       params,
		KubeProvider: kubeProvider,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to provision simple container for stack %q in %q", stackName, input.StackParams.Environment)
	}
	out.SimpleContainer = sc

	return &api.ResourceOutput{Ref: out}, nil
}

func authAgainstRegistry(ctx *sdk.Context, input api.ResourceInput, registryURL string) ([]sdk.ResourceOption, error) {
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
		configureDockerCmd, err := local.NewCommand(ctx, fmt.Sprintf("%s-%s", input.StackParams.StackName, input.StackParams.Environment), &local.CommandArgs{
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
	return opts, nil
}
