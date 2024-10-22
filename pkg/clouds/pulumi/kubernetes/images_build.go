package kubernetes

import (
	"fmt"
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/samber/lo"

	"github.com/pulumi/pulumi-docker/sdk/v4/go/docker"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/k8s"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	pDocker "github.com/simple-container-com/api/pkg/clouds/pulumi/docker"
	"github.com/simple-container-com/api/pkg/util"
)

type BuildArgs struct {
	RegistryURL      string
	RegistryUsername *string
	RegistryPassword *string
	Stack            api.Stack
	Input            api.ResourceInput
	Params           pApi.ProvisionParams
	Deployment       k8s.DeploymentConfig
	Opts             []sdk.ResourceOption
}

func BuildAndPushImages(ctx *sdk.Context, args BuildArgs) ([]*ContainerImage, error) {
	return util.MapErr(args.Deployment.Containers, func(container k8s.CloudRunContainer, _ int) (*ContainerImage, error) {
		dockerfile := container.Image.Dockerfile
		if dockerfile == "" && container.Image.Context == "" && container.Name != "" {
			// do not build and return right away
			return &ContainerImage{
				Container: container,
				ImageName: sdk.String(container.Name).ToStringOutput(),
			}, nil
		}
		if !filepath.IsAbs(dockerfile) && container.Image.Context == "" {
			dockerfile = filepath.Join(container.ComposeDir, dockerfile)
		} else if !filepath.IsAbs(dockerfile) && container.Image.Context != "" {
			dockerfile = filepath.Join(container.ComposeDir, container.Image.Context, dockerfile)
		}

		image, err := pDocker.BuildAndPushImage(ctx, args.Stack, args.Params, *args.Input.StackParams, pDocker.Image{
			Name:                   fmt.Sprintf("%s--%s", args.Stack.Name, container.Name),
			Dockerfile:             dockerfile,
			Context:                container.Image.Context,
			Args:                   lo.FromPtr(container.Image.Build).Args,
			RepositoryUrlWithImage: false,
			Version:                lo.If(args.Input.StackParams.Version != "", args.Input.StackParams.Version).Else("latest"),
			RepositoryUrl:          sdk.String(args.RegistryURL).ToStringOutput(),
			ProviderOptions:        args.Opts,
			Platform:               lo.If(container.Image.Platform != "", lo.ToPtr(string(container.Image.Platform))).Else(nil),
			Registry: docker.RegistryArgs{
				Password: lo.If(args.RegistryPassword != nil, sdk.StringPtr(lo.FromPtr(args.RegistryPassword))).Else(nil),
				Server:   sdk.String(args.RegistryURL),
				Username: lo.If(args.RegistryUsername != nil, sdk.StringPtr(lo.FromPtr(args.RegistryUsername))).Else(nil),
			},
		})
		if err != nil {
			return nil, errors.Wrapf(err, "failed to build and push image for container %q in stack %q env %q", container.Name, args.Stack.Name, args.Input.StackParams.Environment)
		}
		return &ContainerImage{
			Container: container,
			ImageName: image.Image.ImageName,
			AddOpts:   image.AddOpts,
		}, nil
	})
}
