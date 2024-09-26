package docker

import (
	"fmt"
	"github.com/samber/lo"

	"github.com/pkg/errors"

	"github.com/pulumi/pulumi-docker/sdk/v4/go/docker"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

type Image struct {
	Name                   string
	Dockerfile             string
	Args                   map[string]string
	Context                string
	Version                string
	RepositoryUrlWithImage bool
	ProviderOptions        []sdk.ResourceOption
	RepositoryUrl          sdk.StringOutput
	Registry               docker.RegistryArgs
	Platform               *string
}

type ImageOut struct {
	Image   *docker.Image
	AddOpts []sdk.ResourceOption
}

func BuildAndPushImage(ctx *sdk.Context, stack api.Stack, params pApi.ProvisionParams, deployParams api.StackParams, image Image) (*ImageOut, error) {
	imageFullUrl := image.RepositoryUrl.ApplyT(func(repoUri string) string {
		if image.RepositoryUrlWithImage {
			return fmt.Sprintf("%s:%s", repoUri, image.Version)
		}
		return fmt.Sprintf("%s/%s:%s", repoUri, image.Name, image.Version)
	}).(sdk.StringOutput)
	params.Log.Info(ctx.Context(), "building and pushing docker image %q (from %q) for stack %q env %q",
		image.Name, image.Context, stack.Name, deployParams.Environment)
	args := sdk.StringMap{
		"VERSION": sdk.String(image.Version),
	}
	for k, v := range image.Args {
		args[k] = sdk.String(v)
	}
	res, err := docker.NewImage(ctx, image.Name, &docker.ImageArgs{
		Build: &docker.DockerBuildArgs{
			Context:    sdk.String(image.Context),
			Dockerfile: sdk.String(image.Dockerfile),
			Args:       args,
			Platform:   sdk.String(lo.If(image.Platform != nil, lo.FromPtr(image.Platform)).Else(string(api.ImagePlatformLinuxAmd64))),
		},
		SkipPush:  sdk.Bool(ctx.DryRun()),
		ImageName: imageFullUrl,
		Registry:  image.Registry,
	}, append(image.ProviderOptions, sdk.DependsOn(params.ComputeContext.Dependencies()))...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to build and push image %q for stack %q env %q", image.Name, stack.Name, deployParams.Environment)
	}

	var addOpts []sdk.ResourceOption
	//if !ctx.DryRun() {
	//	cmd, err := local.NewCommand(ctx, fmt.Sprintf("%s-push", image.name), &local.CommandArgs{
	//		Create: sdk.Sprintf("docker push %s", res.ImageName),
	//		Update: sdk.Sprintf("docker push %s", res.ImageName),
	//	}, sdk.DependsOn([]sdk.Resource{res}))
	//	if err != nil {
	//		return nil, errors.Wrapf(err, "failed to invoke docker push")
	//	}
	//	addOpts = append(addOpts, sdk.DependsOn([]sdk.Resource{cmd}))
	//}
	addOpts = append(addOpts, sdk.DependsOn([]sdk.Resource{res}))
	return &ImageOut{
		Image:   res,
		AddOpts: addOpts,
	}, nil
}
