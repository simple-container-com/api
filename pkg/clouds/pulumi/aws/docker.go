package aws

import (
	"fmt"

	"github.com/pkg/errors"

	"github.com/pulumi/pulumi-docker/sdk/v4/go/docker"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

type dockerImage struct {
	name       string
	dockerfile string
	args       map[string]string
	context    string
	version    string
}

type dockerImageOut struct {
	image   *docker.Image
	addOpts []sdk.ResourceOption
}

func buildAndPushDockerImage(ctx *sdk.Context, stack api.Stack, params pApi.ProvisionParams, deployParams api.StackParams, image dockerImage) (*dockerImageOut, error) {
	repository, err := createEcrRegistry(ctx, stack, params, deployParams, image.name)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create ecr repository")
	}

	imageFullUrl := repository.Repository.RepositoryUrl.ApplyT(func(repoUri string) string {
		return fmt.Sprintf("%s:%s", repoUri, image.version)
	}).(sdk.StringOutput)
	params.Log.Info(ctx.Context(), "building and pushing docker image %q (from %q) for stack %q env %q",
		image.name, image.context, stack.Name, deployParams.Environment)
	args := sdk.StringMap{
		"VERSION": sdk.String(image.version),
	}
	for k, v := range image.args {
		args[k] = sdk.String(v)
	}
	res, err := docker.NewImage(ctx, image.name, &docker.ImageArgs{
		Build: &docker.DockerBuildArgs{
			Context:    sdk.String(image.context),
			Dockerfile: sdk.String(image.dockerfile),
			Args:       args,
		},
		SkipPush:  sdk.Bool(ctx.DryRun()),
		ImageName: imageFullUrl,
		Registry: docker.RegistryArgs{
			Server:   repository.Repository.RepositoryUrl,
			Username: sdk.String("AWS"), // Use 'AWS' for ECR registry authentication
			Password: repository.Password,
		},
	}, sdk.DependsOn(params.ComputeContext.Dependencies()))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to build and push image %q for stack %q env %q", image.name, stack.Name, deployParams.Environment)
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
	return &dockerImageOut{
		image:   res,
		addOpts: addOpts,
	}, nil
}
