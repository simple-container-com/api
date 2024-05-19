package aws

import (
	"fmt"

	"github.com/pkg/errors"
	"github.com/pulumi/pulumi-docker/sdk/v3/go/docker"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

type dockerImage struct {
	name       string
	dockerfile string
	context    string
	version    string
}

func buildAndPushDockerImage(ctx *sdk.Context, stack api.Stack, params pApi.ProvisionParams, deployParams api.StackParams, image dockerImage) (*docker.Image, error) {
	repository, err := createEcrRegistry(ctx, stack, params, deployParams, image.name)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create ecr repository")
	}

	imageFullUrl := repository.Repository.RepositoryUrl.ApplyT(func(repoUri string) string {
		return fmt.Sprintf("%s:%s", repoUri, image.version)
	}).(sdk.StringOutput)
	params.Log.Info(ctx.Context(), "building and pushing docker image %q (from %q) for stack %q env %q",
		image.name, image.context, stack.Name, deployParams.Environment)
	res, err := docker.NewImage(ctx, image.name, &docker.ImageArgs{
		Build: &docker.DockerBuildArgs{
			Context:    sdk.String(image.context),
			Dockerfile: sdk.String(image.dockerfile),
			Args: map[string]sdk.StringInput{
				"VERSION": sdk.String(image.version),
			},
		},
		SkipPush:  sdk.Bool(ctx.DryRun()),
		ImageName: imageFullUrl,
		Registry: docker.ImageRegistryArgs{
			Server:   repository.Repository.RepositoryUrl,
			Username: sdk.String("AWS"), // Use 'AWS' for ECR registry authentication
			Password: repository.Password,
		},
	}, sdk.DependsOn(params.ComputeContext.Dependencies()))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to build and push image %q for stack %q env %q", image.name, stack.Name, deployParams.Environment)
	}
	return res, nil
}
