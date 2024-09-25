package gcp

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"github.com/samber/lo"

	"github.com/pulumi/pulumi-docker/sdk/v4/go/docker"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/gcloud"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

type DockerImageOutput struct {
	Image *RemoteImage
}

func RemoteImagePush(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	if input.Descriptor.Type != gcloud.ResourceTypeRemoteDockerImagePush {
		return nil, errors.Errorf("unsupported docker image type %q", input.Descriptor.Type)
	}

	dockerImage, ok := input.Descriptor.Config.Config.(*gcloud.RemoteImagePush)
	if !ok {
		return nil, errors.Errorf("failed to convert docker image config for %q", input.Descriptor.Type)
	}
	stackName := input.StackParams.StackName
	environment := input.StackParams.Environment
	registryResource := dockerImage.ArtifactRegistryResource

	if registryResource == "" {
		return nil, errors.Errorf("`artifactRegistryResource` must be specified for docker image for %q/%q in %q", stackName, input.Descriptor.Name, environment)
	}

	out := &DockerImageOutput{}

	if params.ResourceOutputs == nil || params.ResourceOutputs[registryResource] == nil {
		return nil, errors.Errorf("expected registry resource %q to be present in outputs for %q in %q", registryResource, dockerImage.Name, environment)
	}

	registryOut, ok := params.ResourceOutputs[registryResource].Ref.(*ArtifactRegistryOut)
	if !ok {
		return nil, errors.Errorf("resource output for %q could not be casted to *artifactregistry.Repository for %q in %q", registryResource, dockerImage.Name, environment)
	}

	opts := []sdk.ResourceOption{
		sdk.DependsOn([]sdk.Resource{registryOut.Repository}),
	}
	if input.StackParams.Timeouts.DeployTimeout != "" {
		opts = append(opts, sdk.Timeouts(&sdk.CustomTimeouts{
			Create: input.StackParams.Timeouts.DeployTimeout,
			Update: input.StackParams.Timeouts.DeployTimeout,
			Delete: input.StackParams.Timeouts.DeployTimeout,
		}))
	}

	image, err := PushRemoteImageToRegistry(ctx, RemoteImageArgs{
		Image:       dockerImage,
		RegistryURL: registryOut.URL,
		Stack:       stack,
		Input:       input,
		Params:      params,
		Opts:        opts,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to build and push docker images for stack %q in %q", stackName, input.StackParams.Environment)
	}
	out.Image = image

	return &api.ResourceOutput{Ref: out}, nil
}

type RemoteImageArgs struct {
	RegistryURL sdk.StringOutput
	Params      pApi.ProvisionParams
	Image       *gcloud.RemoteImagePush
	Stack       api.Stack
	Input       api.ResourceInput
	Opts        []sdk.ResourceOption
}

type RemoteImage struct {
	Image *docker.Image
}

func PushRemoteImageToRegistry(ctx *sdk.Context, args RemoteImageArgs) (*RemoteImage, error) {
	remoteImageName := fmt.Sprintf("%s-%s-%s-remote", args.Stack.Name, args.Input.StackParams.Environment, args.Image.RemoteImage)
	pushImageName := fmt.Sprintf("%s-%s-%s-push", args.Stack.Name, args.Input.StackParams.Environment, args.Image.Name)
	if args.Image.RemoteImage == "" {
		return nil, errors.Errorf("`remoteImage` must be specified for image %q in %q", args.Image.Name, args.Stack.Name)
	}
	if args.Image.Name == "" {
		return nil, errors.Errorf("`name` must be specified for image %q in %q", args.Image.Name, args.Stack.Name)
	}

	opts := args.Opts

	remoteImageUrl, err := url.Parse(fmt.Sprintf("https://%s", args.Image.RemoteImage))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse remote image url")
	}
	stackName := args.Input.StackParams.StackName
	pushRegistryURL := args.RegistryURL

	// TODO: we only support images in the same project or images that are accessible with the same service account
	remoteRegistryHost := remoteImageUrl.Host

	args.Params.Log.Info(ctx.Context(), "Authenticating against registry %q for stack %q", remoteRegistryHost, stackName)
	gcpCreds, err := getDockerCredentialsWithAuthToken(ctx, args.Input)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to obtain access token for registry %q for stack %q", remoteRegistryHost, stackName)
	}

	version := lo.If(args.Image.Tag == "", "latest").Else(args.Image.Tag)
	platform := lo.If(args.Image.Platform == "", api.ImagePlatformLinuxAmd64).Else(args.Image.Platform)

	remoteImageBuildArgs, err := dockerImageWrapper(args.Image.RemoteImage, version, platform)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to prepare docker image wrapper with Dockerfile for remote image")
	}

	// hacky way of pulling image via docker build
	remoteImage, err := docker.NewImage(ctx, remoteImageName, &docker.ImageArgs{
		Build:     remoteImageBuildArgs,
		SkipPush:  sdk.Bool(true),
		ImageName: sdk.String(args.Image.RemoteImage),
		Registry: docker.RegistryArgs{
			Server:   sdk.String(remoteRegistryHost),
			Password: sdk.String(gcpCreds.Password),
			Username: sdk.String(gcpCreds.Username),
		},
	}, opts...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to wrap remote image with a new build for %q in stack %q in %q",
			args.Image.RemoteImage, args.Stack.Name, args.Input.StackParams.Environment)
	}

	buildArgs, err := dockerImageWrapper(args.Image.RemoteImage, version, platform)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to prepare docker image wrapper with Dockerfile")
	}

	opts = append(opts, sdk.DependsOn([]sdk.Resource{remoteImage}))

	image, err := docker.NewImage(ctx, pushImageName, &docker.ImageArgs{
		Build:     buildArgs,
		SkipPush:  sdk.Bool(ctx.DryRun()),
		ImageName: sdk.Sprintf("%s/%s:%s", pushRegistryURL, args.Image.Name, version),
		Registry: docker.RegistryArgs{
			Server:   pushRegistryURL,
			Password: sdk.String(gcpCreds.Password),
			Username: sdk.String(gcpCreds.Username),
		},
	}, opts...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to push image %q in stack %q env %q", args.Image.Name, args.Stack.Name, args.Input.StackParams.Environment)
	}
	return &RemoteImage{
		Image: image,
	}, nil
}

func dockerImageWrapper(imageName string, version string, platform api.ImagePlatform) (*docker.DockerBuildArgs, error) {
	// hack taken from here https://github.com/pulumi/pulumi-docker/issues/54#issuecomment-772250411
	var dockerFilePath string
	if depDir, err := os.MkdirTemp(os.TempDir(), strings.ReplaceAll(imageName, string(filepath.Separator), "-")); err != nil {
		return nil, errors.Wrapf(err, "failed to create tempDir")
	} else if err = os.WriteFile(filepath.Join(depDir, "Dockerfile"), []byte("ARG SOURCE_IMAGE\n\nFROM ${SOURCE_IMAGE}\nARG VERSION\nLABEL VERSION=${VERSION}"), os.ModePerm); err != nil {
		return nil, errors.Wrapf(err, "failed to write temporary Dockerfile")
	} else {
		dockerFilePath = filepath.Join(depDir, "Dockerfile")
	}

	return &docker.DockerBuildArgs{
		Context:    sdk.String("."),
		Dockerfile: sdk.String(dockerFilePath),
		Platform:   sdk.String(platform),
		Args: sdk.StringMap{
			"SOURCE_IMAGE": sdk.String(imageName),
			"VERSION":      sdk.String(version),
		},
	}, nil
}
